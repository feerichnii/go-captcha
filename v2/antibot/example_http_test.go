package antibot_test

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/feerichnii/go-captcha/v2/antibot"
	"github.com/feerichnii/go-captcha/v2/slide"
)

// Example shows Issue → Verify. Client solves the puzzle first; on «Проверить
// решение» it finishes Issue-attached JS/PoW then POSTs /verify.
// ClientKey is a server-issued session cookie — never RemoteAddr (IP:port).
func Example_httpHandlers() {
	secret := []byte("replace-with-32-random-bytes-from-env")
	layer, err := antibot.New(antibot.NewMemoryStore(), antibot.Config{
		SecretKey: secret,
		TTL:       90 * time.Second,
	})
	if err != nil {
		panic(err)
	}

	builder := slide.NewBuilder()
	// builder.SetResources(slide.WithGraphImages(...), slide.WithBackgrounds(...))
	capt := builder.Make()

	http.HandleFunc("/captcha/issue", func(w http.ResponseWriter, r *http.Request) {
		if err := antibot.AssertBrowserHeaders(r); err != nil {
			writeErr(w, err)
			return
		}
		sess, _, err := antibot.EnsureSessionCookie(w, r, secret, antibot.DefaultSessionCookie, antibot.DefaultSessionTTL)
		if err != nil {
			http.Error(w, "session unavailable", http.StatusInternalServerError)
			return
		}
		signals := antibot.SignalsFromRequest(r, sess)

		var in struct {
			Capabilities antibot.ClientCapabilities `json:"capabilities"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)

		if err := layer.PreflightIssue(r.Context(), sess.ClientKey, signals); err != nil {
			writeErr(w, err)
			return
		}

		data, err := capt.Generate()
		if err != nil {
			http.Error(w, "captcha unavailable", http.StatusInternalServerError)
			return
		}
		answer, _ := json.Marshal(data.GetData())
		iss, err := layer.Issue(r.Context(), antibot.IssueRequest{
			Kind:         antibot.KindSlide,
			Answer:       answer,
			ClientKey:    sess.ClientKey,
			Signals:      signals,
			Capabilities: in.Capabilities,
		})
		if err != nil {
			writeErr(w, err)
			return
		}
		master, _ := data.GetMasterImage().ToBase64()
		tile, _ := data.GetTileImage().ToBase64()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": iss.ID, "expires_at": iss.ExpiresAt, "ttl_seconds": iss.TTLSeconds,
			"pow": iss.PoW, "js_challenge": iss.JSChallenge,
			"public": data.GetPublicData(), "master": master, "tile": tile,
		})
	})

	http.HandleFunc("/captcha/verify", func(w http.ResponseWriter, r *http.Request) {
		if err := antibot.AssertBrowserHeaders(r); err != nil {
			writeErr(w, err)
			return
		}
		sess, _, err := antibot.EnsureSessionCookie(w, r, secret, antibot.DefaultSessionCookie, antibot.DefaultSessionTTL)
		if err != nil {
			http.Error(w, "session unavailable", http.StatusInternalServerError)
			return
		}
		signals := antibot.SignalsFromRequest(r, sess)

		var in struct {
			ID         string                 `json:"id"`
			Answer     json.RawMessage        `json:"answer"`
			Trajectory antibot.Trajectory     `json:"trajectory"`
			PoWNonce   string                 `json:"pow_nonce"`
			Browser    antibot.BrowserSignals `json:"browser"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10)).Decode(&in); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		res, err := layer.Verify(r.Context(), antibot.VerifyRequest{
			ID: in.ID, Answer: in.Answer, Trajectory: in.Trajectory,
			PoWNonce: in.PoWNonce, ClientKey: sess.ClientKey, Signals: signals, Browser: in.Browser,
		})
		if err != nil {
			writeErr(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "require_pow_next": res.RequirePoWNext})
	})

	_ = context.Background()
}

func writeErr(w http.ResponseWriter, err error) {
	code := antibot.ErrorCode(err)
	retry := antibot.RetryAfterMs(err)
	status := http.StatusForbidden
	switch code {
	case antibot.CodeRateLimited:
		status = http.StatusTooManyRequests
	case antibot.CodeInternal:
		status = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	out := map[string]any{"error": code, "error_code": code, "detail": err.Error()}
	if retry > 0 {
		out["retry_after_ms"] = retry
	}
	_ = json.NewEncoder(w).Encode(out)
}

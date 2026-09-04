package antibot_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/feerichnii/go-captcha/v2/antibot"
	"github.com/feerichnii/go-captcha/v2/slide"
)

// This example shows the two HTTP handlers the JS client
// (client/antibot-client.js) talks to. ClientKey is a server-issued session
// cookie — never RemoteAddr (IP:port). IP/UA go into ClientSignals only.
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
		sess, _, err := antibot.EnsureSessionCookie(w, r, secret, antibot.DefaultSessionCookie, antibot.DefaultSessionTTL)
		if err != nil {
			http.Error(w, "session unavailable", http.StatusInternalServerError)
			return
		}
		signals := antibot.SignalsFromRequest(r, sess)

		data, err := capt.Generate()
		if err != nil {
			http.Error(w, "captcha unavailable", http.StatusInternalServerError)
			return
		}
		answer, _ := json.Marshal(data.GetData()) // secret: goes only to Issue
		iss, err := layer.Issue(r.Context(), antibot.IssueRequest{
			Kind:      antibot.KindSlide,
			Answer:    answer,
			ClientKey: sess.ClientKey,
			Signals:   signals,
		})
		if err != nil {
			writeErr(w, err)
			return
		}
		master, _ := data.GetMasterImage().ToBase64()
		tile, _ := data.GetTileImage().ToBase64()
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":           iss.ID,
			"expires_at":   iss.ExpiresAt,
			"ttl_seconds":  iss.TTLSeconds,
			"pow":          iss.PoW,
			"js_challenge": iss.JSChallenge,
			"public":       data.GetPublicData(),
			"master":       master,
			"tile":         tile,
		})
	})

	http.HandleFunc("/captcha/verify", func(w http.ResponseWriter, r *http.Request) {
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
			ID:         in.ID,
			Answer:     in.Answer,
			Trajectory: in.Trajectory,
			PoWNonce:   in.PoWNonce,
			ClientKey:  sess.ClientKey,
			Signals:    signals,
			Browser:    in.Browser,
		})
		if err != nil {
			writeErr(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":               true,
			"require_pow_next": res.RequirePoWNext,
		})
	})

	_ = context.Background()
}

// writeErr shows one generic message to users; the typed error is for logs.
func writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, antibot.ErrRateLimited):
		http.Error(w, "too many requests", http.StatusTooManyRequests)
	case antibot.IsClientError(err):
		http.Error(w, "captcha failed", http.StatusForbidden)
	default:
		// log.Printf("captcha internal: %v", err)
		http.Error(w, "captcha unavailable", http.StatusInternalServerError)
	}
}

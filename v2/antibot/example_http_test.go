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
// (client/antibot-client.js) talks to. Adapt the session/client key and the
// captcha builder to your app; everything else is wire format.
func Example_httpHandlers() {
	layer, err := antibot.New(antibot.NewMemoryStore(), antibot.Config{
		SecretKey: []byte("replace-with-32-random-bytes-from-env"),
		TTL:       90 * time.Second,
	})
	if err != nil {
		panic(err)
	}

	builder := slide.NewBuilder()
	// builder.SetResources(slide.WithGraphImages(...), slide.WithBackgrounds(...))
	capt := builder.Make()

	clientKey := func(r *http.Request) string {
		if c, err := r.Cookie("sid"); err == nil && c.Value != "" {
			return "sid:" + c.Value
		}
		return "ip:" + r.RemoteAddr + "|ua:" + r.UserAgent()
	}

	http.HandleFunc("/captcha/issue", func(w http.ResponseWriter, r *http.Request) {
		data, err := capt.Generate()
		if err != nil {
			http.Error(w, "captcha unavailable", http.StatusInternalServerError)
			return
		}
		answer, _ := json.Marshal(data.GetData()) // secret: goes only to Issue
		iss, err := layer.Issue(r.Context(), antibot.IssueRequest{
			Kind:      antibot.KindSlide,
			Answer:    answer,
			ClientKey: clientKey(r),
		})
		if err != nil {
			writeErr(w, err)
			return
		}
		master, _ := data.GetMasterImage().ToBase64()
		tile, _ := data.GetTileImage().ToBase64()
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":          iss.ID,
			"expires_at":  iss.ExpiresAt,
			"ttl_seconds": iss.TTLSeconds,
			"pow":         iss.PoW, // null unless the client is risky
			"public":      data.GetPublicData(),
			"master":      master,
			"tile":        tile,
		})
	})

	http.HandleFunc("/captcha/verify", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID         string             `json:"id"`
			Answer     json.RawMessage    `json:"answer"`
			Trajectory antibot.Trajectory `json:"trajectory"`
			PoWNonce   string             `json:"pow_nonce"`
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
			ClientKey:  clientKey(r),
		})
		if err != nil {
			writeErr(w, err)
			return
		}
		// Success: mark the session as verified for the next N minutes, etc.
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

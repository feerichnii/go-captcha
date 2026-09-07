package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/feerichnii/go-captcha/v2/antibot"
	"github.com/feerichnii/go-captcha/v2/base/codec"
	"github.com/feerichnii/go-captcha/v2/base/option"
	"github.com/feerichnii/go-captcha/v2/rotate"
	"github.com/feerichnii/go-captcha/v2/slide"
)

func main() {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		log.Fatal(err)
	}

	cfg := antibot.Config{
		SecretKey:     secret,
		TTL:           2 * time.Minute,
		PoWProbeProb:  -1, // quieter demo
		PoWJitterBits: -1,
		MinSolveTime:  200 * time.Millisecond,
		// Demo still sends piece_down for slide; rotate uses the angle track.
		AllowMissingPiecePress: false,
		// TrustedProxies empty → RemoteAddr only (spoofed XFF ignored).
	}
	layer, err := antibot.New(antibot.NewMemoryStore(), cfg)
	if err != nil {
		log.Fatal(err)
	}
	trustedProxies := cfg.TrustedProxies

	bgs, err := loadBackgrounds()
	if err != nil {
		log.Fatal(err)
	}
	graphs := synthGraphs()

	slideBasic := slide.NewBuilder(
		slide.WithRangeGraphSize(option.RangeVal{Min: 64, Max: 70}),
	)
	slideBasic.SetResources(slide.WithBackgrounds(bgs), slide.WithGraphImages(graphs))
	slideCapt := slideBasic.Make()

	rotBuilder := rotate.NewBuilder()
	rotBuilder.SetResources(rotate.WithImages(bgs))
	rotCapt := rotBuilder.Make()

	mux := http.NewServeMux()
	staticDir := filepath.Join(mustWD(), "static")
	mux.Handle("/", http.FileServer(http.Dir(staticDir)))
	mux.Handle("/antibot-client.js", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(mustWD(), "..", "antibot", "client", "antibot-client.js"))
	}))

	mux.HandleFunc("/api/issue", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		_ = antibot.AssertBrowserHeaders(r) // soft in browsers; demos often lack Sec-Fetch in file:// — ignore empty

		var in struct {
			Kind string `json:"kind"` // slide | rotate
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in.Kind == "" {
			in.Kind = "slide"
		}

		sess, _, err := antibot.EnsureSessionCookie(w, r, secret, antibot.DefaultSessionCookie, antibot.DefaultSessionTTL)
		if err != nil {
			http.Error(w, "session", 500)
			return
		}
		signals := antibot.SignalsFromRequestTrusted(r, sess, trustedProxies)

		var (
			kind    string
			answer  []byte
			public  any
			master  string
			thumb   string
			tileKey = "tile"
		)

		switch in.Kind {
		case "rotate":
			data, err := rotCapt.Generate()
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			kind = antibot.KindRotate
			answer, _ = json.Marshal(data.GetData())
			public = data.GetPublicData()
			master, _ = data.GetMasterImage().ToBase64()
			thumb, _ = data.GetThumbImage().ToBase64()
			tileKey = "thumb"
		default: // slide
			data, err := slideCapt.Generate()
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			kind = antibot.KindSlide
			answer, _ = json.Marshal(data.GetData())
			public = data.GetPublicData()
			master, _ = data.GetMasterImage().ToBase64()
			thumb, _ = data.GetTileImage().ToBase64()
		}

		iss, err := layer.Issue(r.Context(), antibot.IssueRequest{
			Kind:      kind,
			Answer:    answer,
			ClientKey: sess.ClientKey,
			Signals:   signals,
		})
		if err != nil {
			writeErr(w, err)
			return
		}

		out := map[string]any{
			"id":           iss.ID,
			"kind":         in.Kind,
			"expires_at":   iss.ExpiresAt,
			"ttl_seconds":  iss.TTLSeconds,
			"pow":          iss.PoW,
			"js_challenge": iss.JSChallenge,
			"public":       public,
			"master":       master,
			tileKey:        thumb,
			"tile":         thumb,
		}
		writeJSON(w, out)
	})

	mux.HandleFunc("/api/verify", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		sess, _, err := antibot.EnsureSessionCookie(w, r, secret, antibot.DefaultSessionCookie, antibot.DefaultSessionTTL)
		if err != nil {
			http.Error(w, "session", 500)
			return
		}
		signals := antibot.SignalsFromRequestTrusted(r, sess, trustedProxies)

		var in struct {
			ID         string                 `json:"id"`
			Answer     json.RawMessage        `json:"answer"`
			Trajectory antibot.Trajectory     `json:"trajectory"`
			PoWNonce   string                 `json:"pow_nonce"`
			Browser    antibot.BrowserSignals `json:"browser"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10)).Decode(&in); err != nil {
			http.Error(w, "bad request", 400)
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
		writeJSON(w, map[string]any{"ok": true, "score": res.Score, "risk_level": res.RiskLevel})
	})

	addr := ":8080"
	if v := os.Getenv("PORT"); v != "" {
		addr = ":" + v
	}
	fmt.Printf("GoCaptcha AntiBot demo → http://127.0.0.1%s\n", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	retry := antibot.RetryAfterMs(err)
	code := http.StatusInternalServerError
	msg := "captcha unavailable"
	switch {
	case errors.Is(err, antibot.ErrRateLimited):
		code = http.StatusTooManyRequests
		msg = "too many requests"
	case errors.Is(err, antibot.ErrLocked):
		code = http.StatusForbidden
		msg = "locked"
	case errors.Is(err, antibot.ErrBadAnswer):
		code = http.StatusForbidden
		msg = "bad_answer"
	case antibot.IsClientError(err):
		code = http.StatusForbidden
		msg = "captcha failed"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	out := map[string]any{"error": msg, "detail": err.Error()}
	if retry > 0 {
		out["retry_after_ms"] = retry
	}
	_ = json.NewEncoder(w).Encode(out)
}

func mustWD() string {
	wd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	return wd
}

func loadBackgrounds() ([]image.Image, error) {
	dir := filepath.Join(mustWD(), "..", "resources", "backgrounds")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []image.Image
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".png" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		img, err := codec.DecodeByteToPng(b)
		if err != nil {
			return nil, err
		}
		out = append(out, img)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no backgrounds in %s", dir)
	}
	return out, nil
}

// synthGraphs builds one classic jigsaw silhouette. All drop slots reuse it;
// only background alignment distinguishes the correct notch.
func synthGraphs() []*slide.GraphImage {
	return []*slide.GraphImage{makeGraph(jigsawPiece)}
}

func makeGraph(shape func(img *image.NRGBA)) *slide.GraphImage {
	const s = 70
	mask := image.NewNRGBA(image.Rect(0, 0, s, s))
	shadow := image.NewNRGBA(image.Rect(0, 0, s, s))
	overlay := image.NewNRGBA(image.Rect(0, 0, s, s))
	shape(mask)
	for y := 0; y < s; y++ {
		for x := 0; x < s; x++ {
			a := mask.NRGBAAt(x, y).A
			if a == 0 {
				continue
			}
			// Dual-tone notch: light rim + dark fill so holes read on both
			// bright picnic art and dark neon/underwater crops.
			if maskEdge(mask, x, y, s) {
				shadow.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: uint8(float64(a) * 170 / 255)})
			} else {
				shadow.SetNRGBA(x, y, color.NRGBA{R: 8, G: 10, B: 14, A: uint8(float64(a) * 145 / 255)})
			}
			overlay.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: uint8(float64(a) * 90 / 255)})
		}
	}
	return &slide.GraphImage{OverlayImage: overlay, ShadowImage: shadow, MaskImage: mask}
}

func maskEdge(mask *image.NRGBA, x, y, s int) bool {
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			nx, ny := x+dx, y+dy
			if nx < 0 || ny < 0 || nx >= s || ny >= s || mask.NRGBAAt(nx, ny).A == 0 {
				return true
			}
		}
	}
	return false
}

// jigsawPiece draws a smooth puzzle tile: rounded body, top tab, right socket.
// Uses 4× supersampling so edges stay clean after BiLinear downscale.
func jigsawPiece(img *image.NRGBA) {
	const (
		s      = 70
		scale  = 4
		hi     = s * scale
		margin = 10.0 * scale
		corner = 8.0 * scale
		tabR   = 9.0 * scale
		neckW  = 7.0 * scale
	)
	bodyL := margin
	bodyT := margin + 8*scale
	bodyR := float64(hi) - margin
	bodyB := float64(hi) - margin
	tabCX := (bodyL + bodyR) / 2
	tabCY := bodyT - tabR*0.55
	sockCX := bodyR + tabR*0.35
	sockCY := (bodyT + bodyB) / 2

	hiMask := image.NewNRGBA(image.Rect(0, 0, hi, hi))
	for y := 0; y < hi; y++ {
		for x := 0; x < hi; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			dBody := sdRoundRect(fx, fy, bodyL, bodyT, bodyR, bodyB, corner)
			dTab := sdCircle(fx, fy, tabCX, tabCY, tabR)
			dNeck := sdRoundRect(fx, fy, tabCX-neckW/2, tabCY, tabCX+neckW/2, bodyT+2*scale, 2*scale)
			dSock := sdCircle(fx, fy, sockCX, sockCY, tabR)
			d := sdUnion(sdUnion(dBody, dTab), dNeck)
			d = sdSubtract(d, dSock)
			a := aaCover(d)
			if a > 0 {
				hiMask.SetNRGBA(x, y, color.NRGBA{A: a})
			}
		}
	}
	// Box-filter downsample to the final mask size.
	for y := 0; y < s; y++ {
		for x := 0; x < s; x++ {
			var sum int
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					sum += int(hiMask.NRGBAAt(x*scale+dx, y*scale+dy).A)
				}
			}
			a := uint8(sum / (scale * scale))
			if a > 0 {
				img.SetNRGBA(x, y, color.NRGBA{A: a})
			}
		}
	}
}
func sdCircle(x, y, cx, cy, r float64) float64 {
	dx, dy := x-cx, y-cy
	return math.Sqrt(dx*dx+dy*dy) - r
}

func sdRoundRect(x, y, left, top, right, bottom, radius float64) float64 {
	cx := clamp(x, left+radius, right-radius)
	cy := clamp(y, top+radius, bottom-radius)
	dx, dy := x-cx, y-cy
	return math.Sqrt(dx*dx+dy*dy) - radius
}

func sdUnion(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func sdSubtract(a, b float64) float64 {
	nb := -b
	if a > nb {
		return a
	}
	return nb
}

func aaCover(dist float64) uint8 {
	if dist <= -0.5 {
		return 255
	}
	if dist >= 0.5 {
		return 0
	}
	return uint8((0.5 - dist) * 255)
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

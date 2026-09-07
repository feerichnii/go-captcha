package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/feerichnii/go-captcha/v2/antibot"
	"github.com/feerichnii/go-captcha/v2/base/codec"
	"github.com/feerichnii/go-captcha/v2/rotate"
	"github.com/feerichnii/go-captcha/v2/slide"
)

func main() {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		log.Fatal(err)
	}

	layer, err := antibot.New(antibot.NewMemoryStore(), antibot.Config{
		SecretKey:     secret,
		TTL:           2 * time.Minute,
		PoWProbeProb:  -1, // quieter demo
		PoWJitterBits: -1,
		MinSolveTime:  200 * time.Millisecond,
		// Demo still sends piece_down for slide; rotate uses the angle track.
		AllowMissingPiecePress: false,
	})
	if err != nil {
		log.Fatal(err)
	}

	bgs, err := loadBackgrounds()
	if err != nil {
		log.Fatal(err)
	}
	graphs := synthGraphs()

	slideBasic := slide.NewBuilder()
	slideBasic.SetResources(slide.WithBackgrounds(bgs), slide.WithGraphImages(graphs))
	slideCapt := slideBasic.Make()

	slideDrag := slide.NewBuilder()
	slideDrag.SetResources(slide.WithBackgrounds(bgs), slide.WithGraphImages(graphs))
	dragCapt := slideDrag.MakeWithRegion()

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
			Kind string `json:"kind"` // slide | drag | rotate
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
		signals := antibot.SignalsFromRequest(r, sess)

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
		case "drag":
			data, err := dragCapt.Generate()
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			kind = antibot.KindSlide
			answer, _ = json.Marshal(data.GetData())
			public = data.GetPublicData()
			master, _ = data.GetMasterImage().ToBase64()
			thumb, _ = data.GetTileImage().ToBase64()
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
		signals := antibot.SignalsFromRequest(r, sess)

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
	switch {
	case err == antibot.ErrRateLimited:
		http.Error(w, "too many requests", http.StatusTooManyRequests)
	case antibot.IsClientError(err):
		http.Error(w, "captcha failed: "+err.Error(), http.StatusForbidden)
	default:
		http.Error(w, "captcha unavailable: "+err.Error(), http.StatusInternalServerError)
	}
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

// synthGraphs builds a few distinct tile shapes so the 3-slot decoys look different.
func synthGraphs() []*slide.GraphImage {
	return []*slide.GraphImage{
		makeGraph(blobA),
		makeGraph(blobB),
		makeGraph(blobC),
	}
}

func makeGraph(shape func(img *image.NRGBA)) *slide.GraphImage {
	const s = 70
	mask := image.NewNRGBA(image.Rect(0, 0, s, s))
	shadow := image.NewNRGBA(image.Rect(0, 0, s, s))
	overlay := image.NewNRGBA(image.Rect(0, 0, s, s))
	shape(mask)
	draw.Draw(shadow, shadow.Bounds(), &image.Uniform{C: color.NRGBA{A: 140}}, image.Point{}, draw.Src)
	// Apply mask to shadow/overlay.
	for y := 0; y < s; y++ {
		for x := 0; x < s; x++ {
			a := mask.NRGBAAt(x, y).A
			if a == 0 {
				shadow.SetNRGBA(x, y, color.NRGBA{})
				overlay.SetNRGBA(x, y, color.NRGBA{})
			} else {
				shadow.SetNRGBA(x, y, color.NRGBA{R: 20, G: 20, B: 20, A: 160})
				overlay.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 90})
			}
		}
	}
	return &slide.GraphImage{OverlayImage: overlay, ShadowImage: shadow, MaskImage: mask}
}

func blobA(img *image.NRGBA) {
	fillCircle(img, 35, 35, 28)
	fillCircle(img, 18, 22, 12)
	fillCircle(img, 52, 20, 11)
	fillCircle(img, 50, 50, 12)
}

func blobB(img *image.NRGBA) {
	fillRect(img, 10, 10, 50, 50)
	fillCircle(img, 35, 8, 10)
	fillCircle(img, 8, 35, 10)
	clearCircle(img, 35, 62, 10)
	clearCircle(img, 62, 35, 10)
}

func blobC(img *image.NRGBA) {
	fillCircle(img, 35, 35, 26)
	fillRect(img, 30, 5, 10, 60)
	fillRect(img, 5, 30, 60, 10)
}

func fillCircle(img *image.NRGBA, cx, cy, r int) {
	r2 := r * r
	b := img.Bounds()
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			if x < b.Min.X || y < b.Min.Y || x >= b.Max.X || y >= b.Max.Y {
				continue
			}
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= r2 {
				img.SetNRGBA(x, y, color.NRGBA{A: 255})
			}
		}
	}
}

func clearCircle(img *image.NRGBA, cx, cy, r int) {
	r2 := r * r
	b := img.Bounds()
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			if x < b.Min.X || y < b.Min.Y || x >= b.Max.X || y >= b.Max.Y {
				continue
			}
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= r2 {
				img.SetNRGBA(x, y, color.NRGBA{})
			}
		}
	}
}

func fillRect(img *image.NRGBA, x0, y0, w, h int) {
	b := img.Bounds()
	for y := y0; y < y0+h; y++ {
		for x := x0; x < x0+w; x++ {
			if x < b.Min.X || y < b.Min.Y || x >= b.Max.X || y >= b.Max.Y {
				continue
			}
			img.SetNRGBA(x, y, color.NRGBA{A: 255})
		}
	}
}

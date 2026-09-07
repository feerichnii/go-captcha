package antibot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/feerichnii/go-captcha/v2/slide"
)

func TestTechFailKeepsChallenge(t *testing.T) {
	l, err := New(NewMemoryStore(), Config{
		SecretKey: testKey, PoWProbeProb: -1, PoWJitterBits: -1,
		MinSolveTime: time.Millisecond, AllowMissingPiecePress: true,
		DisableSessionWarmup: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	clk := &fakeClock{t: time.Now()}
	l.now = clk.now

	iss, err := l.Issue(context.Background(), IssueRequest{
		Kind: KindSlide, Answer: mustJSON(slide.Block{X: 120, Y: 80, Width: 60, Height: 60}),
		ClientKey: "sid:tech", Signals: testSig(""),
	})
	if err != nil {
		t.Fatal(err)
	}
	clk.advance(2 * time.Second)

	_, err = l.Verify(context.Background(), VerifyRequest{
		ID: iss.ID, Answer: mustJSON(SlideSubmit{X: 120, Y: 80}),
		Trajectory: humanTrajectory(), ClientKey: "sid:tech", Signals: testSig(""),
		// missing JS challenge response → tech fail, challenge kept
	})
	if !errors.Is(err, ErrJSChallengeFailed) {
		t.Fatalf("want ErrJSChallengeFailed, got %v", err)
	}

	browser := BrowserSignals{Languages: []string{"en"}, Platform: "MacIntel", HardwareConcurrency: 8}
	js := jsResponse(iss, browser)
	res, err := l.Verify(context.Background(), VerifyRequest{
		ID: iss.ID, Answer: mustJSON(SlideSubmit{X: 120, Y: 80}),
		Trajectory: humanTrajectory(), ClientKey: "sid:tech", Signals: testSig(""),
		Browser: BrowserSignals{Languages: []string{"en"}, Platform: "MacIntel", HardwareConcurrency: 8, JSChallengeResponse: js},
	})
	if err != nil {
		t.Fatalf("geometry after tech fail should work once: %v", err)
	}
	if res == nil || res.Score < 0 {
		t.Fatalf("%+v", res)
	}
}

func TestPoWFailKeepsChallenge(t *testing.T) {
	l, err := New(NewMemoryStore(), Config{
		SecretKey: testKey, PoWAlways: true, PoWBaseDifficulty: 8,
		PoWProbeProb: -1, PoWJitterBits: -1,
		MinSolveTime: time.Millisecond, AllowNonBrowser: true, AllowMissingPiecePress: true,
		DisableSessionWarmup: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	clk := &fakeClock{t: time.Now()}
	l.now = clk.now
	iss, err := l.Issue(context.Background(), IssueRequest{
		Kind: KindSlide, Answer: mustJSON(slide.Block{X: 120, Y: 80, Width: 60, Height: 60}),
		ClientKey: "sid:pow", Signals: testSig(""),
	})
	if err != nil {
		t.Fatal(err)
	}
	if iss.PoW == nil || iss.PoW.Difficulty <= 0 {
		t.Fatal("expected PoW")
	}
	clk.advance(2 * time.Second)

	_, err = l.Verify(context.Background(), VerifyRequest{
		ID: iss.ID, Answer: mustJSON(SlideSubmit{X: 120, Y: 80}),
		Trajectory: humanTrajectory(), ClientKey: "sid:pow", Signals: testSig(""),
		PoWNonce: "not-a-valid-nonce",
	})
	if !errors.Is(err, ErrPoWInvalid) {
		t.Fatalf("want ErrPoWInvalid, got %v", err)
	}

	nonce, err := SolvePoW(iss.PoW.ChallengeID, iss.PoW.Bind, iss.PoW.Salt, iss.PoW.Difficulty)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Verify(context.Background(), VerifyRequest{
		ID: iss.ID, Answer: mustJSON(SlideSubmit{X: 120, Y: 80}),
		Trajectory: humanTrajectory(), ClientKey: "sid:pow", Signals: testSig(""),
		PoWNonce: nonce,
	}); err != nil {
		t.Fatalf("after PoW fail, correct verify should pass: %v", err)
	}
}

func TestParallelMultiChallengeGeoLock(t *testing.T) {
	l, clk := newLayer(t, Config{VerifyRateMax: 1000, IssueRateMax: 1000})
	const n = 5
	ids := make([]*IssueResponse, n)
	clients := make([]string, n)
	for i := 0; i < n; i++ {
		clients[i] = fmt.Sprintf("sess-%d", i)
		ids[i] = issueSlide(t, l, clk, clients[i])
	}
	bad := mustJSON(SlideSubmit{X: 1, Y: 1})
	var badAns, locked, other int64
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		iss := ids[i]
		client := clients[i]
		go func() {
			defer wg.Done()
			_, err := l.Verify(context.Background(), VerifyRequest{
				ID: iss.ID, Answer: bad, Trajectory: humanTrajectory(),
				ClientKey: client, Signals: testSig(""), PoWNonce: powNonce(t, iss),
			})
			switch {
			case errors.Is(err, ErrBadAnswer):
				atomic.AddInt64(&badAns, 1)
			case errors.Is(err, ErrLocked):
				atomic.AddInt64(&locked, 1)
			default:
				atomic.AddInt64(&other, 1)
			}
		}()
	}
	wg.Wait()
	// One /32 may run only one geometry claim at a time; the winner freezes the IP.
	if badAns != 1 {
		t.Fatalf("want exactly 1 BadAnswer under geo lock, got bad=%d locked=%d other=%d", badAns, locked, other)
	}
	if locked+other < 1 {
		t.Fatalf("expected contention failures, locked=%d other=%d", locked, other)
	}
}

func TestFreezeSurvivesNewSession(t *testing.T) {
	l, clk := newLayer(t, Config{})
	iss := issueSlide(t, l, clk, "cookie-1")
	_, err := l.Verify(context.Background(), VerifyRequest{
		ID: iss.ID, Answer: mustJSON(SlideSubmit{X: 1, Y: 1}),
		Trajectory: humanTrajectory(), ClientKey: "cookie-1", Signals: testSig(""),
	})
	if !errors.Is(err, ErrBadAnswer) {
		t.Fatalf("bad: %v", err)
	}
	retry := RetryAfterMs(err)
	if retry <= 0 {
		t.Fatalf("want retry_after_ms > 0, got %d", retry)
	}

	_, err = l.Issue(context.Background(), IssueRequest{
		Kind: KindSlide, Answer: mustJSON(slide.Block{X: 120, Y: 80, Width: 60, Height: 60}),
		ClientKey: "cookie-new", Signals: testSig(""), // same IP, new session
	})
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("Issue after freeze want ErrLocked, got %v", err)
	}

	iss2, _ := l.Issue(context.Background(), IssueRequest{
		Kind: KindSlide, Answer: mustJSON(slide.Block{X: 120, Y: 80, Width: 60, Height: 60}),
		ClientKey: "cookie-new", Signals: testSig("198.51.100.9"), // different IP ok
	})
	if iss2 == nil {
		t.Fatal("different IP should issue")
	}
}

func TestEscalatingFreezeTTL(t *testing.T) {
	want := []time.Duration{2 * time.Second, 5 * time.Second, 15 * time.Second, 60 * time.Second, 300 * time.Second, 300 * time.Second}
	for i, w := range want {
		got := freezeTTLForBadCount(int64(i + 1))
		if got != w {
			t.Fatalf("bad#%d: got %v want %v", i+1, got, w)
		}
	}

	l, clk := newLayer(t, Config{IssueRateMax: 100, VerifyRateMax: 100})
	ip := "203.0.113.77"
	ipHash := HashIP(testKey, mustAddr(ip))
	for round := 1; round <= 3; round++ {
		_ = l.store.Delete(context.Background(), l.freezeKey(ipHash))
		client := fmt.Sprintf("esc-%d", round)
		iss, err := l.Issue(context.Background(), IssueRequest{
			Kind: KindSlide, Answer: mustJSON(slide.Block{X: 120, Y: 80, Width: 60, Height: 60}),
			ClientKey: client, Signals: testSig(ip),
		})
		if err != nil {
			t.Fatal(err)
		}
		clk.advance(2 * time.Second)
		_, err = l.Verify(context.Background(), VerifyRequest{
			ID: iss.ID, Answer: mustJSON(SlideSubmit{X: 9, Y: 9}),
			Trajectory: humanTrajectory(), ClientKey: client, Signals: testSig(ip),
			PoWNonce: powNonce(t, iss),
		})
		if !errors.Is(err, ErrBadAnswer) {
			t.Fatalf("round %d: %v", round, err)
		}
		got := RetryAfterMs(err)
		wantMs := freezeTTLForBadCount(int64(round)).Milliseconds()
		if got != wantMs {
			t.Fatalf("round %d retry_after_ms=%d want %d", round, got, wantMs)
		}
	}
}

func mustAddr(ip string) netip.Addr {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		panic(err)
	}
	return a
}

func TestTrustedProxiesIgnoresSpoofedXFF(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.50:443"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	addr, err := ClientIP(req, trusted)
	if err != nil {
		t.Fatal(err)
	}
	if addr.String() != "203.0.113.50" {
		t.Fatalf("untrusted peer must use RemoteAddr, got %s", addr)
	}

	req2, _ := http.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "10.0.0.1:443"
	req2.Header.Set("X-Forwarded-For", "198.51.100.20")
	addr2, err := ClientIP(req2, trusted)
	if err != nil {
		t.Fatal(err)
	}
	if addr2.String() != "198.51.100.20" {
		t.Fatalf("trusted peer should honor XFF, got %s", addr2)
	}
}

func TestHMACIPHashStableAndKeyed(t *testing.T) {
	a := netip.MustParseAddr("203.0.113.10")
	h1 := HashIP(testKey, a)
	h2 := HashIP(testKey, a)
	if h1 != h2 || len(h1) != 32 {
		t.Fatalf("stable 128-bit hex: %q %q", h1, h2)
	}
	other := []byte("other-secret-key-32-bytes-long!!")
	if HashIP(other, a) == h1 {
		t.Fatal("different SecretKey must change hash")
	}
}

func TestNoConsumedFieldOrAttemptSpray(t *testing.T) {
	store := NewMemoryStore()
	l, err := New(store, Config{
		SecretKey: testKey, AllowNonBrowser: true, AllowMissingPiecePress: true,
		PoWProbeProb: -1, PoWJitterBits: -1, DisableSessionWarmup: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = l.Verify(context.Background(), VerifyRequest{
		ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Answer: mustJSON(SlideSubmit{X: 1, Y: 1}),
		Trajectory: humanTrajectory(), ClientKey: "c", Signals: testSig(""),
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing challenge: %v", err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	for k := range store.data {
		if len(k) >= 4 && (k[len(k)-4:] == "att:" || containsSubstr([]byte(k), "att:")) {
			t.Fatalf("unexpected att key %q", k)
		}
	}
	raw, _ := json.Marshal(ChallengeRecord{})
	if containsSubstr(raw, "consumed") || containsSubstr(raw, "Consumed") {
		t.Fatal("ChallengeRecord must not serialize Consumed")
	}
}

func TestWarmupClearedOnlyOnCleanSuccess(t *testing.T) {
	l, err := New(NewMemoryStore(), Config{
		SecretKey: testKey, AllowNonBrowser: true, AllowMissingPiecePress: true,
		PoWProbeProb: -1, PoWJitterBits: -1, StretchPoWRiskMin: -1,
		MinSolveTime: time.Millisecond, MinSessionAge: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	clk := &fakeClock{t: time.Now()}
	l.now = clk.now
	client := "sid:warmup"
	hash := hashClient(client)

	iss, err := l.Issue(context.Background(), IssueRequest{
		Kind: KindSlide, Answer: mustJSON(slide.Block{X: 120, Y: 80, Width: 60, Height: 60}),
		ClientKey: client, Signals: testSig(""),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !l.hasWarmup(context.Background(), hash) {
		t.Fatal("warmup should be set after Issue")
	}
	clk.advance(2 * time.Second)

	// Wrong geometry freezes briefly but must not clear warmup.
	_, err = l.Verify(context.Background(), VerifyRequest{
		ID: iss.ID, Answer: mustJSON(SlideSubmit{X: 1, Y: 1}),
		Trajectory: humanTrajectory(), ClientKey: client, Signals: testSig(""),
		PoWNonce: powNonce(t, iss),
	})
	if !errors.Is(err, ErrBadAnswer) {
		t.Fatalf("want bad answer, got %v", err)
	}
	if !l.hasWarmup(context.Background(), hash) {
		t.Fatal("warmup must survive bad geometry")
	}
	_ = l.store.Delete(context.Background(), l.freezeKey(HashIP(testKey, mustAddr(testClientIP))))

	iss2, err := l.Issue(context.Background(), IssueRequest{
		Kind: KindSlide, Answer: mustJSON(slide.Block{X: 120, Y: 80, Width: 60, Height: 60}),
		ClientKey: client, Signals: testSig(""),
	})
	if err != nil {
		t.Fatal(err)
	}
	clk.advance(2 * time.Second)
	if _, err := l.Verify(context.Background(), VerifyRequest{
		ID: iss2.ID, Answer: mustJSON(SlideSubmit{X: 120, Y: 80}),
		Trajectory: humanTrajectory(), ClientKey: client, Signals: testSig(""),
		PoWNonce: powNonce(t, iss2),
	}); err != nil {
		t.Fatal(err)
	}
	if l.hasWarmup(context.Background(), hash) {
		t.Fatal("warmup should clear after clean success")
	}
}

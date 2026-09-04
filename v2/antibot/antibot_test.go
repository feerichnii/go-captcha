package antibot

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/feerichnii/go-captcha/v2/click"
	"github.com/feerichnii/go-captcha/v2/slide"
)

var testKey = []byte("unit-test-secret-key-please-rotate")

// fakeClock lets tests simulate a realistic solve time without sleeping.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newLayer(t *testing.T, cfg Config) (*Layer, *fakeClock) {
	t.Helper()
	cfg.SecretKey = testKey
	l, err := New(NewMemoryStore(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	clk := &fakeClock{t: time.Now()}
	l.now = clk.now
	return l, clk
}

func humanTrajectory() Trajectory {
	pts := make([]Point, 0, 25)
	t0 := int64(1_000)
	x, y := 10.0, 100.0
	for i := 0; i < 25; i++ {
		t0 += int64(16 + i%7)
		x += 3 + float64(i%3)
		y += float64((i%5)-2) * 0.4
		pts = append(pts, Point{X: x, Y: y, T: t0})
	}
	return Trajectory{Points: pts, Events: []string{"pointerdown", "pointermove", "pointerup"}}
}

func botTrajectory() Trajectory {
	pts := make([]Point, 0, 10)
	for i := 0; i < 10; i++ {
		pts = append(pts, Point{X: float64(i * 10), Y: 50, T: 100 + int64(i*5)})
	}
	return Trajectory{Points: pts}
}

func mustJSON(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// issueSlide issues and advances the clock by a plausible human solve time (2s).
func issueSlide(t *testing.T, l *Layer, clk *fakeClock, client string) *IssueResponse {
	t.Helper()
	iss, err := l.Issue(context.Background(), IssueRequest{
		Kind:      KindSlide,
		Answer:    mustJSON(slide.Block{X: 120, Y: 80, Width: 60, Height: 60}),
		ClientKey: client,
	})
	if err != nil {
		t.Fatal(err)
	}
	clk.advance(2 * time.Second)
	return iss
}

func TestNewRequiresSecret(t *testing.T) {
	if _, err := New(NewMemoryStore(), Config{}); !errors.Is(err, ErrNoSecretKey) {
		t.Fatalf("want ErrNoSecretKey, got %v", err)
	}
}

func TestIssueAndVerifySlide(t *testing.T) {
	l, clk := newLayer(t, Config{})
	iss := issueSlide(t, l, clk, "ip-1")
	good := mustJSON(SlideSubmit{X: 122, Y: 81})
	res, err := l.Verify(context.Background(), VerifyRequest{ID: iss.ID, Answer: good, Trajectory: humanTrajectory(), ClientKey: "ip-1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Score <= 0 || res.Risk < 0 {
		t.Fatalf("%+v", res)
	}
	_, err = l.Verify(context.Background(), VerifyRequest{ID: iss.ID, Answer: good, Trajectory: humanTrajectory(), ClientKey: "ip-1"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound after consume, got %v", err)
	}
}

func TestAnswerEncryptedAtRest(t *testing.T) {
	store := NewMemoryStore()
	l, _ := New(store, Config{SecretKey: testKey})
	iss, _ := l.Issue(context.Background(), IssueRequest{Kind: KindSlide, Answer: mustJSON(slide.Block{X: 4242, Y: 1}), ClientKey: "c"})
	raw, _ := store.Get(context.Background(), l.challengeKey(iss.ID))
	if json.Valid(raw) {
		var rec ChallengeRecord
		_ = json.Unmarshal(raw, &rec)
		if json.Valid(rec.Answer) {
			t.Fatal("answer stored in plaintext")
		}
	}
	if string(raw) == "" || containsSubstr(raw, "4242") {
		t.Fatal("answer coordinate visible in stored record")
	}
}

func containsSubstr(b []byte, s string) bool {
	return len(s) > 0 && len(b) >= len(s) && indexOf(b, s) >= 0
}

func indexOf(b []byte, s string) int {
outer:
	for i := 0; i+len(s) <= len(b); i++ {
		for j := 0; j < len(s); j++ {
			if b[i+j] != s[j] {
				continue outer
			}
		}
		return i
	}
	return -1
}

func TestClientBinding(t *testing.T) {
	l, clk := newLayer(t, Config{})
	iss := issueSlide(t, l, clk, "session-A")
	_, err := l.Verify(context.Background(), VerifyRequest{ID: iss.ID, Answer: mustJSON(SlideSubmit{X: 120, Y: 80}), Trajectory: humanTrajectory(), ClientKey: "session-B"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound for foreign client, got %v", err)
	}
	if _, err := l.Verify(context.Background(), VerifyRequest{ID: iss.ID, Answer: mustJSON(SlideSubmit{X: 120, Y: 80}), Trajectory: humanTrajectory(), ClientKey: "session-A"}); err != nil {
		t.Fatalf("owner should still pass: %v", err)
	}
}

func TestMaxAttemptsSequential(t *testing.T) {
	l, clk := newLayer(t, Config{MaxAttempts: 3})
	iss := issueSlide(t, l, clk, "c")
	bad := mustJSON(SlideSubmit{X: 1, Y: 1})
	var errs []error
	for i := 0; i < 4; i++ {
		_, err := l.Verify(context.Background(), VerifyRequest{ID: iss.ID, Answer: bad, Trajectory: humanTrajectory(), ClientKey: "c"})
		errs = append(errs, err)
	}
	if !errors.Is(errs[0], ErrBadAnswer) || !errors.Is(errs[1], ErrBadAnswer) {
		t.Fatalf("first two: %v %v", errs[0], errs[1])
	}
	if !errors.Is(errs[2], ErrMaxAttempts) {
		t.Fatalf("third must hit max attempts, got %v", errs[2])
	}
	if !errors.Is(errs[3], ErrMaxAttempts) && !errors.Is(errs[3], ErrNotFound) {
		t.Fatalf("fourth: %v", errs[3])
	}
}

func TestConcurrentCorrectAnswersConsumeOnce(t *testing.T) {
	l, clk := newLayer(t, Config{MaxAttempts: 100, VerifyRateMax: 1000})
	iss := issueSlide(t, l, clk, "c")
	good := mustJSON(SlideSubmit{X: 120, Y: 80})
	var ok int64
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := l.Verify(context.Background(), VerifyRequest{ID: iss.ID, Answer: good, Trajectory: humanTrajectory(), ClientKey: "c"}); err == nil {
				atomic.AddInt64(&ok, 1)
			}
		}()
	}
	wg.Wait()
	if ok != 1 {
		t.Fatalf("single-use violated: %d successes", ok)
	}
}

func TestConcurrentGuessesBoundedByMaxAttempts(t *testing.T) {
	l, clk := newLayer(t, Config{MaxAttempts: 3, VerifyRateMax: 1000})
	iss := issueSlide(t, l, clk, "c")
	var evaluated int64
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := l.Verify(context.Background(), VerifyRequest{ID: iss.ID, Answer: mustJSON(SlideSubmit{X: i, Y: i}), Trajectory: humanTrajectory(), ClientKey: "c"})
			if errors.Is(err, ErrBadAnswer) {
				atomic.AddInt64(&evaluated, 1)
			}
		}(i)
	}
	wg.Wait()
	if evaluated > 3 {
		t.Fatalf("%d guesses were evaluated; limit 3", evaluated)
	}
}

func TestExpiry(t *testing.T) {
	l, clk := newLayer(t, Config{TTL: 20 * time.Millisecond})
	iss := issueSlide(t, l, clk, "c")
	time.Sleep(30 * time.Millisecond) // memory store TTL uses the real clock
	_, err := l.Verify(context.Background(), VerifyRequest{ID: iss.ID, Answer: mustJSON(SlideSubmit{X: 120, Y: 80}), Trajectory: humanTrajectory(), ClientKey: "c"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound after TTL, got %v", err)
	}
}

func TestTooFast(t *testing.T) {
	l, clk := newLayer(t, Config{MinSolveTime: time.Second})
	iss, _ := l.Issue(context.Background(), IssueRequest{Kind: KindSlide, Answer: mustJSON(slide.Block{X: 1, Y: 1}), ClientKey: "c"})
	clk.advance(100 * time.Millisecond)
	_, err := l.Verify(context.Background(), VerifyRequest{ID: iss.ID, Answer: mustJSON(SlideSubmit{X: 1, Y: 1}), Trajectory: humanTrajectory(), ClientKey: "c"})
	if !errors.Is(err, ErrTooFast) {
		t.Fatalf("want ErrTooFast, got %v", err)
	}
}

func TestVerifyRateLimit(t *testing.T) {
	l, clk := newLayer(t, Config{VerifyRateMax: 2, MaxAttempts: 10})
	iss := issueSlide(t, l, clk, "c")
	bad := mustJSON(SlideSubmit{X: 1, Y: 1})
	for i := 0; i < 2; i++ {
		_, _ = l.Verify(context.Background(), VerifyRequest{ID: iss.ID, Answer: bad, Trajectory: humanTrajectory(), ClientKey: "c"})
	}
	_, err := l.Verify(context.Background(), VerifyRequest{ID: iss.ID, Answer: bad, Trajectory: humanTrajectory(), ClientKey: "c"})
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("want ErrRateLimited on verify, got %v", err)
	}
}

func TestIssueRateLimit(t *testing.T) {
	l, _ := newLayer(t, Config{IssueRateMax: 2})
	ans := mustJSON(slide.Block{X: 1, Y: 1})
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if _, err := l.Issue(ctx, IssueRequest{Kind: KindSlide, Answer: ans, ClientKey: "rl"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := l.Issue(ctx, IssueRequest{Kind: KindSlide, Answer: ans, ClientKey: "rl"}); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("want ErrRateLimited, got %v", err)
	}
	if _, err := l.Issue(ctx, IssueRequest{Kind: KindSlide, Answer: ans, ClientKey: ""}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("empty client key must be rejected, got %v", err)
	}
}

func TestInputLimits(t *testing.T) {
	l, clk := newLayer(t, Config{MaxTrajectoryPoints: 5, MaxNonceLen: 4, MaxAnswerBytes: 16})
	iss := issueSlide(t, l, clk, "c")
	ctx := context.Background()
	big := Trajectory{Points: make([]Point, 6)}
	if _, err := l.Verify(ctx, VerifyRequest{ID: iss.ID, Answer: mustJSON(SlideSubmit{}), Trajectory: big, ClientKey: "c"}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("points cap: %v", err)
	}
	if _, err := l.Verify(ctx, VerifyRequest{ID: iss.ID, Answer: mustJSON(SlideSubmit{}), PoWNonce: "12345", ClientKey: "c"}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("nonce cap: %v", err)
	}
	if _, err := l.Verify(ctx, VerifyRequest{ID: iss.ID, Answer: json.RawMessage(`{"x":1,"y":1,"pad":"xxxxxxxxxxxx"}`), ClientKey: "c"}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("answer cap: %v", err)
	}
	if _, err := l.Verify(ctx, VerifyRequest{ID: "not-hex", Answer: mustJSON(SlideSubmit{}), ClientKey: "c"}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("id format: %v", err)
	}
}

func TestBotScoreEscalatesRiskAndPoW(t *testing.T) {
	l, clk := newLayer(t, Config{PoWBaseDifficulty: 6, RiskThreshold: 0.5})
	ctx := context.Background()
	iss := issueSlide(t, l, clk, "bot")
	if iss.PoW != nil {
		t.Fatal("clean client must not get PoW")
	}
	// Correct geometry but bot-like trajectory: passes, but raises risk.
	res, err := l.Verify(ctx, VerifyRequest{ID: iss.ID, Answer: mustJSON(SlideSubmit{X: 120, Y: 80}), Trajectory: botTrajectory(), ClientKey: "bot"})
	if err != nil {
		t.Fatalf("score must not hard-reject by default: %v", err)
	}
	if res.RiskLevel != 1 || !res.RequirePoWNext {
		t.Fatalf("%+v", res)
	}
	// Next issue carries PoW; wrong nonce fails, solved nonce passes.
	iss2 := issueSlide(t, l, clk, "bot")
	if iss2.PoW == nil || iss2.PoW.Difficulty != 6 {
		t.Fatalf("expected pow at level 1: %+v", iss2.PoW)
	}
	_, err = l.Verify(ctx, VerifyRequest{ID: iss2.ID, Answer: mustJSON(SlideSubmit{X: 120, Y: 80}), Trajectory: humanTrajectory(), PoWNonce: "nope", ClientKey: "bot"})
	if !errors.Is(err, ErrPoWInvalid) {
		t.Fatalf("want ErrPoWInvalid, got %v", err)
	}
	nonce, _ := SolvePoW(iss2.PoW.Salt, iss2.PoW.Difficulty)
	res2, err := l.Verify(ctx, VerifyRequest{ID: iss2.ID, Answer: mustJSON(SlideSubmit{X: 120, Y: 80}), Trajectory: humanTrajectory(), PoWNonce: nonce, ClientKey: "bot"})
	if err != nil {
		t.Fatal(err)
	}
	if res2.RiskLevel != 0 {
		t.Fatalf("clean solve should de-escalate: %+v", res2)
	}
}

func TestHardRejectOptIn(t *testing.T) {
	l, clk := newLayer(t, Config{HardRejectScore: 0.45})
	iss := issueSlide(t, l, clk, "c")
	_, err := l.Verify(context.Background(), VerifyRequest{ID: iss.ID, Answer: mustJSON(SlideSubmit{X: 120, Y: 80}), Trajectory: botTrajectory(), ClientKey: "c"})
	if !errors.Is(err, ErrLowScore) {
		t.Fatalf("want ErrLowScore, got %v", err)
	}
}

func TestInconsistentTrajectoryHalvesScore(t *testing.T) {
	tr := humanTrajectory()
	base := HeuristicScorer{Weights: DefaultWeights()}.Score(tr, ScoreContext{})
	// Client claims ~500ms interaction but only 10ms elapsed server-side.
	inc := HeuristicScorer{Weights: DefaultWeights()}.Score(tr, ScoreContext{ElapsedMs: 10})
	if base.Consistent != true || inc.Consistent != false || inc.Score >= base.Score {
		t.Fatalf("base=%+v inc=%+v", base, inc)
	}
}

func TestTelemetryEmitted(t *testing.T) {
	var mu sync.Mutex
	var events []VerifyEvent
	tel := TelemetryFunc{Verify: func(e VerifyEvent) { mu.Lock(); events = append(events, e); mu.Unlock() }}
	l, clk := newLayer(t, Config{Telemetry: tel})
	iss := issueSlide(t, l, clk, "c")
	_, _ = l.Verify(context.Background(), VerifyRequest{ID: iss.ID, Answer: mustJSON(SlideSubmit{X: 1, Y: 1}), Trajectory: humanTrajectory(), ClientKey: "c"})
	mu.Lock()
	defer mu.Unlock()
	if len(events) != 1 || events[0].Outcome != "bad_answer" || events[0].Score == 0 {
		t.Fatalf("%+v", events)
	}
}

func TestCheckClickServerPadding(t *testing.T) {
	dots := map[int]*click.Dot{0: {Index: 0, X: 10, Y: 10, Width: 20, Height: 20}}
	stored := mustJSON(dots)
	if !CheckClick(stored, mustJSON(ClickSubmit{Points: []click.Point{{X: 15, Y: 15}}}), 2) {
		t.Fatal("expected click match")
	}
	if CheckClick(stored, mustJSON(ClickSubmit{Points: []click.Point{{X: 90, Y: 90}}}), 2) {
		t.Fatal("far click must fail")
	}
}

func TestScoreHumanVsBot(t *testing.T) {
	h := ScoreBehavior(humanTrajectory())
	b := ScoreBehavior(botTrajectory())
	if h.Score <= b.Score {
		t.Fatalf("human=%v bot=%v", h.Score, b.Score)
	}
}

func TestCalibrator(t *testing.T) {
	c := NewCalibrator()
	for i := 0; i < 100; i++ {
		c.Record(0.6+float64(i%40)/100, true)
		c.Record(0.1+float64(i%30)/100, false)
	}
	r := c.Report()
	if r.Humans != 100 || r.Bots != 100 || r.HumanP05 <= r.BotP95 {
		t.Fatalf("%+v", r)
	}
}

func FuzzVerifyPoW(f *testing.F) {
	f.Add("salt", "0", 4)
	f.Add("", "", 0)
	f.Fuzz(func(t *testing.T, salt, nonce string, diff int) {
		_ = VerifyPoW(salt, nonce, diff, 64)
	})
}

func FuzzScoreBehavior(f *testing.F) {
	f.Add(int64(1), int64(2), 3.0)
	f.Fuzz(func(t *testing.T, t0, dt int64, step float64) {
		pts := make([]Point, 0, 8)
		for i := int64(0); i < 8; i++ {
			pts = append(pts, Point{X: float64(i) * step, Y: step, T: t0 + i*dt})
		}
		r := ScoreBehavior(Trajectory{Points: pts})
		if r.Score < 0 || r.Score > 1 {
			t.Fatalf("score out of range: %v", r.Score)
		}
	})
}

func FuzzVerifyRequest(f *testing.F) {
	l, _ := New(NewMemoryStore(), Config{SecretKey: testKey})
	f.Add("00000000000000000000000000000000", `{"x":1}`, "n")
	f.Fuzz(func(t *testing.T, id, answer, nonce string) {
		_, _ = l.Verify(context.Background(), VerifyRequest{ID: id, Answer: json.RawMessage(answer), PoWNonce: nonce, ClientKey: "fz"})
	})
}

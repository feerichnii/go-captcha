package antibot

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/wenlng/go-captcha/v2/click"
	"github.com/wenlng/go-captcha/v2/slide"
)

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
	return Trajectory{
		Points: pts,
		Events: []string{"pointerdown", "pointermove", "pointerup"},
	}
}

func botTrajectory() Trajectory {
	pts := make([]Point, 0, 10)
	t0 := int64(100)
	for i := 0; i < 10; i++ {
		pts = append(pts, Point{X: float64(i * 10), Y: 50, T: t0 + int64(i*5)})
	}
	return Trajectory{Points: pts}
}

func TestIssueAndVerifySlide(t *testing.T) {
	layer := New(NewMemoryStore(), Config{HMACKey: []byte("test-key")})
	answer, _ := json.Marshal(slide.Block{X: 120, Y: 80, Width: 60, Height: 60})
	iss, err := layer.Issue(context.Background(), IssueRequest{
		Kind:      KindSlide,
		Answer:    answer,
		ClientKey: "ip-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	sub, _ := json.Marshal(SlideSubmit{X: 122, Y: 81, Padding: 5})
	res, err := layer.Verify(context.Background(), VerifyRequest{
		ID:         iss.ID,
		Answer:     sub,
		Trajectory: humanTrajectory(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Score <= 0 {
		t.Fatalf("score=%v", res.Score)
	}
	// single-use
	_, err = layer.Verify(context.Background(), VerifyRequest{
		ID:         iss.ID,
		Answer:     sub,
		Trajectory: humanTrajectory(),
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound after consume, got %v", err)
	}
}

func TestMaxAttempts(t *testing.T) {
	layer := New(NewMemoryStore(), Config{MaxAttempts: 3})
	answer, _ := json.Marshal(slide.Block{X: 100, Y: 50})
	iss, err := layer.Issue(context.Background(), IssueRequest{Kind: KindSlide, Answer: answer})
	if err != nil {
		t.Fatal(err)
	}
	bad, _ := json.Marshal(SlideSubmit{X: 10, Y: 10})
	var last error
	for i := 0; i < 3; i++ {
		_, last = layer.Verify(context.Background(), VerifyRequest{
			ID:         iss.ID,
			Answer:     bad,
			Trajectory: humanTrajectory(),
		})
	}
	if !errors.Is(last, ErrMaxAttempts) && !errors.Is(last, ErrBadAnswer) {
		// third failure should be max or bad; fourth must be not found / max
		t.Fatalf("last=%v", last)
	}
	_, err = layer.Verify(context.Background(), VerifyRequest{
		ID:         iss.ID,
		Answer:     bad,
		Trajectory: humanTrajectory(),
	})
	if !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrMaxAttempts) {
		t.Fatalf("want gone after max attempts, got %v", err)
	}
}

func TestLowBehaviorScore(t *testing.T) {
	layer := New(NewMemoryStore(), Config{MinBehaviorScore: 0.45})
	answer, _ := json.Marshal(slide.Block{X: 50, Y: 50})
	iss, _ := layer.Issue(context.Background(), IssueRequest{Kind: KindSlide, Answer: answer})
	sub, _ := json.Marshal(SlideSubmit{X: 50, Y: 50})
	_, err := layer.Verify(context.Background(), VerifyRequest{
		ID:         iss.ID,
		Answer:     sub,
		Trajectory: botTrajectory(),
	})
	if !errors.Is(err, ErrLowScore) {
		t.Fatalf("want ErrLowScore, got %v", err)
	}
}

func TestPoWRequired(t *testing.T) {
	layer := New(NewMemoryStore(), Config{PoWAlways: true, PoWDifficulty: 8})
	answer, _ := json.Marshal(slide.Block{X: 40, Y: 40})
	iss, err := layer.Issue(context.Background(), IssueRequest{Kind: KindSlide, Answer: answer, Suspicious: true})
	if err != nil {
		t.Fatal(err)
	}
	if iss.PoW == nil || iss.PoW.Salt == "" {
		t.Fatal("expected pow challenge")
	}
	sub, _ := json.Marshal(SlideSubmit{X: 40, Y: 40})
	_, err = layer.Verify(context.Background(), VerifyRequest{
		ID:         iss.ID,
		Answer:     sub,
		Trajectory: humanTrajectory(),
		PoWNonce:   "wrong",
	})
	if !errors.Is(err, ErrPoWInvalid) {
		t.Fatalf("want ErrPoWInvalid, got %v", err)
	}
	nonce, err := SolvePoW(iss.PoW.Salt, iss.PoW.Difficulty)
	if err != nil {
		t.Fatal(err)
	}
	// re-issue because previous attempt burned one try but challenge still exists
	_, err = layer.Verify(context.Background(), VerifyRequest{
		ID:         iss.ID,
		Answer:     sub,
		Trajectory: humanTrajectory(),
		PoWNonce:   nonce,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRateLimit(t *testing.T) {
	layer := New(NewMemoryStore(), Config{RateLimitMax: 2, RateLimitWindow: time.Minute})
	answer, _ := json.Marshal(slide.Block{X: 1, Y: 1})
	ctx := context.Background()
	_, err := layer.Issue(ctx, IssueRequest{Kind: KindSlide, Answer: answer, ClientKey: "rl"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = layer.Issue(ctx, IssueRequest{Kind: KindSlide, Answer: answer, ClientKey: "rl"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = layer.Issue(ctx, IssueRequest{Kind: KindSlide, Answer: answer, ClientKey: "rl"})
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("want ErrRateLimited, got %v", err)
	}
}

func TestCheckClick(t *testing.T) {
	dots := map[int]*click.Dot{
		0: {Index: 0, X: 10, Y: 10, Width: 20, Height: 20},
	}
	stored, _ := json.Marshal(dots)
	sub, _ := json.Marshal(ClickSubmit{Points: []click.Point{{X: 15, Y: 15}}, Padding: 2})
	if !CheckClick(stored, sub) {
		t.Fatal("expected click match")
	}
}

func TestScoreHumanVsBot(t *testing.T) {
	h := ScoreBehavior(humanTrajectory())
	b := ScoreBehavior(botTrajectory())
	if h.Score <= b.Score {
		t.Fatalf("human=%v bot=%v", h.Score, b.Score)
	}
}

package antibot

import (
	"context"
	"errors"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/feerichnii/go-captcha/v2/slide"
)

func passPrecheck(t *testing.T, l *Layer, clk *fakeClock, client string) *PrecheckVerifyResult {
	t.Helper()
	ctx := context.Background()
	iss, err := l.PrecheckIssue(ctx, PrecheckIssueRequest{
		ClientKey: client, Signals: testSig(""),
		Capabilities: ClientCapabilities{Protocol: 2, PoW: []string{PoWAlgoSHA256V1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	clk.advance(100 * time.Millisecond)
	nonce, err := SolvePoW(iss.PoW.ChallengeID, iss.PoW.Bind, iss.PoW.Salt, iss.PoW.Difficulty)
	if err != nil {
		t.Fatal(err)
	}
	res, err := l.PrecheckVerify(ctx, PrecheckVerifyRequest{
		PrecheckID: iss.PrecheckID, ClientKey: client, Signals: testSig(""),
		PoWNonce: nonce,
		Interaction: PrecheckInteraction{PointerDownMs: 10, PointerUpMs: 80, Visible: true, HadFocus: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestIssueRequiresPrecheck(t *testing.T) {
	l, _ := newLayer(t, Config{RequirePrecheck: true})
	_, err := l.Issue(context.Background(), IssueRequest{
		Kind: KindSlide, Answer: mustJSON(slide.Block{X: 120, Y: 80, Width: 60, Height: 60}),
		ClientKey: "need-pc", Signals: testSig(""),
	})
	if !errors.Is(err, ErrPrecheckRequired) {
		t.Fatalf("want ErrPrecheckRequired, got %v", err)
	}
}

func TestPrecheckReplayCannotIssueTwice(t *testing.T) {
	l, clk := newLayer(t, Config{RequirePrecheck: true})
	ctx := context.Background()
	key := "pc-replay"
	passPrecheck(t, l, clk, key)
	iss1, err := l.Issue(ctx, IssueRequest{
		Kind: KindSlide, Answer: mustJSON(slide.Block{X: 120, Y: 80, Width: 60, Height: 60}),
		ClientKey: key, Signals: testSig(""),
	})
	if err != nil {
		t.Fatal(err)
	}
	if iss1.ID == "" {
		t.Fatal("expected challenge")
	}
	_, err = l.Issue(ctx, IssueRequest{
		Kind: KindSlide, Answer: mustJSON(slide.Block{X: 120, Y: 80, Width: 60, Height: 60}),
		ClientKey: key, Signals: testSig(""),
	})
	if !errors.Is(err, ErrPrecheckRequired) {
		t.Fatalf("second Issue without new precheck want ErrPrecheckRequired, got %v", err)
	}
}

func TestPrecheckSessionMismatch(t *testing.T) {
	l, clk := newLayer(t, Config{RequirePrecheck: true})
	passPrecheck(t, l, clk, "sess-a")
	_, err := l.Issue(context.Background(), IssueRequest{
		Kind: KindSlide, Answer: mustJSON(slide.Block{X: 120, Y: 80, Width: 60, Height: 60}),
		ClientKey: "sess-b", Signals: testSig(""),
	})
	if !errors.Is(err, ErrPrecheckRequired) {
		t.Fatalf("want ErrPrecheckRequired across sessions, got %v", err)
	}
}

func TestPrecheckIPMismatch(t *testing.T) {
	l, clk := newLayer(t, Config{RequirePrecheck: true})
	ctx := context.Background()
	key := "pc-ip"
	iss, err := l.PrecheckIssue(ctx, PrecheckIssueRequest{
		ClientKey: key, Signals: testSig("203.0.113.10"),
		Capabilities: ClientCapabilities{Protocol: 2, PoW: []string{PoWAlgoSHA256V1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	clk.advance(100 * time.Millisecond)
	nonce, _ := SolvePoW(iss.PoW.ChallengeID, iss.PoW.Bind, iss.PoW.Salt, iss.PoW.Difficulty)
	_, err = l.PrecheckVerify(ctx, PrecheckVerifyRequest{
		PrecheckID: iss.PrecheckID, ClientKey: key, Signals: testSig("198.51.100.1"),
		PoWNonce: nonce,
	})
	if !errors.Is(err, ErrPrecheckExpired) {
		t.Fatalf("IP change want ErrPrecheckExpired, got %v", err)
	}
}

func TestPrecheckBadPoWNoGeometryFreeze(t *testing.T) {
	l, clk := newLayer(t, Config{RequirePrecheck: true})
	ctx := context.Background()
	key := "pc-badpow"
	sig := testSig("")
	iss, err := l.PrecheckIssue(ctx, PrecheckIssueRequest{
		ClientKey: key, Signals: sig,
		Capabilities: ClientCapabilities{Protocol: 2, PoW: []string{PoWAlgoSHA256V1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	clk.advance(100 * time.Millisecond)
	_, err = l.PrecheckVerify(ctx, PrecheckVerifyRequest{
		PrecheckID: iss.PrecheckID, ClientKey: key, Signals: sig, PoWNonce: "0",
	})
	if !errors.Is(err, ErrPoWInvalid) {
		t.Fatalf("want ErrPoWInvalid, got %v", err)
	}
	addr := netip.MustParseAddr(testClientIP)
	ipHash := HashIP(l.cfg.SecretKey, addr)
	if err := l.CheckFrozen(ctx, ipHash); err != nil {
		t.Fatalf("precheck PoW fail must not freeze geometry: %v", err)
	}
	_, err = l.Issue(ctx, IssueRequest{
		Kind: KindSlide, Answer: mustJSON(slide.Block{X: 1, Y: 1, Width: 60, Height: 60}),
		ClientKey: key, Signals: sig,
	})
	if !errors.Is(err, ErrPrecheckRequired) {
		t.Fatalf("no interactive without successful precheck, got %v", err)
	}
}

func TestPrecheckHappyPathThenGeometry(t *testing.T) {
	l, clk := newLayer(t, Config{RequirePrecheck: true})
	ctx := context.Background()
	key := "pc-happy"
	pc := passPrecheck(t, l, clk, key)
	if pc.Status != "challenge_required" {
		t.Fatalf("%+v", pc)
	}
	iss, err := l.Issue(ctx, IssueRequest{
		Kind: KindSlide, Answer: mustJSON(slide.Block{X: 120, Y: 80, Width: 60, Height: 60}),
		ClientKey: key, Signals: testSig(""),
	})
	if err != nil {
		t.Fatal(err)
	}
	clk.advance(2 * time.Second)
	res, err := l.Verify(ctx, VerifyRequest{
		ID: iss.ID, Answer: mustJSON(SlideSubmit{X: 120, Y: 80}),
		Trajectory: humanTrajectory(), ClientKey: key, Signals: testSig(""),
		PoWNonce: powNonce(t, iss),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Score <= 0 {
		t.Fatalf("%+v", res)
	}
}

func TestPrecheckThenWrongGeometryFreezes(t *testing.T) {
	l, clk := newLayer(t, Config{RequirePrecheck: true})
	ctx := context.Background()
	key := "pc-wrong"
	passPrecheck(t, l, clk, key)
	iss, err := l.Issue(ctx, IssueRequest{
		Kind: KindSlide, Answer: mustJSON(slide.Block{X: 120, Y: 80, Width: 60, Height: 60}),
		ClientKey: key, Signals: testSig(""),
	})
	if err != nil {
		t.Fatal(err)
	}
	clk.advance(2 * time.Second)
	_, err = l.Verify(ctx, VerifyRequest{
		ID: iss.ID, Answer: mustJSON(SlideSubmit{X: 1, Y: 1}),
		Trajectory: humanTrajectory(), ClientKey: key, Signals: testSig(""),
		PoWNonce: powNonce(t, iss),
	})
	if !errors.Is(err, ErrBadAnswer) {
		t.Fatalf("want ErrBadAnswer, got %v", err)
	}
	addr := netip.MustParseAddr(testClientIP)
	ipHash := HashIP(l.cfg.SecretKey, addr)
	if err := l.CheckFrozen(ctx, ipHash); err == nil {
		t.Fatal("expected geometry freeze after bad answer")
	}
}

func TestPrecheckOneShotID(t *testing.T) {
	l, clk := newLayer(t, Config{RequirePrecheck: true})
	ctx := context.Background()
	key := "pc-oneshot"
	iss, err := l.PrecheckIssue(ctx, PrecheckIssueRequest{
		ClientKey: key, Signals: testSig(""),
		Capabilities: ClientCapabilities{Protocol: 2, PoW: []string{PoWAlgoSHA256V1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	clk.advance(100 * time.Millisecond)
	nonce, _ := SolvePoW(iss.PoW.ChallengeID, iss.PoW.Bind, iss.PoW.Salt, iss.PoW.Difficulty)
	if _, err := l.PrecheckVerify(ctx, PrecheckVerifyRequest{
		PrecheckID: iss.PrecheckID, ClientKey: key, Signals: testSig(""), PoWNonce: nonce,
	}); err != nil {
		t.Fatal(err)
	}
	_, err = l.PrecheckVerify(ctx, PrecheckVerifyRequest{
		PrecheckID: iss.PrecheckID, ClientKey: key, Signals: testSig(""), PoWNonce: nonce,
	})
	if !errors.Is(err, ErrPrecheckExpired) {
		t.Fatalf("replay precheck_id want ErrPrecheckExpired, got %v", err)
	}
}

func TestPrecheckErrorCodes(t *testing.T) {
	if ErrorCode(ErrPrecheckRequired) != CodePrecheckRequired {
		t.Fatal(ErrorCode(ErrPrecheckRequired))
	}
	if ErrorCode(ErrPrecheckExpired) != CodePrecheckExpired {
		t.Fatal(ErrorCode(ErrPrecheckExpired))
	}
	if ErrorCode(ErrUnsupportedClient) != CodeUnsupportedClient {
		t.Fatal(ErrorCode(ErrUnsupportedClient))
	}
}

func TestPrecheckRateLimit(t *testing.T) {
	l, _ := newLayer(t, Config{RequirePrecheck: true, PrecheckIssueRateMax: 2})
	ctx := context.Background()
	key := "pc-rate"
	caps := ClientCapabilities{Protocol: 2, PoW: []string{PoWAlgoSHA256V1}}
	for i := 0; i < 2; i++ {
		if _, err := l.PrecheckIssue(ctx, PrecheckIssueRequest{ClientKey: key, Signals: testSig(""), Capabilities: caps}); err != nil {
			t.Fatal(err)
		}
	}
	_, err := l.PrecheckIssue(ctx, PrecheckIssueRequest{ClientKey: key, Signals: testSig(""), Capabilities: caps})
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("want ErrRateLimited, got %v", err)
	}
}

func TestConcurrentPrecheckVerifyOneWins(t *testing.T) {
	l, clk := newLayer(t, Config{RequirePrecheck: true})
	ctx := context.Background()
	key := "pc-race"
	iss, err := l.PrecheckIssue(ctx, PrecheckIssueRequest{
		ClientKey: key, Signals: testSig(""),
		Capabilities: ClientCapabilities{Protocol: 2, PoW: []string{PoWAlgoSHA256V1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	clk.advance(100 * time.Millisecond)
	nonce, _ := SolvePoW(iss.PoW.ChallengeID, iss.PoW.Bind, iss.PoW.Salt, iss.PoW.Difficulty)

	var wg sync.WaitGroup
	var okN, failN int
	var mu sync.Mutex
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := l.PrecheckVerify(ctx, PrecheckVerifyRequest{
				PrecheckID: iss.PrecheckID, ClientKey: key, Signals: testSig(""), PoWNonce: nonce,
			})
			mu.Lock()
			if err == nil {
				okN++
			} else {
				failN++
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	if okN != 1 || failN != 7 {
		t.Fatalf("one-shot race: ok=%d fail=%d", okN, failN)
	}
}

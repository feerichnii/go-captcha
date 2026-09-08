package antibot

import (
	"context"
	"errors"
	"testing"

	"github.com/feerichnii/go-captcha/v2/slide"
)

func TestStretchPoWRiskMinZeroMeansOff(t *testing.T) {
	cfg := (&Config{}).withDefaults()
	if cfg.StretchPoWRiskMin != 0 {
		t.Fatalf("StretchPoWRiskMin default want 0 (off), got %d", cfg.StretchPoWRiskMin)
	}
	cfg2 := (&Config{StretchPoWRiskMin: 0}).withDefaults()
	if cfg2.StretchPoWRiskMin != 0 {
		t.Fatalf("explicit 0 must stay off, got %d", cfg2.StretchPoWRiskMin)
	}
}

func TestStretchNotIssuedWithoutCapability(t *testing.T) {
	l, _ := newLayer(t, Config{
		StretchPoWRiskMin: 1,
		PoWAlways:         true,
		PoWBaseDifficulty: 6,
		PoWMaxDifficulty:  8,
		PoWStepPerLevel:   1,
	})
	ctx := context.Background()
	iss, err := l.Issue(ctx, IssueRequest{
		Kind: KindSlide, Answer: mustJSON(slide.Block{X: 120, Y: 80, Width: 60, Height: 60}),
		ClientKey: "stretch-cap", Signals: testSig(""), Suspicious: true,
		Capabilities: ClientCapabilities{PoW: []string{PoWAlgoSHA256V1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if iss.PoW == nil {
		t.Fatal("expected sha256 PoW")
	}
	if iss.PoW.Kind == PoWKindStretch {
		t.Fatalf("stretch must not issue without stretch-v2 capability: %+v", iss.PoW)
	}
}

func TestStretchIssuedOnlyWithCapability(t *testing.T) {
	l, _ := newLayer(t, Config{
		StretchPoWRiskMin: 1,
		StretchMemoryMB:   1,
		StretchRounds:     1,
		PoWAlways:         true,
		PoWBaseDifficulty: 6,
		PoWMaxDifficulty:  8,
	})
	iss, err := l.Issue(context.Background(), IssueRequest{
		Kind: KindSlide, Answer: mustJSON(slide.Block{X: 120, Y: 80, Width: 60, Height: 60}),
		ClientKey: "stretch-on", Signals: testSig(""), Suspicious: true,
		Capabilities: ClientCapabilities{PoW: []string{PoWAlgoSHA256V1, PoWAlgoStretchV2}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if iss.PoW == nil || iss.PoW.Kind != PoWKindStretch {
		t.Fatalf("want stretch PoW when advertised, got %+v", iss.PoW)
	}
}

func TestErrorCodeMapping(t *testing.T) {
	cases := []struct {
		err  error
		code string
	}{
		{ErrPoWInvalid, CodePoWInvalid},
		{ErrJSChallengeFailed, CodeJSFailed},
		{ErrTooFast, CodeTooFast},
		{ErrBadAnswer, CodeBadGeometry},
		{&BadAnswerError{RetryAfterMs: 2000}, CodeBadGeometry},
		{ErrLocked, CodeLocked},
		{ErrNotFound, CodeNotFound},
		{ErrRateLimited, CodeRateLimited},
		{ErrLowScore, CodeLowScore},
		{errors.New("redis down"), CodeInternal},
	}
	for _, tc := range cases {
		if got := ErrorCode(tc.err); got != tc.code {
			t.Fatalf("%v: want %q got %q", tc.err, tc.code, got)
		}
	}
}

func TestPreflightIssueFrozen(t *testing.T) {
	l, clk := newLayer(t, Config{})
	ctx := context.Background()
	key := "preflight"
	sig := testSig("")
	iss := issueSlide(t, l, clk, key)
	_, err := l.Verify(ctx, VerifyRequest{
		ID: iss.ID, Answer: mustJSON(SlideSubmit{X: 1, Y: 1}),
		Trajectory: humanTrajectory(), ClientKey: key, Signals: sig,
	})
	if !errors.Is(err, ErrBadAnswer) {
		t.Fatalf("want bad answer to freeze, got %v", err)
	}
	if err := l.PreflightIssue(ctx, key, sig); !errors.Is(err, ErrLocked) {
		t.Fatalf("preflight want ErrLocked, got %v", err)
	}
}

func TestHardRejectBeforeFinalizeSuccess(t *testing.T) {
	l, clk := newLayer(t, Config{HardRejectScore: 0.45})
	ctx := context.Background()
	key := "hard-rej"
	iss := issueSlide(t, l, clk, key)
	_, err := l.Verify(ctx, VerifyRequest{
		ID: iss.ID, Answer: mustJSON(SlideSubmit{X: 120, Y: 80}),
		Trajectory: botTrajectory(), ClientKey: key, Signals: testSig(""),
	})
	if !errors.Is(err, ErrLowScore) {
		t.Fatalf("want ErrLowScore, got %v", err)
	}
	_, err = l.Verify(ctx, VerifyRequest{
		ID: iss.ID, Answer: mustJSON(SlideSubmit{X: 120, Y: 80}),
		Trajectory: humanTrajectory(), ClientKey: key, Signals: testSig(""),
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("consumed challenge want ErrNotFound, got %v", err)
	}
	level, err := l.RiskLevel(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if level < 1 {
		t.Fatalf("low-score should escalate risk as Failed, level=%d", level)
	}
}

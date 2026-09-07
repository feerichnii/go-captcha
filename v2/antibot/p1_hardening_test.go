package antibot

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/feerichnii/go-captcha/v2/slide"
)

func TestPoWBindingRejectsUnboundNonce(t *testing.T) {
	l, clk := newLayer(t, Config{PoWAlways: true, PoWBaseDifficulty: 8, PoWProbeProb: -1, PoWJitterBits: -1})
	iss := issueSlide(t, l, clk, "pow-bind")
	if iss.PoW == nil {
		t.Fatal("expected PoW")
	}
	if VerifyPoW(iss.PoW.ChallengeID, "wrong-bind", iss.PoW.Salt, "0", iss.PoW.Difficulty, 64) {
		t.Fatal("wrong bind must fail")
	}
	_, err := l.Verify(context.Background(), VerifyRequest{
		ID: iss.ID, Answer: mustJSON(SlideSubmit{X: 120, Y: 80}),
		Trajectory: humanTrajectory(), ClientKey: "pow-bind", Signals: testSig(""),
		PoWNonce: "0",
	})
	if !errors.Is(err, ErrPoWInvalid) {
		t.Fatalf("want ErrPoWInvalid, got %v", err)
	}
	if _, err := verifyOK(t, l, iss, "pow-bind", mustJSON(SlideSubmit{X: 120, Y: 80}), humanTrajectory()); err != nil {
		t.Fatal(err)
	}
}

func TestJSRejectsHardcodedProbeFillers(t *testing.T) {
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
		ClientKey: "sid:js", Signals: testSig(""),
	})
	if err != nil {
		t.Fatal(err)
	}
	clk.advance(2 * time.Second)

	// Client reports 2 languages but sends response for probe "3".
	browser := BrowserSignals{Languages: []string{"en", "fr"}, Platform: "MacIntel", HardwareConcurrency: 8}
	bad := ExpectedJSResponse(iss.JSChallenge.Nonce, iss.ID, iss.JSChallenge.Token, WorkloadDigest(*iss.JSChallenge), "3")
	_, err = l.Verify(context.Background(), VerifyRequest{
		ID: iss.ID, Answer: mustJSON(SlideSubmit{X: 120, Y: 80}),
		Trajectory: humanTrajectory(), ClientKey: "sid:js", Signals: testSig(""),
		Browser: BrowserSignals{Languages: browser.Languages, Platform: browser.Platform, HardwareConcurrency: 8, JSChallengeResponse: bad},
	})
	if !errors.Is(err, ErrJSChallengeFailed) {
		t.Fatalf("want ErrJSChallengeFailed for filler probe, got %v", err)
	}

	good := jsResponse(iss, browser)
	if _, err := l.Verify(context.Background(), VerifyRequest{
		ID: iss.ID, Answer: mustJSON(SlideSubmit{X: 120, Y: 80}),
		Trajectory: humanTrajectory(), ClientKey: "sid:js", Signals: testSig(""),
		Browser: BrowserSignals{Languages: browser.Languages, Platform: browser.Platform, HardwareConcurrency: 8, JSChallengeResponse: good},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestHardModeActivatesOnGlobalBadSpike(t *testing.T) {
	l, clk := newLayer(t, Config{
		HardModeAbsBad: 5, HardModeMinIssues: 1000, // force abs path
		DisableHardMode: false,
	})
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		l.noteGlobalBad(ctx)
	}
	hm := l.HardMode(ctx)
	if !hm.Active || hm.ExtraPoWBits < 1 || hm.TTLMillis <= 0 {
		t.Fatalf("expected active hard mode, got %+v", hm)
	}
	iss := issueSlide(t, l, clk, "hm")
	if iss.HardMode == nil || !iss.HardMode.Active {
		t.Fatalf("Issue should surface hard mode: %+v", iss.HardMode)
	}
	if iss.TTLSeconds >= int64(l.cfg.TTL/time.Second) {
		t.Fatalf("hard mode should shorten TTL: got %d cfg %d", iss.TTLSeconds, l.cfg.TTL/time.Second)
	}
}

func TestCalibratorROCAndF1(t *testing.T) {
	c := NewCalibrator()
	for _, s := range []float64{0.9, 0.85, 0.8, 0.75} {
		c.Record(s, true)
	}
	for _, s := range []float64{0.2, 0.25, 0.3, 0.15} {
		c.Record(s, false)
	}
	r := c.Report()
	if r.BestF1 <= 0 || r.SuggestedHardReject <= 0 {
		t.Fatalf("%+v", r)
	}
	roc := c.ROC()
	if len(roc) == 0 {
		t.Fatal("expected ROC points")
	}
}

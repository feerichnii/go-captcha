package antibot

import (
	"context"
	"testing"
	"time"

	"github.com/feerichnii/go-captcha/v2/slide"
)

func BenchmarkVerifyPoWBound(b *testing.B) {
	id, bind, salt := "cid", "sess", "0123456789abcdef0123456789abcdef"
	nonce, err := SolvePoW(id, bind, salt, 8)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !VerifyPoW(id, bind, salt, nonce, 8, 64) {
			b.Fatal("verify failed")
		}
	}
}

func BenchmarkIssueVerifyHappyPath(b *testing.B) {
	l, err := New(NewMemoryStore(), Config{
		SecretKey: testKey, AllowNonBrowser: true, AllowMissingPiecePress: true,
		PoWProbeProb: -1, PoWJitterBits: -1, StretchPoWRiskMin: -1,
		DisableSessionWarmup: true, MinSolveTime: time.Millisecond,
		IssueRateMax: 1_000_000, VerifyRateMax: 1_000_000,
		GlobalIssueRateMax: 1_000_000, GlobalVerifyRateMax: 1_000_000,
	})
	if err != nil {
		b.Fatal(err)
	}
	clk := &fakeClock{t: time.Now()}
	l.now = clk.now
	ans := mustJSON(slide.Block{X: 120, Y: 80, Width: 60, Height: 60})
	good := mustJSON(SlideSubmit{X: 120, Y: 80})
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		iss, err := l.Issue(ctx, IssueRequest{
			Kind: KindSlide, Answer: ans,
			ClientKey: "bench", Signals: testSig(""),
		})
		if err != nil {
			b.Fatal(err)
		}
		var nonce string
		if iss.PoW != nil && iss.PoW.Difficulty > 0 {
			nonce, err = SolvePoW(iss.PoW.ChallengeID, iss.PoW.Bind, iss.PoW.Salt, iss.PoW.Difficulty)
			if err != nil {
				b.Fatal(err)
			}
		}
		clk.advance(2 * time.Second)
		b.StartTimer()
		_, err = l.Verify(ctx, VerifyRequest{
			ID: iss.ID, Answer: good, Trajectory: humanTrajectory(),
			ClientKey: "bench", Signals: testSig(""), PoWNonce: nonce,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTrajectoryFingerprint(b *testing.B) {
	tr := humanTrajectory()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = TrajectoryFingerprint(tr)
	}
}

package antibot

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/feerichnii/go-captcha/v2/slide"
)

func TestJSWorkloadLoopRoundTrip(t *testing.T) {
	ch, err := NewJSChallenge()
	if err != nil {
		t.Fatal(err)
	}
	ch.Workload = JSWorkloadLoop
	ch.LoopCount = 40
	id := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	work := WorkloadDigest(ch)
	resp := ExpectedJSResponse(ch.Nonce, id, ch.Token, work, "1")
	if !CheckJSChallenge(id, ch, resp, "1") {
		t.Fatal("loop workload must verify")
	}
}

func TestTrajectoryReplayRaisesRiskSignal(t *testing.T) {
	l, _ := newLayer(t, Config{})
	ctx := context.Background()
	ipHash := HashIP(testKey, mustAddr(testClientIP))
	tr := humanTrajectory()
	replay, err := l.CheckTrajectoryReplay(ctx, ipHash, tr)
	if err != nil || replay {
		t.Fatalf("first peek: replay=%v err=%v", replay, err)
	}
	l.NoteTrajectoryReplay(ctx, ipHash, tr)
	replay, err = l.CheckTrajectoryReplay(ctx, ipHash, tr)
	if err != nil || !replay {
		t.Fatalf("after note must be replay, got %v err=%v", replay, err)
	}
}

func TestBrowserConsistencySoftRisk(t *testing.T) {
	d, reasons := BrowserConsistencyRisk(BrowserSignals{
		Platform: "Win32", Languages: nil, MaxTouchPoints: 0,
	}, "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)")
	if d < 1 {
		t.Fatalf("want mismatch delta, got %d %v", d, reasons)
	}
}

func TestPoWDifficultyForP95(t *testing.T) {
	d := PoWDifficultyForP95Ms(100_000, 500) // 50k hashes → ~2^15.6
	if d < 10 || d > 20 {
		t.Fatalf("got %d", d)
	}
}

type stubRep struct{ n int }

func (s stubRep) RiskDelta(addr netip.Addr, sessionKey, deviceKey string) int { return s.n }

func TestReputationProviderRaisesIssueRisk(t *testing.T) {
	l, err := New(NewMemoryStore(), Config{
		SecretKey: testKey, AllowNonBrowser: true, AllowMissingPiecePress: true,
		PoWProbeProb: -1, PoWJitterBits: -1, StretchPoWRiskMin: -1,
		DisableSessionWarmup: true, ReputationProvider: stubRep{n: 2},
		PoWAlways: true, PoWBaseDifficulty: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	iss, err := l.Issue(context.Background(), IssueRequest{
		Kind: KindSlide, Answer: mustJSON(slide.Block{X: 120, Y: 80, Width: 60, Height: 60}),
		ClientKey: "sid:rep", Signals: testSig(""),
	})
	if err != nil {
		t.Fatal(err)
	}
	if iss.RiskLevel < 2 {
		t.Fatalf("reputation should bump risk, got %d", iss.RiskLevel)
	}
	if iss.PoW == nil {
		t.Fatal("expected PoW from elevated risk")
	}
}

func TestRequireBrowserSignalsAlias(t *testing.T) {
	c := Config{}
	if c.RequireBrowser() != c.RequireBrowserSignals() {
		t.Fatal("aliases must match")
	}
	c.AllowNonBrowser = true
	if c.RequireBrowserSignals() {
		t.Fatal("opt-out")
	}
}

func TestStretchPoWRoundTrip(t *testing.T) {
	id, bind, salt := "cid", "sess", "0123456789abcdef0123456789abcdef"
	nonce, err := SolveStretchPoW(id, bind, salt, 4, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyStretchPoW(id, bind, salt, nonce, 4, 1, 1, 64) {
		t.Fatal("stretch verify failed")
	}
	if VerifyStretchPoW(id, "other", salt, nonce, 4, 1, 1, 64) {
		t.Fatal("wrong bind must fail")
	}
}

func TestDynamicsAndIntervalsComponents(t *testing.T) {
	tr := humanTrajectory()
	sr := ScoreBehavior(tr)
	if sr.Components["intervals"] <= 0 || sr.Components["dynamics"] <= 0 {
		t.Fatalf("missing components: %+v", sr.Components)
	}
	_ = errors.New
	_ = time.Second
}

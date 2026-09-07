package antibot

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/feerichnii/go-captcha/v2/slide"
)

func TestGenerateSecretKey(t *testing.T) {
	k, err := GenerateSecretKey(32)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSecretKey(k); err != nil {
		t.Fatal(err)
	}
}

func TestPrivacyHashStable(t *testing.T) {
	a := PrivacyHash(testKey, "fp:canvas", "abc")
	b := PrivacyHash(testKey, "fp:canvas", "abc")
	if a != b || len(a) != 32 {
		t.Fatalf("%q %q", a, b)
	}
	if PrivacyHash(testKey, "fp:canvas", "abd") == a {
		t.Fatal("different value must differ")
	}
	if PrivacyHash(testKey, "ip", "abc") == a {
		t.Fatal("domain must separate")
	}
}

func TestFingerprintRiskTrivial(t *testing.T) {
	d, _ := FingerprintRisk(BrowserSignals{CanvasHash: "00000000"})
	if d < 1 {
		t.Fatal("trivial hash should raise risk")
	}
	d, _ = FingerprintRisk(BrowserSignals{})
	if d != 0 {
		t.Fatal("empty fingerprints ok")
	}
}

func TestInvisibleIssueAndVerify(t *testing.T) {
	l, err := New(NewMemoryStore(), Config{
		SecretKey: testKey, AllowNonBrowser: true, AllowMissingPiecePress: true,
		PoWProbeProb: -1, PoWJitterBits: -1, StretchPoWRiskMin: -1,
		DisableSessionWarmup: true, EnableInvisible: true, InvisibleMaxRisk: 0,
		MinSolveTime: time.Millisecond, PoWProbeDifficulty: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	clk := &fakeClock{t: time.Now()}
	l.now = clk.now

	iss, err := l.Issue(context.Background(), IssueRequest{
		Kind: KindSlide, Answer: mustJSON(slide.Block{X: 1, Y: 1}),
		ClientKey: "sid:inv", Signals: testSig(""), PreferInvisible: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if iss.Kind != KindInvisible || iss.Mode != "invisible" {
		t.Fatalf("%+v", iss)
	}
	if iss.PoW == nil {
		t.Fatal("invisible should carry PoW")
	}
	clk.advance(2 * time.Second)
	nonce := powNonce(t, iss)
	res, err := l.Verify(context.Background(), VerifyRequest{
		ID: iss.ID, Answer: mustJSON(map[string]bool{"ok": true}),
		ClientKey: "sid:inv", Signals: testSig(""), PoWNonce: nonce,
		Trajectory: Trajectory{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
}

func TestInvisibleDeniedWhenRisky(t *testing.T) {
	l, err := New(NewMemoryStore(), Config{
		SecretKey: testKey, AllowNonBrowser: true, AllowMissingPiecePress: true,
		PoWProbeProb: -1, PoWJitterBits: -1, StretchPoWRiskMin: -1,
		DisableSessionWarmup: true, EnableInvisible: true, InvisibleMaxRisk: 0,
		ReputationProvider: stubRep{n: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	iss, err := l.Issue(context.Background(), IssueRequest{
		Kind: KindSlide, Answer: mustJSON(slide.Block{X: 1, Y: 1, Width: 60, Height: 60}),
		ClientKey: "sid:inv2", Signals: testSig(""), PreferInvisible: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if iss.Kind == KindInvisible {
		t.Fatal("elevated risk must keep visual challenge")
	}
	_ = errors.New
}

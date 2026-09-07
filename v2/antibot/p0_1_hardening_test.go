package antibot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/feerichnii/go-captcha/v2/slide"
)

func TestDistinctSessionRotation(t *testing.T) {
	l, clk := newLayer(t, Config{IssueRateMax: 1000})
	ip := "203.0.113.50"
	ipHash := HashIP(testKey, mustAddr(ip))
	ctx := context.Background()

	for i := 0; i < 8; i++ {
		_, err := l.Issue(ctx, IssueRequest{
			Kind: KindSlide, Answer: mustJSON(slide.Block{X: 1, Y: 1, Width: 60, Height: 60}),
			ClientKey: "sid:same", Signals: testSig(ip),
		})
		if err != nil {
			t.Fatal(err)
		}
		clk.advance(time.Millisecond)
	}
	n, err := l.store.IncrBy(ctx, l.sessRotKey(ipHash), 0, l.cfg.FailRateWindow)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("same session: sessrot want 1, got %d", n)
	}

	l2, _ := newLayer(t, Config{IssueRateMax: 1000})
	ip2 := "203.0.113.51"
	ipHash2 := HashIP(testKey, mustAddr(ip2))
	for i := 0; i < 8; i++ {
		_, err := l2.Issue(ctx, IssueRequest{
			Kind: KindSlide, Answer: mustJSON(slide.Block{X: 1, Y: 1, Width: 60, Height: 60}),
			ClientKey: fmt.Sprintf("sid:rot-%d", i), Signals: testSig(ip2),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	n2, err := l2.store.IncrBy(ctx, l2.sessRotKey(ipHash2), 0, l2.cfg.FailRateWindow)
	if err != nil {
		t.Fatal(err)
	}
	if n2 != 8 {
		t.Fatalf("distinct sessions: sessrot want 8, got %d", n2)
	}
}

func TestPrefetchReplacedByActive(t *testing.T) {
	l, clk := newLayer(t, Config{})
	a := issueSlide(t, l, clk, "sess-a")
	b := issueSlide(t, l, clk, "sess-a") // same IP via testSig — replaces A
	_, err := l.Verify(context.Background(), VerifyRequest{
		ID: a.ID, Answer: mustJSON(SlideSubmit{X: 120, Y: 80}),
		Trajectory: humanTrajectory(), ClientKey: "sess-a", Signals: testSig(""),
		PoWNonce: powNonce(t, a),
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("superseded challenge A want NotFound, got %v", err)
	}
	if _, err := verifyOK(t, l, b, "sess-a", mustJSON(SlideSubmit{X: 120, Y: 80}), humanTrajectory()); err != nil {
		t.Fatalf("active B should work: %v", err)
	}
}

func TestEpochInvalidatesAfterBadGeometry(t *testing.T) {
	l, clk := newLayer(t, Config{IssueRateMax: 100})
	ip := "203.0.113.60"
	iss := issueWithIP(t, l, clk, "s1", ip)
	_, err := l.Verify(context.Background(), VerifyRequest{
		ID: iss.ID, Answer: mustJSON(SlideSubmit{X: 1, Y: 1}),
		Trajectory: humanTrajectory(), ClientKey: "s1", Signals: testSig(ip),
	})
	if !errors.Is(err, ErrBadAnswer) {
		t.Fatalf("bad: %v", err)
	}
	_ = l.store.Delete(context.Background(), l.freezeKey(HashIP(testKey, mustAddr(ip))))

	// Manually plant a stale-epoch challenge (simulates leaked old record).
	stale := &ChallengeRecord{
		ID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Kind: KindSlide,
		ClientHash: hashClient("s2"), IPHash: HashIP(testKey, mustAddr(ip)),
		IPEpoch: 0, Answer: []byte("x"),
		CreatedAtMs: clk.now().UnixMilli(), ExpiresAtMs: clk.now().Add(time.Minute).UnixMilli(),
	}
	raw, _ := encodeRecord(stale)
	_ = l.store.Set(context.Background(), l.challengeKey(stale.IPHash, stale.ID), raw, time.Minute)
	_, err = l.Verify(context.Background(), VerifyRequest{
		ID: stale.ID, Answer: mustJSON(SlideSubmit{X: 1, Y: 1}),
		Trajectory: humanTrajectory(), ClientKey: "s2", Signals: testSig(ip),
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("stale epoch want NotFound, got %v", err)
	}
}

func issueWithIP(t *testing.T, l *Layer, clk *fakeClock, client, ip string) *IssueResponse {
	t.Helper()
	iss, err := l.Issue(context.Background(), IssueRequest{
		Kind: KindSlide, Answer: mustJSON(slide.Block{X: 120, Y: 80, Width: 60, Height: 60}),
		ClientKey: client, Signals: testSig(ip),
	})
	if err != nil {
		t.Fatal(err)
	}
	clk.advance(2 * time.Second)
	return iss
}

func TestParallelIssueOneActive(t *testing.T) {
	l, _ := newLayer(t, Config{IssueRateMax: 1000})
	var wg sync.WaitGroup
	ids := make(chan string, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			iss, err := l.Issue(context.Background(), IssueRequest{
				Kind: KindSlide, Answer: mustJSON(slide.Block{X: 1, Y: 1, Width: 60, Height: 60}),
				ClientKey: fmt.Sprintf("p-%d", i), Signals: testSig("198.51.100.1"),
			})
			if err != nil {
				return
			}
			ids <- iss.ID
		}(i)
	}
	wg.Wait()
	close(ids)
	ipHash := HashIP(testKey, mustAddr("198.51.100.1"))
	active, err := l.store.Get(context.Background(), l.activeKey(ipHash))
	if err != nil {
		t.Fatal(err)
	}
	activeID := string(active)
	var found int
	for id := range ids {
		if id == activeID {
			found++
		}
		_, err := l.store.Get(context.Background(), l.challengeKey(ipHash, id))
		if id == activeID {
			if err != nil {
				t.Fatalf("active challenge missing: %v", err)
			}
		} else if err == nil {
			t.Fatalf("superseded challenge %s still present", id)
		}
	}
	if found != 1 {
		t.Fatalf("active id among issued want 1 match, got %d", found)
	}
}

func TestStaleFinalizeFailureNoSideEffects(t *testing.T) {
	l, clk := newLayer(t, Config{})
	iss := issueSlide(t, l, clk, "stale-fin")
	ipHash := HashIP(testKey, mustAddr(testClientIP))
	claim, err := l.ClaimGeometry(context.Background(), iss.ID, hashClient("stale-fin"), ipHash)
	if err != nil {
		t.Fatal(err)
	}
	// Correct finalize releases geo; stale failure must no-op.
	if err := l.FinalizeSuccess(context.Background(), ipHash, claim.ClaimToken); err != nil {
		t.Fatal(err)
	}
	epochBefore, _ := l.currentEpoch(context.Background(), ipHash)
	retry, err := l.FinalizeFailure(context.Background(), hashClient("stale-fin"), ipHash, claim.ClaimToken)
	if err != nil {
		t.Fatal(err)
	}
	if retry != 0 {
		t.Fatalf("stale failure retry want 0, got %d", retry)
	}
	if err := l.CheckFrozen(context.Background(), ipHash); err != nil {
		t.Fatalf("stale must not freeze: %v", err)
	}
	epochAfter, _ := l.currentEpoch(context.Background(), ipHash)
	if epochAfter != epochBefore {
		t.Fatalf("epoch changed on stale finalize: %d → %d", epochBefore, epochAfter)
	}
}

func TestFinalizeAbortNoFreeze(t *testing.T) {
	l, clk := newLayer(t, Config{})
	iss := issueSlide(t, l, clk, "abort")
	ipHash := HashIP(testKey, mustAddr(testClientIP))
	hash := hashClient("abort")
	claim, err := l.ClaimGeometry(context.Background(), iss.ID, hash, ipHash)
	if err != nil {
		t.Fatal(err)
	}
	epochBefore, _ := l.currentEpoch(context.Background(), ipHash)
	riskBefore, _ := l.store.IncrBy(context.Background(), l.riskIPKey(ipHash), 0, l.cfg.RiskTTL)
	if err := l.FinalizeAbort(context.Background(), ipHash, claim.ClaimToken); err != nil {
		t.Fatal(err)
	}
	if err := l.CheckFrozen(context.Background(), ipHash); err != nil {
		t.Fatalf("abort must not freeze: %v", err)
	}
	epochAfter, _ := l.currentEpoch(context.Background(), ipHash)
	if epochAfter != epochBefore {
		t.Fatal("abort must not bump epoch")
	}
	riskAfter, _ := l.store.IncrBy(context.Background(), l.riskIPKey(ipHash), 0, l.cfg.RiskTTL)
	if riskAfter != riskBefore {
		t.Fatal("abort must not bump risk")
	}
	_, err = l.store.Get(context.Background(), l.geoKey(ipHash))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("geo should be released, got %v", err)
	}
}

func TestGeoLockTTLFromConfig(t *testing.T) {
	l, clk := newLayer(t, Config{GeoLockTTL: time.Second, VerifyRateMax: 100})
	if l.geoLockTTL() != time.Second {
		t.Fatalf("geoLockTTL=%v", l.geoLockTTL())
	}
	iss := issueSlide(t, l, clk, "geo-ttl")
	ipHash := HashIP(testKey, mustAddr(testClientIP))
	hash := hashClient("geo-ttl")
	claim, err := l.ClaimGeometry(context.Background(), iss.ID, hash, ipHash)
	if err != nil {
		t.Fatal(err)
	}
	_ = claim
	// Second claim while geo held — Issue a new one first won't work (geo busy on claim).
	iss2, err := l.Issue(context.Background(), IssueRequest{
		Kind: KindSlide, Answer: mustJSON(slide.Block{X: 120, Y: 80, Width: 60, Height: 60}),
		ClientKey: "geo-ttl-2", Signals: testSig(""),
	})
	if err != nil {
		t.Fatal(err)
	}
	clk.advance(2 * time.Second)
	_, err = l.ClaimGeometry(context.Background(), iss2.ID, hashClient("geo-ttl-2"), ipHash)
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("want geo busy Locked, got %v", err)
	}
	if RetryAfterMs(err) != time.Second.Milliseconds() {
		t.Fatalf("retry_after want 1000, got %d", RetryAfterMs(err))
	}
}

func TestIPKeysUseHashTag(t *testing.T) {
	l, _ := newLayer(t, Config{})
	ipHash := HashIP(testKey, mustAddr(testClientIP))
	keys := []string{
		l.freezeKey(ipHash), l.geoKey(ipHash), l.badGeoKey(ipHash), l.epochKey(ipHash),
		l.activeKey(ipHash), l.riskIPKey(ipHash), l.sessRotKey(ipHash),
		l.challengeKey(ipHash, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		l.bindLockKey("sess", ipHash), l.sessSeenKey(ipHash, "sess"),
	}
	tag := "{" + ipHash + "}"
	for _, k := range keys {
		if !containsSubstr([]byte(k), tag) {
			t.Fatalf("key %q missing hash tag %s", k, tag)
		}
	}
}

func TestDecryptAbortPathViaVerify(t *testing.T) {
	// Corrupt answer after issue so decrypt fails → Abort (no freeze).
	store := NewMemoryStore()
	l, err := New(store, Config{
		SecretKey: testKey, AllowNonBrowser: true, AllowMissingPiecePress: true,
		PoWProbeProb: -1, PoWJitterBits: -1, MinSolveTime: time.Millisecond, DisableSessionWarmup: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	clk := &fakeClock{t: time.Now()}
	l.now = clk.now
	iss, err := l.Issue(context.Background(), IssueRequest{
		Kind: KindSlide, Answer: mustJSON(slide.Block{X: 120, Y: 80, Width: 60, Height: 60}),
		ClientKey: "dec", Signals: testSig(""),
	})
	if err != nil {
		t.Fatal(err)
	}
	ipHash := HashIP(testKey, mustAddr(testClientIP))
	chKey := l.challengeKey(ipHash, iss.ID)
	raw, _ := store.Get(context.Background(), chKey)
	rec, _ := decodeRecord(raw)
	rec.Answer = []byte("not-valid-ciphertext!!!!!!!!!!!!!")
	raw2, _ := encodeRecord(rec)
	_ = store.Set(context.Background(), chKey, raw2, time.Minute)
	clk.advance(2 * time.Second)

	_, err = l.Verify(context.Background(), VerifyRequest{
		ID: iss.ID, Answer: mustJSON(SlideSubmit{X: 120, Y: 80}),
		Trajectory: humanTrajectory(), ClientKey: "dec", Signals: testSig(""),
	})
	if err == nil || !errors.Is(err, ErrStore) {
		t.Fatalf("want store/decrypt error, got %v", err)
	}
	if err := l.CheckFrozen(context.Background(), ipHash); err != nil {
		t.Fatalf("decrypt failure must not freeze: %v", err)
	}
	epoch, _ := l.currentEpoch(context.Background(), ipHash)
	if epoch != 0 {
		t.Fatalf("epoch want 0 after abort, got %d", epoch)
	}
}

func TestParallelGeometryOneClaim(t *testing.T) {
	l, clk := newLayer(t, Config{VerifyRateMax: 1000, IssueRateMax: 1000})
	// Single active challenge — hammer Verify.
	iss := issueSlide(t, l, clk, "par-geo")
	good := mustJSON(SlideSubmit{X: 120, Y: 80})
	nonce := powNonce(t, iss)
	var ok int64
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := l.Verify(context.Background(), VerifyRequest{
				ID: iss.ID, Answer: good, Trajectory: humanTrajectory(),
				ClientKey: "par-geo", Signals: testSig(""), PoWNonce: nonce,
			}); err == nil {
				atomic.AddInt64(&ok, 1)
			}
		}()
	}
	wg.Wait()
	if ok != 1 {
		t.Fatalf("want 1 success, got %d", ok)
	}
}

func TestEpochCounterString(t *testing.T) {
	// sanity: freezeTTL table still matches plan
	if freezeTTLForBadCount(1) != 2*time.Second || freezeTTLForBadCount(5) != 300*time.Second {
		t.Fatal(freezeTTLForBadCount(1), freezeTTLForBadCount(5))
	}
	_ = strconv.Itoa(0)
}

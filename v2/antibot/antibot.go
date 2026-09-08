package antibot

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/feerichnii/go-captcha/v2/base/challenge"
)

// Layer is the AntiBot facade over go-captcha generation (Issue/Verify).
type Layer struct {
	cfg     Config
	store   Store
	checker AnswerChecker
	now     func() time.Time
}

// Option customizes a Layer at construction time.
type Option func(*Layer)

// WithChecker overrides the geometry answer checker.
func WithChecker(c AnswerChecker) Option {
	return func(l *Layer) {
		if c != nil {
			l.checker = c
		}
	}
}

// New creates an AntiBot layer. Config.SecretKey must be >= 32 high-entropy bytes.
func New(store Store, cfg Config, opts ...Option) (*Layer, error) {
	if len(cfg.SecretKey) == 0 {
		return nil, ErrNoSecretKey
	}
	if err := ValidateSecretKey(cfg.SecretKey); err != nil {
		return nil, err
	}
	if store == nil {
		store = NewMemoryStore()
	}
	l := &Layer{
		cfg:     cfg.withDefaults(),
		store:   store,
		checker: DefaultChecker(),
		now:     time.Now,
	}
	for _, o := range opts {
		o(l)
	}
	return l, nil
}

// ClientCapabilities is advertised by the browser on Issue so the server never
// selects a PoW (or future feature) the client cannot solve.
type ClientCapabilities struct {
	// Protocol is the client wire version (0/omitted → treat as 1).
	Protocol int `json:"protocol,omitempty"`
	// PoW lists supported algorithms, e.g. ["sha256-v1"]. Unknown names ignored.
	PoW []string `json:"pow,omitempty"`
}

// PoW algorithm capability tokens (client ↔ server negotiation).
const (
	PoWAlgoSHA256V1  = "sha256-v1"
	PoWAlgoStretchV2 = "stretch-v2" // experimental; not issued unless opted in + advertised
	ProtocolVersion  = 1
)

// IssueRequest creates a new challenge from a generated captcha answer.
type IssueRequest struct {
	Kind string // slide | rotate
	// Answer is json of GetData() — server only; encrypted at rest.
	Answer json.RawMessage
	// ClientKey binds the challenge to a server-issued session id (required).
	ClientKey string
	// Signals must include authoritative ClientSignals.IP (exact client address).
	Signals ClientSignals
	// Browser is optional client-reported environment hints (untrusted).
	Browser BrowserSignals
	// Capabilities from the client; empty PoW list means SHA-256 v1 only.
	Capabilities ClientCapabilities
	// Suspicious forces at least risk level 1 (PoW) for this challenge.
	Suspicious bool
	// PreferInvisible requests signals-only flow when EnableInvisible and risk allows.
	PreferInvisible bool
	// PreferA11Y hints the client should offer keyboard solve (IssueResponse.Mode).
	PreferA11Y bool
}

// PoWChallenge is returned to clients that must solve proof-of-work.
// Hash input is PoWPreimage(ChallengeID, Bind, Salt, nonce) where Bind is the
// opaque session hash (same as server ClientHash). Kind "stretch" adds a
// memory buffer mix before the leading-zero check (high-risk optional mode).
type PoWChallenge struct {
	Salt        string `json:"salt"`
	Difficulty  int    `json:"difficulty"`
	ChallengeID string `json:"challenge_id"`
	Bind        string `json:"bind"` // session hash
	Kind        string `json:"kind,omitempty"`
	MemoryMB    int    `json:"memory_mb,omitempty"`
	Rounds      int    `json:"rounds,omitempty"`
}

// IssueResponse is safe to return to the client (no answer).
type IssueResponse struct {
	ID          string         `json:"id"`
	ExpiresAt   int64          `json:"expires_at"` // unix seconds
	TTLSeconds  int64          `json:"ttl_seconds"`
	PoW         *PoWChallenge  `json:"pow,omitempty"`
	JSChallenge *JSChallenge   `json:"js_challenge,omitempty"`
	HardMode    *HardModeState `json:"hard_mode,omitempty"`
	// Mode is "visual" | "invisible" | "a11y" — client UX hint (server still enforces Kind).
	Mode      string `json:"mode,omitempty"`
	Kind      string `json:"kind,omitempty"`
	RiskLevel int    `json:"-"`
}

// VerifyRequest is the client solve payload.
type VerifyRequest struct {
	ID         string
	Answer     json.RawMessage // SlideSubmit / RotateSubmit
	Trajectory Trajectory
	PoWNonce   string
	ClientKey  string
	Signals    ClientSignals
	Browser    BrowserSignals
}

// VerifyResult is returned on successful verification.
type VerifyResult struct {
	Score          float64 `json:"score"`
	Risk           float64 `json:"risk"`
	RiskLevel      int     `json:"risk_level"`
	RequirePoWNext bool    `json:"require_pow_next"`
}

func hashClient(clientKey string) string {
	sum := sha256.Sum256([]byte(clientKey))
	return hex.EncodeToString(sum[:16])
}

func wrapStore(err error) error {
	if err == nil || errors.Is(err, ErrNotFound) {
		return err
	}
	return fmt.Errorf("%w: %v", ErrStore, err)
}

func validateClientKey(key string) error {
	if key == "" {
		return ErrInvalidRequest
	}
	if LooksLikeIP(key) {
		return ErrClientKeyLooksLikeIP
	}
	return nil
}

func (l *Layer) requireIPHash(signals ClientSignals) (netip.Addr, string, error) {
	if signals.IP == "" {
		return netip.Addr{}, "", ErrMissingClientIP
	}
	addr, err := parseCanonicalIP(signals.IP)
	if err != nil {
		return netip.Addr{}, "", ErrMissingClientIP
	}
	return addr, HashIP(l.cfg.SecretKey, addr), nil
}

// PreflightIssue checks freeze, rate peek, IP, and browser UA before expensive
// captcha image generation. Call this from HTTP handlers before slide/rotate.Generate.
// It does not consume an Issue rate slot (Issue still Incr's).
func (l *Layer) PreflightIssue(ctx context.Context, clientKey string, signals ClientSignals) error {
	if err := validateClientKey(clientKey); err != nil {
		return err
	}
	_, ipHash, err := l.requireIPHash(signals)
	if err != nil {
		return err
	}
	if err := l.CheckFrozen(ctx, ipHash); err != nil {
		return err
	}
	if err := l.PeekIssueRate(ctx, clientKey, ipHash); err != nil {
		return err
	}
	if l.cfg.RequireBrowser() && LooksLikeNonBrowserUA(signals.UserAgent) {
		return ErrBrowserRequired
	}
	return nil
}

// clientSupportsPoW reports whether caps advertise algo (empty PoW → sha256-v1 only).
func clientSupportsPoW(caps ClientCapabilities, algo string) bool {
	if len(caps.PoW) == 0 {
		return algo == PoWAlgoSHA256V1
	}
	for _, a := range caps.PoW {
		if a == algo {
			return true
		}
	}
	return false
}

// Issue stores a challenge and returns a public id (+ PoW when the client is risky).
func (l *Layer) Issue(ctx context.Context, req IssueRequest) (*IssueResponse, error) {
	if err := validateClientKey(req.ClientKey); err != nil {
		return nil, err
	}
	addr, ipHash, err := l.requireIPHash(req.Signals)
	if err != nil {
		return nil, err
	}
	if err := l.CheckFrozen(ctx, ipHash); err != nil {
		return nil, err
	}
	if err := l.CheckIssueRate(ctx, req.ClientKey, ipHash); err != nil {
		return nil, err
	}
	if l.cfg.RequireBrowser() && LooksLikeNonBrowserUA(req.Signals.UserAgent) {
		return nil, ErrBrowserRequired
	}

	hash := hashClient(req.ClientKey)
	l.noteIssued(ctx, hash)
	l.noteGlobalIssue(ctx)
	l.noteSessionRotation(ctx, ipHash, hash)
	if !l.cfg.DisableSessionWarmup {
		l.ensureWarmup(ctx, hash)
	}

	level, err := l.effectiveRiskAtIssue(ctx, hash, ipHash, addr, req)
	if err != nil {
		return nil, err
	}

	mode := "visual"
	if req.PreferA11Y && l.cfg.AllowA11YKeyboard {
		mode = "a11y"
	}
	if l.cfg.EnableInvisible && req.PreferInvisible && level <= l.cfg.InvisibleMaxRisk {
		req.Kind = KindInvisible
		req.Answer = json.RawMessage(`{"ok":true}`)
		mode = "invisible"
	}
	if !validKind(req.Kind) || len(req.Answer) == 0 || !json.Valid(req.Answer) {
		return nil, ErrInvalidRequest
	}

	id, err := challenge.NewID()
	if err != nil {
		return nil, err
	}

	enc, err := challenge.Encrypt(l.cfg.SecretKey, req.Answer, []byte(id+":"+req.Kind))
	if err != nil {
		return nil, err
	}

	jsCh, err := NewJSChallenge()
	if err != nil {
		return nil, err
	}
	jsCh.ChallengeID = id

	hm := l.HardMode(ctx)
	ttl := l.cfg.TTL
	if hm.Active && hm.TTLMillis > 0 {
		ttl = time.Duration(hm.TTLMillis) * time.Millisecond
	}

	now := l.now()
	tileW, tileH := tileSizeFromAnswer(req.Kind, req.Answer)
	rec := &ChallengeRecord{
		ID:          id,
		Kind:        req.Kind,
		Answer:      enc,
		ClientHash:  hash,
		IPHash:      ipHash,
		CreatedAtMs: now.UnixMilli(),
		ExpiresAtMs: now.Add(ttl).UnixMilli(),
		JSNonce:     jsCh.Nonce,
		JSProbe:     jsCh.Probe,
		JSToken:     jsCh.Token,
		JSWorkload:  jsCh.Workload,
		JSSeed:      jsCh.Seed,
		JSLoopCount: jsCh.LoopCount,
		TileW:       tileW,
		TileH:       tileH,
	}

	diff := l.cfg.choosePoW(level)
	if hm.Active && hm.ExtraPoWBits > 0 {
		if diff <= 0 {
			diff = l.cfg.PoWBaseDifficulty
		}
		diff += hm.ExtraPoWBits
		if diff > l.cfg.PoWMaxDifficulty {
			diff = l.cfg.PoWMaxDifficulty
		}
	}
	// Invisible always carries at least a light probe PoW when configured.
	if req.Kind == KindInvisible && diff <= 0 {
		diff = l.cfg.PoWProbeDifficulty
		if diff <= 0 {
			diff = 10
		}
	}
	// Production PoW is SHA-256. Stretch is experimental: only if explicitly
	// enabled (StretchPoWRiskMin > 0), risk qualifies, AND the client advertised stretch-v2.
	powKind := PoWKindSHA256
	memMB, rounds := 0, 0
	if l.cfg.StretchPoWRiskMin > 0 &&
		level >= l.cfg.StretchPoWRiskMin &&
		clientSupportsPoW(req.Capabilities, PoWAlgoStretchV2) {
		powKind = PoWKindStretch
		memMB = l.cfg.StretchMemoryMB
		rounds = l.cfg.StretchRounds
		if diff > 10 {
			diff = 10
		}
		if diff < 6 {
			diff = 6
		}
	} else if diff > 0 && !clientSupportsPoW(req.Capabilities, PoWAlgoSHA256V1) {
		// Client declared PoW list without sha256-v1 and stretch not selected → no PoW.
		diff = 0
	}
	var powOut *PoWChallenge
	if diff > 0 {
		salt, err := CreatePoW()
		if err != nil {
			return nil, err
		}
		rec.PoWDiff = diff
		rec.PoWSalt = salt
		rec.PoWKind = powKind
		rec.PoWMemoryMB = memMB
		rec.PoWRounds = rounds
		powOut = &PoWChallenge{
			Salt: salt, Difficulty: diff, ChallengeID: id, Bind: hash,
			Kind: powKind, MemoryMB: memMB, Rounds: rounds,
		}
	}

	gs, err := l.geometryStore()
	if err != nil {
		return nil, err
	}
	if err := gs.IssueChallengeAtomic(ctx, IssueChallengeArgs{
		FreezeKey:    l.freezeKey(ipHash),
		EpochKey:     l.epochKey(ipHash),
		ActiveKey:    l.activeKey(ipHash),
		ChallengeKey: l.challengeKey(ipHash, id),
		ChKeyPrefix:  l.chKeyPrefix(ipHash),
		Record:       rec,
		ChallengeTTL: ttl,
		EpochTTL:     l.epochTTL(),
		NowMs:        now.UnixMilli(),
	}); err != nil {
		return nil, err
	}

	l.cfg.Telemetry.OnIssue(IssueEvent{
		ChallengeID:   id,
		Kind:          req.Kind,
		ClientHash:    hash,
		RiskLevel:     level,
		PoWDifficulty: rec.PoWDiff,
	})

	var hmOut *HardModeState
	if hm.Active {
		hmCopy := hm
		hmOut = &hmCopy
	}
	return &IssueResponse{
		ID:          id,
		ExpiresAt:   rec.ExpiresAtMs / 1000,
		TTLSeconds:  int64(ttl / time.Second),
		PoW:         powOut,
		JSChallenge: &jsCh,
		HardMode:    hmOut,
		Mode:        mode,
		Kind:        req.Kind,
		RiskLevel:   level,
	}, nil
}

func (l *Layer) ensureWarmup(ctx context.Context, sessionHash string) {
	key := l.warmupKey(sessionHash)
	if _, err := l.store.Get(ctx, key); errors.Is(err, ErrNotFound) {
		_ = l.store.Set(ctx, key, []byte("1"), l.cfg.RiskTTL)
	}
}

func (l *Layer) clearWarmup(ctx context.Context, sessionHash string) {
	_ = l.store.Delete(ctx, l.warmupKey(sessionHash))
}

func (l *Layer) hasWarmup(ctx context.Context, sessionHash string) bool {
	_, err := l.store.Get(ctx, l.warmupKey(sessionHash))
	return err == nil
}

func (l *Layer) noteSessionRotation(ctx context.Context, ipHash, sessionHash string) {
	created, err := l.store.SetNX(ctx, l.sessSeenKey(ipHash, sessionHash), []byte("1"), l.cfg.FailRateWindow)
	if err != nil || !created {
		return
	}
	_, _ = l.store.Incr(ctx, l.sessRotKey(ipHash), l.cfg.FailRateWindow)
}

func (l *Layer) effectiveRiskAtIssue(ctx context.Context, sessionHash, ipHash string, addr netip.Addr, req IssueRequest) (int, error) {
	level, err := l.riskLevel(ctx, sessionHash)
	if err != nil {
		return 0, err
	}
	if ipN, err := l.store.IncrBy(ctx, l.riskIPKey(ipHash), 0, l.cfg.RiskTTL); err == nil && int(ipN) > level {
		level = int(ipN)
	}
	if !l.cfg.DisableSessionWarmup && l.hasWarmup(ctx, sessionHash) {
		level++
	}
	if rot, err := l.store.IncrBy(ctx, l.sessRotKey(ipHash), 0, l.cfg.FailRateWindow); err == nil && rot >= 8 {
		level++
	}
	level += l.softPrefixRisk(ctx, addr)
	if req.Suspicious && level < 1 {
		level = 1
	}
	bDelta, _ := BrowserRisk(req.Browser, true)
	if hints := FormatUAHint(req.Signals.UserAgent); len(hints) > 0 {
		bDelta++
	}
	if asn := l.lookupASN(addr); asn != 0 && req.Signals.ASN == 0 {
		req.Signals.ASN = asn
	}
	_ = req.Signals.ASN // soft telemetry only
	level += bDelta
	if cDelta, _ := BrowserConsistencyRisk(req.Browser, req.Signals.UserAgent); cDelta > 0 {
		level += cDelta
	}
	if fDelta, _ := FingerprintRisk(req.Browser); fDelta > 0 {
		level += fDelta
	}
	if l.cfg.ReputationProvider != nil {
		if d := l.cfg.ReputationProvider.RiskDelta(addr, req.ClientKey, req.Signals.DeviceKey); d > 0 {
			level += d
		}
	}
	if level > l.cfg.MaxRiskLevel {
		level = l.cfg.MaxRiskLevel
	}
	return level, nil
}

func (l *Layer) lookupASN(addr netip.Addr) int {
	if l.cfg.ASNProvider == nil {
		return 0
	}
	return l.cfg.ASNProvider.ASN(addr)
}

// Verify enforces rate limits, freeze, tech gates, then one-shot geometry claim.
func (l *Layer) Verify(ctx context.Context, req VerifyRequest) (*VerifyResult, error) {
	if !challenge.IsValidID(req.ID) {
		return nil, ErrInvalidRequest
	}
	if err := validateClientKey(req.ClientKey); err != nil {
		return nil, err
	}
	if len(req.Answer) == 0 || len(req.Answer) > l.cfg.MaxAnswerBytes || !json.Valid(req.Answer) {
		return nil, ErrInvalidRequest
	}
	if len(req.PoWNonce) > l.cfg.MaxNonceLen {
		return nil, ErrInvalidRequest
	}
	if len(req.Trajectory.Points) > l.cfg.MaxTrajectoryPoints || len(req.Trajectory.Events) > l.cfg.MaxTrajectoryEvents {
		return nil, ErrInvalidRequest
	}
	_, ipHash, err := l.requireIPHash(req.Signals)
	if err != nil {
		return nil, err
	}
	if err := l.CheckFrozen(ctx, ipHash); err != nil {
		return nil, err
	}
	if err := l.CheckVerifyRate(ctx, req.ClientKey, ipHash); err != nil {
		return nil, err
	}
	if l.cfg.RequireBrowser() && LooksLikeNonBrowserUA(req.Signals.UserAgent) {
		return nil, ErrBrowserRequired
	}

	hash := hashClient(req.ClientKey)
	ev := VerifyEvent{ChallengeID: req.ID, ClientHash: hash, TrajectoryPoints: len(req.Trajectory.Points)}
	defer func() { l.cfg.Telemetry.OnVerify(ev) }()

	fail := func(err error) (*VerifyResult, error) {
		ev.Outcome = outcomeName(err)
		return nil, err
	}

	// 1. Read-only load + bind (does not consume).
	chKey := l.challengeKey(ipHash, req.ID)
	raw, err := l.store.Get(ctx, chKey)
	if err != nil {
		return fail(wrapStore(err))
	}
	rec, err := decodeRecord(raw)
	if err != nil {
		return fail(fmt.Errorf("%w: corrupt record", ErrStore))
	}
	ev.Kind = rec.Kind
	ev.PoWDifficulty = rec.PoWDiff

	nowMs := l.now().UnixMilli()
	if rec.ExpiresAtMs > 0 && nowMs > rec.ExpiresAtMs {
		_ = l.store.Delete(ctx, chKey)
		return fail(ErrNotFound)
	}
	if subtle.ConstantTimeCompare([]byte(rec.ClientHash), []byte(hash)) != 1 ||
		subtle.ConstantTimeCompare([]byte(rec.IPHash), []byte(ipHash)) != 1 {
		return fail(ErrNotFound)
	}
	epoch, err := l.currentEpoch(ctx, ipHash)
	if err != nil {
		return fail(err)
	}
	if rec.IPEpoch != epoch {
		return fail(ErrNotFound)
	}

	// 2. Tech gates — failures keep the challenge.
	elapsed := nowMs - rec.CreatedAtMs
	ev.ElapsedMs = elapsed
	ev.TrajectoryMs = req.Trajectory.DurationMs()
	if elapsed < l.cfg.MinSolveTime.Milliseconds() {
		return fail(ErrTooFast)
	}

	if rec.PoWDiff > 0 {
		ok := false
		if rec.PoWKind == PoWKindStretch {
			ok = VerifyStretchPoW(rec.ID, hash, rec.PoWSalt, req.PoWNonce, rec.PoWDiff, rec.PoWMemoryMB, rec.PoWRounds, l.cfg.MaxNonceLen)
		} else {
			ok = VerifyPoW(rec.ID, hash, rec.PoWSalt, req.PoWNonce, rec.PoWDiff, l.cfg.MaxNonceLen)
		}
		if !ok {
			return fail(ErrPoWInvalid)
		}
	}

	jsOK := true
	var extraBrowserReasons []string
	if rec.JSNonce != "" {
		if req.Browser.JSChallengeResponse == "" {
			if l.cfg.RequireBrowserSignals() {
				return fail(ErrJSChallengeFailed)
			}
			extraBrowserReasons = append(extraBrowserReasons, "js_challenge_skipped")
		} else {
			candidates := ProbeCandidates(req.Browser, rec.JSProbe)
			ch := JSChallenge{
				Nonce: rec.JSNonce, Probe: rec.JSProbe, Token: rec.JSToken,
				Seed: rec.JSSeed, Workload: rec.JSWorkload, LoopCount: rec.JSLoopCount,
			}
			jsOK = CheckJSChallenge(rec.ID, ch, req.Browser.JSChallengeResponse, candidates...)
			if !jsOK && l.cfg.RequireBrowserSignals() {
				return fail(ErrJSChallengeFailed)
			}
		}
	}
	bDelta, bReasons := BrowserRisk(req.Browser, jsOK)
	bReasons = append(bReasons, extraBrowserReasons...)
	if cDelta, cReasons := BrowserConsistencyRisk(req.Browser, req.Signals.UserAgent); cDelta > 0 {
		bDelta += cDelta
		bReasons = append(bReasons, cReasons...)
	}
	if fDelta, fReasons := FingerprintRisk(req.Browser); fDelta > 0 {
		bDelta += fDelta
		bReasons = append(bReasons, fReasons...)
	}

	invisible := rec.Kind == KindInvisible
	if !invisible && l.cfg.RequirePiecePress() && (rec.Kind == KindSlide || rec.Kind == KindRotate) {
		if err := ValidatePieceDown(req.Trajectory, rec.TileW, rec.TileH, l.cfg.MinPiecePressDwellMs); err != nil {
			return fail(err)
		}
	}

	var sr ScoreResult
	if invisible {
		sr = ScoreResult{Score: 0.7, Consistent: true, Components: map[string]float64{"invisible": 1}}
	} else {
		issues := ValidateTrajectory(req.Trajectory, l.cfg.MaxJumpPx)
		if rec.Kind == KindSlide {
			var sub SlideSubmit
			if json.Unmarshal(req.Answer, &sub) == nil {
				if !FinalPointNear(req.Trajectory, float64(sub.X), float64(sub.Y), float64(l.cfg.SlidePadding)+40) {
					issues.FinalFarFromAnswer = true
					issues.add("final_far_from_answer")
				}
			}
		}
		sr = l.cfg.Scorer.Score(req.Trajectory, ScoreContext{ElapsedMs: elapsed, Issues: issues})
	}
	ev.Score, ev.Components, ev.TrajectoryConsistent = sr.Score, sr.Components, sr.Consistent

	levelBefore, err := l.riskLevel(ctx, hash)
	if err != nil {
		return fail(err)
	}
	ev.RiskLevelBefore = levelBefore

	// 3. Atomic geometry claim (consumes challenge + IP geo lock).
	geoStart := l.now()
	claim, err := l.ClaimGeometry(ctx, req.ID, hash, ipHash)
	if err != nil {
		return fail(err)
	}
	rec = claim.Record
	ev.Attempt = 1
	ev.GeometryDurationMs = l.now().Sub(geoStart).Milliseconds()

	// Replay peek after claim — soft signal only; recorded on success.
	if !invisible {
		if replay, err := l.CheckTrajectoryReplay(ctx, ipHash, req.Trajectory); err != nil {
			_ = l.FinalizeAbort(ctx, ipHash, claim.ClaimToken)
			return fail(err)
		} else if replay {
			bDelta++
			bReasons = append(bReasons, "trajectory_replay")
		}
	}

	if invisible {
		if err := l.FinalizeSuccess(ctx, ipHash, claim.ClaimToken); err != nil {
			return fail(wrapStore(err))
		}
		clean := isCleanSuccess(sr, bDelta, req.Signals, nowMs, l.cfg.MinSessionAge)
		dec, err := l.EvaluateRisk(ctx, hash, RiskInputs{
			Score: sr, BrowserDelta: bDelta, BrowserReasons: bReasons,
			Signals: req.Signals, Solved: clean, NowMs: nowMs,
		})
		if err != nil {
			return fail(err)
		}
		if clean {
			l.clearWarmup(ctx, hash)
		}
		ev.RiskLevelAfter = dec.LevelAfter
		ev.Outcome = "ok"
		return &VerifyResult{
			Score:          sr.Score,
			Risk:           1 - sr.Score,
			RiskLevel:      dec.LevelAfter,
			RequirePoWNext: l.cfg.powDifficultyFor(dec.LevelAfter) > 0,
		}, nil
	}

	plain, err := challenge.Decrypt(l.cfg.SecretKey, rec.Answer, []byte(rec.ID+":"+rec.Kind))
	if err != nil {
		_ = l.FinalizeAbort(ctx, ipHash, claim.ClaimToken)
		return fail(fmt.Errorf("%w: answer decrypt: %v", ErrStore, err))
	}
	tol := Tolerance{Slide: l.cfg.SlidePadding, Rotate: l.cfg.RotatePadding}
	if !l.checker(rec.Kind, plain, req.Answer, tol) {
		retry, err := l.FinalizeFailure(ctx, hash, ipHash, claim.ClaimToken)
		if err != nil {
			return fail(wrapStore(err))
		}
		dec, err := l.EvaluateRisk(ctx, hash, RiskInputs{
			Score: sr, BrowserDelta: bDelta, BrowserReasons: bReasons,
			Signals: req.Signals, Failed: true, NowMs: nowMs,
		})
		if err != nil {
			return fail(err)
		}
		ev.RiskLevelAfter = dec.LevelAfter
		ev.Outcome = outcomeName(ErrBadAnswer)
		return nil, &BadAnswerError{RetryAfterMs: retry}
	}

	// HardRejectScore: geometry was correct but behavior score is below threshold.
	// Challenge is already consumed; abort geo lock (no freeze) and do not FinalizeSuccess.
	if l.cfg.HardRejectScore > 0 && sr.Score < l.cfg.HardRejectScore {
		_ = l.FinalizeAbort(ctx, ipHash, claim.ClaimToken)
		dec, err := l.EvaluateRisk(ctx, hash, RiskInputs{
			Score: sr, BrowserDelta: bDelta, BrowserReasons: bReasons,
			Signals: req.Signals, Failed: true, NowMs: nowMs,
		})
		if err != nil {
			return fail(err)
		}
		ev.RiskLevelAfter = dec.LevelAfter
		return fail(ErrLowScore)
	}

	if err := l.FinalizeSuccess(ctx, ipHash, claim.ClaimToken); err != nil {
		return fail(wrapStore(err))
	}
	l.NoteTrajectoryReplay(ctx, ipHash, req.Trajectory)

	clean := isCleanSuccess(sr, bDelta, req.Signals, nowMs, l.cfg.MinSessionAge)
	dec, err := l.EvaluateRisk(ctx, hash, RiskInputs{
		Score: sr, BrowserDelta: bDelta, BrowserReasons: bReasons,
		Signals: req.Signals, Solved: clean, NowMs: nowMs,
	})
	if err != nil {
		return fail(err)
	}
	if clean {
		l.clearWarmup(ctx, hash)
	}
	ev.RiskLevelAfter = dec.LevelAfter

	ev.Outcome = "ok"
	return &VerifyResult{
		Score:          sr.Score,
		Risk:           1 - sr.Score,
		RiskLevel:      dec.LevelAfter,
		RequirePoWNext: l.cfg.powDifficultyFor(dec.LevelAfter) > 0,
	}, nil
}

// BadAnswerError wraps ErrBadAnswer with retry_after after freeze.
type BadAnswerError struct {
	RetryAfterMs int64
}

func (e *BadAnswerError) Error() string {
	if e == nil {
		return ErrBadAnswer.Error()
	}
	return fmt.Sprintf("%s (retry_after_ms=%d)", ErrBadAnswer.Error(), e.RetryAfterMs)
}

func (e *BadAnswerError) Is(target error) bool { return target == ErrBadAnswer }

func isCleanSuccess(sr ScoreResult, browserDelta int, sig ClientSignals, nowMs int64, minAge time.Duration) bool {
	if sr.Issues.Suspicious() || !sr.Consistent {
		return false
	}
	if browserDelta > 0 {
		return false
	}
	if minAge > 0 && sig.SessionIssuedAtMs > 0 {
		age := nowMs - sig.SessionIssuedAtMs
		if age >= 0 && age < minAge.Milliseconds() {
			return false
		}
	}
	return true
}

func tileSizeFromAnswer(kind string, answer json.RawMessage) (w, h int) {
	switch kind {
	case KindSlide, KindRotate:
		var block struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		}
		if json.Unmarshal(answer, &block) == nil {
			return block.Width, block.Height
		}
	}
	return 0, 0
}

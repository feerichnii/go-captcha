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
	// Suspicious forces at least risk level 1 (PoW) for this challenge.
	Suspicious bool
}

// PoWChallenge is returned to clients that must solve proof-of-work.
type PoWChallenge struct {
	Salt       string `json:"salt"`
	Difficulty int    `json:"difficulty"`
}

// IssueResponse is safe to return to the client (no answer).
type IssueResponse struct {
	ID          string        `json:"id"`
	ExpiresAt   int64         `json:"expires_at"` // unix seconds
	TTLSeconds  int64         `json:"ttl_seconds"`
	PoW         *PoWChallenge `json:"pow,omitempty"`
	JSChallenge *JSChallenge  `json:"js_challenge,omitempty"`
	RiskLevel   int           `json:"-"`
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

func (l *Layer) challengeKey(id string) string { return l.cfg.KeyPrefix + "ch:" + id }

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

// Issue stores a challenge and returns a public id (+ PoW when the client is risky).
func (l *Layer) Issue(ctx context.Context, req IssueRequest) (*IssueResponse, error) {
	if !validKind(req.Kind) || len(req.Answer) == 0 {
		return nil, ErrInvalidRequest
	}
	if err := validateClientKey(req.ClientKey); err != nil {
		return nil, err
	}
	if !json.Valid(req.Answer) {
		return nil, ErrInvalidRequest
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
	l.noteSessionRotation(ctx, ipHash, hash)
	if !l.cfg.DisableSessionWarmup {
		l.ensureWarmup(ctx, hash)
	}

	level, err := l.effectiveRiskAtIssue(ctx, hash, ipHash, addr, req)
	if err != nil {
		return nil, err
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

	now := l.now()
	tileW, tileH := tileSizeFromAnswer(req.Kind, req.Answer)
	rec := &ChallengeRecord{
		ID:          id,
		Kind:        req.Kind,
		Answer:      enc,
		ClientHash:  hash,
		IPHash:      ipHash,
		CreatedAtMs: now.UnixMilli(),
		ExpiresAtMs: now.Add(l.cfg.TTL).UnixMilli(),
		JSNonce:     jsCh.Nonce,
		JSProbe:     jsCh.Probe,
		TileW:       tileW,
		TileH:       tileH,
	}

	diff := l.cfg.choosePoW(level)
	var powOut *PoWChallenge
	if diff > 0 {
		salt, err := CreatePoW()
		if err != nil {
			return nil, err
		}
		rec.PoWDiff = diff
		rec.PoWSalt = salt
		powOut = &PoWChallenge{Salt: salt, Difficulty: diff}
	}

	raw, err := encodeRecord(rec)
	if err != nil {
		return nil, err
	}
	if err := l.store.Set(ctx, l.challengeKey(id), raw, l.cfg.TTL); err != nil {
		return nil, wrapStore(err)
	}

	l.cfg.Telemetry.OnIssue(IssueEvent{
		ChallengeID:   id,
		Kind:          req.Kind,
		ClientHash:    hash,
		RiskLevel:     level,
		PoWDifficulty: rec.PoWDiff,
	})

	return &IssueResponse{
		ID:          id,
		ExpiresAt:   rec.ExpiresAtMs / 1000,
		TTLSeconds:  int64(l.cfg.TTL / time.Second),
		PoW:         powOut,
		JSChallenge: &jsCh,
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
	// Count distinct sessions per IP roughly via issue-time increments.
	_, _ = l.store.Incr(ctx, l.sessRotKey(ipHash), l.cfg.FailRateWindow)
	_ = sessionHash
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
	chKey := l.challengeKey(req.ID)
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

	// 2. Tech gates — failures keep the challenge.
	elapsed := nowMs - rec.CreatedAtMs
	ev.ElapsedMs = elapsed
	ev.TrajectoryMs = req.Trajectory.DurationMs()
	if elapsed < l.cfg.MinSolveTime.Milliseconds() {
		return fail(ErrTooFast)
	}

	if rec.PoWDiff > 0 && !VerifyPoW(rec.PoWSalt, req.PoWNonce, rec.PoWDiff, l.cfg.MaxNonceLen) {
		return fail(ErrPoWInvalid)
	}

	jsOK := true
	var extraBrowserReasons []string
	if rec.JSNonce != "" {
		if req.Browser.JSChallengeResponse == "" {
			if l.cfg.RequireBrowser() {
				return fail(ErrJSChallengeFailed)
			}
			extraBrowserReasons = append(extraBrowserReasons, "js_challenge_skipped")
		} else {
			candidates := ProbeCandidates(req.Browser, rec.JSProbe)
			jsOK = CheckJSChallenge(JSChallenge{Nonce: rec.JSNonce, Probe: rec.JSProbe}, req.Browser.JSChallengeResponse, candidates...)
			if !jsOK && l.cfg.RequireBrowser() {
				return fail(ErrJSChallengeFailed)
			}
		}
	}
	bDelta, bReasons := BrowserRisk(req.Browser, jsOK)
	bReasons = append(bReasons, extraBrowserReasons...)

	if l.cfg.RequirePiecePress() && (rec.Kind == KindSlide || rec.Kind == KindRotate) {
		if err := ValidatePieceDown(req.Trajectory, rec.TileW, rec.TileH, l.cfg.MinPiecePressDwellMs); err != nil {
			return fail(err)
		}
	}

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
	sr := l.cfg.Scorer.Score(req.Trajectory, ScoreContext{ElapsedMs: elapsed, Issues: issues})
	ev.Score, ev.Components, ev.TrajectoryConsistent = sr.Score, sr.Components, sr.Consistent

	levelBefore, err := l.riskLevel(ctx, hash)
	if err != nil {
		return fail(err)
	}
	ev.RiskLevelBefore = levelBefore

	// 3. Atomic geometry claim (consumes challenge + IP geo lock).
	claim, err := l.ClaimGeometry(ctx, req.ID, hash, ipHash)
	if err != nil {
		return fail(err)
	}
	rec = claim.Record
	ev.Attempt = 1

	plain, err := challenge.Decrypt(l.cfg.SecretKey, rec.Answer, []byte(rec.ID+":"+rec.Kind))
	if err != nil {
		_, _ = l.FinalizeFailure(ctx, hash, ipHash, claim.ClaimToken)
		return fail(fmt.Errorf("%w: answer decrypt: %v", ErrStore, err))
	}
	tol := Tolerance{Slide: l.cfg.SlidePadding, Rotate: l.cfg.RotatePadding}
	if !l.checker(rec.Kind, plain, req.Answer, tol) {
		retry, _ := l.FinalizeFailure(ctx, hash, ipHash, claim.ClaimToken)
		dec, _ := l.EvaluateRisk(ctx, hash, RiskInputs{
			Score: sr, BrowserDelta: bDelta, BrowserReasons: bReasons,
			Signals: req.Signals, Failed: true, NowMs: nowMs,
		})
		ev.RiskLevelAfter = dec.LevelAfter
		ev.Outcome = outcomeName(ErrBadAnswer)
		return nil, &BadAnswerError{RetryAfterMs: retry}
	}

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

	if l.cfg.HardRejectScore > 0 && sr.Score < l.cfg.HardRejectScore {
		return fail(ErrLowScore)
	}

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

package antibot

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	Kind string // click | slide | rotate
	// Answer is json of GetData() — server only; encrypted at rest.
	Answer json.RawMessage
	// ClientKey binds the challenge to a server-issued session id (required).
	// Must NOT be an IP or IP:port — use MintSession / EnsureSessionCookie.
	// Pass IP/UA via Signals instead.
	ClientKey string
	// Signals are optional side-channels (IP, UA, ASN, session age) for risk.
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
	Answer     json.RawMessage // ClickSubmit / SlideSubmit / RotateSubmit
	Trajectory Trajectory
	PoWNonce   string
	ClientKey  string // must match the key used at Issue
	Signals    ClientSignals
	Browser    BrowserSignals
}

// VerifyResult is returned on successful verification.
type VerifyResult struct {
	// Score is the behavior estimate in [0,1]; treat as risk input, not proof.
	Score float64 `json:"score"`
	// Risk is 1-Score.
	Risk float64 `json:"risk"`
	// RiskLevel is the client's persistent escalation level after this verify.
	RiskLevel int `json:"risk_level"`
	// RequirePoWNext reports whether the next Issue for this client gets PoW.
	RequirePoWNext bool `json:"require_pow_next"`
}

func (l *Layer) challengeKey(id string) string { return l.cfg.KeyPrefix + "ch:" + id }
func (l *Layer) attemptKey(id string) string   { return l.cfg.KeyPrefix + "att:" + id }

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
	if err := l.CheckIssueRate(ctx, req.ClientKey); err != nil {
		return nil, err
	}

	hash := hashClient(req.ClientKey)
	l.noteIssued(ctx, hash)

	level, err := l.riskLevel(ctx, hash)
	if err != nil {
		return nil, err
	}
	if req.Suspicious && level < 1 {
		level = 1
	}
	// Browser / UA signals at issue time can raise the effective level for PoW
	// selection only (persistent risk is updated on Verify).
	bDelta, _ := BrowserRisk(req.Browser, true /* JS not verifiable yet */)
	if hints := FormatUAHint(req.Signals.UserAgent); len(hints) > 0 {
		bDelta++
	}
	level += bDelta
	if level > l.cfg.MaxRiskLevel {
		level = l.cfg.MaxRiskLevel
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
	rec := &ChallengeRecord{
		ID:          id,
		Kind:        req.Kind,
		Answer:      enc,
		ClientHash:  hash,
		CreatedAtMs: now.UnixMilli(),
		ExpiresAtMs: now.Add(l.cfg.TTL).UnixMilli(),
		JSNonce:     jsCh.Nonce,
		JSProbe:     jsCh.Probe,
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

// Verify enforces rate limits, atomic attempt counting, client binding,
// server-side timing, PoW, JS challenge and geometry; the challenge is consumed
// atomically on success. Behavior score and browser signals adjust the client's
// persistent risk level unless Config.HardRejectScore is set.
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
	if err := l.CheckVerifyRate(ctx, req.ClientKey); err != nil {
		return nil, err
	}

	hash := hashClient(req.ClientKey)
	ev := VerifyEvent{ChallengeID: req.ID, ClientHash: hash, TrajectoryPoints: len(req.Trajectory.Points)}
	defer func() { l.cfg.Telemetry.OnVerify(ev) }()

	fail := func(err error) (*VerifyResult, error) {
		ev.Outcome = outcomeName(err)
		return nil, err
	}

	// 1. Atomic attempt accounting happens before anything is evaluated, so
	//    parallel guesses cannot exceed MaxAttempts.
	chKey := l.challengeKey(req.ID)
	attempts, err := l.store.Incr(ctx, l.attemptKey(req.ID), l.cfg.TTL)
	if err != nil {
		return fail(wrapStore(err))
	}
	ev.Attempt = attempts
	if attempts > int64(l.cfg.MaxAttempts) {
		_ = l.store.Delete(ctx, chKey)
		return fail(ErrMaxAttempts)
	}

	// 2. Load and bind.
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
	if subtle.ConstantTimeCompare([]byte(rec.ClientHash), []byte(hash)) != 1 {
		// Do not reveal that the id exists for another client.
		return fail(ErrNotFound)
	}

	// 3. Server-side timing.
	elapsed := nowMs - rec.CreatedAtMs
	ev.ElapsedMs = elapsed
	ev.TrajectoryMs = req.Trajectory.DurationMs()
	if elapsed < l.cfg.MinSolveTime.Milliseconds() {
		return fail(ErrTooFast)
	}

	// 4. Proof-of-work (when attached at issue time — includes probe PoW).
	if rec.PoWDiff > 0 && !VerifyPoW(rec.PoWSalt, req.PoWNonce, rec.PoWDiff, l.cfg.MaxNonceLen) {
		l.recordFail(ctx, hash)
		return fail(ErrPoWInvalid)
	}

	// 5. JS / DOM challenge (when minted at issue).
	// Missing response is soft (legacy clients); a wrong response raises risk.
	jsOK := true
	var extraBrowserReasons []string
	if rec.JSNonce != "" {
		if req.Browser.JSChallengeResponse == "" {
			extraBrowserReasons = append(extraBrowserReasons, "js_challenge_skipped")
		} else {
			candidates := ProbeCandidates(req.Browser, rec.JSProbe)
			jsOK = CheckJSChallenge(JSChallenge{Nonce: rec.JSNonce, Probe: rec.JSProbe}, req.Browser.JSChallengeResponse, candidates...)
		}
	}
	bDelta, bReasons := BrowserRisk(req.Browser, jsOK)
	bReasons = append(bReasons, extraBrowserReasons...)

	// 6. Trajectory structure + behavior score.
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

	// 7. Geometry.
	plain, err := challenge.Decrypt(l.cfg.SecretKey, rec.Answer, []byte(rec.ID+":"+rec.Kind))
	if err != nil {
		return fail(fmt.Errorf("%w: answer decrypt: %v", ErrStore, err))
	}
	tol := Tolerance{Click: l.cfg.ClickPadding, Slide: l.cfg.SlidePadding, Rotate: l.cfg.RotatePadding}
	if !l.checker(rec.Kind, plain, req.Answer, tol) {
		dec, _ := l.EvaluateRisk(ctx, hash, RiskInputs{
			Score: sr, BrowserDelta: bDelta, BrowserReasons: bReasons,
			Signals: req.Signals, Failed: true, NowMs: nowMs,
		})
		ev.RiskLevelAfter = dec.LevelAfter
		if attempts >= int64(l.cfg.MaxAttempts) {
			_ = l.store.Delete(ctx, chKey)
			return fail(ErrMaxAttempts)
		}
		return fail(ErrBadAnswer)
	}

	// 8. Consume atomically; exactly one concurrent correct submission wins.
	if _, err := l.store.GetDel(ctx, chKey); err != nil {
		return fail(wrapStore(err))
	}
	_ = l.store.Delete(ctx, l.attemptKey(req.ID))

	dec, err := l.EvaluateRisk(ctx, hash, RiskInputs{
		Score: sr, BrowserDelta: bDelta, BrowserReasons: bReasons,
		Signals: req.Signals, Solved: true, NowMs: nowMs,
	})
	if err != nil {
		return fail(err)
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

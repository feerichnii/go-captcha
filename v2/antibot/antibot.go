package antibot

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
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

// New creates an AntiBot layer. Config.SecretKey is required.
func New(store Store, cfg Config, opts ...Option) (*Layer, error) {
	if len(cfg.SecretKey) == 0 {
		return nil, ErrNoSecretKey
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
	// ClientKey binds the challenge to a session / client (required).
	// Use a session id when available; fall back to IP + UA hash.
	ClientKey string
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
	ID         string        `json:"id"`
	ExpiresAt  int64         `json:"expires_at"` // unix seconds
	TTLSeconds int64         `json:"ttl_seconds"`
	PoW        *PoWChallenge `json:"pow,omitempty"`
	RiskLevel  int           `json:"-"`
}

// VerifyRequest is the client solve payload.
type VerifyRequest struct {
	ID         string
	Answer     json.RawMessage // ClickSubmit / SlideSubmit / RotateSubmit
	Trajectory Trajectory
	PoWNonce   string
	ClientKey  string // must match the key used at Issue
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
func (l *Layer) riskKey(hash string) string    { return l.cfg.KeyPrefix + "risk:" + hash }

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

// RiskLevel returns the persistent risk level for a client key (0 = clean).
func (l *Layer) RiskLevel(ctx context.Context, clientKey string) (int, error) {
	return l.riskLevel(ctx, hashClient(clientKey))
}

func (l *Layer) riskLevel(ctx context.Context, hash string) (int, error) {
	b, err := l.store.Get(ctx, l.riskKey(hash))
	if errors.Is(err, ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, wrapStore(err)
	}
	n, _ := strconv.Atoi(string(b))
	return n, nil
}

func (l *Layer) setRiskLevel(ctx context.Context, hash string, level int) error {
	if level < 0 {
		level = 0
	}
	if level > l.cfg.MaxRiskLevel {
		level = l.cfg.MaxRiskLevel
	}
	key := l.riskKey(hash)
	if level == 0 {
		return wrapStore(l.store.Delete(ctx, key))
	}
	return wrapStore(l.store.Set(ctx, key, []byte(strconv.Itoa(level)), l.cfg.RiskTTL))
}

// Issue stores a challenge and returns a public id (+ PoW when the client is risky).
func (l *Layer) Issue(ctx context.Context, req IssueRequest) (*IssueResponse, error) {
	if !validKind(req.Kind) || len(req.Answer) == 0 || req.ClientKey == "" {
		return nil, ErrInvalidRequest
	}
	if !json.Valid(req.Answer) {
		return nil, ErrInvalidRequest
	}
	if err := l.CheckIssueRate(ctx, req.ClientKey); err != nil {
		return nil, err
	}

	hash := hashClient(req.ClientKey)
	level, err := l.riskLevel(ctx, hash)
	if err != nil {
		return nil, err
	}
	if req.Suspicious && level < 1 {
		level = 1
	}

	id, err := challenge.NewID()
	if err != nil {
		return nil, err
	}

	enc, err := challenge.Encrypt(l.cfg.SecretKey, req.Answer, []byte(id+":"+req.Kind))
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
	}

	var powOut *PoWChallenge
	if diff := l.cfg.powDifficultyFor(level); diff > 0 {
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
		ID:         id,
		ExpiresAt:  rec.ExpiresAtMs / 1000,
		TTLSeconds: int64(l.cfg.TTL / time.Second),
		PoW:        powOut,
		RiskLevel:  level,
	}, nil
}

// Verify enforces rate limits, atomic attempt counting, client binding,
// server-side timing, PoW and geometry; the challenge is consumed atomically
// on success. Behavior score only adjusts the client's risk level unless
// Config.HardRejectScore is set.
func (l *Layer) Verify(ctx context.Context, req VerifyRequest) (*VerifyResult, error) {
	if !challenge.IsValidID(req.ID) || req.ClientKey == "" {
		return nil, ErrInvalidRequest
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

	// 4. Proof-of-work (when the client was risky at issue time).
	if rec.PoWDiff > 0 && !VerifyPoW(rec.PoWSalt, req.PoWNonce, rec.PoWDiff, l.cfg.MaxNonceLen) {
		return fail(ErrPoWInvalid)
	}

	// 5. Behavior score → risk (computed before geometry so telemetry sees it on failures too).
	sr := l.cfg.Scorer.Score(req.Trajectory, ScoreContext{ElapsedMs: elapsed})
	ev.Score, ev.Components, ev.TrajectoryConsistent = sr.Score, sr.Components, sr.Consistent

	levelBefore, err := l.riskLevel(ctx, hash)
	if err != nil {
		return fail(err)
	}
	ev.RiskLevelBefore = levelBefore

	// 6. Geometry.
	plain, err := challenge.Decrypt(l.cfg.SecretKey, rec.Answer, []byte(rec.ID+":"+rec.Kind))
	if err != nil {
		return fail(fmt.Errorf("%w: answer decrypt: %v", ErrStore, err))
	}
	tol := Tolerance{Click: l.cfg.ClickPadding, Slide: l.cfg.SlidePadding, Rotate: l.cfg.RotatePadding}
	if !l.checker(rec.Kind, plain, req.Answer, tol) {
		levelAfter := l.adjustRisk(ctx, hash, levelBefore, sr, false)
		ev.RiskLevelAfter = levelAfter
		if attempts >= int64(l.cfg.MaxAttempts) {
			_ = l.store.Delete(ctx, chKey)
			return fail(ErrMaxAttempts)
		}
		return fail(ErrBadAnswer)
	}

	// 7. Consume atomically; exactly one concurrent correct submission wins.
	if _, err := l.store.GetDel(ctx, chKey); err != nil {
		return fail(wrapStore(err))
	}
	_ = l.store.Delete(ctx, l.attemptKey(req.ID))

	levelAfter := l.adjustRisk(ctx, hash, levelBefore, sr, true)
	ev.RiskLevelAfter = levelAfter

	if l.cfg.HardRejectScore > 0 && sr.Score < l.cfg.HardRejectScore {
		return fail(ErrLowScore)
	}

	ev.Outcome = "ok"
	return &VerifyResult{
		Score:          sr.Score,
		Risk:           1 - sr.Score,
		RiskLevel:      levelAfter,
		RequirePoWNext: l.cfg.powDifficultyFor(levelAfter) > 0,
	}, nil
}

// adjustRisk escalates on bot-like signals and slowly de-escalates on clean solves.
func (l *Layer) adjustRisk(ctx context.Context, hash string, level int, sr ScoreResult, solved bool) int {
	suspicious := !sr.Consistent || sr.Score < l.cfg.RiskThreshold
	switch {
	case suspicious:
		level++
	case solved && level > 0:
		level--
	default:
		return level
	}
	if level > l.cfg.MaxRiskLevel {
		level = l.cfg.MaxRiskLevel
	}
	_ = l.setRiskLevel(ctx, hash, level)
	return level
}

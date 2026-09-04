package antibot

import (
	"context"
	"encoding/json"
	"time"

	"github.com/wenlng/go-captcha/v2/base/challenge"
)

// Layer is the AntiBot facade over go-captcha generation (Issue/Verify).
type Layer struct {
	cfg     Config
	store   Store
	checker AnswerChecker
}

// New creates an AntiBot layer. store may be MemoryStore or RedisStore.
func New(store Store, cfg Config) *Layer {
	if store == nil {
		store = NewMemoryStore()
	}
	c := cfg.withDefaults()
	return &Layer{
		cfg:     c,
		store:   store,
		checker: DefaultChecker(),
	}
}

// WithChecker overrides the geometry answer checker.
func (l *Layer) WithChecker(c AnswerChecker) *Layer {
	if c != nil {
		l.checker = c
	}
	return l
}

// IssueRequest creates a new challenge from a generated captcha answer.
type IssueRequest struct {
	Kind      string          // click | slide | rotate
	Answer    json.RawMessage // json of GetData() — server only
	ClientKey string          // IP / user id for rate limiting
	// Suspicious forces PoW on this challenge (adaptive).
	Suspicious bool
}

// PoWChallenge is returned to clients that must solve proof-of-work.
type PoWChallenge struct {
	Salt       string `json:"salt"`
	Difficulty int    `json:"difficulty"`
}

// IssueResponse is safe to return to the client (no answer).
type IssueResponse struct {
	ID  string        `json:"id"`
	TTL time.Duration `json:"ttl_ns"`
	PoW *PoWChallenge `json:"pow,omitempty"`
}

// VerifyRequest is the client solve payload.
type VerifyRequest struct {
	ID        string
	Answer    json.RawMessage // submitted geometry (ClickSubmit / SlideSubmit / RotateSubmit)
	Trajectory Trajectory
	PoWNonce  string
	ClientKey string
}

// VerifyResult is returned on successful verification.
type VerifyResult struct {
	Score          float64 `json:"score"`
	RequirePoWNext bool    `json:"require_pow_next"`
}

func (l *Layer) challengeKey(id string) string {
	return l.cfg.KeyPrefix + "ch:" + id
}

// Issue stores a challenge and returns a public id (+ optional PoW).
func (l *Layer) Issue(ctx context.Context, req IssueRequest) (*IssueResponse, error) {
	if req.Kind == "" || len(req.Answer) == 0 {
		return nil, ErrInvalidRequest
	}
	if err := l.CheckRate(ctx, req.ClientKey); err != nil {
		return nil, err
	}

	id, err := challenge.NewID()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	rec := &ChallengeRecord{
		ID:        id,
		Kind:      req.Kind,
		Answer:    append(json.RawMessage(nil), req.Answer...),
		Attempts:  0,
		Consumed:  false,
		ClientKey: req.ClientKey,
		CreatedAt: now.Unix(),
		ExpiresAt: now.Add(l.cfg.TTL).Unix(),
	}

	var powOut *PoWChallenge
	if l.cfg.PoWAlways || req.Suspicious {
		salt, err := CreatePoW()
		if err != nil {
			return nil, err
		}
		rec.PoWDiff = l.cfg.PoWDifficulty
		rec.PoWSalt = salt
		powOut = &PoWChallenge{Salt: salt, Difficulty: l.cfg.PoWDifficulty}
	}

	raw, err := encodeRecord(rec)
	if err != nil {
		return nil, err
	}
	if err := l.store.Set(ctx, l.challengeKey(id), raw, l.cfg.TTL); err != nil {
		return nil, err
	}

	return &IssueResponse{
		ID:  id,
		TTL: l.cfg.TTL,
		PoW: powOut,
	}, nil
}

// Verify checks PoW, behavior score, attempt limits, and geometry; consumes on success.
func (l *Layer) Verify(ctx context.Context, req VerifyRequest) (*VerifyResult, error) {
	if req.ID == "" || len(req.Answer) == 0 {
		return nil, ErrInvalidRequest
	}

	key := l.challengeKey(req.ID)
	raw, err := l.store.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	rec, err := decodeRecord(raw)
	if err != nil {
		return nil, ErrInvalidRequest
	}

	now := time.Now().Unix()
	if rec.ExpiresAt > 0 && now > rec.ExpiresAt {
		_ = l.store.Delete(ctx, key)
		return nil, ErrNotFound
	}
	if rec.Consumed {
		return nil, ErrConsumed
	}
	if rec.Attempts >= l.cfg.MaxAttempts {
		_ = l.store.Delete(ctx, key)
		return nil, ErrMaxAttempts
	}

	if rec.PoWDiff > 0 {
		if !VerifyPoW(rec.PoWSalt, req.PoWNonce, rec.PoWDiff) {
			if err := l.bumpAttempt(ctx, key, rec); err != nil {
				return nil, err
			}
			if rec.Attempts >= l.cfg.MaxAttempts {
				return nil, ErrMaxAttempts
			}
			return nil, ErrPoWInvalid
		}
	}

	scoreRes := ScoreBehavior(req.Trajectory)
	if scoreRes.Score < l.cfg.MinBehaviorScore {
		if err := l.bumpAttempt(ctx, key, rec); err != nil {
			return nil, err
		}
		if rec.Attempts >= l.cfg.MaxAttempts {
			return nil, ErrMaxAttempts
		}
		return nil, ErrLowScore
	}

	if !l.checker(rec.Kind, rec.Answer, req.Answer) {
		if err := l.bumpAttempt(ctx, key, rec); err != nil {
			return nil, err
		}
		if rec.Attempts >= l.cfg.MaxAttempts {
			return nil, ErrMaxAttempts
		}
		return nil, ErrBadAnswer
	}

	// Single-use: delete on success.
	_ = l.store.Delete(ctx, key)

	return &VerifyResult{
		Score:          scoreRes.Score,
		RequirePoWNext: scoreRes.Score < l.cfg.SoftBehaviorScore,
	}, nil
}

func (l *Layer) bumpAttempt(ctx context.Context, key string, rec *ChallengeRecord) error {
	rec.Attempts++
	if rec.Attempts >= l.cfg.MaxAttempts {
		_ = l.store.Delete(ctx, key)
		return nil
	}
	ttl := time.Until(time.Unix(rec.ExpiresAt, 0))
	if ttl <= 0 {
		_ = l.store.Delete(ctx, key)
		return nil
	}
	raw, err := encodeRecord(rec)
	if err != nil {
		return err
	}
	return l.store.Set(ctx, key, raw, ttl)
}

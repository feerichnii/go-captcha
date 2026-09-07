package antibot

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"time"
)

const (
	geoLockTTL       = 30 * time.Second
	badAnswerHistTTL = 6 * time.Hour
)

// GeometryClaim is returned by ClaimGeometry after the challenge is consumed.
type GeometryClaim struct {
	Record     *ChallengeRecord
	ClaimToken string
}

func (l *Layer) freezeKey(ipHash string) string {
	return l.cfg.KeyPrefix + "freeze:" + ipHash
}
func (l *Layer) bindLockKey(sessionHash, ipHash string) string {
	return l.cfg.KeyPrefix + "lock:bind:" + sessionHash + ":" + ipHash
}
func (l *Layer) geoKey(ipHash string) string {
	return l.cfg.KeyPrefix + "geo:" + ipHash
}
func (l *Layer) badGeoKey(ipHash string) string {
	return l.cfg.KeyPrefix + "badgeo:" + ipHash
}
func (l *Layer) warmupKey(sessionHash string) string {
	return l.cfg.KeyPrefix + "warmup:" + sessionHash
}
func (l *Layer) sessRotKey(ipHash string) string {
	return l.cfg.KeyPrefix + "sessrot:" + ipHash
}
func (l *Layer) riskIPKey(ipHash string) string {
	return l.cfg.KeyPrefix + "risk:ip:" + ipHash
}

// LockedError is returned when an IP (or binding) is in cooldown.
type LockedError struct {
	RetryAfterMs int64
}

func (e *LockedError) Error() string {
	if e == nil {
		return ErrLocked.Error()
	}
	return fmt.Sprintf("%s (retry_after_ms=%d)", ErrLocked.Error(), e.RetryAfterMs)
}

func (e *LockedError) Is(target error) bool {
	return target == ErrLocked
}

// RetryAfterMs extracts retry_after from LockedError or BadAnswerError.
func RetryAfterMs(err error) int64 {
	var le *LockedError
	if errors.As(err, &le) && le != nil {
		return le.RetryAfterMs
	}
	var be *BadAnswerError
	if errors.As(err, &be) && be != nil {
		return be.RetryAfterMs
	}
	return 0
}

// CheckFrozen returns ErrLocked with retry_after if freeze:<ipHash> is set.
func (l *Layer) CheckFrozen(ctx context.Context, ipHash string) error {
	if ipHash == "" {
		return ErrInvalidRequest
	}
	raw, err := l.store.Get(ctx, l.freezeKey(ipHash))
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return wrapStore(err)
	}
	var untilMs int64
	if len(raw) > 0 {
		untilMs, _ = strconv.ParseInt(string(raw), 10, 64)
	}
	nowMs := l.now().UnixMilli()
	retry := untilMs - nowMs
	if retry < 0 {
		retry = 0
	}
	return &LockedError{RetryAfterMs: retry}
}

func freezeTTLForBadCount(n int64) time.Duration {
	switch {
	case n <= 1:
		return 2 * time.Second
	case n == 2:
		return 5 * time.Second
	case n == 3:
		return 15 * time.Second
	case n == 4:
		return 60 * time.Second
	default:
		return 300 * time.Second
	}
}

func newClaimToken() (string, error) {
	var b [18]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// ClaimGeometryArgs is passed to GeometryStore.ClaimGeometryAtomic.
type ClaimGeometryArgs struct {
	ChallengeKey string
	FreezeKey    string
	GeoKey       string
	ExpectClient string
	ExpectIP     string
	ClaimToken   string
	GeoTTL       time.Duration
	NowMs        int64
}

// GeometryStore extends Store with atomic geometry claim / finalize.
type GeometryStore interface {
	Store
	ClaimGeometryAtomic(ctx context.Context, args ClaimGeometryArgs) (*ChallengeRecord, error)
	ReleaseGeoLock(ctx context.Context, geoKey, claimToken string) error
	SetFreeze(ctx context.Context, freezeKey string, untilMs int64, ttl time.Duration) error
}

// ClaimGeometry atomically consumes the challenge under an IP geo lock.
// Call only after tech gates (PoW/JS/trajectory) succeeded.
func (l *Layer) ClaimGeometry(ctx context.Context, challengeID, sessionHash, ipHash string) (*GeometryClaim, error) {
	if challengeID == "" || sessionHash == "" || ipHash == "" {
		return nil, ErrInvalidRequest
	}
	if err := l.CheckFrozen(ctx, ipHash); err != nil {
		return nil, err
	}

	token, err := newClaimToken()
	if err != nil {
		return nil, err
	}

	gs, ok := l.store.(GeometryStore)
	if !ok {
		return nil, fmt.Errorf("%w: store does not implement GeometryStore", ErrStore)
	}
	rec, err := gs.ClaimGeometryAtomic(ctx, ClaimGeometryArgs{
		ChallengeKey: l.challengeKey(challengeID),
		FreezeKey:    l.freezeKey(ipHash),
		GeoKey:       l.geoKey(ipHash),
		ExpectClient: sessionHash,
		ExpectIP:     ipHash,
		ClaimToken:   token,
		GeoTTL:       geoLockTTL,
		NowMs:        l.now().UnixMilli(),
	})
	if err != nil {
		return nil, err
	}
	return &GeometryClaim{Record: rec, ClaimToken: token}, nil
}

// FinalizeSuccess releases the IP geo lock after a correct geometry check.
func (l *Layer) FinalizeSuccess(ctx context.Context, ipHash, claimToken string) error {
	gs, ok := l.store.(GeometryStore)
	if !ok {
		return nil
	}
	return gs.ReleaseGeoLock(ctx, l.geoKey(ipHash), claimToken)
}

// FinalizeFailure applies escalating IP freeze + bind lock and releases geo lock.
func (l *Layer) FinalizeFailure(ctx context.Context, sessionHash, ipHash, claimToken string) (retryAfterMs int64, err error) {
	n, err := l.store.Incr(ctx, l.badGeoKey(ipHash), badAnswerHistTTL)
	if err != nil {
		return 0, wrapStore(err)
	}
	ttl := freezeTTLForBadCount(n)
	until := l.now().Add(ttl).UnixMilli()

	if gs, ok := l.store.(GeometryStore); ok {
		_ = gs.SetFreeze(ctx, l.freezeKey(ipHash), until, ttl)
		_ = gs.ReleaseGeoLock(ctx, l.geoKey(ipHash), claimToken)
	} else {
		_ = l.store.Set(ctx, l.freezeKey(ipHash), []byte(strconv.FormatInt(until, 10)), ttl)
	}
	_ = l.store.Set(ctx, l.bindLockKey(sessionHash, ipHash), []byte(strconv.FormatInt(until, 10)), ttl)
	_, _ = l.store.IncrBy(ctx, l.riskIPKey(ipHash), 1, l.cfg.RiskTTL)
	return ttl.Milliseconds(), nil
}

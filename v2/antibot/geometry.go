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

const badAnswerHistTTLFallback = 6 * time.Hour

// GeometryClaim is returned by ClaimGeometry after the challenge is consumed.
type GeometryClaim struct {
	Record     *ChallengeRecord
	ClaimToken string
}

func (l *Layer) ipRoot(ipHash string) string {
	return l.cfg.KeyPrefix + "ip:{" + ipHash + "}:"
}

func (l *Layer) freezeKey(ipHash string) string  { return l.ipRoot(ipHash) + "freeze" }
func (l *Layer) geoKey(ipHash string) string     { return l.ipRoot(ipHash) + "geo" }
func (l *Layer) badGeoKey(ipHash string) string  { return l.ipRoot(ipHash) + "badgeo" }
func (l *Layer) epochKey(ipHash string) string   { return l.ipRoot(ipHash) + "epoch" }
func (l *Layer) activeKey(ipHash string) string  { return l.ipRoot(ipHash) + "active" }
func (l *Layer) riskIPKey(ipHash string) string  { return l.ipRoot(ipHash) + "risk" }
func (l *Layer) sessRotKey(ipHash string) string { return l.ipRoot(ipHash) + "sessrot" }
func (l *Layer) sessSeenKey(ipHash, sessionHash string) string {
	return l.ipRoot(ipHash) + "sessseen:" + sessionHash
}
func (l *Layer) bindLockKey(sessionHash, ipHash string) string {
	return l.ipRoot(ipHash) + "bind:" + sessionHash
}
func (l *Layer) challengeKey(ipHash, id string) string {
	return l.ipRoot(ipHash) + "ch:" + id
}
func (l *Layer) chKeyPrefix(ipHash string) string {
	return l.ipRoot(ipHash) + "ch:"
}
func (l *Layer) warmupKey(sessionHash string) string {
	return l.cfg.KeyPrefix + "warmup:" + sessionHash
}

func (l *Layer) geoLockTTL() time.Duration {
	if l.cfg.GeoLockTTL > 0 {
		return l.cfg.GeoLockTTL
	}
	return 5 * time.Second
}

func (l *Layer) epochTTL() time.Duration {
	fail := l.cfg.FailRateWindow
	if fail <= 0 {
		fail = time.Hour
	}
	ch := l.cfg.TTL
	if ch <= 0 {
		ch = 90 * time.Second
	}
	if 2*ch > fail {
		return 2 * ch
	}
	return fail
}

func (l *Layer) badGeoTTL() time.Duration {
	if l.cfg.FailRateWindow > 0 {
		return l.cfg.FailRateWindow
	}
	return badAnswerHistTTLFallback
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

// IssueChallengeArgs is passed to GeometryStore.IssueChallengeAtomic.
type IssueChallengeArgs struct {
	FreezeKey    string
	EpochKey     string
	ActiveKey    string
	ChallengeKey string
	ChKeyPrefix  string
	Record       *ChallengeRecord
	ChallengeTTL time.Duration
	EpochTTL     time.Duration
	NowMs        int64
}

// ClaimGeometryArgs is passed to GeometryStore.ClaimGeometryAtomic.
type ClaimGeometryArgs struct {
	FreezeKey    string
	GeoKey       string
	EpochKey     string
	ActiveKey    string
	ChallengeKey string
	ChallengeID  string
	ExpectClient string
	ExpectIP     string
	ClaimToken   string
	GeoTTL       time.Duration
	NowMs        int64
}

// FinalizeFailureArgs is passed to GeometryStore.FinalizeFailureAtomic.
type FinalizeFailureArgs struct {
	GeoKey     string
	FreezeKey  string
	BindKey    string
	BadGeoKey  string
	RiskKey    string
	EpochKey   string
	ClaimToken string
	NowMs      int64
	BadGeoTTL  time.Duration
	EpochTTL   time.Duration
	RiskTTL    time.Duration
}

// GeometryStore extends Store with atomic IP-scoped challenge ops.
type GeometryStore interface {
	Store
	IssueChallengeAtomic(ctx context.Context, args IssueChallengeArgs) error
	ClaimGeometryAtomic(ctx context.Context, args ClaimGeometryArgs) (*ChallengeRecord, error)
	FinalizeFailureAtomic(ctx context.Context, args FinalizeFailureArgs) (retryAfterMs int64, err error)
	ReleaseGeoLock(ctx context.Context, geoKey, claimToken string) error
}

func (l *Layer) geometryStore() (GeometryStore, error) {
	gs, ok := l.store.(GeometryStore)
	if !ok {
		return nil, fmt.Errorf("%w: store does not implement GeometryStore", ErrStore)
	}
	return gs, nil
}

// currentEpoch reads epoch key (0 if missing).
func (l *Layer) currentEpoch(ctx context.Context, ipHash string) (int64, error) {
	raw, err := l.store.Get(ctx, l.epochKey(ipHash))
	if errors.Is(err, ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, wrapStore(err)
	}
	n, _ := strconv.ParseInt(string(raw), 10, 64)
	return n, nil
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
	gs, err := l.geometryStore()
	if err != nil {
		return nil, err
	}
	rec, err := gs.ClaimGeometryAtomic(ctx, ClaimGeometryArgs{
		FreezeKey:    l.freezeKey(ipHash),
		GeoKey:       l.geoKey(ipHash),
		EpochKey:     l.epochKey(ipHash),
		ActiveKey:    l.activeKey(ipHash),
		ChallengeKey: l.challengeKey(ipHash, challengeID),
		ChallengeID:  challengeID,
		ExpectClient: sessionHash,
		ExpectIP:     ipHash,
		ClaimToken:   token,
		GeoTTL:       l.geoLockTTL(),
		NowMs:        l.now().UnixMilli(),
	})
	if err != nil {
		return nil, err
	}
	return &GeometryClaim{Record: rec, ClaimToken: token}, nil
}

// FinalizeSuccess releases the IP geo lock after a correct geometry check.
func (l *Layer) FinalizeSuccess(ctx context.Context, ipHash, claimToken string) error {
	gs, err := l.geometryStore()
	if err != nil {
		return nil
	}
	return gs.ReleaseGeoLock(ctx, l.geoKey(ipHash), claimToken)
}

// FinalizeAbort releases geo lock only (internal errors after claim).
func (l *Layer) FinalizeAbort(ctx context.Context, ipHash, claimToken string) error {
	return l.FinalizeSuccess(ctx, ipHash, claimToken)
}

// FinalizeFailure applies escalating IP freeze + bind lock + epoch bump.
func (l *Layer) FinalizeFailure(ctx context.Context, sessionHash, ipHash, claimToken string) (retryAfterMs int64, err error) {
	gs, err := l.geometryStore()
	if err != nil {
		return 0, err
	}
	retry, err := gs.FinalizeFailureAtomic(ctx, FinalizeFailureArgs{
		GeoKey:     l.geoKey(ipHash),
		FreezeKey:  l.freezeKey(ipHash),
		BindKey:    l.bindLockKey(sessionHash, ipHash),
		BadGeoKey:  l.badGeoKey(ipHash),
		RiskKey:    l.riskIPKey(ipHash),
		EpochKey:   l.epochKey(ipHash),
		ClaimToken: claimToken,
		NowMs:      l.now().UnixMilli(),
		BadGeoTTL:  l.badGeoTTL(),
		EpochTTL:   l.epochTTL(),
		RiskTTL:    l.cfg.RiskTTL,
	})
	if err == nil && retry > 0 {
		l.noteGlobalBad(ctx)
	}
	return retry, err
}

package antibot

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"
)

type memItem struct {
	value     []byte
	expiresAt time.Time
	counter   int64
}

func (it *memItem) expired(now time.Time) bool {
	return !it.expiresAt.IsZero() && now.After(it.expiresAt)
}

// MemoryStore is a process-local Store for tests / single-node demos.
type MemoryStore struct {
	mu   sync.Mutex
	data map[string]*memItem
	ops  int
}

// NewMemoryStore creates an empty in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: make(map[string]*memItem)}
}

var _ GeometryStore = (*MemoryStore)(nil)

func (m *MemoryStore) sweepLocked(now time.Time) {
	m.ops++
	if m.ops%256 != 0 {
		return
	}
	for k, it := range m.data {
		if it.expired(now) {
			delete(m.data, k)
		}
	}
}

func (m *MemoryStore) getLocked(key string, now time.Time) (*memItem, bool) {
	it, ok := m.data[key]
	if !ok {
		return nil, false
	}
	if it.expired(now) {
		delete(m.data, key)
		return nil, false
	}
	return it, true
}

func (m *MemoryStore) setLocked(key string, value []byte, ttl time.Duration, now time.Time) {
	cp := make([]byte, len(value))
	copy(cp, value)
	it := &memItem{value: cp}
	if ttl > 0 {
		it.expiresAt = now.Add(ttl)
	}
	m.data[key] = it
}

func (m *MemoryStore) Get(_ context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	it, ok := m.getLocked(key, time.Now())
	if !ok {
		return nil, ErrNotFound
	}
	out := make([]byte, len(it.value))
	copy(out, it.value)
	return out, nil
}

func (m *MemoryStore) GetDel(_ context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	it, ok := m.getLocked(key, time.Now())
	if !ok {
		return nil, ErrNotFound
	}
	delete(m.data, key)
	return it.value, nil
}

func (m *MemoryStore) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	m.sweepLocked(now)
	m.setLocked(key, value, ttl, now)
	return nil
}

func (m *MemoryStore) SetNX(_ context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	m.sweepLocked(now)
	if _, ok := m.getLocked(key, now); ok {
		return false, nil
	}
	m.setLocked(key, value, ttl, now)
	return true, nil
}

func (m *MemoryStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *MemoryStore) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	return m.IncrBy(ctx, key, 1, ttl)
}

func (m *MemoryStore) IncrBy(_ context.Context, key string, delta int64, ttl time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	m.sweepLocked(now)
	it, ok := m.getLocked(key, now)
	if !ok {
		it = &memItem{}
		if ttl > 0 {
			it.expiresAt = now.Add(ttl)
		}
		m.data[key] = it
	} else if ttl > 0 && it.expiresAt.IsZero() {
		it.expiresAt = now.Add(ttl)
	}
	it.counter += delta
	if it.counter < 0 {
		it.counter = 0
	}
	it.value = []byte(strconv.FormatInt(it.counter, 10))
	return it.counter, nil
}

func (m *MemoryStore) readEpochLocked(epochKey string, now time.Time) int64 {
	it, ok := m.getLocked(epochKey, now)
	if !ok || len(it.value) == 0 {
		return 0
	}
	n, _ := strconv.ParseInt(string(it.value), 10, 64)
	return n
}

// IssueChallengeAtomic implements GeometryStore.
func (m *MemoryStore) IssueChallengeAtomic(_ context.Context, args IssueChallengeArgs) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	m.sweepLocked(now)

	if fr, ok := m.getLocked(args.FreezeKey, now); ok {
		untilMs, _ := strconv.ParseInt(string(fr.value), 10, 64)
		retry := untilMs - args.NowMs
		if retry < 0 {
			retry = 0
		}
		return &LockedError{RetryAfterMs: retry}
	}

	epoch := m.readEpochLocked(args.EpochKey, now)
	if act, ok := m.getLocked(args.ActiveKey, now); ok && len(act.value) > 0 {
		prevID := string(act.value)
		delete(m.data, args.ChKeyPrefix+prevID)
	}

	if args.Record == nil {
		return fmt.Errorf("%w: nil record", ErrStore)
	}
	args.Record.IPEpoch = epoch
	raw, err := encodeRecord(args.Record)
	if err != nil {
		return err
	}
	m.setLocked(args.ChallengeKey, raw, args.ChallengeTTL, now)
	m.setLocked(args.ActiveKey, []byte(args.Record.ID), args.ChallengeTTL, now)
	// Refresh epoch key TTL (keep value).
	m.setLocked(args.EpochKey, []byte(strconv.FormatInt(epoch, 10)), args.EpochTTL, now)
	return nil
}

// ClaimGeometryAtomic implements GeometryStore.
func (m *MemoryStore) ClaimGeometryAtomic(_ context.Context, args ClaimGeometryArgs) (*ChallengeRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	m.sweepLocked(now)

	if fr, ok := m.getLocked(args.FreezeKey, now); ok {
		untilMs, _ := strconv.ParseInt(string(fr.value), 10, 64)
		retry := untilMs - args.NowMs
		if retry < 0 {
			retry = 0
		}
		return nil, &LockedError{RetryAfterMs: retry}
	}
	if _, ok := m.getLocked(args.GeoKey, now); ok {
		ttl := args.GeoTTL
		if ttl <= 0 {
			ttl = 5 * time.Second
		}
		return nil, &LockedError{RetryAfterMs: ttl.Milliseconds()}
	}
	it, ok := m.getLocked(args.ChallengeKey, now)
	if !ok {
		return nil, ErrNotFound
	}
	rec, err := decodeRecord(it.value)
	if err != nil {
		return nil, fmt.Errorf("%w: corrupt record", ErrStore)
	}
	if rec.ClientHash != args.ExpectClient || rec.IPHash != args.ExpectIP {
		return nil, ErrNotFound
	}
	epoch := m.readEpochLocked(args.EpochKey, now)
	if rec.IPEpoch != epoch {
		return nil, ErrNotFound
	}
	act, ok := m.getLocked(args.ActiveKey, now)
	if !ok || string(act.value) != args.ChallengeID {
		return nil, ErrNotFound
	}

	tok := append([]byte(nil), []byte(args.ClaimToken)...)
	geo := &memItem{value: tok}
	if args.GeoTTL > 0 {
		geo.expiresAt = now.Add(args.GeoTTL)
	}
	m.data[args.GeoKey] = geo
	delete(m.data, args.ChallengeKey)
	delete(m.data, args.ActiveKey)
	return rec, nil
}

func (m *MemoryStore) ReleaseGeoLock(_ context.Context, geoKey, claimToken string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	it, ok := m.getLocked(geoKey, now)
	if !ok {
		return nil
	}
	if string(it.value) != claimToken {
		return nil
	}
	delete(m.data, geoKey)
	return nil
}

// FinalizeFailureAtomic implements GeometryStore.
func (m *MemoryStore) FinalizeFailureAtomic(_ context.Context, args FinalizeFailureArgs) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	m.sweepLocked(now)

	geo, ok := m.getLocked(args.GeoKey, now)
	if !ok || string(geo.value) != args.ClaimToken {
		return 0, nil // STALE_CLAIM — no side effects
	}

	n, err := m.incrCounterLocked(args.BadGeoKey, 1, args.BadGeoTTL, now)
	if err != nil {
		return 0, err
	}
	ttl := freezeTTLForBadCount(n)
	until := args.NowMs + ttl.Milliseconds()
	untilB := []byte(strconv.FormatInt(until, 10))
	m.setLocked(args.FreezeKey, untilB, ttl, now)
	m.setLocked(args.BindKey, untilB, ttl, now)
	_, _ = m.incrCounterLocked(args.RiskKey, 1, args.RiskTTL, now)

	epoch := m.readEpochLocked(args.EpochKey, now) + 1
	m.setLocked(args.EpochKey, []byte(strconv.FormatInt(epoch, 10)), args.EpochTTL, now)
	delete(m.data, args.GeoKey)
	return ttl.Milliseconds(), nil
}

func (m *MemoryStore) incrCounterLocked(key string, delta int64, ttl time.Duration, now time.Time) (int64, error) {
	it, ok := m.getLocked(key, now)
	if !ok {
		it = &memItem{}
		if ttl > 0 {
			it.expiresAt = now.Add(ttl)
		}
		m.data[key] = it
	} else if ttl > 0 {
		it.expiresAt = now.Add(ttl)
	}
	it.counter += delta
	if it.counter < 0 {
		it.counter = 0
	}
	it.value = []byte(strconv.FormatInt(it.counter, 10))
	return it.counter, nil
}

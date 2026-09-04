package antibot

import (
	"context"
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

// sweepLocked amortizes expiry cleanup: full sweep every 256 mutating ops.
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
	cp := make([]byte, len(value))
	copy(cp, value)
	it := &memItem{value: cp}
	if ttl > 0 {
		it.expiresAt = now.Add(ttl)
	}
	m.data[key] = it
	return nil
}

func (m *MemoryStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *MemoryStore) Incr(_ context.Context, key string, ttl time.Duration) (int64, error) {
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
	}
	it.counter++
	return it.counter, nil
}

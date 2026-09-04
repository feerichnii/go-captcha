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

// MemoryStore is a process-local Store for tests / single-node demos.
type MemoryStore struct {
	mu   sync.Mutex
	data map[string]*memItem
}

// NewMemoryStore creates an empty in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: make(map[string]*memItem)}
}

func (m *MemoryStore) purgeLocked(now time.Time) {
	for k, it := range m.data {
		if !it.expiresAt.IsZero() && now.After(it.expiresAt) {
			delete(m.data, k)
		}
	}
}

func (m *MemoryStore) Get(_ context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.purgeLocked(time.Now())
	it, ok := m.data[key]
	if !ok {
		return nil, ErrNotFound
	}
	out := make([]byte, len(it.value))
	copy(out, it.value)
	return out, nil
}

func (m *MemoryStore) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]byte, len(value))
	copy(cp, value)
	it := &memItem{value: cp}
	if ttl > 0 {
		it.expiresAt = time.Now().Add(ttl)
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
	m.purgeLocked(now)
	it, ok := m.data[key]
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

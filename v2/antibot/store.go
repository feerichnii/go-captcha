package antibot

import (
	"context"
	"encoding/json"
	"time"
)

// Store is the persistence backend (Redis or in-memory).
// Implementations must make Incr/IncrBy and GetDel atomic.
type Store interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	// GetDel atomically returns the value and removes the key (ErrNotFound if absent).
	GetDel(ctx context.Context, key string) ([]byte, error)
	// Incr atomically increments by 1; sets TTL on first create.
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
	// IncrBy atomically adds delta (may be negative); sets TTL on first create.
	// The resulting value is clamped to >= 0.
	IncrBy(ctx context.Context, key string, delta int64, ttl time.Duration) (int64, error)
}

// ChallengeRecord is the server-side challenge state.
// Answer is AEAD-encrypted with Config.SecretKey and bound to ID.
type ChallengeRecord struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"` // click | slide | rotate
	Answer      []byte `json:"answer"`
	PoWDiff     int    `json:"pow_diff,omitempty"`
	PoWSalt     string `json:"pow_salt,omitempty"`
	ClientHash  string `json:"client_hash"`
	CreatedAtMs int64  `json:"created_at_ms"`
	ExpiresAtMs int64  `json:"expires_at_ms"`
	// JS challenge minted at Issue; verified on Verify.
	JSNonce string `json:"js_nonce,omitempty"`
	JSProbe string `json:"js_probe,omitempty"`
}

func encodeRecord(r *ChallengeRecord) ([]byte, error) {
	return json.Marshal(r)
}

func decodeRecord(b []byte) (*ChallengeRecord, error) {
	var r ChallengeRecord
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

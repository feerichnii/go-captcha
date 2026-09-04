package antibot

import (
	"context"
	"encoding/json"
	"time"
)

// Store is the persistence backend (Redis or in-memory).
type Store interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	// Incr increments a counter and returns the new value; sets TTL on first create.
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
}

// ChallengeRecord is the server-side challenge state.
type ChallengeRecord struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"` // click | slide | rotate
	Answer    json.RawMessage `json:"answer"`
	Attempts  int             `json:"attempts"`
	Consumed  bool            `json:"consumed"`
	PoWDiff   int             `json:"pow_diff,omitempty"`
	PoWSalt   string          `json:"pow_salt,omitempty"`
	ClientKey string          `json:"client_key,omitempty"`
	CreatedAt int64           `json:"created_at"`
	ExpiresAt int64           `json:"expires_at"`
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

package antibot

import "time"

// Config controls the AntiBot layer.
type Config struct {
	// TTL for challenges (default 90s).
	TTL time.Duration
	// MaxAttempts per challenge (default 3).
	MaxAttempts int
	// HMACKey seals stored answer payloads (required for production).
	HMACKey []byte

	// RateLimitMax requests per RateLimitWindow per client key (default 30 / min).
	RateLimitMax    int
	RateLimitWindow time.Duration

	// MinBehaviorScore in [0,1]; below → reject even if geometry matches (default 0.45).
	MinBehaviorScore float64
	// SoftBehaviorScore: pass geometry but require PoW next time / flag (default 0.65).
	SoftBehaviorScore float64

	// PoWDifficulty is leading zero bits for suspicious clients (default 16).
	PoWDifficulty int
	// PoWAlways if true, always attach PoW on issue (default false).
	PoWAlways bool

	// Prefix for store keys (default "gocaptcha:antibot:").
	KeyPrefix string
}

func (c *Config) withDefaults() Config {
	out := *c
	if out.TTL <= 0 {
		out.TTL = 90 * time.Second
	}
	if out.MaxAttempts <= 0 {
		out.MaxAttempts = 3
	}
	if out.RateLimitMax <= 0 {
		out.RateLimitMax = 30
	}
	if out.RateLimitWindow <= 0 {
		out.RateLimitWindow = time.Minute
	}
	if out.MinBehaviorScore <= 0 {
		out.MinBehaviorScore = 0.45
	}
	if out.SoftBehaviorScore <= 0 {
		out.SoftBehaviorScore = 0.65
	}
	if out.PoWDifficulty <= 0 {
		out.PoWDifficulty = 16
	}
	if out.KeyPrefix == "" {
		out.KeyPrefix = "gocaptcha:antibot:"
	}
	return out
}

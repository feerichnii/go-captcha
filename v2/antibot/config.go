package antibot

import (
	"crypto/rand"
	"errors"
	"math"
	"time"
)

const (
	// MinSecretKeyLen is the minimum SecretKey length in bytes.
	MinSecretKeyLen = 32
	// MaxPoWDifficultyBits is the hard cap on leading zero bits.
	MaxPoWDifficultyBits = 32
)

// Config controls the AntiBot layer. Zero values take the documented defaults;
// fields marked "0 = disabled" are opt-in.
type Config struct {
	// SecretKey encrypts stored answers (AES-256-GCM). Required: >= 32 bytes
	// of high-entropy secret (crypto/rand), not a passphrase or short string.
	SecretKey []byte

	// TTL for challenges (default 90s).
	TTL time.Duration
	// MaxAttempts per challenge, counted atomically (default 3).
	MaxAttempts int

	// IssueRateMax challenges per RateWindow per client key (default 30).
	IssueRateMax int
	// VerifyRateMax verify calls per RateWindow per client key (default 60).
	VerifyRateMax int
	// RateWindow is the fixed window for both limits (default 1m).
	RateWindow time.Duration

	// MinSolveTime: verify earlier than this after issue is rejected (default 300ms).
	MinSolveTime time.Duration
	// MaxTrajectoryPoints / MaxTrajectoryEvents cap request size (default 2000 / 500).
	MaxTrajectoryPoints int
	MaxTrajectoryEvents int
	// MaxJumpPx rejects trajectories with a single-step jump larger than this (default 800).
	MaxJumpPx float64
	// MaxNonceLen caps the PoW nonce length (default 64).
	MaxNonceLen int
	// MaxAnswerBytes caps the submitted answer JSON (default 4096).
	MaxAnswerBytes int

	// Padding tolerances are server-side and never taken from the client.
	ClickPadding  int // px, default 5
	SlidePadding  int // px, default 5
	RotatePadding int // degrees, default 5

	// Scorer produces the behavior score; default HeuristicScorer with DefaultWeights.
	Scorer Scorer
	// RiskThreshold: score below this raises the client's persistent risk level (default 0.5).
	RiskThreshold float64
	// HardRejectScore: 0 = disabled. If > 0, score below this fails the verify.
	// Behavior score is a risk signal, not proof of humanity; keep this low or off.
	HardRejectScore float64
	// RiskTTL is how long a client's risk level / stats persist (default 1h).
	RiskTTL time.Duration
	// MaxRiskLevel caps escalation. Default is derived so PoWMaxDifficulty is reachable:
	// 1 + ceil((PoWMaxDifficulty - PoWBaseDifficulty) / PoWStepPerLevel).
	MaxRiskLevel int

	// FailRateWindow is the window for fail/issue counters (default = RiskTTL).
	FailRateWindow time.Duration
	// HighFailRate triggers +1 risk when fails/issues >= this (default 0.6, min 5 issues).
	HighFailRate float64
	// HighIssueRatePerMin triggers +1 risk when Issue rate exceeds this (default 20).
	HighIssueRatePerMin float64
	// MinSessionAge: session younger than this adds risk (default 2s). 0 disables.
	MinSessionAge time.Duration

	// PoWBaseDifficulty is leading zero bits at risk level 1 (default 14).
	PoWBaseDifficulty int
	// PoWStepPerLevel adds bits per extra risk level (default 2).
	PoWStepPerLevel int
	// PoWMaxDifficulty caps difficulty (default 22).
	PoWMaxDifficulty int
	// PoWAlways attaches PoW at base difficulty to every challenge (default false).
	PoWAlways bool
	// PoWProbeProb is the probability [0,1] of attaching a small PoW even to
	// clean (risk 0) clients (default 0.08). Negative disables. Makes bots pay
	// even when "lucky".
	PoWProbeProb float64
	// PoWProbeDifficulty is the soft PoW for probe challenges (default 10).
	PoWProbeDifficulty int
	// PoWJitterBits randomly adds 0..N bits to the chosen difficulty (default 1).
	// Negative disables jitter.
	PoWJitterBits int

	// Telemetry receives issue/verify events (default NoopTelemetry).
	Telemetry Telemetry

	// KeyPrefix for store keys (default "gocaptcha:antibot:").
	KeyPrefix string
}

func (c *Config) withDefaults() Config {
	out := *c
	setDur := func(d *time.Duration, def time.Duration) {
		if *d <= 0 {
			*d = def
		}
	}
	setInt := func(v *int, def int) {
		if *v <= 0 {
			*v = def
		}
	}
	setDur(&out.TTL, 90*time.Second)
	setInt(&out.MaxAttempts, 3)
	setInt(&out.IssueRateMax, 30)
	setInt(&out.VerifyRateMax, 60)
	setDur(&out.RateWindow, time.Minute)
	setDur(&out.MinSolveTime, 300*time.Millisecond)
	setInt(&out.MaxTrajectoryPoints, 2000)
	setInt(&out.MaxTrajectoryEvents, 500)
	if out.MaxJumpPx <= 0 {
		out.MaxJumpPx = 800
	}
	setInt(&out.MaxNonceLen, 64)
	setInt(&out.MaxAnswerBytes, 4096)
	setInt(&out.ClickPadding, 5)
	setInt(&out.SlidePadding, 5)
	setInt(&out.RotatePadding, 5)
	if out.Scorer == nil {
		out.Scorer = HeuristicScorer{Weights: DefaultWeights()}
	}
	if out.RiskThreshold <= 0 {
		out.RiskThreshold = 0.5
	}
	setDur(&out.RiskTTL, time.Hour)
	setDur(&out.FailRateWindow, out.RiskTTL)
	if out.HighFailRate <= 0 {
		out.HighFailRate = 0.6
	}
	if out.HighIssueRatePerMin <= 0 {
		out.HighIssueRatePerMin = 20
	}
	setDur(&out.MinSessionAge, 2*time.Second)
	setInt(&out.PoWBaseDifficulty, 14)
	setInt(&out.PoWStepPerLevel, 2)
	setInt(&out.PoWMaxDifficulty, 22)
	if out.PoWMaxDifficulty > MaxPoWDifficultyBits {
		out.PoWMaxDifficulty = MaxPoWDifficultyBits
	}
	// PoWProbeProb: negative = disabled; zero (unset) = default 0.08.
	if c.PoWProbeProb < 0 {
		out.PoWProbeProb = 0
	} else if c.PoWProbeProb == 0 && !c.PoWAlways {
		out.PoWProbeProb = 0.08
	}
	setInt(&out.PoWProbeDifficulty, 10)
	// PoWJitterBits: negative = disabled; zero (unset) = default 1.
	if c.PoWJitterBits < 0 {
		out.PoWJitterBits = 0
	} else if c.PoWJitterBits == 0 {
		out.PoWJitterBits = 1
	}
	// Ensure MaxRiskLevel can actually reach PoWMaxDifficulty.
	need := 1
	if out.PoWStepPerLevel > 0 && out.PoWMaxDifficulty > out.PoWBaseDifficulty {
		need = 1 + int(math.Ceil(float64(out.PoWMaxDifficulty-out.PoWBaseDifficulty)/float64(out.PoWStepPerLevel)))
	}
	if out.MaxRiskLevel <= 0 {
		out.MaxRiskLevel = need
	} else if out.MaxRiskLevel < need {
		out.MaxRiskLevel = need
	}
	if out.Telemetry == nil {
		out.Telemetry = NoopTelemetry{}
	}
	if out.KeyPrefix == "" {
		out.KeyPrefix = "gocaptcha:antibot:"
	}
	return out
}

// ValidateSecretKey checks length and rejects trivially weak keys.
func ValidateSecretKey(key []byte) error {
	if len(key) < MinSecretKeyLen {
		return ErrWeakSecretKey
	}
	// Reject all-zero and low-entropy repeats (e.g. "aaaaaaaa...").
	allSame := true
	for i := 1; i < len(key); i++ {
		if key[i] != key[0] {
			allSame = false
			break
		}
	}
	if allSame {
		return ErrWeakSecretKey
	}
	return nil
}

// powDifficultyFor maps a risk level to a clamped PoW difficulty (before jitter).
func (c *Config) powDifficultyFor(level int) int {
	if level <= 0 {
		return 0
	}
	d := c.PoWBaseDifficulty + (level-1)*c.PoWStepPerLevel
	if d > c.PoWMaxDifficulty {
		d = c.PoWMaxDifficulty
	}
	return d
}

// choosePoW picks the difficulty for a challenge, applying probe probability
// for clean clients and small random jitter so bots cannot precompute.
func (c *Config) choosePoW(level int) int {
	d := c.powDifficultyFor(level)
	if d == 0 {
		if c.PoWAlways {
			d = c.PoWBaseDifficulty
		} else if c.PoWProbeProb > 0 && coinFlip(c.PoWProbeProb) {
			d = c.PoWProbeDifficulty
		}
	}
	if d <= 0 {
		return 0
	}
	if c.PoWJitterBits > 0 {
		d += cryptoIntn(c.PoWJitterBits + 1)
		if d > c.PoWMaxDifficulty {
			d = c.PoWMaxDifficulty
		}
	}
	return d
}

func cryptoIntn(n int) int {
	if n <= 1 {
		return 0
	}
	var b [1]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0
	}
	return int(b[0]) % n
}

func coinFlip(p float64) bool {
	if p <= 0 {
		return false
	}
	if p >= 1 {
		return true
	}
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		return false
	}
	v := float64(uint16(b[0])<<8|uint16(b[1])) / 65535.0
	return v < p
}

// ErrWeakSecretKey is returned when SecretKey is too short or low-entropy.
var ErrWeakSecretKey = errors.New("antibot: SecretKey must be >= 32 high-entropy bytes")

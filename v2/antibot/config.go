package antibot

import "time"

// Config controls the AntiBot layer. Zero values take the documented defaults;
// fields marked "0 = disabled" are opt-in.
type Config struct {
	// SecretKey encrypts stored answers (AES-256-GCM). Required.
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
	// RiskTTL is how long a client's risk level persists (default 1h).
	RiskTTL time.Duration
	// MaxRiskLevel caps escalation (default 4).
	MaxRiskLevel int

	// PoWBaseDifficulty is leading zero bits at risk level 1 (default 14).
	PoWBaseDifficulty int
	// PoWStepPerLevel adds bits per extra risk level (default 2).
	PoWStepPerLevel int
	// PoWMaxDifficulty caps difficulty (default 22).
	PoWMaxDifficulty int
	// PoWAlways attaches PoW at base difficulty to every challenge (default false).
	PoWAlways bool

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
	setInt(&out.MaxRiskLevel, 4)
	setInt(&out.PoWBaseDifficulty, 14)
	setInt(&out.PoWStepPerLevel, 2)
	setInt(&out.PoWMaxDifficulty, 22)
	if out.PoWMaxDifficulty > 32 {
		out.PoWMaxDifficulty = 32
	}
	if out.Telemetry == nil {
		out.Telemetry = NoopTelemetry{}
	}
	if out.KeyPrefix == "" {
		out.KeyPrefix = "gocaptcha:antibot:"
	}
	return out
}

// powDifficultyFor maps a risk level to a clamped PoW difficulty.
func (c *Config) powDifficultyFor(level int) int {
	if level <= 0 {
		if c.PoWAlways {
			return c.PoWBaseDifficulty
		}
		return 0
	}
	d := c.PoWBaseDifficulty + (level-1)*c.PoWStepPerLevel
	if d > c.PoWMaxDifficulty {
		d = c.PoWMaxDifficulty
	}
	return d
}

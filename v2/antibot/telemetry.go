package antibot

import "errors"

// Telemetry receives structured events for logging, metrics and score calibration.
// Implementations must be safe for concurrent use and must not block.
type Telemetry interface {
	OnIssue(IssueEvent)
	OnVerify(VerifyEvent)
}

// IssueEvent is emitted after a challenge is stored.
type IssueEvent struct {
	ChallengeID   string
	Kind          string
	ClientHash    string
	RiskLevel     int
	PoWDifficulty int
}

// VerifyEvent is emitted once per Verify call, whatever the outcome.
type VerifyEvent struct {
	ChallengeID string
	Kind        string
	ClientHash  string
	// Outcome is "ok" or the error name (bad_answer, pow_invalid, too_fast, ...).
	Outcome string
	Attempt int64
	// ElapsedMs is server-measured time since issue.
	ElapsedMs int64
	// TrajectoryMs is the client-claimed interaction duration.
	TrajectoryMs         int64
	TrajectoryPoints     int
	TrajectoryConsistent bool
	Score                float64
	Components           map[string]float64
	RiskLevelBefore      int
	RiskLevelAfter       int
	PoWDifficulty        int
	GeometryDurationMs   int64
}

// NoopTelemetry drops all events.
type NoopTelemetry struct{}

func (NoopTelemetry) OnIssue(IssueEvent)   {}
func (NoopTelemetry) OnVerify(VerifyEvent) {}

// TelemetryFunc adapts plain functions to Telemetry.
type TelemetryFunc struct {
	Issue  func(IssueEvent)
	Verify func(VerifyEvent)
}

func (t TelemetryFunc) OnIssue(e IssueEvent) {
	if t.Issue != nil {
		t.Issue(e)
	}
}

func (t TelemetryFunc) OnVerify(e VerifyEvent) {
	if t.Verify != nil {
		t.Verify(e)
	}
}

func outcomeName(err error) string {
	if err == nil {
		return "ok"
	}
	switch {
	case errors.Is(err, ErrNotFound):
		return "not_found"
	case errors.Is(err, ErrMaxAttempts):
		return "max_attempts"
	case errors.Is(err, ErrLocked):
		return "locked"
	case errors.Is(err, ErrRateLimited):
		return "rate_limited"
	case errors.Is(err, ErrBadAnswer):
		return "bad_answer"
	case errors.Is(err, ErrLowScore):
		return "low_score"
	case errors.Is(err, ErrPoWInvalid):
		return "pow_invalid"
	case errors.Is(err, ErrTooFast):
		return "too_fast"
	case errors.Is(err, ErrInvalidRequest):
		return "invalid_request"
	case errors.Is(err, ErrJSChallengeFailed):
		return "js_challenge_failed"
	case errors.Is(err, ErrBrowserRequired):
		return "browser_required"
	case errors.Is(err, ErrPiecePressRequired):
		return "piece_press_required"
	case errors.Is(err, ErrMissingClientIP):
		return "missing_client_ip"
	default:
		return "error"
	}
}

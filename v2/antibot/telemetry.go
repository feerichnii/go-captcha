package antibot

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
	TrajectoryMs        int64
	TrajectoryPoints    int
	TrajectoryConsistent bool
	Score               float64
	Components          map[string]float64
	RiskLevelBefore     int
	RiskLevelAfter      int
	PoWDifficulty       int
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
	switch err {
	case nil:
		return "ok"
	case ErrNotFound:
		return "not_found"
	case ErrMaxAttempts:
		return "max_attempts"
	case ErrRateLimited:
		return "rate_limited"
	case ErrBadAnswer:
		return "bad_answer"
	case ErrLowScore:
		return "low_score"
	case ErrPoWInvalid:
		return "pow_invalid"
	case ErrTooFast:
		return "too_fast"
	case ErrInvalidRequest:
		return "invalid_request"
	default:
		return "internal"
	}
}

package antibot

import (
	"context"
	"time"
)

// RiskInputs aggregates every signal that can move the persistent risk level.
type RiskInputs struct {
	Score          ScoreResult
	BrowserDelta   int
	BrowserReasons []string
	Signals        ClientSignals
	Solved         bool
	Failed         bool // geometry / pow / trajectory hard-fail
	NowMs          int64
}

// RiskDecision is the outcome of EvaluateRisk.
type RiskDecision struct {
	LevelBefore int
	LevelAfter  int
	Delta       int
	Reasons     []string
}

func (l *Layer) riskKey(hash string) string  { return l.cfg.KeyPrefix + "risk:" + hash }
func (l *Layer) failsKey(hash string) string { return l.cfg.KeyPrefix + "fails:" + hash }
func (l *Layer) issueNKey(hash string) string {
	return l.cfg.KeyPrefix + "issues:" + hash
}

// RiskLevel returns the persistent risk level for a client key (0 = clean).
func (l *Layer) RiskLevel(ctx context.Context, clientKey string) (int, error) {
	return l.riskLevel(ctx, hashClient(clientKey))
}

func (l *Layer) riskLevel(ctx context.Context, hash string) (int, error) {
	n, err := l.store.IncrBy(ctx, l.riskKey(hash), 0, l.cfg.RiskTTL)
	if err != nil {
		return 0, wrapStore(err)
	}
	return int(n), nil
}

// bumpRisk atomically adjusts the risk counter and clamps to [0, MaxRiskLevel].
func (l *Layer) bumpRisk(ctx context.Context, hash string, delta int) (int, error) {
	if delta == 0 {
		return l.riskLevel(ctx, hash)
	}
	n, err := l.store.IncrBy(ctx, l.riskKey(hash), int64(delta), l.cfg.RiskTTL)
	if err != nil {
		return 0, wrapStore(err)
	}
	if int(n) > l.cfg.MaxRiskLevel {
		// Clamp: set exactly MaxRiskLevel via compensating decrement.
		_, _ = l.store.IncrBy(ctx, l.riskKey(hash), int64(l.cfg.MaxRiskLevel)-n, l.cfg.RiskTTL)
		return l.cfg.MaxRiskLevel, nil
	}
	return int(n), nil
}

func (l *Layer) recordIssue(ctx context.Context, hash string) {
	_, _ = l.store.Incr(ctx, l.issueNKey(hash), l.cfg.FailRateWindow)
}

func (l *Layer) recordFail(ctx context.Context, hash string) {
	_, _ = l.store.Incr(ctx, l.failsKey(hash), l.cfg.FailRateWindow)
}

// EvaluateRisk computes the delta from all signals and applies it atomically.
func (l *Layer) EvaluateRisk(ctx context.Context, hash string, in RiskInputs) (RiskDecision, error) {
	before, err := l.riskLevel(ctx, hash)
	if err != nil {
		return RiskDecision{}, err
	}
	dec := RiskDecision{LevelBefore: before, Reasons: append([]string{}, in.BrowserReasons...)}
	delta := in.BrowserDelta

	if in.Score.Issues.Suspicious() || !in.Score.Consistent {
		delta++
		dec.Reasons = append(dec.Reasons, "trajectory_suspicious")
	} else if in.Score.Score < l.cfg.RiskThreshold {
		delta++
		dec.Reasons = append(dec.Reasons, "low_score")
	}

	if in.Signals.SessionIssuedAtMs > 0 && l.cfg.MinSessionAge > 0 {
		age := in.NowMs - in.Signals.SessionIssuedAtMs
		if age >= 0 && age < l.cfg.MinSessionAge.Milliseconds() {
			delta++
			dec.Reasons = append(dec.Reasons, "young_session")
		}
	}

	// Fail-rate / issue-frequency over the rolling window.
	issues, _ := l.store.IncrBy(ctx, l.issueNKey(hash), 0, l.cfg.FailRateWindow)
	fails, _ := l.store.IncrBy(ctx, l.failsKey(hash), 0, l.cfg.FailRateWindow)
	if issues >= 5 {
		rate := float64(fails) / float64(issues)
		if rate >= l.cfg.HighFailRate {
			delta++
			dec.Reasons = append(dec.Reasons, "high_fail_rate")
		}
	}
	windowMin := l.cfg.FailRateWindow.Minutes()
	if windowMin <= 0 {
		windowMin = 60
	}
	if float64(issues)/windowMin >= l.cfg.HighIssueRatePerMin {
		delta++
		dec.Reasons = append(dec.Reasons, "high_issue_rate")
	}

	// Empty UA is only suspicious when the caller is supplying other signals
	// (otherwise most server integrations leave Signals zero-valued).
	if in.Signals.UserAgent == "" && (in.Signals.IP != "" || in.Signals.ASN != 0) {
		delta++
		dec.Reasons = append(dec.Reasons, "empty_ua")
	}
	if hints := FormatUAHint(in.Signals.UserAgent); len(hints) > 0 {
		delta++
		dec.Reasons = append(dec.Reasons, hints...)
	}

	if in.Failed {
		l.recordFail(ctx, hash)
	}

	// Clean solve de-escalates by 1 unless other signals still scream.
	if in.Solved && delta == 0 && before > 0 {
		delta = -1
		dec.Reasons = append(dec.Reasons, "clean_solve")
	}

	after, err := l.bumpRisk(ctx, hash, delta)
	if err != nil {
		return dec, err
	}
	dec.Delta = delta
	dec.LevelAfter = after
	return dec, nil
}

// noteIssued increments the per-client issue counter (call from Issue).
func (l *Layer) noteIssued(ctx context.Context, hash string) {
	l.recordIssue(ctx, hash)
}

// timeNowMs is a tiny helper for tests that override l.now.
func (l *Layer) timeNowMs() int64 { return l.now().UnixMilli() }

var _ = time.Second

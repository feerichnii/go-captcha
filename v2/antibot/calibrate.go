package antibot

import (
	"sort"
	"sync"
)

// Calibrator collects labeled behavior scores (human / bot) from telemetry
// and suggests thresholds. Feed it from your own ground truth (e.g. verified
// accounts vs. known abuse) — the heuristic score is only as good as its calibration.
type Calibrator struct {
	mu     sync.Mutex
	humans []float64
	bots   []float64
}

// NewCalibrator returns an empty calibrator.
func NewCalibrator() *Calibrator { return &Calibrator{} }

// Record adds one labeled sample.
func (c *Calibrator) Record(score float64, human bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if human {
		c.humans = append(c.humans, score)
	} else {
		c.bots = append(c.bots, score)
	}
}

// CalibrationReport summarizes collected samples.
type CalibrationReport struct {
	Humans, Bots int
	// HumanP05 is the 5th percentile of human scores: a RiskThreshold at or
	// below this flags <= 5% of humans.
	HumanP05 float64
	// BotP95 is the 95th percentile of bot scores: a HardRejectScore above
	// this rejects >= 95% of observed bots.
	BotP95 float64
	// SuggestedRiskThreshold is HumanP05 (never above BotP95 when both exist).
	SuggestedRiskThreshold float64
}

// Report computes percentiles; zero values when there is no data.
func (c *Calibrator) Report() CalibrationReport {
	c.mu.Lock()
	h := append([]float64(nil), c.humans...)
	b := append([]float64(nil), c.bots...)
	c.mu.Unlock()

	r := CalibrationReport{Humans: len(h), Bots: len(b)}
	if len(h) > 0 {
		r.HumanP05 = percentile(h, 0.05)
		r.SuggestedRiskThreshold = r.HumanP05
	}
	if len(b) > 0 {
		r.BotP95 = percentile(b, 0.95)
		if len(h) > 0 && r.SuggestedRiskThreshold > r.BotP95 {
			r.SuggestedRiskThreshold = r.BotP95
		}
	}
	return r
}

func percentile(v []float64, p float64) float64 {
	sort.Float64s(v)
	if len(v) == 1 {
		return v[0]
	}
	idx := p * float64(len(v)-1)
	lo := int(idx)
	hi := lo + 1
	if hi >= len(v) {
		return v[lo]
	}
	frac := idx - float64(lo)
	return v[lo]*(1-frac) + v[hi]*frac
}

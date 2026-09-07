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
	// SuggestedHardReject is BotP95 when bots exist (0 if none).
	SuggestedHardReject float64
	// BestF1Threshold maximizes F1 treating score>=t as human.
	BestF1Threshold float64
	BestF1          float64
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
		r.SuggestedHardReject = r.BotP95
		if len(h) > 0 && r.SuggestedRiskThreshold > r.BotP95 {
			r.SuggestedRiskThreshold = r.BotP95
		}
	}
	r.BestF1Threshold, r.BestF1 = bestF1Threshold(h, b)
	return r
}

// ROCPoint is true-positive / false-positive rate at a score threshold
// (score >= t classified as human).
type ROCPoint struct {
	Threshold float64
	TPR       float64
	FPR       float64
}

// ROC returns TPR/FPR for each unique sample score as threshold.
func (c *Calibrator) ROC() []ROCPoint {
	c.mu.Lock()
	h := append([]float64(nil), c.humans...)
	b := append([]float64(nil), c.bots...)
	c.mu.Unlock()
	if len(h)+len(b) == 0 {
		return nil
	}
	thresh := uniqueSorted(append(append([]float64{}, h...), b...))
	out := make([]ROCPoint, 0, len(thresh))
	for _, t := range thresh {
		var tp, fn, fp, tn float64
		for _, s := range h {
			if s >= t {
				tp++
			} else {
				fn++
			}
		}
		for _, s := range b {
			if s >= t {
				fp++
			} else {
				tn++
			}
		}
		tpr, fpr := 0.0, 0.0
		if tp+fn > 0 {
			tpr = tp / (tp + fn)
		}
		if fp+tn > 0 {
			fpr = fp / (fp + tn)
		}
		out = append(out, ROCPoint{Threshold: t, TPR: tpr, FPR: fpr})
	}
	return out
}

func bestF1Threshold(humans, bots []float64) (thresh, f1 float64) {
	if len(humans) == 0 || len(bots) == 0 {
		return 0, 0
	}
	cands := uniqueSorted(append(append([]float64{}, humans...), bots...))
	bestT, best := cands[0], -1.0
	for _, t := range cands {
		var tp, fp, fn float64
		for _, s := range humans {
			if s >= t {
				tp++
			} else {
				fn++
			}
		}
		for _, s := range bots {
			if s >= t {
				fp++
			}
		}
		prec := 0.0
		if tp+fp > 0 {
			prec = tp / (tp + fp)
		}
		rec := 0.0
		if tp+fn > 0 {
			rec = tp / (tp + fn)
		}
		cur := 0.0
		if prec+rec > 0 {
			cur = 2 * prec * rec / (prec + rec)
		}
		if cur > best {
			best, bestT = cur, t
		}
	}
	return bestT, best
}

func uniqueSorted(v []float64) []float64 {
	sort.Float64s(v)
	out := v[:0]
	for i, x := range v {
		if i == 0 || x != v[i-1] {
			out = append(out, x)
		}
	}
	return out
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

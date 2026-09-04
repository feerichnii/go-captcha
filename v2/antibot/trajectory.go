package antibot

import (
	"math"
)

// Point is a trajectory sample (coordinates + client timestamp in ms).
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	T int64   `json:"t"`
}

// Trajectory is pointer/touch movement collected on the client.
// It is untrusted input: treat the resulting score as a risk signal only.
type Trajectory struct {
	Points []Point  `json:"points"`
	Events []string `json:"events,omitempty"` // pointerdown, pointermove, pointerup, ...
}

// DurationMs returns the client-claimed interaction time.
func (tr Trajectory) DurationMs() int64 {
	if len(tr.Points) < 2 {
		return 0
	}
	return tr.Points[len(tr.Points)-1].T - tr.Points[0].T
}

// ScoreContext carries server-side facts the scorer may use.
type ScoreContext struct {
	// ElapsedMs is the server-measured time between issue and verify.
	ElapsedMs int64
}

// ScoreResult holds the behavior score and diagnostics.
type ScoreResult struct {
	Score      float64            `json:"score"`
	Components map[string]float64 `json:"components,omitempty"`
	// Consistent is false when client-claimed timing contradicts server timing.
	Consistent bool `json:"consistent"`
}

// Scorer maps a trajectory to a [0,1] human-likeness estimate.
type Scorer interface {
	Score(tr Trajectory, sc ScoreContext) ScoreResult
}

// ScoreWeights tune HeuristicScorer. They are normalized at scoring time.
type ScoreWeights struct {
	Points       float64
	Duration     float64
	Velocity     float64
	Acceleration float64
	Timing       float64
	Corrections  float64
	Events       float64
}

// DefaultWeights are a starting point; calibrate on your own traffic.
func DefaultWeights() ScoreWeights {
	return ScoreWeights{
		Points:       0.15,
		Duration:     0.20,
		Velocity:     0.20,
		Acceleration: 0.15,
		Timing:       0.15,
		Corrections:  0.10,
		Events:       0.05,
	}
}

func (w ScoreWeights) sum() float64 {
	return w.Points + w.Duration + w.Velocity + w.Acceleration + w.Timing + w.Corrections + w.Events
}

// HeuristicScorer is the default Scorer.
type HeuristicScorer struct {
	Weights ScoreWeights
}

// ScoreBehavior scores with DefaultWeights and no server context.
func ScoreBehavior(tr Trajectory) ScoreResult {
	return HeuristicScorer{Weights: DefaultWeights()}.Score(tr, ScoreContext{})
}

// Score implements Scorer.
func (h HeuristicScorer) Score(tr Trajectory, sc ScoreContext) ScoreResult {
	w := h.Weights
	if w.sum() <= 0 {
		w = DefaultWeights()
	}
	comp := map[string]float64{}
	pts := tr.Points
	res := ScoreResult{Components: comp, Consistent: true}

	if len(pts) < 3 {
		comp["points"] = 0
		res.Score = 0.05
		return res
	}
	comp["points"] = clamp01(float64(len(pts)) / 40)

	duration := float64(tr.DurationMs())
	if duration <= 0 {
		comp["duration"] = 0
		res.Score = 0.05
		return res
	}
	comp["duration"] = durationScore(duration)

	// The interaction happened entirely between issue and verify, so its
	// claimed duration cannot exceed server-observed elapsed time (small slack
	// for timer granularity).
	if sc.ElapsedMs > 0 && duration > float64(sc.ElapsedMs)+100 {
		res.Consistent = false
	}

	vels, accs, gaps, corrections := motionStats(pts)
	comp["velocity"] = varianceScore(vels, 0.02, 8)
	comp["acceleration"] = varianceScore(accs, 0.01, 20)
	comp["timing"] = varianceScore(gaps, 2, 120)
	comp["corrections"] = clamp01(float64(corrections) / 4)
	comp["events"] = eventScore(tr.Events)

	score := w.Points*comp["points"] +
		w.Duration*comp["duration"] +
		w.Velocity*comp["velocity"] +
		w.Acceleration*comp["acceleration"] +
		w.Timing*comp["timing"] +
		w.Corrections*comp["corrections"] +
		w.Events*comp["events"]
	score /= w.sum()

	if !res.Consistent {
		score *= 0.5
	}
	res.Score = clamp01(score)
	return res
}

func eventScore(events []string) float64 {
	if len(events) == 0 {
		return 0.3
	}
	hasDown, hasUp, hasMove := false, false, false
	for _, e := range events {
		switch e {
		case "pointerdown", "mousedown", "touchstart":
			hasDown = true
		case "pointerup", "mouseup", "touchend":
			hasUp = true
		case "pointermove", "mousemove", "touchmove":
			hasMove = true
		}
	}
	switch {
	case hasDown && hasUp && hasMove:
		return 1
	case hasMove:
		return 0.75
	default:
		return 0.7
	}
}

func durationScore(ms float64) float64 {
	switch {
	case ms < 80:
		return 0.05
	case ms < 250:
		return 0.35
	case ms <= 12000:
		return 1
	case ms <= 30000:
		return 0.7
	default:
		return 0.4
	}
}

func motionStats(pts []Point) (vels, accs, gaps []float64, corrections int) {
	vels = make([]float64, 0, len(pts)-1)
	gaps = make([]float64, 0, len(pts)-1)
	var prevVx, prevVy float64
	hasPrevV := false

	for i := 1; i < len(pts); i++ {
		dt := float64(pts[i].T - pts[i-1].T)
		if dt <= 0 {
			dt = 1
		}
		gaps = append(gaps, dt)
		dx := pts[i].X - pts[i-1].X
		dy := pts[i].Y - pts[i-1].Y
		vx := dx / dt
		vy := dy / dt
		speed := math.Hypot(vx, vy)
		vels = append(vels, speed)

		if hasPrevV {
			ax := (vx - prevVx) / dt
			ay := (vy - prevVy) / dt
			accs = append(accs, math.Hypot(ax, ay))
			dot := prevVx*vx + prevVy*vy
			if dot < 0 && speed > 0.01 && math.Hypot(prevVx, prevVy) > 0.01 {
				corrections++
			}
		}
		prevVx, prevVy = vx, vy
		hasPrevV = true
	}
	return vels, accs, gaps, corrections
}

// varianceScore rewards natural variance; near-zero or huge variance looks bot-like.
func varianceScore(samples []float64, lowGood, highGood float64) float64 {
	if len(samples) < 2 {
		return 0.2
	}
	mean := 0.0
	for _, v := range samples {
		mean += v
	}
	mean /= float64(len(samples))
	var sum float64
	for _, v := range samples {
		d := v - mean
		sum += d * d
	}
	variance := sum / float64(len(samples))
	if math.IsNaN(variance) || math.IsInf(variance, 0) {
		return 0
	}
	if variance < lowGood*lowGood*0.01 {
		return 0.15
	}
	if variance >= lowGood && variance <= highGood {
		return 1
	}
	if variance < lowGood {
		return clamp01(variance / lowGood)
	}
	return clamp01(highGood / variance)
}

func clamp01(v float64) float64 {
	if math.IsNaN(v) || v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

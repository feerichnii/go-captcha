package antibot

import (
	"math"
	"strings"
)

// Point is a trajectory sample. Optional pointer fields make fabricated
// trajectories harder: real PointerEvents carry pointerType/buttons/pressure.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	T int64   `json:"t"` // epoch ms

	// PointerEvent fields (optional but scored).
	PointerType string  `json:"pointer_type,omitempty"` // mouse | touch | pen
	PointerID   int     `json:"pointer_id,omitempty"`
	Buttons     int     `json:"buttons,omitempty"`
	Pressure    float64 `json:"pressure,omitempty"` // 0..1; mouse often 0.5 when down
	TiltX       float64 `json:"tilt_x,omitempty"`
	TiltY       float64 `json:"tilt_y,omitempty"`
	Width       float64 `json:"width,omitempty"`  // contact geometry
	Height      float64 `json:"height,omitempty"`
	IsPrimary   *bool   `json:"is_primary,omitempty"`
	Coalesced   int     `json:"coalesced,omitempty"` // len(getCoalescedEvents())
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

// TrajectoryIssues lists structural problems; empty means structurally OK.
type TrajectoryIssues struct {
	TooFewPoints       bool
	BadEventOrder      bool // missing down→move→up
	NonMonotonicTime   bool
	HugeJump           bool
	FinalFarFromAnswer bool // set by caller when answer coords known
	MissingPointerMeta bool // no pointerType/buttons on any point
	NoCoalesced        bool // zero coalesced samples on a long drag (suspicious on modern browsers)
	Reasons            []string
}

func (i TrajectoryIssues) Suspicious() bool {
	// MissingPointerMeta / NoCoalesced are soft signals (score components only).
	return i.TooFewPoints || i.BadEventOrder || i.NonMonotonicTime || i.HugeJump ||
		i.FinalFarFromAnswer
}

func (i *TrajectoryIssues) add(reason string) {
	i.Reasons = append(i.Reasons, reason)
}

// ValidateTrajectory checks structural invariants. It never "proves" a human —
// failures just raise risk / can hard-fail when Config demands it.
func ValidateTrajectory(tr Trajectory, maxJumpPx float64) TrajectoryIssues {
	var iss TrajectoryIssues
	if maxJumpPx <= 0 {
		maxJumpPx = 800
	}
	pts := tr.Points
	if len(pts) < 3 {
		iss.TooFewPoints = true
		iss.add("too_few_points")
		return iss
	}

	if !hasEventOrder(tr.Events) {
		iss.BadEventOrder = true
		iss.add("bad_event_order")
	}

	hasMeta := false
	coalescedSum := 0
	for i := 0; i < len(pts); i++ {
		if i > 0 {
			if pts[i].T < pts[i-1].T {
				iss.NonMonotonicTime = true
				iss.add("non_monotonic_time")
			}
			dx := pts[i].X - pts[i-1].X
			dy := pts[i].Y - pts[i-1].Y
			if math.Hypot(dx, dy) > maxJumpPx {
				iss.HugeJump = true
				iss.add("huge_jump")
			}
		}
		if pts[i].PointerType != "" || pts[i].Buttons != 0 || pts[i].Pressure > 0 {
			hasMeta = true
		}
		coalescedSum += pts[i].Coalesced
	}
	if !hasMeta {
		iss.MissingPointerMeta = true
		iss.add("missing_pointer_meta")
	}
	if len(pts) >= 20 && coalescedSum == 0 {
		iss.NoCoalesced = true
		iss.add("no_coalesced_events")
	}
	return iss
}

func hasEventOrder(events []string) bool {
	if len(events) == 0 {
		return false
	}
	seenDown, seenMove, seenUp := false, false, false
	upAfterDown := false
	for _, e := range events {
		e = strings.ToLower(e)
		switch e {
		case "pointerdown", "mousedown", "touchstart":
			seenDown = true
		case "pointermove", "mousemove", "touchmove":
			if seenDown {
				seenMove = true
			}
		case "pointerup", "mouseup", "touchend", "pointercancel", "touchcancel":
			if seenDown {
				seenUp = true
				upAfterDown = true
			}
		}
	}
	return seenDown && seenMove && seenUp && upAfterDown
}

// FinalPointNear reports whether the last trajectory point is within pad px of (x,y).
func FinalPointNear(tr Trajectory, x, y, pad float64) bool {
	if len(tr.Points) == 0 {
		return false
	}
	p := tr.Points[len(tr.Points)-1]
	return math.Hypot(p.X-x, p.Y-y) <= pad
}

// ScoreContext carries server-side facts the scorer may use.
type ScoreContext struct {
	ElapsedMs int64
	Issues    TrajectoryIssues
}

// ScoreResult holds the behavior score and diagnostics.
type ScoreResult struct {
	Score      float64            `json:"score"`
	Components map[string]float64 `json:"components,omitempty"`
	Consistent bool               `json:"consistent"`
	Issues     TrajectoryIssues   `json:"issues,omitempty"`
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
	PointerMeta  float64
}

// DefaultWeights are a starting point; calibrate on your own traffic.
func DefaultWeights() ScoreWeights {
	return ScoreWeights{
		Points: 0.12, Duration: 0.18, Velocity: 0.18, Acceleration: 0.12,
		Timing: 0.12, Corrections: 0.08, Events: 0.10, PointerMeta: 0.10,
	}
}

func (w ScoreWeights) sum() float64 {
	return w.Points + w.Duration + w.Velocity + w.Acceleration + w.Timing + w.Corrections + w.Events + w.PointerMeta
}

// HeuristicScorer is the default Scorer.
type HeuristicScorer struct{ Weights ScoreWeights }

// ScoreBehavior scores with DefaultWeights and no server context.
func ScoreBehavior(tr Trajectory) ScoreResult {
	iss := ValidateTrajectory(tr, 800)
	return HeuristicScorer{Weights: DefaultWeights()}.Score(tr, ScoreContext{Issues: iss})
}

// Score implements Scorer.
func (h HeuristicScorer) Score(tr Trajectory, sc ScoreContext) ScoreResult {
	w := h.Weights
	if w.sum() <= 0 {
		w = DefaultWeights()
	}
	comp := map[string]float64{}
	pts := tr.Points
	res := ScoreResult{Components: comp, Consistent: true, Issues: sc.Issues}

	if sc.Issues.TooFewPoints || len(pts) < 3 {
		comp["points"] = 0
		res.Score = 0.05
		res.Consistent = false
		return res
	}
	comp["points"] = clamp01(float64(len(pts)) / 40)

	duration := float64(tr.DurationMs())
	if duration <= 0 || sc.Issues.NonMonotonicTime {
		comp["duration"] = 0
		res.Consistent = false
	} else {
		comp["duration"] = durationScore(duration)
	}
	if sc.ElapsedMs > 0 && duration > float64(sc.ElapsedMs)+100 {
		res.Consistent = false
	}

	vels, accs, gaps, corrections := motionStats(pts)
	comp["velocity"] = varianceScore(vels, 0.02, 8)
	comp["acceleration"] = varianceScore(accs, 0.01, 20)
	comp["timing"] = varianceScore(gaps, 2, 120)
	comp["corrections"] = clamp01(float64(corrections) / 4)
	comp["events"] = eventScore(tr.Events)
	comp["pointer_meta"] = pointerMetaScore(pts, sc.Issues)

	score := (w.Points*comp["points"] + w.Duration*comp["duration"] + w.Velocity*comp["velocity"] +
		w.Acceleration*comp["acceleration"] + w.Timing*comp["timing"] + w.Corrections*comp["corrections"] +
		w.Events*comp["events"] + w.PointerMeta*comp["pointer_meta"]) / w.sum()

	if !res.Consistent || sc.Issues.BadEventOrder || sc.Issues.HugeJump {
		score *= 0.4
	}
	res.Score = clamp01(score)
	return res
}

func pointerMetaScore(pts []Point, iss TrajectoryIssues) float64 {
	if iss.MissingPointerMeta {
		return 0.15
	}
	types, withPressure, withCoalesced := map[string]bool{}, 0, 0
	for _, p := range pts {
		if p.PointerType != "" {
			types[p.PointerType] = true
		}
		if p.Pressure > 0 {
			withPressure++
		}
		if p.Coalesced > 0 {
			withCoalesced++
		}
	}
	s := 0.5
	if len(types) == 1 {
		s += 0.25
	}
	if withPressure > len(pts)/4 {
		s += 0.15
	}
	if withCoalesced > 0 {
		s += 0.1
	} else if iss.NoCoalesced {
		s -= 0.15
	}
	return clamp01(s)
}

func eventScore(events []string) float64 {
	if !hasEventOrder(events) {
		return 0.15
	}
	return 1
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
		vx, vy := dx/dt, dy/dt
		speed := math.Hypot(vx, vy)
		vels = append(vels, speed)
		if hasPrevV {
			ax := (vx - prevVx) / dt
			ay := (vy - prevVy) / dt
			accs = append(accs, math.Hypot(ax, ay))
			if prevVx*vx+prevVy*vy < 0 && speed > 0.01 && math.Hypot(prevVx, prevVy) > 0.01 {
				corrections++
			}
		}
		prevVx, prevVy, hasPrevV = vx, vy, true
	}
	return
}

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

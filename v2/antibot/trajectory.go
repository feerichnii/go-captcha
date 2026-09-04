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
type Trajectory struct {
	Points []Point  `json:"points"`
	Events []string `json:"events,omitempty"` // pointerdown, pointermove, pointerup, ...
}

// ScoreResult holds the behavior score and diagnostics.
type ScoreResult struct {
	Score      float64            `json:"score"`
	Components map[string]float64 `json:"components,omitempty"`
}

// ScoreBehavior evaluates how human-like a trajectory looks. Result is in [0,1].
func ScoreBehavior(tr Trajectory) ScoreResult {
	comp := map[string]float64{}
	pts := tr.Points

	if len(pts) < 3 {
		comp["points"] = 0
		return ScoreResult{Score: 0.05, Components: comp}
	}
	comp["points"] = clamp01(float64(len(pts)) / 40)

	duration := float64(pts[len(pts)-1].T - pts[0].T)
	if duration <= 0 {
		comp["duration"] = 0
		return ScoreResult{Score: 0.05, Components: comp}
	}
	// Humans typically take ~300ms–15s for interactive captchas.
	comp["duration"] = durationScore(duration)

	vels, accs, gaps, corrections := motionStats(pts)
	comp["velocity"] = varianceScore(vels, 0.02, 8)
	comp["acceleration"] = varianceScore(accs, 0.01, 20)
	comp["timing"] = varianceScore(gaps, 2, 120)
	comp["corrections"] = clamp01(float64(corrections) / 4)

	eventScore := 0.3
	if len(tr.Events) > 0 {
		eventScore = 0.7
		hasDown, hasUp, hasMove := false, false, false
		for _, e := range tr.Events {
			switch e {
			case "pointerdown", "mousedown", "touchstart":
				hasDown = true
			case "pointerup", "mouseup", "touchend":
				hasUp = true
			case "pointermove", "mousemove", "touchmove":
				hasMove = true
			}
		}
		if hasDown && hasUp && hasMove {
			eventScore = 1
		} else if hasMove {
			eventScore = 0.75
		}
	}
	comp["events"] = eventScore

	// Weighted average.
	score := 0.15*comp["points"] +
		0.2*comp["duration"] +
		0.2*comp["velocity"] +
		0.15*comp["acceleration"] +
		0.15*comp["timing"] +
		0.1*comp["corrections"] +
		0.05*comp["events"]

	return ScoreResult{Score: clamp01(score), Components: comp}
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
			// Direction reversal / sharp correction.
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

// varianceScore rewards some natural variance; near-zero or huge variance looks bot-like.
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
	// Perfect bots often have near-zero variance.
	if variance < lowGood*lowGood*0.01 {
		return 0.15
	}
	if variance >= lowGood && variance <= highGood {
		return 1
	}
	if variance < lowGood {
		return clamp01(variance / lowGood)
	}
	// Too chaotic.
	return clamp01(highGood / variance)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

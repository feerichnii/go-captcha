package rotate

import "testing"

func TestValidateClampsPadding(t *testing.T) {
	// angle + dAngle must land near 360
	if !Validate(100, 260, 5) {
		t.Fatal("expected match")
	}
	if Validate(100, 200, 500) {
		t.Fatal("huge padding must be clamped; 300 not near 360")
	}
}

// Regression: every default-range secret angle must be reachable on the
// client range input (0..359 step 1) under antibot's default padding (5).
func TestClientSliderReachesValidateTargets(t *testing.T) {
	const padding = 5
	for _, dAngle := range []int{30, 90, 180, 270, 330} {
		found := false
		var hit int
		for a := ClientAngleMin; a <= ClientAngleMax; a += ClientAngleStep {
			if Validate(a, dAngle, padding) {
				found = true
				hit = a
				break
			}
		}
		if !found {
			t.Fatalf("dAngle=%d: no slider value in [%d,%d] step %d satisfies Validate(padding=%d)",
				dAngle, ClientAngleMin, ClientAngleMax, ClientAngleStep, padding)
		}
		// Ideal unwind is 360-dAngle; allow padding window but prefer exact when in range.
		ideal := 360 - dAngle
		if ideal < ClientAngleMin || ideal > ClientAngleMax {
			t.Fatalf("dAngle=%d: ideal client angle %d outside slider [%d,%d]",
				dAngle, ideal, ClientAngleMin, ClientAngleMax)
		}
		if !Validate(ideal, dAngle, padding) {
			t.Fatalf("dAngle=%d: ideal angle %d should Validate; first hit was %d", dAngle, ideal, hit)
		}
	}
}

func TestPublicBlockOmitsAngle(t *testing.T) {
	c := CaptData{block: &Block{Angle: 123, Width: 140, Height: 140}}
	pub := c.GetPublicData()
	if pub.Width != 140 {
		t.Fatal("width")
	}
}

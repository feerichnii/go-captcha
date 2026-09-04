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

func TestPublicBlockOmitsAngle(t *testing.T) {
	c := CaptData{block: &Block{Angle: 123, Width: 140, Height: 140}}
	pub := c.GetPublicData()
	if pub.Width != 140 {
		t.Fatal("width")
	}
}

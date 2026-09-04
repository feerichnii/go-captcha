package slide

import "testing"

func TestValidateClampsPadding(t *testing.T) {
	if Validate(100, 100, 10, 10, 500) {
		t.Fatal("padding should be clamped")
	}
	if !Validate(12, 12, 10, 10, 5) {
		t.Fatal("expected near match")
	}
}

func TestPublicBlockOmitsTarget(t *testing.T) {
	c := CaptData{block: &Block{X: 99, Y: 88, Width: 60, Height: 60, DX: 5, DY: 10}}
	pub := c.GetPublicData()
	if pub.DX != 5 || pub.DY != 10 || pub.Width != 60 {
		t.Fatalf("%+v", pub)
	}
}

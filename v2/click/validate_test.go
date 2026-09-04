package click

import "testing"

func TestValidateOrdered(t *testing.T) {
	dots := map[int]*Dot{
		0: {Index: 0, X: 10, Y: 10, Width: 20, Height: 20},
		1: {Index: 1, X: 50, Y: 40, Width: 20, Height: 20},
	}
	ok := ValidateOrdered([]Point{{X: 15, Y: 15}, {X: 55, Y: 45}}, dots, 2)
	if !ok {
		t.Fatal("expected ordered match")
	}
	bad := ValidateOrdered([]Point{{X: 55, Y: 45}, {X: 15, Y: 15}}, dots, 2)
	if bad {
		t.Fatal("expected order fail")
	}
}

func TestValidateClampsPadding(t *testing.T) {
	// Far outside even with huge requested padding should fail after clamp.
	if Validate(200, 200, 10, 10, 10, 10, 500) {
		t.Fatal("padding should be clamped")
	}
}

func TestPublicDotOmitsSecrets(t *testing.T) {
	c := CaptData{dots: map[int]*Dot{0: {Index: 0, X: 9, Y: 8, Text: "secret"}}}
	pub := c.GetPublicData()
	if pub[0].Index != 0 {
		t.Fatal("index")
	}
}

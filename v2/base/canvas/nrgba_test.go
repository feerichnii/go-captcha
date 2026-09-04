package canvas

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func TestNewNRGBAFill(t *testing.T) {
	a := NewNRGBA(image.Rect(0, 0, 7, 5), true)
	for i, v := range a.Get().Pix {
		if v != 0 {
			t.Fatalf("alpha canvas not transparent at %d", i)
		}
	}
	w := NewNRGBA(image.Rect(0, 0, 7, 5), false)
	for i, v := range w.Get().Pix {
		if v != 0xFF {
			t.Fatalf("opaque canvas not white at %d", i)
		}
	}
}

// circleMask must match the reference per-pixel Hypot definition exactly.
func TestCircleMaskMatchesReference(t *testing.T) {
	for _, tc := range []struct{ w, h, cx, cy, r int }{
		{50, 50, 25, 25, 25}, {51, 41, 25, 20, 20}, {30, 30, 5, 5, 12}, {30, 30, 15, 15, 0},
	} {
		b := image.Rect(0, 0, tc.w, tc.h)
		got := circleMask(b, tc.cx, tc.cy, tc.r)
		for y := 0; y < tc.h; y++ {
			for x := 0; x < tc.w; x++ {
				in := math.Hypot(float64(x-tc.cx), float64(y-tc.cy)) <= float64(tc.r) && tc.r > 0
				a := got.NRGBAAt(x, y).A
				if in && a != 0xFF || !in && a != 0 {
					t.Fatalf("%+v mismatch at (%d,%d): in=%v a=%d", tc, x, y, in, a)
				}
			}
		}
	}
}

func TestCalcMarginBlankArea(t *testing.T) {
	c := NewNRGBA(image.Rect(0, 0, 40, 30), true)
	c.Get().SetNRGBA(10, 12, color.NRGBA{A: 255})
	c.Get().SetNRGBA(20, 18, color.NRGBA{A: 255})
	ar := c.CalcMarginBlankArea()
	if ar.MinX != 8 || ar.MaxX != 22 || ar.MinY != 10 || ar.MaxY != 20 {
		t.Fatalf("%+v", ar)
	}
}

func BenchmarkNewNRGBAAlpha(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NewNRGBA(image.Rect(0, 0, 300, 220), true)
	}
}

func BenchmarkCropCircle(b *testing.B) {
	for i := 0; i < b.N; i++ {
		c := NewNRGBA(image.Rect(0, 0, 300, 300), false)
		c.CropCircle(150, 150, 150)
	}
}

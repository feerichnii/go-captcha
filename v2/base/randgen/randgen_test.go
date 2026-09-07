package randgen

import (
	"image"
	"image/color"
	"testing"
)

func TestRangCutImagePosTexturedPrefersBusyRegion(t *testing.T) {
	// Left half flat, right half striped — crop should prefer the right.
	const W, H = 200, 120
	const cw, ch = 80, 60
	img := image.NewNRGBA(image.Rect(0, 0, W, H))
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			if x < W/2 {
				img.SetNRGBA(x, y, color.NRGBA{R: 120, G: 160, B: 220, A: 255})
			} else {
				v := uint8((x * 37 + y*19) % 256)
				img.SetNRGBA(x, y, color.NRGBA{R: v, G: 255 - v, B: uint8((x + y) % 256), A: 255})
			}
		}
	}

	flat := cropTextureScore(img, 0, 0, cw, ch)
	busy := cropTextureScore(img, W-cw, 0, cw, ch)
	if !(busy > flat*100 && busy > 1e-6) {
		t.Fatalf("busy score %v should exceed flat %v", busy, flat)
	}

	hitsBusy := 0
	for i := 0; i < 20; i++ {
		p := RangCutImagePosTextured(cw, ch, img, 24)
		if p.X >= W/2-cw/4 {
			hitsBusy++
		}
	}
	if hitsBusy < 15 {
		t.Fatalf("expected textured crop to prefer busy half, hits=%d/20", hitsBusy)
	}
}

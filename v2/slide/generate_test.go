package slide

import (
	"image"
	"image/color"
	"testing"
)

func solid(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

// texturedBG has spatial variety so slot placement / features are meaningful.
func texturedBG(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(40 + (x*3+y*5)%180),
				G: uint8(60 + (x*7+y*2)%160),
				B: uint8(80 + (x*2+y*11)%140),
				A: 255,
			})
		}
	}
	return img
}

func shapeGraph(r, g, b, a uint8) *GraphImage {
	c := color.NRGBA{R: r, G: g, B: b, A: a}
	img := solid(64, 64, c)
	return &GraphImage{OverlayImage: img, ShadowImage: img, MaskImage: img}
}

func testSlideCaptcha(t *testing.T, graphs []*GraphImage) Captcha {
	t.Helper()
	builder := NewBuilder()
	builder.SetResources(WithGraphImages(graphs), WithBackgrounds([]image.Image{texturedBG(300, 220)}))
	return builder.Make()
}

func TestDefaultAutoSlotCount(t *testing.T) {
	opts := NewOptions()
	defaultOptions()(opts)
	if opts.GetGenGraphNumber() != 0 {
		t.Fatalf("default slots want 0 (auto), got %d", opts.GetGenGraphNumber())
	}
	if opts.GetCandidateSlotsMin() != 4 || opts.GetCandidateSlotsMax() != 4 {
		t.Fatalf("auto range want 4–4 (1 real + 3 decoy), got %d–%d",
			opts.GetCandidateSlotsMin(), opts.GetCandidateSlotsMax())
	}
	d := opts.GetTileDistort()
	if d.ScaleDelta != 0 || d.WarpPxMax != 0 || d.SoftBlurProb != 0 {
		t.Fatalf("default distort must be photometric-only, got %+v", d)
	}
}

func TestGenerateThreeSlotsOneCorrect(t *testing.T) {
	capt := testSlideCaptcha(t, []*GraphImage{
		shapeGraph(255, 0, 0, 200),
		shapeGraph(0, 255, 0, 200),
		shapeGraph(0, 0, 255, 200),
	})
	data, err := capt.Generate()
	if err != nil {
		t.Fatal(err)
	}
	secret := data.GetData()
	pub := data.GetPublicData()
	if secret == nil || pub == nil {
		t.Fatal("nil data")
	}
	if secret.X == 0 && secret.Y == 0 {
		t.Fatal("expected non-zero target")
	}
	if pub.Width != secret.Width || pub.Height != secret.Height {
		t.Fatalf("size mismatch pub=%+v secret=%+v", pub, secret)
	}
	if data.GetMasterImage() == nil || data.GetTileImage() == nil {
		t.Fatal("missing images")
	}
	if !Validate(secret.X, secret.Y, secret.X, secret.Y, 5) {
		t.Fatal("correct position must validate")
	}
	if Validate(secret.X+80, secret.Y, secret.X, secret.Y, 5) {
		t.Fatal("offset decoy-like position must fail")
	}
}

func TestPickSlotGraphsIdenticalSilhouette(t *testing.T) {
	c := &captcha{resources: NewResources()}
	g0 := shapeGraph(1, 0, 0, 255)
	g1 := shapeGraph(0, 1, 0, 255)
	g2 := shapeGraph(0, 0, 1, 255)
	c.resources.rangGraphImage = []*GraphImage{g0, g1, g2}

	correctIdx := 1
	got := c.pickSlotGraphs(3, correctIdx)
	if got == nil || len(got) != 3 {
		t.Fatalf("%v", got)
	}
	if got[correctIdx] == nil {
		t.Fatal("correct slot empty")
	}
	for i, g := range got {
		if g != got[correctIdx] {
			t.Fatalf("slot %d shape differs from correct slot", i)
		}
	}
}

func TestWithGenGraphNumberAutoZero(t *testing.T) {
	opts := NewOptions()
	WithGenGraphNumber(0)(opts)
	if opts.GetGenGraphNumber() != 0 {
		t.Fatalf("got %d", opts.GetGenGraphNumber())
	}
}

func TestGenerateSlotCountInRange(t *testing.T) {
	capt := testSlideCaptcha(t, []*GraphImage{
		shapeGraph(255, 0, 0, 200),
		shapeGraph(0, 255, 0, 200),
		shapeGraph(0, 0, 255, 200),
	})
	for i := 0; i < 20; i++ {
		data, err := capt.Generate()
		if err != nil {
			t.Fatal(err)
		}
		n := data.GetSlotCount()
		if n != 4 {
			t.Fatalf("default slot count want 4 (1+3), got %d", n)
		}
	}
}

func TestExactRealHoleMatchesTileCrop(t *testing.T) {
	c := &captcha{opts: NewOptions(), drawImage: NewDrawImage(), resources: NewResources()}
	defaultOptions()(c.opts)
	bg := texturedBG(300, 220)
	mask := solid(64, 64, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	overlay := solid(64, 64, color.NRGBA{R: 0, G: 0, B: 0, A: 0})
	block := &Block{X: 120, Y: 80, Width: 64, Height: 64}

	tile, err := c.genTileImage(mask, bg, overlay, block)
	if err != nil {
		t.Fatal(err)
	}
	// Without photometric distort, masked opaque pixels must equal the bg crop.
	tb := tile.Bounds()
	mismatch := 0
	checked := 0
	for y := 0; y < block.Height; y++ {
		for x := 0; x < block.Width; x++ {
			tc := color.NRGBAModel.Convert(tile.At(tb.Min.X+x, tb.Min.Y+y)).(color.NRGBA)
			if tc.A < 200 {
				continue
			}
			bc := bg.NRGBAAt(block.X+x, block.Y+y)
			checked++
			if abs8(int(tc.R)-int(bc.R)) > 2 || abs8(int(tc.G)-int(bc.G)) > 2 || abs8(int(tc.B)-int(bc.B)) > 2 {
				mismatch++
			}
		}
	}
	if checked < 100 {
		t.Fatalf("too few opaque tile pixels: %d", checked)
	}
	if mismatch > checked/50 {
		t.Fatalf("tile crop must match hole coordinates, mismatch=%d/%d", mismatch, checked)
	}
}

func TestSlotsShareSameYAsTile(t *testing.T) {
	c := &captcha{opts: NewOptions(), resources: NewResources()}
	defaultOptions()(c.opts)
	bg := texturedBG(300, 220)
	for i := 0; i < 30; i++ {
		blocks, tilePoint := c.genGraphBlocksScattered(bg, c.opts.imageSize, c.opts.rangeGraphSize, 4)
		if len(blocks) < 2 {
			t.Fatalf("too few blocks: %d", len(blocks))
		}
		y0 := blocks[0].Y
		if tilePoint.Y != y0 {
			t.Fatalf("tile start Y=%d != hole Y=%d", tilePoint.Y, y0)
		}
		for j, b := range blocks {
			if b.Y != y0 {
				t.Fatalf("hole %d Y=%d want shared Y=%d", j, b.Y, y0)
			}
		}
		// X positions must differ (not stacked).
		xs := map[int]bool{}
		for _, b := range blocks {
			xs[b.X] = true
		}
		if len(xs) < 2 {
			t.Fatal("expected distinct X positions on the shared row")
		}
	}
}

func TestTileNotExactCrop(t *testing.T) {
	src := solid(64, 64, color.NRGBA{R: 40, G: 80, B: 120, A: 255})
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			if (x/8+y/8)%2 == 0 {
				src.SetNRGBA(x, y, color.NRGBA{R: 200, G: 40, B: 40, A: 255})
			}
		}
	}
	out := DistortTile(src)
	if out == nil {
		t.Fatal("nil distort")
	}
	ob := out.Bounds()
	diff := 0
	var sumAbs float64
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			a := src.NRGBAAt(x, y)
			b := color.NRGBAModel.Convert(out.At(ob.Min.X+x, ob.Min.Y+y)).(color.NRGBA)
			if a != b {
				diff++
			}
			sumAbs += float64(abs8(int(a.R)-int(b.R)) + abs8(int(a.G)-int(b.G)) + abs8(int(a.B)-int(b.B)))
		}
	}
	if diff < 20 {
		t.Fatalf("photometric noise/gamma must break pixel-perfect match, changed=%d", diff)
	}
	meanAbs := sumAbs / float64(64*64*3)
	if meanAbs > 20 {
		t.Fatalf("photometric distort too strong (mean abs=%.1f)", meanAbs)
	}
}

func abs8(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func TestBasicTargetsReachableBySlider(t *testing.T) {
	capt := testSlideCaptcha(t, []*GraphImage{
		shapeGraph(255, 0, 0, 200),
		shapeGraph(0, 255, 0, 200),
		shapeGraph(0, 0, 255, 200),
	})
	const masterW = 300
	for i := 0; i < 80; i++ {
		data, err := capt.Generate()
		if err != nil {
			t.Fatal(err)
		}
		blk := data.GetData()
		maxX := masterW - blk.Width
		if blk.X < 0 || blk.X > maxX {
			t.Fatalf("target X=%d outside slider reach [0,%d] (W=%d)", blk.X, maxX, blk.Width)
		}
		if blk.X+blk.Width > masterW {
			t.Fatalf("notch spills past master: X=%d W=%d", blk.X, blk.Width)
		}
	}
}

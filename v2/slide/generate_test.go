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
	if opts.GetCandidateSlotsMin() != 4 || opts.GetCandidateSlotsMax() != 5 {
		t.Fatalf("auto range want 4–5, got %d–%d", opts.GetCandidateSlotsMin(), opts.GetCandidateSlotsMax())
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
	seen := map[int]bool{}
	for i := 0; i < 40; i++ {
		data, err := capt.Generate()
		if err != nil {
			t.Fatal(err)
		}
		n := data.GetSlotCount()
		if n < 4 || n > 5 {
			t.Fatalf("slot count %d outside [4,5]", n)
		}
		seen[n] = true
	}
	if len(seen) < 2 {
		t.Fatalf("expected variety in auto slot counts, got %v", seen)
	}
}

func TestSlotsNotFenceLine(t *testing.T) {
	c := &captcha{opts: NewOptions(), resources: NewResources()}
	defaultOptions()(c.opts)
	bg := texturedBG(300, 220)
	var multiY int
	for i := 0; i < 30; i++ {
		blocks, _ := c.genGraphBlocksScattered(bg, c.opts.imageSize, c.opts.rangeGraphSize, 5)
		if len(blocks) < 3 {
			t.Fatalf("too few blocks: %d", len(blocks))
		}
		ys := map[int]bool{}
		for _, b := range blocks {
			ys[b.Y] = true
		}
		if len(ys) > 1 {
			multiY++
		}
		// Min separation
		minSep := minSlotSeparation(c.opts, blocks[0].Width)
		for i := 0; i < len(blocks); i++ {
			for j := i + 1; j < len(blocks); j++ {
				dx := blocks[i].X - blocks[j].X
				dy := blocks[i].Y - blocks[j].Y
				if dx*dx+dy*dy < (minSep*minSep)/4 {
					// allow some relax cases but centers shouldn't coincide
					if dx == 0 && dy == 0 {
						t.Fatal("duplicate slot positions")
					}
				}
			}
		}
	}
	if multiY < 20 {
		t.Fatalf("expected scattered Y (not fence); multiY=%d/30", multiY)
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
	if diff < 50 {
		t.Fatalf("tile must not be pixel-perfect crop, changed=%d", diff)
	}
	meanAbs := sumAbs / float64(64*64*3)
	// Mild transforms: average channel drift should stay small for humans.
	if meanAbs > 35 {
		t.Fatalf("distort too strong for UX (mean abs channel delta=%.1f)", meanAbs)
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

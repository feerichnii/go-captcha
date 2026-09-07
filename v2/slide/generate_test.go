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

func shapeGraph(r, g, b, a uint8) *GraphImage {
	c := color.NRGBA{R: r, G: g, B: b, A: a}
	img := solid(64, 64, c)
	return &GraphImage{OverlayImage: img, ShadowImage: img, MaskImage: img}
}

func testSlideCaptcha(t *testing.T, graphs []*GraphImage) Captcha {
	t.Helper()
	bg := solid(300, 220, color.NRGBA{R: 200, G: 200, B: 200, A: 255})
	builder := NewBuilder()
	builder.SetResources(WithGraphImages(graphs), WithBackgrounds([]image.Image{bg}))
	return builder.Make()
}

func TestDefaultAutoSlotCount(t *testing.T) {
	opts := NewOptions()
	defaultOptions()(opts)
	if opts.GetGenGraphNumber() != 0 {
		t.Fatalf("default slots want 0 (auto 4–7), got %d", opts.GetGenGraphNumber())
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
	// Public must not mirror secret target as tile start in a leaky way that
	// equals the answer for ModeBasic — DX is tile start, X is target.
	if pub.Width != secret.Width || pub.Height != secret.Height {
		t.Fatalf("size mismatch pub=%+v secret=%+v", pub, secret)
	}
	if data.GetMasterImage() == nil || data.GetTileImage() == nil {
		t.Fatal("missing images")
	}
	// Correct submit passes; far-away decoy position fails.
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
	// All slots must share one silhouette so matching is by image content.
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
		if n < 4 || n > 7 {
			t.Fatalf("slot count %d outside [4,7]", n)
		}
		seen[n] = true
	}
	if len(seen) < 2 {
		t.Fatalf("expected some variety in auto slot counts, got %v", seen)
	}
}

func TestTileNotExactCrop(t *testing.T) {
	src := solid(64, 64, color.NRGBA{R: 40, G: 80, B: 120, A: 255})
	// Checker so warp/noise has structure to change.
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
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			a := src.NRGBAAt(x, y)
			b := color.NRGBAModel.Convert(out.At(ob.Min.X+x, ob.Min.Y+y)).(color.NRGBA)
			if a != b {
				diff++
			}
		}
	}
	if diff < 100 {
		t.Fatalf("expected distorted tile to differ substantially, changed=%d", diff)
	}
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

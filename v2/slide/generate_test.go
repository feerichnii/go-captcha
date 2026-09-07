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

func TestDefaultThreeDropSlots(t *testing.T) {
	opts := NewOptions()
	defaultOptions()(opts)
	if opts.GetGenGraphNumber() != 3 {
		t.Fatalf("default slots want 3, got %d", opts.GetGenGraphNumber())
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

func TestPickSlotGraphsPreferDistinctDecoys(t *testing.T) {
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
	// Decoys should not all be the correct graph when pool has alternatives.
	sameAsCorrect := 0
	for i, g := range got {
		if i == correctIdx {
			continue
		}
		if g == got[correctIdx] {
			sameAsCorrect++
		}
	}
	if sameAsCorrect == 2 {
		t.Fatal("expected at least one decoy with a different graph shape")
	}
}

func TestWithGenGraphNumberMinOne(t *testing.T) {
	opts := NewOptions()
	WithGenGraphNumber(0)(opts)
	if opts.GetGenGraphNumber() != 1 {
		t.Fatalf("got %d", opts.GetGenGraphNumber())
	}
}

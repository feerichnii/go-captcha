package slide

import (
	"image"
	"image/color"
	"math"

	"github.com/feerichnii/go-captcha/v2/base/random"
	"golang.org/x/image/draw"
)

// DistortTile applies default mild post-crop transforms.
func DistortTile(src image.Image) image.Image {
	return DistortTileWith(src, defaultTileDistort())
}

// DistortTileWith applies configurable mild transforms so the public tile is
// not a pixel-perfect crop, while remaining a clear visual hint for humans.
func DistortTileWith(src image.Image, cfg TileDistortConfig) image.Image {
	if src == nil {
		return nil
	}
	cfg = normalizeTileDistort(cfg)
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 2 || h < 2 {
		return cloneNRGBA(src)
	}

	out := cloneNRGBA(src)

	if cfg.ScaleDelta > 0 {
		sx := 1.0 + randSignedFloat(cfg.ScaleDelta)
		sy := 1.0 + randSignedFloat(cfg.ScaleDelta)
		sw := int(math.Max(2, math.Round(float64(w)*sx)))
		sh := int(math.Max(2, math.Round(float64(h)*sy)))
		scaled := image.NewNRGBA(image.Rect(0, 0, sw, sh))
		draw.BiLinear.Scale(scaled, scaled.Bounds(), out, out.Bounds(), draw.Src, nil)
		tmp := image.NewNRGBA(image.Rect(0, 0, w, h))
		draw.BiLinear.Scale(tmp, tmp.Bounds(), scaled, scaled.Bounds(), draw.Src, nil)
		out = tmp
	}

	amp := cfg.WarpPxMin
	if cfg.WarpPxMax > cfg.WarpPxMin {
		amp = cfg.WarpPxMin + randFloat()*(cfg.WarpPxMax-cfg.WarpPxMin)
	}
	if amp > 0 {
		period := 8.0 + float64(random.RandInt(0, 6))
		out = warpNRGBA(out, amp, period)
	}

	gamma := cfg.GammaMin
	if cfg.GammaMax > cfg.GammaMin {
		gamma = cfg.GammaMin + randFloat()*(cfg.GammaMax-cfg.GammaMin)
	}
	bright := 0
	if cfg.BrightnessDelta > 0 {
		bright = random.RandInt(-cfg.BrightnessDelta, cfg.BrightnessDelta)
	}
	applyGammaBrightnessNoise(out, gamma, bright, cfg.NoiseAmt)

	blurRoll := randFloat() < cfg.SoftBlurProb
	sharpRoll := randFloat() < cfg.SoftSharpenProb
	switch {
	case blurRoll && !sharpRoll:
		out = softBlur(out)
	case sharpRoll && !blurRoll:
		out = softSharpen(out)
	}

	return out
}

func normalizeTileDistort(cfg TileDistortConfig) TileDistortConfig {
	d := defaultTileDistort()
	if cfg.ScaleDelta <= 0 && cfg.WarpPxMax <= 0 && cfg.GammaMax <= 0 {
		return d
	}
	if cfg.ScaleDelta > 0 {
		d.ScaleDelta = cfg.ScaleDelta
	}
	if cfg.WarpPxMax > 0 || cfg.WarpPxMin > 0 {
		d.WarpPxMin, d.WarpPxMax = cfg.WarpPxMin, cfg.WarpPxMax
		if d.WarpPxMin <= 0 {
			d.WarpPxMin = 1
		}
		if d.WarpPxMax < d.WarpPxMin {
			d.WarpPxMax = d.WarpPxMin
		}
	}
	if cfg.GammaMax > 0 || cfg.GammaMin > 0 {
		d.GammaMin, d.GammaMax = cfg.GammaMin, cfg.GammaMax
		if d.GammaMin <= 0 {
			d.GammaMin = 0.95
		}
		if d.GammaMax < d.GammaMin {
			d.GammaMax = d.GammaMin
		}
	}
	d.BrightnessDelta = cfg.BrightnessDelta
	if d.BrightnessDelta < 0 {
		d.BrightnessDelta = defaultTileDistort().BrightnessDelta
	}
	d.NoiseAmt = cfg.NoiseAmt
	if d.NoiseAmt < 0 {
		d.NoiseAmt = defaultTileDistort().NoiseAmt
	}
	if cfg.SoftBlurProb >= 0 {
		d.SoftBlurProb = cfg.SoftBlurProb
	}
	if cfg.SoftSharpenProb >= 0 {
		d.SoftSharpenProb = cfg.SoftSharpenProb
	}
	return d
}

func randFloat() float64 {
	return float64(random.RandInt(0, 10000)) / 10000.0
}

func randSignedFloat(maxAbs float64) float64 {
	if maxAbs <= 0 {
		return 0
	}
	return (randFloat()*2 - 1) * maxAbs
}

func cloneNRGBA(src image.Image) *image.NRGBA {
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}

func warpNRGBA(src *image.NRGBA, amplitude, period float64) *image.NRGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	dx := 2.0 * math.Pi / math.Max(1, period)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			xo := amplitude * math.Sin(float64(y)*dx)
			yo := amplitude * math.Cos(float64(x)*dx)
			sx := x + int(math.Round(xo))
			sy := y + int(math.Round(yo))
			if sx < 0 || sy < 0 || sx >= w || sy >= h {
				continue
			}
			dst.SetNRGBA(x, y, src.NRGBAAt(sx, sy))
		}
	}
	return dst
}

func applyGammaBrightnessNoise(img *image.NRGBA, gamma float64, brightness, noiseAmt int) {
	b := img.Bounds()
	inv := 1.0 / math.Max(0.01, gamma)
	var noise []byte
	if noiseAmt > 0 {
		noise = random.FastBytes(b.Dx() * b.Dy() * 3)
	}
	ni := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.NRGBAAt(x, y)
			if c.A == 0 {
				if noiseAmt > 0 {
					ni += 3
				}
				continue
			}
			r := gammaByte(c.R, inv) + brightness
			g := gammaByte(c.G, inv) + brightness
			bl := gammaByte(c.B, inv) + brightness
			if noiseAmt > 0 {
				r += noiseDelta(noise[ni], noiseAmt)
				g += noiseDelta(noise[ni+1], noiseAmt)
				bl += noiseDelta(noise[ni+2], noiseAmt)
				ni += 3
			}
			img.SetNRGBA(x, y, color.NRGBA{
				R: clampU8(r), G: clampU8(g), B: clampU8(bl), A: c.A,
			})
		}
	}
}

func gammaByte(v uint8, invGamma float64) int {
	f := math.Pow(float64(v)/255.0, invGamma) * 255.0
	return int(math.Round(f))
}

func noiseDelta(b byte, amt int) int {
	if amt <= 0 {
		return 0
	}
	return int(b)%(2*amt+1) - amt
}

func clampU8(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

func softBlur(src *image.NRGBA) *image.NRGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	tmp := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c0 := src.NRGBAAt(max0(x-1), y)
			c1 := src.NRGBAAt(x, y)
			c2 := src.NRGBAAt(min2(x+1, w-1), y)
			tmp.SetNRGBA(x, y, color.NRGBA{
				R: uint8((int(c0.R) + 2*int(c1.R) + int(c2.R)) / 4),
				G: uint8((int(c0.G) + 2*int(c1.G) + int(c2.G)) / 4),
				B: uint8((int(c0.B) + 2*int(c1.B) + int(c2.B)) / 4),
				A: c1.A,
			})
		}
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c0 := tmp.NRGBAAt(x, max0(y-1))
			c1 := tmp.NRGBAAt(x, y)
			c2 := tmp.NRGBAAt(x, min2(y+1, h-1))
			dst.SetNRGBA(x, y, color.NRGBA{
				R: uint8((int(c0.R) + 2*int(c1.R) + int(c2.R)) / 4),
				G: uint8((int(c0.G) + 2*int(c1.G) + int(c2.G)) / 4),
				B: uint8((int(c0.B) + 2*int(c1.B) + int(c2.B)) / 4),
				A: c1.A,
			})
		}
	}
	return dst
}

func softSharpen(src *image.NRGBA) *image.NRGBA {
	blurred := softBlur(src)
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	const amount = 0.35
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			s := src.NRGBAAt(x, y)
			u := blurred.NRGBAAt(x, y)
			dst.SetNRGBA(x, y, color.NRGBA{
				R: clampU8(int(float64(s.R) + amount*float64(int(s.R)-int(u.R)))),
				G: clampU8(int(float64(s.G) + amount*float64(int(s.G)-int(u.G)))),
				B: clampU8(int(float64(s.B) + amount*float64(int(s.B)-int(u.B)))),
				A: s.A,
			})
		}
	}
	return dst
}

func max0(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
}

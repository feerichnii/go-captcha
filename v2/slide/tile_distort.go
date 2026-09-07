package slide

import (
	"image"
	"image/color"
	"math"

	"github.com/feerichnii/go-captcha/v2/base/random"
	"golang.org/x/image/draw"
)

// DistortTile applies a mild post-crop transform so the public tile is not an
// exact background crop (scale / warp / gamma / color / noise / blur|sharpen).
func DistortTile(src image.Image) image.Image {
	if src == nil {
		return nil
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 2 || h < 2 {
		return cloneNRGBA(src)
	}

	out := cloneNRGBA(src)

	// Slight non-uniform scale (±3%) via resample into same bounds.
	sx := 1.0 + (float64(random.RandInt(-30, 30)) / 1000.0)
	sy := 1.0 + (float64(random.RandInt(-30, 30)) / 1000.0)
	sw := int(math.Max(2, math.Round(float64(w)*sx)))
	sh := int(math.Max(2, math.Round(float64(h)*sy)))
	scaled := image.NewNRGBA(image.Rect(0, 0, sw, sh))
	draw.BiLinear.Scale(scaled, scaled.Bounds(), out, out.Bounds(), draw.Src, nil)
	tmp := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.BiLinear.Scale(tmp, tmp.Bounds(), scaled, scaled.Bounds(), draw.Src, nil)
	out = tmp

	amp := 0.6 + float64(random.RandInt(0, 14))/10.0 // ~0.6–2.0 px
	period := 6.0 + float64(random.RandInt(0, 10))
	out = warpNRGBA(out, amp, period)

	gamma := 0.88 + float64(random.RandInt(0, 28))/100.0 // 0.88–1.16
	dr := random.RandInt(-12, 12)
	dg := random.RandInt(-12, 12)
	db := random.RandInt(-12, 12)
	noiseAmt := random.RandInt(4, 14)
	applyColorNoiseGamma(out, gamma, dr, dg, db, noiseAmt)

	if random.RandInt(0, 1) == 0 {
		out = boxBlur(out, 1)
	} else {
		out = sharpen(out)
	}
	return out
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
	dx := 2.0 * math.Pi / period
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

func applyColorNoiseGamma(img *image.NRGBA, gamma float64, dr, dg, db, noiseAmt int) {
	b := img.Bounds()
	inv := 1.0 / math.Max(0.01, gamma)
	noise := random.FastBytes(b.Dx() * b.Dy() * 3)
	ni := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.NRGBAAt(x, y)
			if c.A == 0 {
				ni += 3
				continue
			}
			r := gammaByte(c.R, inv) + dr + noiseDelta(noise[ni], noiseAmt)
			g := gammaByte(c.G, inv) + dg + noiseDelta(noise[ni+1], noiseAmt)
			bl := gammaByte(c.B, inv) + db + noiseDelta(noise[ni+2], noiseAmt)
			ni += 3
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

func boxBlur(src *image.NRGBA, radius int) *image.NRGBA {
	if radius < 1 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var sr, sg, sb, sa, n int
			for dy := -radius; dy <= radius; dy++ {
				yy := y + dy
				if yy < 0 || yy >= h {
					continue
				}
				for dx := -radius; dx <= radius; dx++ {
					xx := x + dx
					if xx < 0 || xx >= w {
						continue
					}
					c := src.NRGBAAt(xx, yy)
					sr += int(c.R)
					sg += int(c.G)
					sb += int(c.B)
					sa += int(c.A)
					n++
				}
			}
			if n == 0 {
				continue
			}
			dst.SetNRGBA(x, y, color.NRGBA{
				R: uint8(sr / n), G: uint8(sg / n), B: uint8(sb / n), A: uint8(sa / n),
			})
		}
	}
	return dst
}

func sharpen(src *image.NRGBA) *image.NRGBA {
	// 3x3 unsharp-ish kernel.
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	k := [3][3]int{
		{0, -1, 0},
		{-1, 5, -1},
		{0, -1, 0},
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var sr, sg, sb int
			a := src.NRGBAAt(x, y).A
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					xx, yy := x+dx, y+dy
					if xx < 0 {
						xx = 0
					}
					if yy < 0 {
						yy = 0
					}
					if xx >= w {
						xx = w - 1
					}
					if yy >= h {
						yy = h - 1
					}
					c := src.NRGBAAt(xx, yy)
					f := k[dy+1][dx+1]
					sr += int(c.R) * f
					sg += int(c.G) * f
					sb += int(c.B) * f
				}
			}
			dst.SetNRGBA(x, y, color.NRGBA{
				R: clampU8(sr), G: clampU8(sg), B: clampU8(sb), A: a,
			})
		}
	}
	return dst
}

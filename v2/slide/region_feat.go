package slide

import (
	"image"

	"github.com/feerichnii/go-captcha/v2/base/randgen"
	"github.com/feerichnii/go-captcha/v2/base/random"
)

// regionFeat captures local image stats for decoy similarity matching.
type regionFeat struct {
	variance   float64
	luminance  float64
	edgeDensity float64
}

func analyzeRegion(img image.Image, x0, y0, w, h int) regionFeat {
	const step = 2
	var sum, sumSq, edgeSum float64
	var n float64
	prevLum := -1.0
	for y := y0; y < y0+h; y += step {
		prevLum = -1
		for x := x0; x < x0+w; x += step {
			r, g, b, _ := img.At(x, y).RGBA()
			lum := (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 65535.0
			sum += lum
			sumSq += lum * lum
			n++
			if prevLum >= 0 {
				d := lum - prevLum
				if d < 0 {
					d = -d
				}
				edgeSum += d
			}
			prevLum = lum
			// Vertical edge vs pixel above.
			if y > y0 {
				r2, g2, b2, _ := img.At(x, y-step).RGBA()
				lum2 := (0.299*float64(r2) + 0.587*float64(g2) + 0.114*float64(b2)) / 65535.0
				d := lum - lum2
				if d < 0 {
					d = -d
				}
				edgeSum += d
			}
		}
	}
	if n < 2 {
		return regionFeat{}
	}
	mean := sum / n
	return regionFeat{
		variance:    sumSq/n - mean*mean,
		luminance:   mean,
		edgeDensity: edgeSum / n,
	}
}

func featDistance(a, b regionFeat) float64 {
	// Relative scales so one channel doesn't dominate.
	dv := a.variance - b.variance
	dl := a.luminance - b.luminance
	de := a.edgeDensity - b.edgeDensity
	return 8*dv*dv + dl*dl + 4*de*de
}

// sampleCandidates gathers scattered 2D positions (not a single fence line).
func (c *captcha) sampleSlotCandidates(bg image.Image, leftMin, maxX, yLo, yHi, cW, cH, samples int) []slotCand {
	out := make([]slotCand, 0, samples+16)
	seen := map[[2]int]struct{}{}
	add := func(x, y int) {
		if x < leftMin || x > maxX || y < yLo || y > yHi {
			return
		}
		key := [2]int{x, y}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		f := analyzeRegion(bg, x, y, cW, cH)
		tex := randgen.TextureScore(bg, x, y, cW, cH)
		out = append(out, slotCand{x: x, y: y, feat: f, tex: tex})
	}

	stepX := cW / 3
	if stepX < 8 {
		stepX = 8
	}
	stepY := cH / 3
	if stepY < 8 {
		stepY = 8
	}
	for y := yLo; y <= yHi; y += stepY {
		for x := leftMin; x <= maxX; x += stepX {
			jx := x + random.RandInt(-stepX/3, stepX/3)
			jy := y + random.RandInt(-stepY/3, stepY/3)
			add(jx, jy)
		}
	}
	for i := 0; i < samples; i++ {
		add(random.RandInt(leftMin, maxX), random.RandInt(yLo, yHi))
	}
	return out
}

// pickSharedSlotY chooses one horizontal row with good average texture for the slider.
func (c *captcha) pickSharedSlotY(bg image.Image, leftMin, maxX, yLo, yHi, cW, cH int) int {
	bestY := yLo
	bestScore := -1.0
	step := cH / 4
	if step < 4 {
		step = 4
	}
	for y := yLo; y <= yHi; y += step {
		var sum float64
		n := 0
		for x := leftMin; x <= maxX; x += cW / 2 {
			if x > maxX {
				break
			}
			sum += randgen.TextureScore(bg, x, y, cW, cH)
			n++
		}
		if n == 0 {
			continue
		}
		avg := sum / float64(n)
		if avg > bestScore {
			bestScore = avg
			bestY = y
		}
	}
	// Small jitter so the row isn't always identical across challenges.
	jitter := random.RandInt(-6, 6)
	y := bestY + jitter
	if y < yLo {
		y = yLo
	}
	if y > yHi {
		y = yHi
	}
	return y
}

// sampleSlotCandidatesOnRow samples X positions on a fixed Y (same line as the tile).
func (c *captcha) sampleSlotCandidatesOnRow(bg image.Image, leftMin, maxX, y, cW, cH, samples int) []slotCand {
	out := make([]slotCand, 0, samples+16)
	seen := map[int]struct{}{}
	add := func(x int) {
		if x < leftMin || x > maxX {
			return
		}
		if _, ok := seen[x]; ok {
			return
		}
		seen[x] = struct{}{}
		f := analyzeRegion(bg, x, y, cW, cH)
		tex := randgen.TextureScore(bg, x, y, cW, cH)
		out = append(out, slotCand{x: x, y: y, feat: f, tex: tex})
	}

	stepX := cW / 4
	if stepX < 6 {
		stepX = 6
	}
	for x := leftMin; x <= maxX; x += stepX {
		add(x + random.RandInt(-stepX/4, stepX/4))
	}
	for i := 0; i < samples; i++ {
		add(random.RandInt(leftMin, maxX))
	}
	return out
}

type slotCand struct {
	x, y int
	feat regionFeat
	tex  float64
}

func minSlotSeparation(opts *Options, tileW int) int {
	if opts.minSlotSepPx > 0 {
		return opts.minSlotSepPx
	}
	sep := int(float64(tileW) * 0.85)
	if sep < 40 {
		sep = 40
	}
	return sep
}

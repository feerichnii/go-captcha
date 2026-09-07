/**
 * @Author Awen
 * @Date 2024/06/01
 * @Email wengaolng@gmail.com
 **/

package randgen

import (
	"image"
	"image/color"

	"github.com/golang/freetype/truetype"
	"github.com/feerichnii/go-captcha/v2/base/helper"
	"github.com/feerichnii/go-captcha/v2/base/random"
)

// RandFont randomly selects a font
func RandFont(fonts []*truetype.Font) *truetype.Font {
	index := helper.RandIndex(len(fonts))
	if index < 0 {
		return nil
	}

	return fonts[index]
}

// RandHexColor randomly selects a hex color
func RandHexColor(colors []string) string {
	index := helper.RandIndex(len(colors))
	if index < 0 {
		return ""
	}

	return colors[index]
}

// RandImage randomly selects an image
func RandImage(images []image.Image) image.Image {
	index := helper.RandIndex(len(images))
	if index < 0 {
		return nil
	}

	return images[index]
}

// RandString randomly selects a string
func RandString(chars []string) string {
	if len(chars) == 0 {
		return ""
	}
	// Answer characters/shapes are secret material: use crypto/rand.
	index := random.RandInt(0, len(chars)-1)
	return chars[index]
}

// RandColor randomly selects an RGBA color
func RandColor(co []color.Color) color.RGBA {
	colorLen := len(co)
	index := random.RandIntFast(0, colorLen-1)
	if index >= colorLen {
		index = colorLen - 1
	}

	r, g, b, a := co[index].RGBA()
	return color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: uint8(a)}
}

// RangCutImagePos randomly selects an image cropping position
func RangCutImagePos(width int, height int, img image.Image) image.Point {
	b := img.Bounds()
	iW := b.Max.X
	iH := b.Max.Y
	curX := 0
	curY := 0

	if iW-width > 0 {
		curX = random.RandIntFast(0, iW-width)
	}
	if iH-height > 0 {
		curY = random.RandIntFast(0, iH-height)
	}

	return image.Point{
		X: curX,
		Y: curY,
	}
}

// RangCutImagePosTextured picks a crop origin that prefers high-contrast /
// textured regions (avoids flat sky/water). samples is how many random
// candidates to score; values ≤0 use 12.
func RangCutImagePosTextured(width, height int, img image.Image, samples int) image.Point {
	if img == nil || width <= 0 || height <= 0 {
		return image.Point{}
	}
	if samples <= 0 {
		samples = 12
	}
	b := img.Bounds()
	maxX := b.Dx() - width
	maxY := b.Dy() - height
	if maxX <= 0 && maxY <= 0 {
		return image.Point{X: b.Min.X, Y: b.Min.Y}
	}
	if maxX < 0 {
		maxX = 0
	}
	if maxY < 0 {
		maxY = 0
	}

	best := RangCutImagePos(width, height, img)
	bestScore := cropTextureScore(img, best.X, best.Y, width, height)
	for i := 0; i < samples; i++ {
		p := image.Point{X: b.Min.X, Y: b.Min.Y}
		if maxX > 0 {
			p.X = b.Min.X + random.RandIntFast(0, maxX)
		}
		if maxY > 0 {
			p.Y = b.Min.Y + random.RandIntFast(0, maxY)
		}
		s := cropTextureScore(img, p.X, p.Y, width, height)
		if s > bestScore {
			bestScore = s
			best = p
		}
	}
	return best
}

// cropTextureScore estimates how "busy" a crop is via downsampled luminance variance.
func cropTextureScore(img image.Image, x0, y0, w, h int) float64 {
	const step = 3
	var sum, sumSq float64
	var n float64
	for y := y0; y < y0+h; y += step {
		for x := x0; x < x0+w; x += step {
			r, g, b, _ := img.At(x, y).RGBA()
			lum := (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 65535.0
			sum += lum
			sumSq += lum * lum
			n++
		}
	}
	if n < 2 {
		return 0
	}
	mean := sum / n
	return sumSq/n - mean*mean
}

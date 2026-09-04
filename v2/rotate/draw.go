/**
 * @Author Awen
 * @Date 2024/06/01
 * @Email wengaolng@gmail.com
 **/

package rotate

import (
	"image"
	"image/color"
	"math"

	"github.com/wenlng/go-captcha/v2/base/canvas"
	"github.com/wenlng/go-captcha/v2/base/randgen"
	"github.com/wenlng/go-captcha/v2/base/random"
	"golang.org/x/image/draw"
)

// DrawImageParams defines the parameters for drawing the main image
type DrawImageParams struct {
	Rotate     int
	SquareSize int
	Background image.Image
	Alpha      float32
}

// DrawCropCircleImageParams defines the parameters for drawing a cropped circle image
type DrawCropCircleImageParams struct {
	ScaleRatioSize int
	Rotate         int
	SquareSize     int
	Background     image.Image
	Alpha          float32
}

// DrawImage defines the interface for drawing images
type DrawImage interface {
	DrawWithNRGBA(params *DrawImageParams) (img image.Image, err error)
	DrawWithCropCircle(params *DrawCropCircleImageParams) (image.Image, error)
}

var _ DrawImage = (*drawImage)(nil)

// drawImage is the concrete implementation of the DrawImage interface
type drawImage struct {
}

// NewDrawImage creates a new DrawImage instance
// return: DrawImage interface instance
func NewDrawImage() DrawImage {
	return &drawImage{}
}

// DrawWithCropCircle draws a cropped circle image (thumbnail)
// params:
//   - params: Drawing parameters
//
// returns:
//   - image.Image: Drawn thumbnail image
//   - error: Error information
func (d *drawImage) DrawWithCropCircle(params *DrawCropCircleImageParams) (image.Image, error) {
	bgImage := params.Background

	bgBounds := bgImage.Bounds()
	cvs := canvas.CreateNRGBACanvas(bgImage.Bounds().Dx(), bgImage.Bounds().Dy(), true)
	draw.Draw(cvs.Get(), bgImage.Bounds(), bgImage, image.Point{}, draw.Over)
	cvs.CropScaleCircle(bgImage.Bounds().Dx()/2, bgImage.Bounds().Dy()/2, bgImage.Bounds().Dy()/2, params.ScaleRatioSize)
	cvs.Rotate(params.Rotate, true)

	cvBounds := cvs.Bounds()
	if cvBounds.Dy() > bgBounds.Dy() || cvBounds.Dx() > bgBounds.Dx() {
		newCvs := canvas.CreateNRGBACanvas(bgImage.Bounds().Dx(), bgImage.Bounds().Dy(), true)
		draw.Draw(newCvs.Get(), newCvs.Bounds(), cvs.Get(), image.Point{X: (cvBounds.Dy() - bgBounds.Dy()) / 2, Y: (cvBounds.Dx() - bgBounds.Dx()) / 2}, draw.Over)
		cvs = newCvs
	}

	return cvs.Get(), nil
}

// DrawWithNRGBA draws the main CAPTCHA image using NRGBA format
// params:
//   - params: Drawing parameters
//
// return:
//   - image.Image: Drawn image
//   - error: Error information
func (d *drawImage) DrawWithNRGBA(params *DrawImageParams) (img image.Image, err error) {
	var rcm = canvas.CreateNRGBACanvas(params.SquareSize, params.SquareSize, true)
	if params.Background != nil {
		bgImage := params.Background
		b := bgImage.Bounds()
		rc := canvas.CreateNRGBACanvas(b.Dx(), b.Dy(), true)
		point := randgen.RangCutImagePos(params.SquareSize, params.SquareSize, bgImage)
		draw.Draw(rc.Get(), b, bgImage, point, draw.Over)
		rc.SubImage(image.Rect(0, 0, params.SquareSize, params.SquareSize))
		draw.Draw(rcm.Get(), rcm.Bounds(), rc.Get(), image.Point{}, draw.Over)
	}

	rcm.CropCircle(rcm.Bounds().Dx()/2, rcm.Bounds().Dy()/2, rcm.Bounds().Dy()/2)
	addRotateRingNoise(rcm.Get())
	return rcm.Get(), nil
}

// addRotateRingNoise adds subtle rim noise to make angular correlation harder.
func addRotateRingNoise(img *image.NRGBA) {
	if img == nil {
		return
	}
	b := img.Bounds()
	cx := (b.Min.X + b.Max.X) / 2
	cy := (b.Min.Y + b.Max.Y) / 2
	rOuter := b.Dx() / 2
	rInner := rOuter - 6
	if rInner < 10 {
		return
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dx := float64(x - cx)
			dy := float64(y - cy)
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist < float64(rInner) || dist > float64(rOuter) {
				continue
			}
			if random.RandIntFast(0, 100) > 35 {
				continue
			}
			p := img.NRGBAAt(x, y)
			if p.A == 0 {
				continue
			}
			delta := random.RandIntFast(-22, 22)
			p.R = clampU8(int(p.R) + delta)
			p.G = clampU8(int(p.G) + delta)
			p.B = clampU8(int(p.B) + delta)
			img.SetNRGBA(x, y, p)
		}
	}
	ticks := random.RandIntFast(6, 14)
	for i := 0; i < ticks; i++ {
		ang := float64(random.RandIntFast(0, 359)) * math.Pi / 180
		x := cx + int(float64(rOuter-2)*math.Cos(ang))
		y := cy + int(float64(rOuter-2)*math.Sin(ang))
		if x < b.Min.X || x >= b.Max.X || y < b.Min.Y || y >= b.Max.Y {
			continue
		}
		img.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 90})
	}
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

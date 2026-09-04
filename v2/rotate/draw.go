/**
 * @Author Awen
 * @Date 2024/06/01
 * @Email wengaolng@gmail.com
 **/

package rotate

import (
	"image"
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
	// Independent noise field for the thumb (see addDiscNoise).
	addDiscNoise(cvs.Get(), 90, 10)

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
	return rcm.Get(), nil
}

// NoiseMaster applies the master's independent noise field. Call it only after
// the thumb has been cut from the clean master so the two fields are unrelated.
func NoiseMaster(img image.Image) image.Image {
	if n, ok := img.(*image.NRGBA); ok {
		addDiscNoise(n, 90, 10)
		return n
	}
	return img
}

// addDiscNoise applies independent per-pixel luminance noise to the opaque
// disc. Master and thumb each get their own pass, so the noise fields differ
// between the two images and do not line up under any rotation — this is what
// raises the cost of rotation-correlation solvers (identical noise would help them).
//
// density is the fraction of pixels touched in 1/256 units, amplitude the max |delta|.
func addDiscNoise(img *image.NRGBA, density, amplitude int) {
	if img == nil || amplitude <= 0 || density <= 0 {
		return
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 20 || h < 20 {
		return
	}
	cx, cy := w/2, h/2
	r := w / 2
	if h/2 < r {
		r = h / 2
	}
	r2 := r * r

	// Two random bytes per pixel: one gates density, one drives the delta.
	noise := random.FastBytes(w * h * 2)
	ni := 0
	span := 2*amplitude + 1
	for y := 0; y < h; y++ {
		dy := y - cy
		if dy*dy > r2 {
			ni += w * 2
			continue
		}
		half := int(math.Sqrt(float64(r2 - dy*dy)))
		x0, x1 := cx-half, cx+half
		if x0 < 0 {
			x0 = 0
		}
		if x1 > w-1 {
			x1 = w - 1
		}
		ni += x0 * 2
		row := img.Pix[y*img.Stride:]
		for x := x0; x <= x1; x++ {
			gate, raw := noise[ni], noise[ni+1]
			ni += 2
			if int(gate) >= density {
				continue
			}
			i := x * 4
			if row[i+3] == 0 {
				continue
			}
			delta := int(raw)%span - amplitude
			row[i] = clampU8(int(row[i]) + delta)
			row[i+1] = clampU8(int(row[i+1]) + delta)
			row[i+2] = clampU8(int(row[i+2]) + delta)
		}
		ni += (w - 1 - x1) * 2
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

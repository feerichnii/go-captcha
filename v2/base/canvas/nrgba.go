/**
 * @Author Awen
 * @Date 2024/06/01
 * @Email wengaolng@gmail.com
 **/

package canvas

import (
	"image"
	"math"

	"github.com/golang/freetype"
	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/math/f64"
	"golang.org/x/image/math/fixed"
)

// NRGBA interface for NRGBA canvas
type NRGBA interface {
	image.Image
	Get() *image.NRGBA
	DrawImage(img Palette, dotRect *PositionRect, posRect *AreaRect)
	DrawString(params *DrawStringParams, pt fixed.Point26_6) error
	CalcMarginBlankArea() *AreaRect
	Rotate(angle int, overCrop bool)
	Scale(zoomSize int, keepRatio, centerAlign bool)
	CropCircle(x, y, radius int)
	CropScaleCircle(x, y, radius int, zoomSize int)
	SubImage(r image.Rectangle)
}

var _ NRGBA = (*nRGBA)(nil)

// NewNRGBA creates an NRGBA canvas. image.NewNRGBA is already fully
// transparent, so only the opaque white variant needs a fill.
func NewNRGBA(r image.Rectangle, isAlpha bool) NRGBA {
	nrgba := image.NewNRGBA(r)
	if !isAlpha && len(nrgba.Pix) > 0 {
		row := nrgba.Pix[:nrgba.Stride]
		for i := range row {
			row[i] = 0xFF
		}
		for off := nrgba.Stride; off < len(nrgba.Pix); off += nrgba.Stride {
			copy(nrgba.Pix[off:off+nrgba.Stride], row)
		}
	}

	return &nRGBA{
		NRGBA: nrgba,
	}
}

// nRGBA struct for NRGBA canvas
type nRGBA struct {
	*image.NRGBA
}

// Get retrieves the NRGBA canvas
func (n *nRGBA) Get() *image.NRGBA {
	return n.NRGBA
}

// DrawString draws a string on the canvas
func (n *nRGBA) DrawString(params *DrawStringParams, pt fixed.Point26_6) error {
	dc := freetype.NewContext()
	dc.SetDPI(float64(params.FontDPI))
	dc.SetFont(params.Font)
	dc.SetClip(n.Bounds())
	dc.SetDst(n.Get())

	dc.SetFontSize(float64(params.Size))
	dc.SetHinting(font.HintingFull)

	fontColor := image.NewUniform(params.Color)
	dc.SetSrc(fontColor)

	if _, err := dc.DrawString(params.Text, pt); err != nil {
		return err
	}

	return nil
}

// DrawImage draws an image on the canvas
func (n *nRGBA) DrawImage(img Palette, dotRect *PositionRect, posRect *AreaRect) {
	nW := img.Bounds().Max.X
	nH := img.Bounds().Max.Y

	dX := dotRect.X
	dY := dotRect.Y
	dHeight := dotRect.Height

	pMinX := posRect.MinX
	pMinY := posRect.MinY
	pMaxX := posRect.MaxX
	pMaxY := posRect.MaxY

	startX := pMinX
	if startX < 0 {
		startX = 0
	}
	endX := pMaxX
	if endX > nW {
		endX = nW
	}
	startY := pMinY
	if startY < 0 {
		startY = 0
	}
	endY := pMaxY
	if endY > nH {
		endY = nH
	}

	dst := n.NRGBA
	for y := startY; y < endY; y++ {
		for x := startX; x < endX; x++ {
			co := img.At(x, y)
			_, _, _, a := co.RGBA()
			if a > 0 {
				dstX := dX + (x - pMinX)
				dstY := dY - dHeight + (y - pMinY)
				dst.Set(dstX, dstY, co)
			}
		}
	}
}

// CalcMarginBlankArea calculates the blank area of the canvas
func (n *nRGBA) CalcMarginBlankArea() *AreaRect {
	bounds := n.Bounds()
	nW := bounds.Max.X
	nH := bounds.Max.Y
	minX := nW
	maxX := 0
	minY := nH
	maxY := 0

	img := n.NRGBA
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		row := img.Pix[(y-bounds.Min.Y)*img.Stride:]
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if row[(x-bounds.Min.X)*4+3] == 0 {
				continue
			}
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
		}
	}

	minX = int(math.Max(0, float64(minX-2)))
	maxX = int(math.Min(float64(nW), float64(maxX+2)))
	minY = int(math.Max(0, float64(minY-2)))
	maxY = int(math.Min(float64(nH), float64(maxY+2)))

	return &AreaRect{
		minX,
		maxX,
		minY,
		maxY,
	}
}

// Rotate rotates the canvas by any angle
func (n *nRGBA) Rotate(a int, overCrop bool) {
	if a == 0 {
		return
	}

	angle := float64(a) * math.Pi / 180

	sW := n.Get().Bounds().Dx()
	sH := n.Get().Bounds().Dy()
	w, h := RotatedSize(sW, sH, float64(a))
	img := image.NewNRGBA(image.Rect(0, 0, w, h))

	centerX := float64(w) / 2
	centerY := float64(h) / 2

	matrix := Matrix{
		1, 0,
		0, 1,
		0, 0,
	}
	matrix = matrix.Translate(centerX, centerY)
	matrix = matrix.Rotate(angle)
	matrix = matrix.Translate(-centerX, -centerY)

	x := (w - n.Get().Bounds().Dx()) / 2
	y := (h - n.Get().Bounds().Dy()) / 2
	fx, fy := float64(x), float64(y)

	m := matrix.Translate(fx, fy)
	s2d := f64.Aff3{m.XX, m.XY, m.X0, m.YX, m.YY, m.Y0}

	draw.BiLinear.Transform(img, s2d, n.Get(), n.Get().Bounds(), draw.Over, nil)
	n.NRGBA = img

	if overCrop {
		xx := w - sW
		yy := h - sH
		dx := (xx / 2) + 1
		dy := (yy / 2) + 1
		n.SubImage(image.Rect(dx, dy, sW+dx, sH+dy))
	}
}

// circleMask builds an opaque-white disc mask by writing Pix directly.
// Rows outside the disc are skipped; per-row x extents come from the
// integer circle equation, so there is no per-pixel sqrt or color conversion.
func circleMask(bounds image.Rectangle, cx, cy, radius int) *image.NRGBA {
	mask := image.NewNRGBA(bounds)
	if radius <= 0 {
		return mask
	}
	r2 := radius * radius
	yStart := cy - radius
	if yStart < bounds.Min.Y {
		yStart = bounds.Min.Y
	}
	yEnd := cy + radius
	if yEnd > bounds.Max.Y-1 {
		yEnd = bounds.Max.Y - 1
	}
	for py := yStart; py <= yEnd; py++ {
		dy := py - cy
		half := int(math.Sqrt(float64(r2 - dy*dy)))
		x0 := cx - half
		x1 := cx + half
		if x0 < bounds.Min.X {
			x0 = bounds.Min.X
		}
		if x1 > bounds.Max.X-1 {
			x1 = bounds.Max.X - 1
		}
		if x0 > x1 {
			continue
		}
		off := (py-bounds.Min.Y)*mask.Stride + (x0-bounds.Min.X)*4
		row := mask.Pix[off : off+(x1-x0+1)*4]
		for i := range row {
			row[i] = 0xFF
		}
	}
	return mask
}

// CropCircle crops a circular area
func (n *nRGBA) CropCircle(x, y, radius int) {
	bounds := n.Get().Bounds()
	mask := circleMask(bounds, x, y, radius)
	draw.DrawMask(mask, mask.Bounds(), n.Get(), image.Point{X: 0, Y: 0}, mask, image.Point{}, draw.Over)
	n.NRGBA = mask
}

// CropScaleCircle scales and crops a circular area
func (n *nRGBA) CropScaleCircle(x, y, radius int, zoomSize int) {
	bounds := n.Get().Bounds()
	mask := circleMask(bounds, x, y, radius)

	if zoomSize > 0 {
		subtract := zoomSize * 2
		scaleMask := image.NewNRGBA(image.Rect(0, 0, n.Bounds().Dx()-subtract, n.Bounds().Dy()-subtract))
		draw.BiLinear.Scale(scaleMask, scaleMask.Bounds(), mask, mask.Bounds(), draw.Over, nil)
		mask = scaleMask
	}

	draw.DrawMask(mask, mask.Bounds(), n.Get(), image.Point{X: zoomSize, Y: zoomSize}, mask, image.Point{}, draw.Over)
	n.NRGBA = mask
}

// Scale scales the canvas
func (n *nRGBA) Scale(zoomSize int, keepRatio, centerAlign bool) {
	img := n.NRGBA
	if zoomSize > 0 {
		subtract := zoomSize * 2
		newW := n.Get().Bounds().Dx() - subtract
		newH := n.Get().Bounds().Dy() - subtract
		outImg := image.NewNRGBA(image.Rect(0, 0, newW, newH))

		if !keepRatio {
			draw.BiLinear.Scale(outImg, outImg.Bounds(), n.Get(), n.Get().Bounds(), draw.Over, nil)
		} else {
			dst := CalcResizedRect(n.Get().Bounds(), newW, newH, centerAlign)
			draw.ApproxBiLinear.Scale(outImg, dst.Bounds(), n.Get(), n.Get().Bounds(), draw.Over, nil)
		}

		img = outImg
	}

	n.NRGBA = img
}

// SubImage captures a sub-image
func (n *nRGBA) SubImage(r image.Rectangle) {
	n.NRGBA = n.Get().SubImage(r).(*image.NRGBA)
}

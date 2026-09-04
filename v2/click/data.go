/**
 * @Author Awen
 * @Date 2024/06/01
 * @Email wengaolng@gmail.com
 **/

package click

import "github.com/feerichnii/go-captcha/v2/base/imagedata"

// CaptchaData defines the interface for captcha data
type CaptchaData interface {
	// GetData returns server-only answer dots (coordinates / text / shape).
	// Never send this to the client. Prefer GetPublicData() in HTTP responses.
	GetData() map[int]*Dot
	// GetPublicData returns client-safe metadata (click order indices only).
	GetPublicData() map[int]*PublicDot
	GetMasterImage() imagedata.JPEGImageData
	GetThumbImage() imagedata.PNGImageData
}

// CaptData is the concrete implementation of the CaptchaData interface
type CaptData struct {
	dots        map[int]*Dot
	masterImage imagedata.JPEGImageData
	thumbImage  imagedata.PNGImageData
}

var _ CaptchaData = (*CaptData)(nil)

// GetData gets the dot data of the captcha (server-only secret)
// return: Map of dot data
func (c CaptData) GetData() map[int]*Dot {
	return c.dots
}

// GetPublicData returns client-safe click metadata without coordinates or labels.
func (c CaptData) GetPublicData() map[int]*PublicDot {
	out := make(map[int]*PublicDot, len(c.dots))
	for k, d := range c.dots {
		if d == nil {
			continue
		}
		out[k] = &PublicDot{Index: d.Index}
	}
	return out
}

// GetMasterImage gets the main captcha image
// return: Main image in JPEG format
func (c CaptData) GetMasterImage() imagedata.JPEGImageData {
	return c.masterImage
}

// GetThumbImage gets the thumbnail image
// return: Thumbnail image in PNG format
func (c CaptData) GetThumbImage() imagedata.PNGImageData {
	return c.thumbImage
}

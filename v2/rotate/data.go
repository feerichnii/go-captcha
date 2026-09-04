/**
 * @Author Awen
 * @Date 2024/06/01
 * @Email wengaolng@gmail.com
 **/

package rotate

import "github.com/wenlng/go-captcha/v2/base/imagedata"

// CaptchaData defines the interface for rotate CAPTCHA data
type CaptchaData interface {
	// GetData returns the full block including secret Angle.
	// Never send this to the client. Prefer GetPublicData().
	GetData() *Block
	// GetPublicData returns size metadata only (no angle).
	GetPublicData() *PublicBlock
	GetMasterImage() imagedata.PNGImageData
	GetThumbImage() imagedata.PNGImageData
}

// CaptData is the concrete implementation of the CaptchaData interface
type CaptData struct {
	block       *Block
	masterImage imagedata.PNGImageData
	thumbImage  imagedata.PNGImageData
}

var _ CaptchaData = (*CaptData)(nil)

// GetData is to get block (server-only secret)
func (c CaptData) GetData() *Block {
	return c.block
}

// GetPublicData returns client-safe rotate metadata without the secret angle.
func (c CaptData) GetPublicData() *PublicBlock {
	if c.block == nil {
		return nil
	}
	return &PublicBlock{
		ParentWidth:  c.block.ParentWidth,
		ParentHeight: c.block.ParentHeight,
		Width:        c.block.Width,
		Height:       c.block.Height,
	}
}

// GetMasterImage is to get master image
func (c CaptData) GetMasterImage() imagedata.PNGImageData {
	return c.masterImage
}

// GetThumbImage is to get thumb image
func (c CaptData) GetThumbImage() imagedata.PNGImageData {
	return c.thumbImage
}

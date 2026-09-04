/**
 * @Author Awen
 * @Date 2024/06/01
 * @Email wengaolng@gmail.com
 **/

package slide

import "github.com/feerichnii/go-captcha/v2/base/imagedata"

// CaptchaData defines the interface for slide CAPTCHA data
type CaptchaData interface {
	// GetData returns the full block including secret target X/Y.
	// Never send this to the client. Prefer GetPublicData().
	GetData() *Block
	// GetPublicData returns tile start position and size only (no target X/Y).
	GetPublicData() *PublicBlock
	GetMasterImage() imagedata.JPEGImageData
	GetTileImage() imagedata.PNGImageData
}

// CaptData is the concrete implementation of the CaptchaData interface
type CaptData struct {
	block       *Block
	masterImage imagedata.JPEGImageData
	tileImage   imagedata.PNGImageData
}

var _ CaptchaData = (*CaptData)(nil)

// GetData gets the block data of the CAPTCHA (server-only secret)
// return: Pointer to block data
func (c CaptData) GetData() *Block {
	return c.block
}

// GetPublicData returns client-safe slide metadata (tile start + size).
func (c CaptData) GetPublicData() *PublicBlock {
	if c.block == nil {
		return nil
	}
	return &PublicBlock{
		Width:  c.block.Width,
		Height: c.block.Height,
		TileX:  c.block.TileX,
		TileY:  c.block.TileY,
		DX:     c.block.DX,
		DY:     c.block.DY,
	}
}

// GetMasterImage gets the main CAPTCHA image
// return: Main image in JPEG format
func (c CaptData) GetMasterImage() imagedata.JPEGImageData {
	return c.masterImage
}

// GetTileImage gets the tile image
// return: Tile image in PNG format
func (c CaptData) GetTileImage() imagedata.PNGImageData {
	return c.tileImage
}

/**
 * @Author Awen
 * @Date 2024/06/01
 * @Email wengaolng@gmail.com
 **/

package slide

import (
	"errors"
	"image"
	"math"

	"github.com/feerichnii/go-captcha/v2/base/helper"
	"github.com/feerichnii/go-captcha/v2/base/imagedata"
	"github.com/feerichnii/go-captcha/v2/base/logger"
	"github.com/feerichnii/go-captcha/v2/base/option"
	"github.com/feerichnii/go-captcha/v2/base/randgen"
	"github.com/feerichnii/go-captcha/v2/base/random"
)

type Mode int

const (
	ModeBasic Mode = iota
	ModeDrag
)

// Captcha defines the interface for slide CAPTCHA
type Captcha interface {
	setOptions(opts ...Option)
	setResources(resources ...Resource)
	GetOptions() *Options
	Generate() (CaptchaData, error)
}

var _ Captcha = (*captcha)(nil)

var (
	GraphImageErr           = errors.New("graph image is invalid")
	GenerateDataErr         = errors.New("data generation failed")
	ImageTypeErr            = errors.New("tile image must be of type image.Image")
	ShadowImageTypeErr      = errors.New("tile shadow image must be of type image.Image")
	MaskImageTypeErr        = errors.New("tile mask image must be of type image.Image")
	EmptyBackgroundImageErr = errors.New("no background image")
)

// captcha is the concrete implementation of the Captcha interface
type captcha struct {
	version   string
	logger    logger.Logger
	drawImage DrawImage
	opts      *Options
	resources *Resources
	mode      Mode
}

// newWithMode creates a new slide CAPTCHA instance
// params:
//   - mode: CAPTCHA mode
//   - opts: Optional initial options
//
// return: Captcha interface instance
func newWithMode(mode Mode, opts ...Option) Captcha {
	capt := &captcha{
		logger:    logger.New(),
		drawImage: NewDrawImage(),
		opts:      NewOptions(),
		resources: NewResources(),
		mode:      mode,
	}

	defaultOptions()(capt.opts)
	defaultResource()(capt.resources)

	capt.setOptions(opts...)

	if mode == ModeBasic {
		capt.opts.rangeDeadZoneDirections = []DeadZoneDirectionType{DeadZoneDirectionTypeLeft}
		capt.opts.enableGraphVerticalRandom = false
	}

	return capt
}

// setOptions sets the CAPTCHA options
// params:
//   - opts: Options to set
func (c *captcha) setOptions(opts ...Option) {
	for _, opt := range opts {
		opt(c.opts)
	}
}

// setResources sets the CAPTCHA resources
// params:
//   - resources: Resources to set
func (c *captcha) setResources(resources ...Resource) {
	for _, resource := range resources {
		resource(c.resources)
	}
}

// GetOptions gets the CAPTCHA options
// return: Pointer to options
func (c *captcha) GetOptions() *Options {
	return c.opts
}

// Generate generates slide CAPTCHA data with multiple drop slots (default 3).
// All slots share the same silhouette; humans (and bots) must match the tile
// to the background crop — only one position is correct. The secret (X,Y) is
// only available via GetData() — never GetPublicData().
func (c *captcha) Generate() (CaptchaData, error) {
	if err := c.check(); err != nil {
		return nil, err
	}

	nSlots := c.opts.genGraphNumber
	if nSlots < 1 {
		nSlots = 1
	}

	blocks, tilePoint := c.genGraphBlocks(c.opts.imageSize, c.opts.rangeGraphSize, nSlots)
	if len(blocks) == 0 {
		return nil, GenerateDataErr
	}

	correctIdx := 0
	if len(blocks) > 1 {
		correctIdx = helper.RandIndex(len(blocks))
		if correctIdx < 0 {
			correctIdx = 0
		}
	}
	block := blocks[correctIdx]
	if block == nil {
		return nil, GenerateDataErr
	}

	graphs := c.pickSlotGraphs(len(blocks), correctIdx)
	if graphs == nil || graphs[correctIdx] == nil {
		return nil, GraphImageErr
	}
	correct := graphs[correctIdx]
	if correct.OverlayImage == nil || correct.ShadowImage == nil || correct.MaskImage == nil {
		return nil, GraphImageErr
	}

	masterImage, masterBgImage, err := c.genMasterImage(c.opts.imageSize, blocks, graphs)
	if err != nil {
		return nil, err
	}

	tileImage, err := c.genTileImage(correct.MaskImage, masterBgImage, correct.OverlayImage, block)
	if err != nil {
		return nil, err
	}

	if c.mode == ModeBasic {
		block.TileY = block.Y
		block.DY = block.Y
	} else {
		block.TileY = tilePoint.Y
		block.DY = tilePoint.Y
	}
	block.TileX = tilePoint.X
	block.DX = tilePoint.X

	return &CaptData{
		block:       block,
		masterImage: imagedata.NewJPEGImageData(masterImage),
		tileImage:   imagedata.NewPNGImageData(tileImage),
	}, nil
}

// pickSlotGraphs picks one GraphImage from the pool and assigns that same
// silhouette to every drop slot. Decoys differ only by where they sit on the
// background — matching is by image content, not shape.
func (c *captcha) pickSlotGraphs(nSlots, correctIdx int) []*GraphImage {
	pool := c.resources.rangGraphImage
	if len(pool) == 0 || nSlots <= 0 {
		return nil
	}
	_ = correctIdx
	idx := helper.RandIndex(len(pool))
	if idx < 0 {
		idx = 0
	}
	g := pool[idx]
	out := make([]*GraphImage, nSlots)
	for i := 0; i < nSlots; i++ {
		out[i] = g
	}
	return out
}

// genMasterImage generates the master CAPTCHA image and background image.
// Every slot uses the same ShadowImage silhouette at different positions.
func (c *captcha) genMasterImage(size *option.Size, blocks []*Block, graphs []*GraphImage) (image.Image, image.Image, error) {
	var drawBlocks = make([]*DrawBlock, 0, len(blocks))
	for i := 0; i < len(blocks); i++ {
		block := blocks[i]
		shadow := graphs[i].ShadowImage
		drawBlocks = append(drawBlocks, &DrawBlock{
			X:      block.X,
			Y:      block.Y,
			Width:  block.Width,
			Height: block.Height,
			Angle:  block.Angle,
			Block:  block,
			Image:  shadow,
		})
	}

	return c.drawImage.DrawWithNRGBA(&DrawImageParams{
		Width:             size.Width,
		Height:            size.Height,
		Background:        randgen.RandImage(c.resources.rangBackgrounds),
		Alpha:             c.opts.imageAlpha,
		CaptchaDrawBlocks: drawBlocks,
	})
}

// genTileImage generates a tile image
// params:
//   - maskImage: Mask image
//   - bgImage: Background image
//   - overlayImage: Overlay image
//   - block: Block data
//
// returns:
//   - image.Image: Tile image
//   - error: Error information
func (c *captcha) genTileImage(maskImage image.Image, bgImage image.Image, overlayImage image.Image, block *Block) (image.Image, error) {
	return c.drawImage.DrawWithTemplate(&DrawTplImageParams{
		Background: bgImage,
		MaskImage:  maskImage,
		Alpha:      c.opts.imageAlpha,
		Width:      block.Width,
		Height:     block.Height,
		CaptchaDrawBlock: &DrawBlock{
			X:      block.X,
			Y:      block.Y,
			Width:  block.Width,
			Height: block.Height,
			Angle:  block.Angle,
			Block:  block,
			Image:  overlayImage,
		},
	})
}

// randDeadZoneDirection generates a random dead zone direction
// return: Dead zone direction
func (c *captcha) randDeadZoneDirection() DeadZoneDirectionType {
	dirs := c.opts.rangeDeadZoneDirections

	index := helper.RandIndex(len(dirs))
	if index < 0 {
		return 0
	}

	res := dirs[index]
	return res
}

// randGraphAngle generates a random graph angle
// return: Random angle value
func (c *captcha) randGraphAngle() int {
	angles := c.opts.rangeGraphAnglePos

	index := helper.RandIndex(len(angles))
	if index < 0 {
		return 0
	}

	angle := angles[index]
	res := random.RandInt(angle.Min, angle.Max)

	return res
}

// genGraphBlocks generates graph block data
// params:
//   - imageSize: Main image size
//   - size: Graph size range
//   - length: Number of graphs
//
// returns:
//   - []*Block: List of blocks
//   - *option.Point: Tile position
func (c *captcha) genGraphBlocks(imageSize *option.Size, size *option.RangeVal, length int) ([]*Block, *option.Point) {
	var blocks = make([]*Block, 0, length)
	width := imageSize.Width
	height := imageSize.Height

	randAngle := c.randGraphAngle()
	randSize := random.RandInt(size.Min, size.Max)
	cHeight := randSize
	cWidth := randSize

	dzdType := c.randDeadZoneDirection()
	dp := cWidth / 2
	blockWidth := (width - cWidth - 20) / length
	y := c.calcYWithDeadZone(5, height-cHeight-5, cHeight, dzdType)

	for i := 0; i < length; i++ {
		var block = &Block{}
		start, end := c.calcXWithDeadZone((i*blockWidth)+dp+5, ((i+1)*blockWidth)-dp, cWidth, dzdType)

		start = int(math.Max(float64(start), float64(dp+5)))
		block.X = random.RandInt(start+20, end+20) - dp

		if c.opts.enableGraphVerticalRandom {
			y = c.calcYWithDeadZone(5, height-cHeight-5, cHeight, dzdType)
		}

		block.Y = y
		block.Width = cWidth
		block.Height = cHeight
		block.Angle = randAngle

		blocks = append(blocks, block)
	}

	point := &option.Point{}
	if c.mode == ModeBasic {
		point.X = random.RandInt(5, dp)
		point.Y = y
		return blocks, point
	}

	if dzdType == DeadZoneDirectionTypeTop {
		point.X = random.RandInt(5, width-cWidth-5)
		point.Y = 5
	} else if dzdType == DeadZoneDirectionTypeBottom {
		point.X = random.RandInt(5, width-cWidth-5)
		point.Y = height - cHeight - 5
	} else if dzdType == DeadZoneDirectionTypeLeft {
		point.X = 5
		point.Y = random.RandInt(5, height-cHeight-5)
	} else if dzdType == DeadZoneDirectionTypeRight {
		point.X = width - cWidth - 5
		point.Y = random.RandInt(5, height-cHeight-5)
	}

	return blocks, point
}

// calcXWithDeadZone calculates the X coordinate range (considering dead zone)
// params:
//   - start: Start X coordinate
//   - end: End X coordinate
//   - value: Block width
//   - dzdType: Dead zone direction
//
// returns:
//   - int: Adjusted start X coordinate
//   - int: Adjusted end X coordinate
func (c *captcha) calcXWithDeadZone(start, end, value int, dzdType DeadZoneDirectionType) (int, int) {
	if dzdType == DeadZoneDirectionTypeLeft {
		start += value
		end += value
	}
	return start, end
}

// calcYWithDeadZone calculates the Y coordinate (considering dead zone)
// params:
//   - start: Start Y coordinate
//   - end: End Y coordinate
//   - value: Block height
//   - dzdType: Dead zone direction
//
// return: Random Y coordinate
func (c *captcha) calcYWithDeadZone(start, end, value int, dzdType DeadZoneDirectionType) int {
	if dzdType == DeadZoneDirectionTypeTop {
		start += value
	} else if dzdType == DeadZoneDirectionTypeBottom {
		end -= value
	}
	return random.RandInt(start, end)
}

// check checks the CAPTCHA parameters
// return: Error information
func (c *captcha) check() error {
	for _, tile := range c.resources.rangGraphImage {
		if tile.OverlayImage == nil {
			return ImageTypeErr
		} else if tile.ShadowImage == nil {
			return ShadowImageTypeErr
		} else if tile.MaskImage == nil {
			return MaskImageTypeErr
		}
	}

	if len(c.resources.rangBackgrounds) == 0 {
		return EmptyBackgroundImageErr
	}

	return nil
}

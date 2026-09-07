/**
 * @Author Awen
 * @Date 2024/06/01
 * @Email wengaolng@gmail.com
 **/

package slide

import (
	"errors"
	"image"

	"github.com/feerichnii/go-captcha/v2/base/helper"
	"github.com/feerichnii/go-captcha/v2/base/imagedata"
	"github.com/feerichnii/go-captcha/v2/base/logger"
	"github.com/feerichnii/go-captcha/v2/base/option"
	"github.com/feerichnii/go-captcha/v2/base/randgen"
	"github.com/feerichnii/go-captcha/v2/base/random"
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
}

// newCaptcha creates a slide CAPTCHA instance (horizontal slide, fixed Y).
func newCaptcha(opts ...Option) Captcha {
	capt := &captcha{
		logger:    logger.New(),
		drawImage: NewDrawImage(),
		opts:      NewOptions(),
		resources: NewResources(),
	}

	defaultOptions()(capt.opts)
	defaultResource()(capt.resources)
	capt.setOptions(opts...)
	return capt
}

// setOptions sets the CAPTCHA options
func (c *captcha) setOptions(opts ...Option) {
	for _, opt := range opts {
		opt(c.opts)
	}
}

// setResources sets the CAPTCHA resources
func (c *captcha) setResources(resources ...Resource) {
	for _, resource := range resources {
		resource(c.resources)
	}
}

// GetOptions gets the CAPTCHA options
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

	// Horizontal slide: tile starts on the left at the same Y as the notches.
	block.TileX = tilePoint.X
	block.DX = tilePoint.X
	block.TileY = block.Y
	block.DY = block.Y

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

func (c *captcha) randGraphAngle() int {
	angles := c.opts.rangeGraphAnglePos
	index := helper.RandIndex(len(angles))
	if index < 0 {
		return 0
	}
	angle := angles[index]
	return random.RandInt(angle.Min, angle.Max)
}

// genGraphBlocks places drop slots across the master and picks a left-side
// tile start. Notch X is always in [0, width-tileW] so a horizontal slider
// can reach every target.
func (c *captcha) genGraphBlocks(imageSize *option.Size, size *option.RangeVal, length int) ([]*Block, *option.Point) {
	var blocks = make([]*Block, 0, length)
	width := imageSize.Width
	height := imageSize.Height

	randAngle := c.randGraphAngle()
	randSize := random.RandInt(size.Min, size.Max)
	cHeight := randSize
	cWidth := randSize

	maxX := width - cWidth
	if maxX < 0 {
		maxX = 0
	}
	// Leave room on the left for the tile start position.
	leftMin := cWidth + 5
	if leftMin > maxX {
		leftMin = 0
	}
	usable := maxX - leftMin
	if usable < 0 {
		usable = 0
	}

	yLo, yHi := 5, height-cHeight-5
	if yHi < yLo {
		yHi = yLo
	}
	y := random.RandInt(yLo, yHi)

	for i := 0; i < length; i++ {
		block := &Block{}
		seg := 0
		if length > 0 {
			seg = usable / length
		}
		start := leftMin + i*seg
		end := start + seg
		if i == length-1 {
			end = maxX
		}
		if end > maxX {
			end = maxX
		}
		if start > end {
			start = end
		}
		if end-start >= 6 {
			block.X = random.RandInt(start+2, end-2)
		} else {
			block.X = random.RandInt(start, end)
		}
		if block.X > maxX {
			block.X = maxX
		}
		if block.X < 0 {
			block.X = 0
		}

		if c.opts.enableGraphVerticalRandom {
			y = random.RandInt(yLo, yHi)
		}

		block.Y = y
		block.Width = cWidth
		block.Height = cHeight
		block.Angle = randAngle
		blocks = append(blocks, block)
	}

	point := &option.Point{
		X: random.RandInt(5, cWidth/2),
		Y: y,
	}
	if point.X > maxX {
		point.X = maxX
	}
	return blocks, point
}

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

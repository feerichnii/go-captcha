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
	"golang.org/x/image/draw"
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

const (
	fallbackSlotMin = 4
	fallbackSlotMax = 4
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

func (c *captcha) resolveSlotCount() int {
	n := c.opts.genGraphNumber
	if n >= 1 {
		return n
	}
	lo := c.opts.candidateSlotsMin
	hi := c.opts.candidateSlotsMax
	if lo < 1 {
		lo = fallbackSlotMin
	}
	if hi < lo {
		hi = lo
	}
	if hi < 1 {
		hi = fallbackSlotMax
	}
	return random.RandInt(lo, hi)
}

// Generate builds a slide puzzle with exact geometry:
// the real hole is drawn at the same (X,Y) the tile was cropped from;
// all candidate holes share one mask/shape; anti-template protection is
// photometric-only on the tile pixels (default: 1 real + 3 decoys).
func (c *captcha) Generate() (CaptchaData, error) {
	if err := c.check(); err != nil {
		return nil, err
	}

	nSlots := c.resolveSlotCount()
	size := c.opts.imageSize
	bgSrc := randgen.RandImage(c.resources.rangBackgrounds)
	masterBg := cropMasterBackground(bgSrc, size.Width, size.Height)

	blocks, tilePoint := c.genGraphBlocksScattered(masterBg, size, c.opts.rangeGraphSize, nSlots)
	if len(blocks) == 0 {
		return nil, GenerateDataErr
	}

	// Index 0 is always the real hole / tile-source (exact crop coordinates).
	real := blocks[0]
	if real == nil {
		return nil, GenerateDataErr
	}

	graphs := c.pickSlotGraphs(len(blocks), 0)
	if graphs == nil || graphs[0] == nil {
		return nil, GraphImageErr
	}
	piece := graphs[0]
	if piece.OverlayImage == nil || piece.ShadowImage == nil || piece.MaskImage == nil {
		return nil, GraphImageErr
	}

	// Tile from the exact real-hole coordinates on the clean master background.
	tileImage, err := c.genTileImage(piece.MaskImage, masterBg, piece.OverlayImage, real)
	if err != nil {
		return nil, err
	}
	// Photometric anti-template only — does not change puzzle geometry.
	tileImage = DistortTileWith(tileImage, c.opts.tileDistort)

	// Draw identical-shaped holes (same shadow/mask) at real + decoy positions.
	masterImage, err := c.genMasterImageOn(masterBg, size, blocks, graphs)
	if err != nil {
		return nil, err
	}

	real.TileX = tilePoint.X
	real.DX = tilePoint.X
	real.TileY = real.Y
	real.DY = real.Y
	real.Angle = 0

	return &CaptData{
		block:       real,
		slotCount:   len(blocks),
		masterImage: imagedata.NewJPEGImageData(masterImage),
		tileImage:   imagedata.NewPNGImageData(tileImage),
	}, nil
}

func cropMasterBackground(bg image.Image, width, height int) image.Image {
	if bg == nil || width <= 0 || height <= 0 {
		return bg
	}
	b := bg.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	point := randgen.RangCutImagePosTextured(width, height, bg, 16)
	draw.Draw(dst, dst.Bounds(), bg, point, draw.Src)
	_ = b
	return dst
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

// genMasterImageOn draws shadows onto a pre-cropped master background.
func (c *captcha) genMasterImageOn(masterBg image.Image, size *option.Size, blocks []*Block, graphs []*GraphImage) (image.Image, error) {
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

	img, _, err := c.drawImage.DrawWithNRGBA(&DrawImageParams{
		Width:             size.Width,
		Height:            size.Height,
		Background:        masterBg,
		Alpha:             c.opts.imageAlpha,
		CaptchaDrawBlocks: drawBlocks,
		BackgroundPreCropped: true,
	})
	return img, err
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

// genGraphBlocksScattered places one high-texture correct slot, then decoys with
// similar local features and minimum X separation. By default all holes share
// one Y with the tile (horizontal slider). Index 0 is the real tile-source.
func (c *captcha) genGraphBlocksScattered(bg image.Image, imageSize *option.Size, size *option.RangeVal, length int) ([]*Block, *option.Point) {
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
	leftMin := cWidth + 5
	if leftMin > maxX {
		leftMin = 0
	}

	yLo, yHi := 5, height-cHeight-5
	if yHi < yLo {
		yHi = yLo
	}

	var candidates []slotCand
	if c.opts.enableGraphVerticalRandom {
		candidates = c.sampleSlotCandidates(bg, leftMin, maxX, yLo, yHi, cWidth, cHeight, 40)
	} else {
		// One shared row: pick a textured Y, then sample X along that line.
		rowY := c.pickSharedSlotY(bg, leftMin, maxX, yLo, yHi, cWidth, cHeight)
		candidates = c.sampleSlotCandidatesOnRow(bg, leftMin, maxX, rowY, cWidth, cHeight, 48)
	}
	if len(candidates) == 0 {
		candidates = []slotCand{{x: leftMin, y: yLo}}
	}

	best := 0
	for i := 1; i < len(candidates); i++ {
		if candidates[i].tex > candidates[best].tex {
			best = i
		}
	}
	correct := candidates[best]
	// Force shared Y for slider mode (defensive).
	sharedY := correct.y
	if !c.opts.enableGraphVerticalRandom {
		for i := range candidates {
			candidates[i].y = sharedY
		}
	}

	minSep := minSlotSeparation(c.opts, cWidth)

	placed := make([]slotCand, 0, length)
	placed = append(placed, correct)

	tooClose := func(p slotCand, set []slotCand) bool {
		for _, q := range set {
			dx := p.x - q.x
			dy := p.y - q.y
			if !c.opts.enableGraphVerticalRandom {
				// Same row: enforce horizontal gap only.
				if dx < 0 {
					dx = -dx
				}
				if dx < minSep {
					return true
				}
				continue
			}
			if dx*dx+dy*dy < minSep*minSep {
				return true
			}
		}
		return false
	}

	for len(placed) < length {
		bestI := -1
		bestDiff := math.MaxFloat64
		for i, p := range candidates {
			if tooClose(p, placed) {
				continue
			}
			d := featDistance(p.feat, correct.feat)
			if d < bestDiff {
				bestDiff = d
				bestI = i
			}
		}
		if bestI < 0 {
			farthestI := -1
			farthestD := -1.0
			for i, p := range candidates {
				dup := false
				for _, q := range placed {
					if p.x == q.x && p.y == q.y {
						dup = true
						break
					}
				}
				if dup {
					continue
				}
				minD := math.MaxFloat64
				for _, q := range placed {
					dx := float64(p.x - q.x)
					dy := float64(p.y - q.y)
					dist := math.Hypot(dx, dy)
					if dist < minD {
						minD = dist
					}
				}
				if minD > farthestD {
					farthestD = minD
					farthestI = i
				}
			}
			if farthestI < 0 {
				break
			}
			bestI = farthestI
		}
		placed = append(placed, candidates[bestI])
	}

	blocks := make([]*Block, 0, len(placed))
	for _, p := range placed {
		y := p.y
		if !c.opts.enableGraphVerticalRandom {
			y = sharedY
		}
		blocks = append(blocks, &Block{
			X: p.x, Y: y,
			Width: cWidth, Height: cHeight,
			Angle: randAngle,
		})
	}

	point := &option.Point{
		X: random.RandInt(5, cWidth/2),
		Y: blocks[0].Y,
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

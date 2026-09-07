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
	autoSlotMin = 4
	autoSlotMax = 7
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
	if n < 1 {
		return random.RandInt(autoSlotMin, autoSlotMax)
	}
	return n
}

// Generate generates slide CAPTCHA data with multiple drop slots (default random 4–7).
// All slots share the same silhouette; humans (and bots) must match the tile
// to the background crop — only one position is correct. The secret (X,Y) is
// only available via GetData() — never GetPublicData().
func (c *captcha) Generate() (CaptchaData, error) {
	if err := c.check(); err != nil {
		return nil, err
	}

	nSlots := c.resolveSlotCount()
	size := c.opts.imageSize
	bgSrc := randgen.RandImage(c.resources.rangBackgrounds)
	masterBg := cropMasterBackground(bgSrc, size.Width, size.Height)

	blocks, tilePoint := c.genGraphBlocksSimilar(masterBg, size, c.opts.rangeGraphSize, nSlots)
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
	// Re-rank: ensure the block at correctIdx is the texture-chosen target.
	// genGraphBlocksSimilar puts the correct (tile-source) block at index 0.
	if correctIdx != 0 {
		blocks[0], blocks[correctIdx] = blocks[correctIdx], blocks[0]
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

	masterImage, err := c.genMasterImageOn(masterBg, size, blocks, graphs)
	if err != nil {
		return nil, err
	}

	tileImage, err := c.genTileImage(correct.MaskImage, masterBg, correct.OverlayImage, block)
	if err != nil {
		return nil, err
	}
	tileImage = DistortTile(tileImage)

	// Horizontal slide: tile starts on the left at the same Y as the notches.
	block.TileX = tilePoint.X
	block.DX = tilePoint.X
	block.TileY = block.Y
	block.DY = block.Y

	return &CaptData{
		block:       block,
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

// genGraphBlocksSimilar places one correct slot (high texture) then decoys with
// similar local texture and minimum separation. Index 0 is the tile-source slot
// before Generate shuffles it into correctIdx.
func (c *captcha) genGraphBlocksSimilar(bg image.Image, imageSize *option.Size, size *option.RangeVal, length int) ([]*Block, *option.Point) {
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
	baseY := random.RandInt(yLo, yHi)

	type cand struct {
		x, y int
		tex  float64
	}
	candidates := make([]cand, 0, 64)
	step := cWidth / 4
	if step < 4 {
		step = 4
	}
	for x := leftMin; x <= maxX; x += step {
		y := baseY
		if c.opts.enableGraphVerticalRandom {
			y = random.RandInt(yLo, yHi)
		}
		tex := randgen.TextureScore(bg, x, y, cWidth, cHeight)
		candidates = append(candidates, cand{x: x, y: y, tex: tex})
	}
	// Extra random samples for denser coverage.
	for i := 0; i < 24; i++ {
		x := random.RandInt(leftMin, maxX)
		y := baseY
		if c.opts.enableGraphVerticalRandom {
			y = random.RandInt(yLo, yHi)
		}
		tex := randgen.TextureScore(bg, x, y, cWidth, cHeight)
		candidates = append(candidates, cand{x: x, y: y, tex: tex})
	}
	if len(candidates) == 0 {
		candidates = append(candidates, cand{x: leftMin, y: baseY, tex: 0})
	}

	best := 0
	for i := 1; i < len(candidates); i++ {
		if candidates[i].tex > candidates[best].tex {
			best = i
		}
	}
	correct := candidates[best]
	minSep := cWidth * 2 / 3
	if minSep < 24 {
		minSep = 24
	}

	placed := make([]cand, 0, length)
	placed = append(placed, correct)

	abs := func(v float64) float64 {
		if v < 0 {
			return -v
		}
		return v
	}
	tooClose := func(p cand, set []cand) bool {
		for _, q := range set {
			dx := p.x - q.x
			dy := p.y - q.y
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
			d := abs(p.tex - correct.tex)
			if d < bestDiff {
				bestDiff = d
				bestI = i
			}
		}
		if bestI < 0 {
			// Relax separation and take farthest remaining X.
			farthestI := -1
			farthestD := -1
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
				minD := 1 << 30
				for _, q := range placed {
					dx := p.x - q.x
					if dx < 0 {
						dx = -dx
					}
					if dx < minD {
						minD = dx
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
		blocks = append(blocks, &Block{
			X: p.x, Y: p.y,
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

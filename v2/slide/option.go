/**
 * @Author Awen
 * @Date 2024/06/01
 * @Email wengaolng@gmail.com
 **/

package slide

import (
	"github.com/feerichnii/go-captcha/v2/base/option"
)

// TileDistortConfig controls anti-template transforms on the public tile.
// Geometry of the puzzle piece must stay exact: only photometric changes and
// independent noise are applied by default (no scale/warp/blur deformations).
type TileDistortConfig struct {
	// ScaleDelta is max |scale-1|. Default 0 (disabled) — geometric.
	ScaleDelta float64
	// WarpPxMin / WarpPxMax sinusoidal warp in pixels. Default 0 (disabled).
	WarpPxMin float64
	WarpPxMax float64
	// GammaMin / GammaMax (default 0.95–1.05).
	GammaMin float64
	GammaMax float64
	// BrightnessDelta is max absolute RGB brightness shift (default 6).
	BrightnessDelta int
	// NoiseAmt is max per-channel independent noise (default 3).
	NoiseAmt int
	// SoftBlurProb / SoftSharpenProb — default 0 (disabled; geometric softening).
	SoftBlurProb    float64
	SoftSharpenProb float64
}

// Options .
type Options struct {
	imageSize  *option.Size
	imageAlpha float32

	rangeGraphSize            *option.RangeVal
	rangeGraphAnglePos        []*option.RangeVal
	genGraphNumber            int
	enableGraphVerticalRandom bool

	// candidateSlotsMin/Max used when genGraphNumber < 1 (auto). Default 4–4.
	candidateSlotsMin int
	candidateSlotsMax int
	// minSlotSepPx is minimum center-to-center distance between slots (0 = derive).
	minSlotSepPx int

	tileDistort TileDistortConfig
}

// GetImageSize .
func (o *Options) GetImageSize() *option.Size {
	return &option.Size{
		Width:  o.imageSize.Width,
		Height: o.imageSize.Height,
	}
}

// GetRangeGraphAnglePos .
func (o *Options) GetRangeGraphAnglePos() []*option.RangeVal {
	var rv = make([]*option.RangeVal, len(o.rangeGraphAnglePos))
	for i := 0; i < len(o.rangeGraphAnglePos); i++ {
		rv[i] = &option.RangeVal{
			Min: o.rangeGraphAnglePos[i].Min,
			Max: o.rangeGraphAnglePos[i].Max,
		}
	}
	return rv
}

// GetImageAlpha .
func (o *Options) GetImageAlpha() float32 {
	return o.imageAlpha
}

// GetRangeGraphSize .
func (o *Options) GetRangeGraphSize() *option.RangeVal {
	return &option.RangeVal{
		Min: o.rangeGraphSize.Min,
		Max: o.rangeGraphSize.Max,
	}
}

// GetGenGraphNumber returns the configured slot count (0 = auto random in CandidateSlots range).
func (o *Options) GetGenGraphNumber() int {
	return o.genGraphNumber
}

// GetCandidateSlotsMin returns the auto slot-count lower bound.
func (o *Options) GetCandidateSlotsMin() int {
	return o.candidateSlotsMin
}

// GetCandidateSlotsMax returns the auto slot-count upper bound.
func (o *Options) GetCandidateSlotsMax() int {
	return o.candidateSlotsMax
}

// GetTileDistort returns a copy of the mild tile-transform config.
func (o *Options) GetTileDistort() TileDistortConfig {
	return o.tileDistort
}

type Option func(*Options)

// NewOptions .
func NewOptions() *Options {
	return &Options{}
}

//‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾
// Image
//_______________________________________________________________________

// WithImageSize .
func WithImageSize(val option.Size) Option {
	return func(opts *Options) {
		opts.imageSize = &option.Size{Width: val.Width, Height: val.Height}
	}
}

// WithImageAlpha .
func WithImageAlpha(val float32) Option {
	return func(opts *Options) {
		opts.imageAlpha = val
	}
}

//‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾
// Graph Image
//_______________________________________________________________________

// WithRangeGraphSize .
func WithRangeGraphSize(val option.RangeVal) Option {
	return func(opts *Options) {
		opts.rangeGraphSize = &option.RangeVal{Min: val.Min, Max: val.Max}
	}
}

// WithRangeGraphAnglePos .
func WithRangeGraphAnglePos(vals []option.RangeVal) Option {
	return func(opts *Options) {
		var newVals = make([]*option.RangeVal, 0, len(vals))
		for i := 0; i < len(vals); i++ {
			val := vals[i]
			newVals = append(newVals, &option.RangeVal{Min: val.Min, Max: val.Max})
		}
		opts.rangeGraphAnglePos = newVals
	}
}

// WithGenGraphNumber sets how many drop slots (notches) are drawn on the master
// image. Values < 1 mean auto (CandidateSlotsMin–Max; default exactly 4).
// Only the secret target from GetData() is valid; decoy coordinates are never
// exposed via GetPublicData().
func WithGenGraphNumber(val int) Option {
	return func(opts *Options) {
		if val < 1 {
			val = 0
		}
		opts.genGraphNumber = val
	}
}

// WithCandidateSlots sets the auto random slot-count range (inclusive).
// Invalid ranges are clamped to at least 1.
func WithCandidateSlots(min, max int) Option {
	return func(opts *Options) {
		if min < 1 {
			min = 1
		}
		if max < min {
			max = min
		}
		opts.candidateSlotsMin = min
		opts.candidateSlotsMax = max
	}
}

// WithMinSlotSeparation sets minimum center distance between slots in pixels.
// 0 keeps the derived default (~0.85×tile size, at least 40px).
func WithMinSlotSeparation(px int) Option {
	return func(opts *Options) {
		if px < 0 {
			px = 0
		}
		opts.minSlotSepPx = px
	}
}

// WithTileDistort overrides mild anti-template tile transforms.
func WithTileDistort(cfg TileDistortConfig) Option {
	return func(opts *Options) {
		opts.tileDistort = cfg
	}
}

// WithEnableGraphVerticalRandom widens vertical scatter of drop slots.
// Natural 2D placement is always on; this expands the usable Y band.
func WithEnableGraphVerticalRandom(val bool) Option {
	return func(opts *Options) {
		opts.enableGraphVerticalRandom = val
	}
}

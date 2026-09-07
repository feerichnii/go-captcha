/**
 * @Author Awen
 * @Date 2024/06/01
 * @Email wengaolng@gmail.com
 **/

package slide

import (
	"github.com/feerichnii/go-captcha/v2/base/option"
)

// Options .
type Options struct {
	imageSize  *option.Size
	imageAlpha float32

	rangeGraphSize            *option.RangeVal
	rangeGraphAnglePos        []*option.RangeVal
	genGraphNumber            int
	enableGraphVerticalRandom bool
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

// GetGenGraphNumber returns how many drop slots are drawn (default 3).
func (o *Options) GetGenGraphNumber() int {
	return o.genGraphNumber
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
// image. Default is 3: identical silhouette at each notch, one correct position.
// Only the secret target from GetData() is valid; decoy coordinates are never
// exposed via GetPublicData().
func WithGenGraphNumber(val int) Option {
	return func(opts *Options) {
		if val < 1 {
			val = 1
		}
		opts.genGraphNumber = val
	}
}

// WithEnableGraphVerticalRandom allows each drop slot to pick its own Y.
// Default false: all notches share one Y so a horizontal slider can solve it.
func WithEnableGraphVerticalRandom(val bool) Option {
	return func(opts *Options) {
		opts.enableGraphVerticalRandom = val
	}
}

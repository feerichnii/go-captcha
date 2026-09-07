/**
 * @Author Awen
 * @Date 2024/06/01
 * @Email wengaolng@gmail.com
 **/

package slide

import (
	"github.com/feerichnii/go-captcha/v2/base/option"
)

// defaultOptions is to the default configuration
func defaultOptions() Option {
	return func(opts *Options) {
		opts.imageSize = &option.Size{Width: 300, Height: 220}
		opts.imageAlpha = 1
		// Mild extra Y scatter on top of natural 2D placement.
		opts.enableGraphVerticalRandom = true

		// 0 = auto: random CandidateSlotsMin–Max (identical silhouette; one correct).
		opts.genGraphNumber = 0
		opts.candidateSlotsMin = 4
		opts.candidateSlotsMax = 5
		opts.minSlotSepPx = 0 // derive from tile size

		opts.rangeGraphAnglePos = []*option.RangeVal{
			{Min: -8, Max: 8},
		}
		opts.rangeGraphSize = &option.RangeVal{Min: 60, Max: 70}

		opts.tileDistort = defaultTileDistort()
	}
}

func defaultTileDistort() TileDistortConfig {
	return TileDistortConfig{
		ScaleDelta:      0.02,
		WarpPxMin:       1.0,
		WarpPxMax:       2.0,
		GammaMin:        0.95,
		GammaMax:        1.05,
		BrightnessDelta: 6,
		NoiseAmt:        3,
		SoftBlurProb:    0.35,
		SoftSharpenProb: 0.35,
	}
}

// defaultResource is to the default resource
func defaultResource() Resource {
	return func(resources *Resources) {
		// ...
	}
}

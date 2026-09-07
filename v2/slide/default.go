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
		// Natural 2D scatter for decoys (not a fence line).
		opts.enableGraphVerticalRandom = true

		// Default: exactly 4 candidate holes (1 real + 3 decoy).
		opts.genGraphNumber = 0
		opts.candidateSlotsMin = 4
		opts.candidateSlotsMax = 4
		opts.minSlotSepPx = 0 // derive from tile size

		// No geometric rotation — exact puzzle mask alignment.
		opts.rangeGraphAnglePos = []*option.RangeVal{
			{Min: 0, Max: 0},
		}
		opts.rangeGraphSize = &option.RangeVal{Min: 60, Max: 70}

		opts.tileDistort = defaultTileDistort()
	}
}

// defaultTileDistort is photometric-only (no scale/warp/blur geometry).
func defaultTileDistort() TileDistortConfig {
	return TileDistortConfig{
		ScaleDelta:      0,
		WarpPxMin:       0,
		WarpPxMax:       0,
		GammaMin:        0.95,
		GammaMax:        1.05,
		BrightnessDelta: 6,
		NoiseAmt:        3,
		SoftBlurProb:    0,
		SoftSharpenProb: 0,
	}
}

// defaultResource is to the default resource
func defaultResource() Resource {
	return func(resources *Resources) {
		// ...
	}
}

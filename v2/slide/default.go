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
		opts.enableGraphVerticalRandom = false

		// 0 = auto: random 4–7 drop slots (identical silhouette; one correct).
		opts.genGraphNumber = 0
		opts.rangeGraphAnglePos = []*option.RangeVal{
			{Min: -8, Max: 8},
		}
		opts.rangeGraphSize = &option.RangeVal{Min: 60, Max: 70}
	}
}

// defaultResource is to the default resource
func defaultResource() Resource {
	return func(resources *Resources) {
		// ...
	}
}

/**
 * @Author Awen
 * @Date 2024/06/01
 * @Email wengaolng@gmail.com
 **/

package slide

import "github.com/wenlng/go-captcha/v2/base/challenge"

// DefaultMaxPadding is the recommended upper bound for slide padding (pixels).
const DefaultMaxPadding = 8

// Validate checks if the point position is within the specified range
// params:
//   - sx: Source X coordinate
//   - sy: Source Y coordinate
//   - dx: Target X coordinate
//   - dy: Target Y coordinate
//   - padding: Padding
//
// return: Whether within range
func Validate(sx, sy, dx, dy, padding int) bool {
	padding = challenge.ClampPadding(padding, DefaultMaxPadding)
	newX := padding * 2
	newY := padding * 2
	newDx := dx - padding
	newDy := dy - padding

	return sx >= newDx &&
		sx <= newDx+newX &&
		sy >= newDy &&
		sy <= newDy+newY
}

// Deprecated: As of 2.1.0, it will be removed, please use [slide.Validate]
func CheckPoint(sx, sy, dx, dy, padding int64) bool {
	return Validate(int(sx), int(sy), int(dx), int(dy), int(padding))
}

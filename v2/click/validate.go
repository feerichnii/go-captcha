/**
 * @Author Awen
 * @Date 2024/06/01
 * @Email wengaolng@gmail.com
 **/

package click

import (
	"math"

	"github.com/wenlng/go-captcha/v2/base/challenge"
)

// DefaultMaxPadding is the recommended upper bound for click padding.
const DefaultMaxPadding = 12

// Point is a click coordinate.
type Point struct {
	X, Y int
}

// Validate checks if a click point is within the specified area
// params:
//   - sx, sy: Coordinates of the click point
//   - dx, dy: Top-left coordinates of the target area
//   - width, height: Width and height of the target area
//   - padding: Padding of the area
//
// return: Whether the point is within the area
func Validate(sx, sy, dx, dy, width, height, padding int) bool {
	padding = challenge.ClampPadding(padding, DefaultMaxPadding)
	newWidth := width + (padding * 2)
	newHeight := height + (padding * 2)
	newDx := int(math.Max(float64(dx), float64(dx-padding)))
	newDy := int(math.Max(float64(dy), float64(dy-padding)))

	return sx >= newDx &&
		sx <= newDx+newWidth &&
		sy >= newDy &&
		sy <= newDy+newHeight
}

// ValidateOrdered verifies that clicks match answer dots in order (index 0..n-1).
// points length must equal len(dots). Returns false on any mismatch.
func ValidateOrdered(points []Point, dots map[int]*Dot, padding int) bool {
	if len(points) == 0 || len(points) != len(dots) {
		return false
	}
	padding = challenge.ClampPadding(padding, DefaultMaxPadding)
	for i := 0; i < len(points); i++ {
		dot, ok := dots[i]
		if !ok || dot == nil {
			return false
		}
		if !Validate(points[i].X, points[i].Y, dot.X, dot.Y, dot.Width, dot.Height, padding) {
			return false
		}
	}
	return true
}

// Deprecated: As of 2.1.0, it will be removed, please use [click.Validate]
func CheckPoint(sx, sy, dx, dy, width, height, padding int64) bool {
	return Validate(int(sx), int(sy), int(dx), int(dy), int(width), int(height), int(padding))
}

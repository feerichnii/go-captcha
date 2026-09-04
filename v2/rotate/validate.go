/**
 * @Author Awen
 * @Date 2024/06/01
 * @Email wengaolng@gmail.com
 **/

package rotate

import "github.com/feerichnii/go-captcha/v2/base/challenge"

// DefaultMaxPadding is the recommended upper bound for rotate angle padding (degrees).
const DefaultMaxPadding = 8

// Validate checks if the rotation angle is within the specified range
// params:
//   - angle: Current angle
//   - dAngle: Target angle
//   - padding: Angle padding
//
// return: Whether within range
func Validate(angle, dAngle, padding int) bool {
	padding = challenge.ClampPadding(padding, DefaultMaxPadding)
	minAngle := 360 - padding
	maxAngle := 360 + padding
	angle += dAngle

	return angle >= minAngle && angle <= maxAngle
}

// Deprecated: As of 2.1.0, it will be removed, please use [rotate.Validate]
func CheckAngle(angle, dAngle, padding int64) bool {
	return Validate(int(angle), int(dAngle), int(padding))
}

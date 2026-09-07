package antibot

import (
	"encoding/json"

	"github.com/feerichnii/go-captcha/v2/rotate"
	"github.com/feerichnii/go-captcha/v2/slide"
)

// AnswerChecker validates a submitted answer against the stored (decrypted) secret.
type AnswerChecker func(kind string, stored json.RawMessage, submitted json.RawMessage, tol Tolerance) bool

// Tolerance holds server-side padding values.
type Tolerance struct {
	Slide, Rotate int
}

// Kind constants for ChallengeRecord.Kind.
const (
	KindSlide     = "slide"
	KindRotate    = "rotate"
	KindInvisible = "invisible" // low-risk signals+PoW/JS only (no geometry)
)

func validKind(k string) bool {
	return k == KindSlide || k == KindRotate || k == KindInvisible
}

// SlideSubmit is the client payload for slide captchas.
type SlideSubmit struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// RotateSubmit is the client payload for rotate captchas.
type RotateSubmit struct {
	Angle int `json:"angle"`
}

// DefaultChecker returns a checker for slide / rotate geometry.
func DefaultChecker() AnswerChecker {
	return func(kind string, stored json.RawMessage, submitted json.RawMessage, tol Tolerance) bool {
		switch kind {
		case KindSlide:
			return CheckSlide(stored, submitted, tol.Slide)
		case KindRotate:
			return CheckRotate(stored, submitted, tol.Rotate)
		default:
			return false
		}
	}
}

// CheckSlide compares submitted x/y to stored slide.Block JSON.
func CheckSlide(stored, submitted json.RawMessage, padding int) bool {
	var block slide.Block
	if err := json.Unmarshal(stored, &block); err != nil {
		return false
	}
	var sub SlideSubmit
	if err := json.Unmarshal(submitted, &sub); err != nil {
		return false
	}
	return slide.Validate(sub.X, sub.Y, block.X, block.Y, padding)
}

// CheckRotate compares submitted angle to stored rotate.Block JSON.
func CheckRotate(stored, submitted json.RawMessage, padding int) bool {
	var block rotate.Block
	if err := json.Unmarshal(stored, &block); err != nil {
		return false
	}
	var sub RotateSubmit
	if err := json.Unmarshal(submitted, &sub); err != nil {
		return false
	}
	return rotate.Validate(sub.Angle, block.Angle, padding)
}

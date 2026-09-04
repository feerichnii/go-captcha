package antibot

import (
	"encoding/json"

	"github.com/wenlng/go-captcha/v2/click"
	"github.com/wenlng/go-captcha/v2/rotate"
	"github.com/wenlng/go-captcha/v2/slide"
)

// AnswerChecker validates a submitted answer against the stored (decrypted) secret.
type AnswerChecker func(kind string, stored json.RawMessage, submitted json.RawMessage, tol Tolerance) bool

// Tolerance holds server-side padding values.
type Tolerance struct {
	Click, Slide, Rotate int
}

// Kind constants for ChallengeRecord.Kind.
const (
	KindClick  = "click"
	KindSlide  = "slide"
	KindRotate = "rotate"
)

func validKind(k string) bool {
	return k == KindClick || k == KindSlide || k == KindRotate
}

// ClickSubmit is the client payload for click captchas (ordered points).
type ClickSubmit struct {
	Points []click.Point `json:"points"`
}

// SlideSubmit is the client payload for slide/drag captchas.
type SlideSubmit struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// RotateSubmit is the client payload for rotate captchas.
type RotateSubmit struct {
	Angle int `json:"angle"`
}

// maxClickPoints bounds ClickSubmit to avoid pathological payloads.
const maxClickPoints = 32

// DefaultChecker returns a checker for click / slide / rotate geometry.
func DefaultChecker() AnswerChecker {
	return func(kind string, stored json.RawMessage, submitted json.RawMessage, tol Tolerance) bool {
		switch kind {
		case KindClick:
			return CheckClick(stored, submitted, tol.Click)
		case KindSlide:
			return CheckSlide(stored, submitted, tol.Slide)
		case KindRotate:
			return CheckRotate(stored, submitted, tol.Rotate)
		default:
			return false
		}
	}
}

// CheckClick compares ordered click points to stored map[int]*click.Dot JSON.
func CheckClick(stored, submitted json.RawMessage, padding int) bool {
	var dots map[int]*click.Dot
	if err := json.Unmarshal(stored, &dots); err != nil || len(dots) == 0 {
		return false
	}
	var sub ClickSubmit
	if err := json.Unmarshal(submitted, &sub); err != nil {
		return false
	}
	if len(sub.Points) == 0 || len(sub.Points) > maxClickPoints {
		return false
	}
	return click.ValidateOrdered(sub.Points, dots, padding)
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

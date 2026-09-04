package antibot

import (
	"encoding/json"

	"github.com/wenlng/go-captcha/v2/click"
	"github.com/wenlng/go-captcha/v2/rotate"
	"github.com/wenlng/go-captcha/v2/slide"
)

// AnswerChecker validates a submitted answer against the stored secret.
type AnswerChecker func(kind string, stored json.RawMessage, submitted json.RawMessage) bool

// Kind constants for ChallengeRecord.Kind.
const (
	KindClick  = "click"
	KindSlide  = "slide"
	KindRotate = "rotate"
)

// ClickSubmit is the client payload for click captchas (ordered points).
type ClickSubmit struct {
	Points  []click.Point `json:"points"`
	Padding int           `json:"padding,omitempty"`
}

// SlideSubmit is the client payload for slide/drag captchas.
type SlideSubmit struct {
	X       int `json:"x"`
	Y       int `json:"y"`
	Padding int `json:"padding,omitempty"`
}

// RotateSubmit is the client payload for rotate captchas.
type RotateSubmit struct {
	Angle   int `json:"angle"`
	Padding int `json:"padding,omitempty"`
}

// DefaultChecker returns a checker for click / slide / rotate geometry.
func DefaultChecker() AnswerChecker {
	return func(kind string, stored json.RawMessage, submitted json.RawMessage) bool {
		switch kind {
		case KindClick:
			return CheckClick(stored, submitted)
		case KindSlide:
			return CheckSlide(stored, submitted)
		case KindRotate:
			return CheckRotate(stored, submitted)
		default:
			return false
		}
	}
}

// CheckClick compares ordered click points to stored map[int]*click.Dot JSON.
func CheckClick(stored, submitted json.RawMessage) bool {
	var dots map[int]*click.Dot
	if err := json.Unmarshal(stored, &dots); err != nil || len(dots) == 0 {
		return false
	}
	var sub ClickSubmit
	if err := json.Unmarshal(submitted, &sub); err != nil {
		return false
	}
	padding := sub.Padding
	if padding <= 0 {
		padding = 5
	}
	return click.ValidateOrdered(sub.Points, dots, padding)
}

// CheckSlide compares submitted x/y to stored slide.Block JSON.
func CheckSlide(stored, submitted json.RawMessage) bool {
	var block slide.Block
	if err := json.Unmarshal(stored, &block); err != nil {
		return false
	}
	var sub SlideSubmit
	if err := json.Unmarshal(submitted, &sub); err != nil {
		return false
	}
	padding := sub.Padding
	if padding <= 0 {
		padding = 5
	}
	return slide.Validate(sub.X, sub.Y, block.X, block.Y, padding)
}

// CheckRotate compares submitted angle to stored rotate.Block JSON.
func CheckRotate(stored, submitted json.RawMessage) bool {
	var block rotate.Block
	if err := json.Unmarshal(stored, &block); err != nil {
		return false
	}
	var sub RotateSubmit
	if err := json.Unmarshal(submitted, &sub); err != nil {
		return false
	}
	padding := sub.Padding
	if padding <= 0 {
		padding = 5
	}
	return rotate.Validate(sub.Angle, block.Angle, padding)
}

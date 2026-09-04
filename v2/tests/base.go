package tests

import (
	"image"
	"os"
	"testing"

	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	"github.com/feerichnii/go-captcha/v2/base/codec"
)

// Backgrounds ship in ../resources/backgrounds. Fonts, click shapes and slide
// tile graphs are sample assets that live in the git-ignored fixtureDir; demo
// tests that need them are skipped when it is absent so `go test ./...` stays
// green in CI.
const fixtureDir = "../.cache"

// fixturesAvailable reports whether the optional sample assets are present.
// The directory alone is not enough (tooling may create an empty one), so a
// representative file from each demo is checked.
func fixturesAvailable() bool {
	for _, f := range []string{"yrdzst-bold.ttf", "shape1.png", "tile-1.png"} {
		if _, err := os.Stat(fixtureDir + "/" + f); err != nil {
			return false
		}
	}
	return true
}

func requireFixtures(t *testing.T) {
	t.Helper()
	if !fixturesAvailable() {
		t.Skipf("optional sample assets missing in %s (fonts / shapes / tile graphs)", fixtureDir)
	}
}

func loadPng(p string) (image.Image, error) {
	imgBytes, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	return codec.DecodeByteToPng(imgBytes)
}

func loadFont(p string) (*truetype.Font, error) {
	fontBytes, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	return freetype.ParseFont(fontBytes)
}

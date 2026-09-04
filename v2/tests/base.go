package tests

import (
	"image"
	"os"
	"testing"

	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	"github.com/feerichnii/go-captcha/v2/base/codec"
)

// fixtureDir holds sample fonts/backgrounds (git-ignored). These demo tests
// are skipped when it is absent so `go test ./...` stays green in CI.
const fixtureDir = "../.cache"

func fixturesAvailable() bool {
	_, err := os.Stat(fixtureDir + "/bg.png")
	return err == nil
}

func requireFixtures(t *testing.T) {
	t.Helper()
	if !fixturesAvailable() {
		t.Skipf("fixtures missing in %s; see README for sample assets", fixtureDir)
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

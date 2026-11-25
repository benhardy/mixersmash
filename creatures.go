package main

import (
	"bytes"
	_ "embed"
	"image"
	_ "image/png"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	text "github.com/hajimehoshi/ebiten/v2/text/v2"
)

//go:embed assets/NotoEmoji-Regular.ttf
var notoEmojiTTF []byte

//go:embed assets/mixersmashtitle.png
var mixerSmashTitlePNG []byte

// titleImageDecoded is the CPU-decoded title PNG, available before the GPU context.
var titleImageDecoded image.Image

// titleImage is the GPU-side ebiten.Image, created lazily on first Draw call.
var titleImage *ebiten.Image

func init() {
	img, _, err := image.Decode(bytes.NewReader(mixerSmashTitlePNG))
	if err != nil {
		log.Printf("title image decode error: %v", err)
		return
	}
	titleImageDecoded = img
}

// loadGlyphFont parses the embedded NotoEmoji TTF and returns a GoTextFaceSource.
func loadGlyphFont() (*text.GoTextFaceSource, error) {
	src, err := text.NewGoTextFaceSource(bytes.NewReader(notoEmojiTTF))
	if err != nil {
		return nil, err
	}
	return src, nil
}

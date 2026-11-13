package main

import (
	"bytes"
	_ "embed"
	"image/color"
	"math/rand"

	text "github.com/hajimehoshi/ebiten/v2/text/v2"
)

//go:embed assets/NotoEmoji-Regular.ttf
var notoEmojiTTF []byte

// Creature pairs a display name with its emoji glyph and a unique background colour.
type Creature struct {
	Name  string
	Glyph rune
	Color color.RGBA
}

// creaturePalette is the master list of creatures.  Every Color must be unique.
var creaturePalette = []Creature{
	{Name: "kangaroo", Glyph: 0x1F998, Color: color.RGBA{R: 230, G: 100, B: 40, A: 255}},
	{Name: "kiwi bird", Glyph: 0x1F426, Color: color.RGBA{R: 90, G: 160, B: 70, A: 255}},
	{Name: "koala", Glyph: 0x1F428, Color: color.RGBA{R: 140, G: 140, B: 200, A: 255}},
	{Name: "wombat", Glyph: 0x1F43F, Color: color.RGBA{R: 180, G: 110, B: 60, A: 255}},
	{Name: "pine tree", Glyph: 0x1F332, Color: color.RGBA{R: 30, G: 130, B: 60, A: 255}},
	{Name: "fern", Glyph: 0x1F33F, Color: color.RGBA{R: 60, G: 180, B: 80, A: 255}},
	{Name: "flower", Glyph: 0x1F338, Color: color.RGBA{R: 240, G: 100, B: 160, A: 255}},
	{Name: "cat", Glyph: 0x1F431, Color: color.RGBA{R: 255, G: 180, B: 50, A: 255}},
	{Name: "wolf", Glyph: 0x1F43A, Color: color.RGBA{R: 100, G: 100, B: 160, A: 255}},
	{Name: "goat", Glyph: 0x1F410, Color: color.RGBA{R: 200, G: 190, B: 140, A: 255}},
	{Name: "horse", Glyph: 0x1F434, Color: color.RGBA{R: 160, G: 90, B: 30, A: 255}},
	{Name: "butterfly", Glyph: 0x1F98B, Color: color.RGBA{R: 255, G: 120, B: 200, A: 255}},
	{Name: "cherry", Glyph: 0x1F352, Color: color.RGBA{R: 200, G: 30, B: 50, A: 255}},
	{Name: "banana", Glyph: 0x1F34C, Color: color.RGBA{R: 255, G: 230, B: 50, A: 255}},
	{Name: "snake", Glyph: 0x1F40D, Color: color.RGBA{R: 50, G: 170, B: 50, A: 255}},
	{Name: "dragon", Glyph: 0x1F409, Color: color.RGBA{R: 180, G: 40, B: 200, A: 255}},
	{Name: "rat", Glyph: 0x1F400, Color: color.RGBA{R: 150, G: 130, B: 110, A: 255}},
	{Name: "ghost", Glyph: 0x1F47B, Color: color.RGBA{R: 200, G: 200, B: 240, A: 255}},
	{Name: "rooster", Glyph: 0x1F413, Color: color.RGBA{R: 220, G: 60, B: 20, A: 255}},
	{Name: "fish", Glyph: 0x1F41F, Color: color.RGBA{R: 50, G: 150, B: 220, A: 255}},
	{Name: "dolphin", Glyph: 0x1F42C, Color: color.RGBA{R: 60, G: 180, B: 200, A: 255}},
	{Name: "octopus", Glyph: 0x1F419, Color: color.RGBA{R: 200, G: 80, B: 120, A: 255}},
	{Name: "palm tree", Glyph: 0x1F334, Color: color.RGBA{R: 40, G: 200, B: 130, A: 255}},
	{Name: "monkey", Glyph: 0x1F412, Color: color.RGBA{R: 170, G: 100, B: 40, A: 255}},
	{Name: "bat", Glyph: 0x1F987, Color: color.RGBA{R: 80, G: 50, B: 130, A: 255}},
	{Name: "jellyfish", Glyph: 0x1FABC, Color: color.RGBA{R: 220, G: 150, B: 220, A: 255}},
	{Name: "mosquito", Glyph: 0x1F99F, Color: color.RGBA{R: 110, G: 160, B: 100, A: 255}},
	{Name: "wheat", Glyph: 0x1F33E, Color: color.RGBA{R: 230, G: 190, B: 60, A: 255}},
	{Name: "pig", Glyph: 0x1F416, Color: color.RGBA{R: 255, G: 150, B: 170, A: 255}},
}

// glyphColorMap maps each palette colour to its creature's glyph for O(1) lookup.
var glyphColorMap map[color.RGBA]rune

func init() {
	glyphColorMap = make(map[color.RGBA]rune, len(creaturePalette))
	for _, c := range creaturePalette {
		glyphColorMap[c.Color] = c.Glyph
	}
}

// darkerShade returns c with each channel multiplied by 0.65 (integer floor) and
// alpha forced to 255.
func darkerShade(c color.RGBA) color.RGBA {
	return color.RGBA{
		R: uint8(float64(c.R) * 0.65),
		G: uint8(float64(c.G) * 0.65),
		B: uint8(float64(c.B) * 0.65),
		A: 255,
	}
}

// glyphForColor returns the glyph rune associated with c in creaturePalette.
// Returns rune(0) if c is not found.
func glyphForColor(c color.RGBA) rune {
	return glyphColorMap[c]
}

// paletteColor returns a random colour from creaturePalette.
func paletteColor() color.RGBA {
	return creaturePalette[rand.Intn(len(creaturePalette))].Color
}

// loadGlyphFont parses the embedded NotoEmoji TTF and returns a GoTextFaceSource.
func loadGlyphFont() (*text.GoTextFaceSource, error) {
	src, err := text.NewGoTextFaceSource(bytes.NewReader(notoEmojiTTF))
	if err != nil {
		return nil, err
	}
	return src, nil
}

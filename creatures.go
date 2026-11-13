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
// All R, G, B channels are drawn from the 5-level set {102, 140, 178, 217, 255}.
// Pure white (255,255,255) is excluded.  Alpha is always 255.
var creaturePalette = []Creature{
	// ---- required creatures ----
	{Name: "kangaroo", Glyph: 0x1F998, Color: color.RGBA{R: 217, G: 140, B: 102, A: 255}},
	{Name: "koala", Glyph: 0x1F428, Color: color.RGBA{R: 140, G: 140, B: 178, A: 255}},
	{Name: "parrot", Glyph: 0x1F99C, Color: color.RGBA{R: 102, G: 217, B: 102, A: 255}},
	{Name: "hedgehog", Glyph: 0x1F994, Color: color.RGBA{R: 178, G: 140, B: 102, A: 255}},
	{Name: "pine tree", Glyph: 0x1F332, Color: color.RGBA{R: 102, G: 140, B: 102, A: 255}},
	{Name: "fern", Glyph: 0x1F33F, Color: color.RGBA{R: 102, G: 178, B: 102, A: 255}},
	{Name: "flower", Glyph: 0x1F338, Color: color.RGBA{R: 255, G: 102, B: 178, A: 255}},
	{Name: "cat", Glyph: 0x1F431, Color: color.RGBA{R: 255, G: 178, B: 102, A: 255}},
	{Name: "wolf", Glyph: 0x1F43A, Color: color.RGBA{R: 102, G: 102, B: 140, A: 255}},
	{Name: "goat", Glyph: 0x1F410, Color: color.RGBA{R: 217, G: 217, B: 140, A: 255}},
	{Name: "horse", Glyph: 0x1F434, Color: color.RGBA{R: 178, G: 102, B: 102, A: 255}},
	{Name: "butterfly", Glyph: 0x1F98B, Color: color.RGBA{R: 255, G: 140, B: 255, A: 255}},
	{Name: "cherry", Glyph: 0x1F352, Color: color.RGBA{R: 217, G: 102, B: 102, A: 255}},
	{Name: "banana", Glyph: 0x1F34C, Color: color.RGBA{R: 255, G: 255, B: 102, A: 255}},
	{Name: "snake", Glyph: 0x1F40D, Color: color.RGBA{R: 102, G: 178, B: 140, A: 255}},
	{Name: "dragon", Glyph: 0x1F409, Color: color.RGBA{R: 178, G: 102, B: 217, A: 255}},
	{Name: "rat", Glyph: 0x1F400, Color: color.RGBA{R: 140, G: 140, B: 102, A: 255}},
	{Name: "ghost", Glyph: 0x1F47B, Color: color.RGBA{R: 217, G: 217, B: 255, A: 255}},
	{Name: "rooster", Glyph: 0x1F413, Color: color.RGBA{R: 255, G: 102, B: 102, A: 255}},
	{Name: "fish", Glyph: 0x1F41F, Color: color.RGBA{R: 102, G: 178, B: 255, A: 255}},
	{Name: "dolphin", Glyph: 0x1F42C, Color: color.RGBA{R: 102, G: 217, B: 217, A: 255}},
	{Name: "octopus", Glyph: 0x1F419, Color: color.RGBA{R: 217, G: 102, B: 178, A: 255}},
	{Name: "palm tree", Glyph: 0x1F334, Color: color.RGBA{R: 102, G: 255, B: 140, A: 255}},
	{Name: "monkey", Glyph: 0x1F412, Color: color.RGBA{R: 178, G: 102, B: 140, A: 255}},
	{Name: "bat", Glyph: 0x1F987, Color: color.RGBA{R: 102, G: 102, B: 178, A: 255}},
	{Name: "jellyfish", Glyph: 0x1FABC, Color: color.RGBA{R: 217, G: 140, B: 217, A: 255}},
	{Name: "mosquito", Glyph: 0x1F99F, Color: color.RGBA{R: 140, G: 178, B: 140, A: 255}},
	{Name: "wheat", Glyph: 0x1F33E, Color: color.RGBA{R: 217, G: 178, B: 102, A: 255}},
	{Name: "pig", Glyph: 0x1F416, Color: color.RGBA{R: 255, G: 140, B: 178, A: 255}},

	// ---- additional creatures ----
	{Name: "dog", Glyph: 0x1F436, Color: color.RGBA{R: 178, G: 140, B: 140, A: 255}},
	{Name: "fox", Glyph: 0x1F98A, Color: color.RGBA{R: 255, G: 140, B: 102, A: 255}},
	{Name: "lion", Glyph: 0x1F981, Color: color.RGBA{R: 255, G: 178, B: 140, A: 255}},
	{Name: "tiger", Glyph: 0x1F42F, Color: color.RGBA{R: 255, G: 217, B: 102, A: 255}},
	{Name: "cow", Glyph: 0x1F42E, Color: color.RGBA{R: 217, G: 255, B: 217, A: 255}},
	{Name: "rabbit", Glyph: 0x1F430, Color: color.RGBA{R: 255, G: 217, B: 217, A: 255}},
	{Name: "bear", Glyph: 0x1F43B, Color: color.RGBA{R: 140, G: 102, B: 102, A: 255}},
	{Name: "panda", Glyph: 0x1F43C, Color: color.RGBA{R: 140, G: 255, B: 255, A: 255}},
	{Name: "hamster", Glyph: 0x1F439, Color: color.RGBA{R: 255, G: 217, B: 178, A: 255}},
	{Name: "chicken", Glyph: 0x1F414, Color: color.RGBA{R: 255, G: 255, B: 178, A: 255}},
	{Name: "penguin", Glyph: 0x1F427, Color: color.RGBA{R: 140, G: 178, B: 255, A: 255}},
	{Name: "eagle", Glyph: 0x1F985, Color: color.RGBA{R: 178, G: 217, B: 255, A: 255}},
	{Name: "duck", Glyph: 0x1F986, Color: color.RGBA{R: 140, G: 255, B: 178, A: 255}},
	{Name: "owl", Glyph: 0x1F989, Color: color.RGBA{R: 217, G: 178, B: 140, A: 255}},
	{Name: "frog", Glyph: 0x1F438, Color: color.RGBA{R: 102, G: 217, B: 140, A: 255}},
	{Name: "crocodile", Glyph: 0x1F40A, Color: color.RGBA{R: 140, G: 178, B: 102, A: 255}},
	{Name: "turtle", Glyph: 0x1F422, Color: color.RGBA{R: 102, G: 140, B: 140, A: 255}},
	{Name: "lizard", Glyph: 0x1F98E, Color: color.RGBA{R: 102, G: 140, B: 178, A: 255}},
	{Name: "whale", Glyph: 0x1F433, Color: color.RGBA{R: 102, G: 102, B: 217, A: 255}},
	{Name: "shark", Glyph: 0x1F988, Color: color.RGBA{R: 102, G: 140, B: 255, A: 255}},
	{Name: "crab", Glyph: 0x1F980, Color: color.RGBA{R: 255, G: 102, B: 140, A: 255}},
	{Name: "shrimp", Glyph: 0x1F990, Color: color.RGBA{R: 255, G: 178, B: 178, A: 255}},
	{Name: "squid", Glyph: 0x1F991, Color: color.RGBA{R: 178, G: 102, B: 178, A: 255}},
	{Name: "snail", Glyph: 0x1F40C, Color: color.RGBA{R: 140, G: 217, B: 140, A: 255}},
	{Name: "ant", Glyph: 0x1F41C, Color: color.RGBA{R: 140, G: 102, B: 140, A: 255}},
	{Name: "honeybee", Glyph: 0x1F41D, Color: color.RGBA{R: 255, G: 255, B: 140, A: 255}},
	{Name: "spider", Glyph: 0x1F577, Color: color.RGBA{R: 102, G: 102, B: 102, A: 255}},
	{Name: "scorpion", Glyph: 0x1F982, Color: color.RGBA{R: 217, G: 102, B: 140, A: 255}},
	{Name: "ladybug", Glyph: 0x1F41E, Color: color.RGBA{R: 255, G: 102, B: 217, A: 255}},
	{Name: "elephant", Glyph: 0x1F418, Color: color.RGBA{R: 140, G: 140, B: 217, A: 255}},
	{Name: "rhino", Glyph: 0x1F98F, Color: color.RGBA{R: 178, G: 178, B: 178, A: 255}},
	{Name: "hippo", Glyph: 0x1F99B, Color: color.RGBA{R: 140, G: 217, B: 217, A: 255}},
	{Name: "zebra", Glyph: 0x1F993, Color: color.RGBA{R: 217, G: 255, B: 255, A: 255}},
	{Name: "giraffe", Glyph: 0x1F992, Color: color.RGBA{R: 255, G: 217, B: 140, A: 255}},
	{Name: "camel", Glyph: 0x1F42A, Color: color.RGBA{R: 217, G: 178, B: 178, A: 255}},
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
		R: uint8(float64(c.R) * 0.35),
		G: uint8(float64(c.G) * 0.35),
		B: uint8(float64(c.B) * 0.35),
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

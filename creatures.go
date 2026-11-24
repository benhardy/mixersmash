package main

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	_ "image/png"
	"log"
	"math"
	"math/rand"

	"github.com/hajimehoshi/ebiten/v2"
	text "github.com/hajimehoshi/ebiten/v2/text/v2"
)

//go:embed assets/NotoEmoji-Regular.ttf
var notoEmojiTTF []byte

//go:embed assets/mixersmashtitle.png
var mixerSmashTitlePNG []byte

// titleImage is the decoded title image, loaded once at startup.
var titleImage *ebiten.Image

func init() {
	img, _, err := image.Decode(bytes.NewReader(mixerSmashTitlePNG))
	if err != nil {
		log.Printf("title image decode error: %v", err)
		return
	}
	titleImage = ebiten.NewImageFromImage(img)
}

// Organism pairs a display name with its emoji glyph and a unique background colour.
// Sprite holds the lazily-baked composite *ebiten.Image for this organism; nil
// until the first call to compositeCachedSprite for this organism pointer.
// PairID groups each animal with its matching food (0..10).
// IsAnimal distinguishes the animal (true) from the food (false) in each pair.
type Organism struct {
	Name     string
	Glyph    rune
	Color    color.RGBA
	Sprite   *ebiten.Image
	PairID   int
	IsAnimal bool
}

// organismPalette is the master list of 22 Australian animal/food pair organisms.
// Each consecutive pair shares a PairID and Color; one entry is the animal (IsAnimal=true),
// the other is the food (IsAnimal=false).
var organismPalette = []Organism{
	{Name: "Kangaroo", Glyph: '🦘', Color: color.RGBA{R: 255, G: 100, B: 111, A: 255}, PairID: 0, IsAnimal: true},
	{Name: "Grass", Glyph: '🌿', Color: color.RGBA{R: 255, G: 100, B: 111, A: 255}, PairID: 0, IsAnimal: false},
	{Name: "Koala", Glyph: '🐨', Color: color.RGBA{R: 255, G: 119, B: 0, A: 255}, PairID: 1, IsAnimal: true},
	{Name: "Eucalyptus Leaf", Glyph: '🍃', Color: color.RGBA{R: 255, G: 119, B: 0, A: 255}, PairID: 1, IsAnimal: false},
	{Name: "Echidna", Glyph: '🦔', Color: color.RGBA{R: 220, G: 150, B: 0, A: 255}, PairID: 2, IsAnimal: true},
	{Name: "Ant", Glyph: '🐜', Color: color.RGBA{R: 220, G: 150, B: 0, A: 255}, PairID: 2, IsAnimal: false},
	{Name: "Cockatoo", Glyph: '🦜', Color: color.RGBA{R: 155, G: 179, B: 0, A: 255}, PairID: 3, IsAnimal: true},
	{Name: "Seeds", Glyph: '🌾', Color: color.RGBA{R: 155, G: 179, B: 0, A: 255}, PairID: 3, IsAnimal: false},
	{Name: "Rainbow Lorikeet", Glyph: '🐦', Color: color.RGBA{R: 0, G: 198, B: 93, A: 255}, PairID: 4, IsAnimal: true},
	{Name: "Nectar", Glyph: '🌺', Color: color.RGBA{R: 0, G: 198, B: 93, A: 255}, PairID: 4, IsAnimal: false},
	{Name: "Magpie", Glyph: '🦆', Color: color.RGBA{R: 0, G: 202, B: 178, A: 255}, PairID: 5, IsAnimal: true},
	{Name: "Lizard", Glyph: '🦎', Color: color.RGBA{R: 0, G: 202, B: 178, A: 255}, PairID: 5, IsAnimal: false},
	{Name: "Flying Fox", Glyph: '🦇', Color: color.RGBA{R: 0, G: 191, B: 244, A: 255}, PairID: 6, IsAnimal: true},
	{Name: "Native Figs", Glyph: '🍈', Color: color.RGBA{R: 0, G: 191, B: 244, A: 255}, PairID: 6, IsAnimal: false},
	{Name: "Crocodile", Glyph: '🐊', Color: color.RGBA{R: 0, G: 169, B: 255, A: 255}, PairID: 7, IsAnimal: true},
	{Name: "Barramundi", Glyph: '🐟', Color: color.RGBA{R: 0, G: 169, B: 255, A: 255}, PairID: 7, IsAnimal: false},
	{Name: "Blue-tongued Lizard", Glyph: '🐸', Color: color.RGBA{R: 148, G: 143, B: 255, A: 255}, PairID: 8, IsAnimal: true},
	{Name: "Beetle", Glyph: '\U0001FAB2', Color: color.RGBA{R: 148, G: 143, B: 255, A: 255}, PairID: 8, IsAnimal: false},
	{Name: "Galah", Glyph: '🦩', Color: color.RGBA{R: 210, G: 119, B: 250, A: 255}, PairID: 9, IsAnimal: true},
	{Name: "Roots", Glyph: '🥕', Color: color.RGBA{R: 210, G: 119, B: 250, A: 255}, PairID: 9, IsAnimal: false},
	{Name: "Kookaburra", Glyph: '🦅', Color: color.RGBA{R: 249, G: 102, B: 188, A: 255}, PairID: 10, IsAnimal: true},
	{Name: "Worms", Glyph: '\U0001FAB1', Color: color.RGBA{R: 249, G: 102, B: 188, A: 255}, PairID: 10, IsAnimal: false},
}

// paletteOrganism returns a pointer to a random entry in organismPalette.
// Spawn probability is uniform across all entries.
// The pointer is stable because organismPalette is a package-level slice
// that is never reallocated after init.
func paletteOrganism() *Organism {
	return &organismPalette[rand.Intn(len(organismPalette))]
}

// sRGBToOKLab converts a colour.RGBA to OKLab [L, a, b].
// Used by glyphTint to determine contrast colour.
// Reference: https://bottosson.github.io/posts/oklab/
func sRGBToOKLab(c color.RGBA) [3]float64 {
	// Linear sRGB (gamma expand).
	toLinear := func(u uint8) float64 {
		s := float64(u) / 255.0
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	r := toLinear(c.R)
	g := toLinear(c.G)
	b := toLinear(c.B)

	// Linear sRGB → LMS (OKLab M1 matrix).
	l := 0.4122214708*r + 0.5363325363*g + 0.0514459929*b
	m := 0.2119034982*r + 0.6806995451*g + 0.1073969566*b
	s := 0.0883024619*r + 0.2817188376*g + 0.6299787005*b

	// Cube root.
	lc := math.Cbrt(l)
	mc := math.Cbrt(m)
	sc := math.Cbrt(s)

	// LMS → OKLab (OKLab M2 matrix).
	L := 0.2104542553*lc + 0.7936177850*mc - 0.0040720468*sc
	a := 1.9779984951*lc - 2.4285922050*mc + 0.4505937099*sc
	bk := 0.0259040371*lc + 0.7827717662*mc - 0.8086757660*sc

	return [3]float64{L, a, bk}
}

// glyphTint returns a near-black or near-white tint for a glyph drawn over c,
// chosen by OKLab luminance: L >= 0.65 → near-black {30,25,20,255};
// L < 0.65 → near-white {230,225,215,255}. Always A=255.
func glyphTint(c color.RGBA) color.RGBA {
	lab := sRGBToOKLab(c)
	if lab[0] >= 0.65 {
		return color.RGBA{R: 30, G: 25, B: 20, A: 255}
	}
	return color.RGBA{R: 230, G: 225, B: 215, A: 255}
}

// loadGlyphFont parses the embedded NotoEmoji TTF and returns a GoTextFaceSource.
func loadGlyphFont() (*text.GoTextFaceSource, error) {
	src, err := text.NewGoTextFaceSource(bytes.NewReader(notoEmojiTTF))
	if err != nil {
		return nil, err
	}
	return src, nil
}

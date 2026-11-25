package board

import (
	"image/color"
	"math"
	"math/rand"
)

const (
	mixerBias = 2
)

// Organism pairs a display name with its emoji glyph and a unique background colour.
// PairID groups each animal with its matching food (0..10).
// IsAnimal distinguishes the animal (true) from the food (false) in each pair.
// The lazily-baked composite sprite for an organism is held separately in the
// render layer (Game.spriteCache), keyed by organism pointer, so this type
// carries no rendering handles.
type Organism struct {
	name     string
	glyph    rune
	color    color.RGBA
	pairID   int
	isAnimal bool
	isMixer  bool
}

// NewOrganism constructs an Organism with the given attributes.
// Use this from outside package board where struct-literal construction is not possible.
func NewOrganism(name string, glyph rune, clr color.RGBA, pairID int, isAnimal, isMixer bool) *Organism {
	return &Organism{name: name, glyph: glyph, color: clr, pairID: pairID, isAnimal: isAnimal, isMixer: isMixer}
}

// Name returns the organism's display name.
func (o *Organism) Name() string { return o.name }

// Glyph returns the organism's emoji rune.
func (o *Organism) Glyph() rune { return o.glyph }

// Color returns the organism's background colour.
func (o *Organism) Color() color.RGBA { return o.color }

// PairID returns the pair identifier grouping each animal with its food (0..10;
// -1 for the Mixer).
func (o *Organism) PairID() int { return o.pairID }

// IsAnimal reports whether this organism is the animal (true) or the food (false)
// in its pair.
func (o *Organism) IsAnimal() bool { return o.isAnimal }

// IsMixer reports whether this organism is the Mixer entry.
func (o *Organism) IsMixer() bool { return o.isMixer }

// organismPalette is the master list of organisms: 22 Australian animal/food
// pair entries (PairIDs 0..10) plus one Mixer entry (PairID -1, IsMixer=true).
// Each animal/food pair shares a PairID and Color; one entry is the animal
// (IsAnimal=true), the other is the food (IsAnimal=false).
var organismPalette = []Organism{
	{name: "Kangaroo", glyph: '🦘', color: color.RGBA{R: 255, G: 100, B: 111, A: 255}, pairID: 0, isAnimal: true},
	{name: "Grass", glyph: '🌿', color: color.RGBA{R: 255, G: 100, B: 111, A: 255}, pairID: 0, isAnimal: false},
	{name: "Koala", glyph: '🐨', color: color.RGBA{R: 255, G: 119, B: 0, A: 255}, pairID: 1, isAnimal: true},
	{name: "Eucalyptus Leaf", glyph: '🍃', color: color.RGBA{R: 255, G: 119, B: 0, A: 255}, pairID: 1, isAnimal: false},
	{name: "Echidna", glyph: '🦔', color: color.RGBA{R: 220, G: 150, B: 0, A: 255}, pairID: 2, isAnimal: true},
	{name: "Ant", glyph: '🐜', color: color.RGBA{R: 220, G: 150, B: 0, A: 255}, pairID: 2, isAnimal: false},
	{name: "Cockatoo", glyph: '🦜', color: color.RGBA{R: 155, G: 179, B: 0, A: 255}, pairID: 3, isAnimal: true},
	{name: "Seeds", glyph: '🌾', color: color.RGBA{R: 155, G: 179, B: 0, A: 255}, pairID: 3, isAnimal: false},
	{name: "Rainbow Lorikeet", glyph: '🐦', color: color.RGBA{R: 0, G: 198, B: 93, A: 255}, pairID: 4, isAnimal: true},
	{name: "Nectar", glyph: '🌺', color: color.RGBA{R: 0, G: 198, B: 93, A: 255}, pairID: 4, isAnimal: false},
	{name: "Magpie", glyph: '🦆', color: color.RGBA{R: 0, G: 202, B: 178, A: 255}, pairID: 5, isAnimal: true},
	{name: "Lizard", glyph: '🦎', color: color.RGBA{R: 0, G: 202, B: 178, A: 255}, pairID: 5, isAnimal: false},
	{name: "Flying Fox", glyph: '🦇', color: color.RGBA{R: 0, G: 191, B: 244, A: 255}, pairID: 6, isAnimal: true},
	{name: "Native Figs", glyph: '🍈', color: color.RGBA{R: 0, G: 191, B: 244, A: 255}, pairID: 6, isAnimal: false},
	{name: "Crocodile", glyph: '🐊', color: color.RGBA{R: 0, G: 169, B: 255, A: 255}, pairID: 7, isAnimal: true},
	{name: "Barramundi", glyph: '🐟', color: color.RGBA{R: 0, G: 169, B: 255, A: 255}, pairID: 7, isAnimal: false},
	{name: "Blue-tongued Lizard", glyph: '🐸', color: color.RGBA{R: 148, G: 143, B: 255, A: 255}, pairID: 8, isAnimal: true},
	{name: "Beetle", glyph: '\U0001FAB2', color: color.RGBA{R: 148, G: 143, B: 255, A: 255}, pairID: 8, isAnimal: false},
	{name: "Galah", glyph: '🦩', color: color.RGBA{R: 210, G: 119, B: 250, A: 255}, pairID: 9, isAnimal: true},
	{name: "Roots", glyph: '🥕', color: color.RGBA{R: 210, G: 119, B: 250, A: 255}, pairID: 9, isAnimal: false},
	{name: "Kookaburra", glyph: '🦅', color: color.RGBA{R: 249, G: 102, B: 188, A: 255}, pairID: 10, isAnimal: true},
	{name: "Worms", glyph: '\U0001FAB1', color: color.RGBA{R: 249, G: 102, B: 188, A: 255}, pairID: 10, isAnimal: false},
	{name: "Mixer", glyph: '\U0001F300', color: color.RGBA{R: 255, G: 255, B: 255, A: 255}, pairID: -1, isAnimal: false, isMixer: true},
}

// PaletteOrganism returns a pointer to a random entry in organismPalette.
// Spawn probability is uniform across all entries.
// The pointer is stable because organismPalette is a package-level slice
// that is never reallocated after init.
func PaletteOrganism() *Organism {
	which := rand.Intn(len(organismPalette) + mixerBias)
	if which >= len(organismPalette) {
		which = len(organismPalette) - 1 // Mixer entry with bias
	}
	return &organismPalette[which]
}

// PaletteOrganismAt returns a pointer to the palette entry at index idx.
// The pointer is stable — organismPalette is never reallocated after init —
// so it is safe to use as a sprite-cache key.
// Panics if idx is out of range.
func PaletteOrganismAt(idx int) *Organism {
	return &organismPalette[idx]
}

// PaletteSize returns the number of entries in the organism palette.
func PaletteSize() int {
	return len(organismPalette)
}

// SRGBToOKLab converts a colour.RGBA to OKLab [L, a, b].
// Used by GlyphTint to determine contrast colour.
// Reference: https://bottosson.github.io/posts/oklab/
func SRGBToOKLab(c color.RGBA) [3]float64 {
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

// GlyphTint returns a near-black or near-white tint for a glyph drawn over c,
// chosen by OKLab luminance: L >= 0.65 → near-black {30,25,20,255};
// L < 0.65 → near-white {230,225,215,255}. Always A=255.
func GlyphTint(c color.RGBA) color.RGBA {
	lab := SRGBToOKLab(c)
	if lab[0] >= 0.65 {
		return color.RGBA{R: 30, G: 25, B: 20, A: 255}
	}
	return color.RGBA{R: 230, G: 225, B: 215, A: 255}
}

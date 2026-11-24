//go:build ignore

// gen_palette generates the organism colour palette for the new power-tier scheme.
//
// Output: 13 colours as color.RGBA literals printed to stdout (paste into
// organismPalette in creatures.go). Optionally also writes a 640x480 PNG
// swatch sheet showing every generated colour as a filled rectangle.
//
// Usage:
//
//	go run tools/gen_palette/main.go [output.png]
//
// If output.png is supplied the swatch sheet is written there. If omitted the
// palette literals are still printed to stdout.
//
//  1. Near-black (power 8, 1 creature): #334433
//     2–4. 3 vivid hues at 0°,120°,240° of a 6-hue OKLCh wheel (power 5, 3 creatures)
//     5–7. 3 vivid hues at 60°,180°,300° of that same wheel (power 3, 6 creatures)
//     8–13. 6 pastel hues at the in-between angles (power 2, 13 creatures, round-robin)
//
// OKLCh parameters for the 6 saturated hues:
//
//	Base hue offset: 30° (keeps hue 0°+30°=30° in orange-red, visually warm anchor)
//	Lightness L = 0.72, Chroma C = 0.22
//
// OKLCh → OKLab: a = C*cos(h_rad), b = C*sin(h_rad)
// OKLab → linear sRGB (Ottosson matrix):
//
//	l_ = L + 0.3963377774*a + 0.2158037573*b
//	m_ = L - 0.1055613458*a - 0.0638541728*b
//	s_ = L - 0.0894841775*a - 1.2914855480*b
//	l  = l_^3; m = m_^3; s = s_^3
//	R  = +4.0767416621*l - 3.3077115913*m + 0.2309699292*s
//	G  = -1.2684380046*l + 2.6097574011*m - 0.3413193965*s
//	B  = -0.0041960863*l - 0.7034186147*m + 1.7076147010*s
//
// Linear → sRGB gamma: c <= 0.0031308 ? 12.92*c : 1.055*c^(1/2.4) - 0.055
// Clamp [0,1], multiply by 255, round to uint8.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
)

func oklchToRGBA(L, C, hDeg float64) (r, g, b uint8) {
	hRad := hDeg * math.Pi / 180.0
	a := C * math.Cos(hRad)
	bLab := C * math.Sin(hRad)

	// OKLab → LMS (cube-root space)
	l_ := L + 0.3963377774*a + 0.2158037573*bLab
	m_ := L - 0.1055613458*a - 0.0638541728*bLab
	s_ := L - 0.0894841775*a - 1.2914855480*bLab

	l := l_ * l_ * l_
	m := m_ * m_ * m_
	s := s_ * s_ * s_

	// LMS → linear sRGB
	R := +4.0767416621*l - 3.3077115913*m + 0.2309699292*s
	G := -1.2684380046*l + 2.6097574011*m - 0.3413193965*s
	B := -0.0041960863*l - 0.7034186147*m + 1.7076147010*s

	// Linear → gamma-corrected sRGB
	linearToSRGB := func(c float64) float64 {
		if c <= 0.0031308 {
			return 12.92 * c
		}
		return 1.055*math.Pow(c, 1.0/2.4) - 0.055
	}

	R = linearToSRGB(R)
	G = linearToSRGB(G)
	B = linearToSRGB(B)

	clamp := func(c float64) float64 {
		if c < 0 {
			return 0
		}
		if c > 1 {
			return 1
		}
		return c
	}
	R = clamp(R)
	G = clamp(G)
	B = clamp(B)

	return uint8(math.Round(R * 255)), uint8(math.Round(G * 255)), uint8(math.Round(B * 255))
}

type entry struct {
	r, g, b uint8
	label   string
}

func main() {
	// Fixed OKLCh parameters for the 6 saturated hues.
	// L=0.72 is bright enough to look vivid on a dark board; C=0.17 gives noticeable
	// saturation without clipping too aggressively.
	const L = 0.72
	const C = 0.22
	// Base offset so the first hue sits at 30° (warm orange-red anchor).
	const baseHue = 30.0

	// 6-hue wheel evenly spaced 60° apart, starting at baseHue.
	// power-5 creatures use hues at indices 0,2,4 (0°,120°,240° relative).
	// power-3 creatures use hues at indices 1,3,5 (60°,180°,300° relative).
	hues := [6]float64{
		baseHue + 0,   // 30°
		baseHue + 60,  // 90°
		baseHue + 120, // 150°
		baseHue + 180, // 210°
		baseHue + 240, // 270°
		baseHue + 300, // 330°
	}

	var palette []entry

	// 1. Near-black — power 8 (1 creature)
	palette = append(palette, entry{0x33, 0x44, 0x33, "power-8 / near-black #334433"})

	// 2–4. Power-5 hues: indices 0,2,4 → 30°,150°,270°
	power5Labels := []string{
		fmt.Sprintf("power-5 / hue 30° (%.0f°)", hues[0]),
		fmt.Sprintf("power-5 / hue 150° (%.0f°)", hues[2]),
		fmt.Sprintf("power-5 / hue 270° (%.0f°)", hues[4]),
	}
	for i, idx := range []int{0, 2, 4} {
		r, g, b := oklchToRGBA(L, C, hues[idx])
		palette = append(palette, entry{r, g, b, power5Labels[i]})
	}

	// 5–7. Power-3 hues: indices 1,3,5 → 90°,210°,330°
	power3Labels := []string{
		fmt.Sprintf("power-3 / hue 90° (%.0f°)", hues[1]),
		fmt.Sprintf("power-3 / hue 210° (%.0f°)", hues[3]),
		fmt.Sprintf("power-3 / hue 330° (%.0f°)", hues[5]),
	}
	for i, idx := range []int{1, 3, 5} {
		r, g, b := oklchToRGBA(L, C, hues[idx])
		palette = append(palette, entry{r, g, b, power3Labels[i]})
	}

	fmt.Println("// Organism colour palette — 13 entries (1 dark + 6 vivid + 6 pastel).")
	fmt.Println("// Paste into organismPalette in creatures.go.")
	fmt.Printf("// OKLCh parameters for saturated hues: L=%.2f, C=%.2f, base hue=%.0f°\n", L, C, baseHue)
	fmt.Println()
	for _, e := range palette {
		fmt.Printf("color.RGBA{R: %3d, G: %3d, B: %3d, A: 255}, // %s\n",
			e.r, e.g, e.b, e.label)
	}

	// 6 pastel hues at the "in-between" hues: halfway between the vivid hues.
	// vivid hues: 30°,90°,150°,210°,270°,330° → in-between: 60°,120°,180°,240°,300°,0°(360°).
	// OKLCh: L=0.91 (high lightness), C=0.12 (moderate chroma — survives bevel shader desaturation).
	const Lp = 0.97
	const Cp = 0.04

	pastelHues := [6]float64{
		baseHue + 30,  // 60°
		baseHue + 90,  // 120°
		baseHue + 150, // 180°
		baseHue + 210, // 240°
		baseHue + 270, // 300°
		baseHue + 330, // 360° (= 0°)
	}

	fmt.Println()
	fmt.Printf("// 6 pastel hues for power-2 organisms: L=%.2f, C=%.2f, in-between hues.\n", Lp, Cp)

	var pastels []entry
	for i, h := range pastelHues {
		r, g, b := oklchToRGBA(Lp, Cp, h)
		lbl := fmt.Sprintf("pastel %d / hue %.0f°", i+1, h)
		fmt.Printf("color.RGBA{R: %3d, G: %3d, B: %3d, A: 255}, // %s\n",
			r, g, b, lbl)
		pastels = append(pastels, entry{r, g, b, lbl})
	}

	// PNG swatch sheet — only written when a path is supplied.
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: go run tools/gen_palette/main.go <output.png>")
		fmt.Fprintln(os.Stderr, "  (no output.png given; palette literals printed to stdout only)")
		os.Exit(1)
	}

	outPath := os.Args[1]
	if err := writeSwatch(outPath, palette, pastels); err != nil {
		fmt.Fprintf(os.Stderr, "error writing swatch: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", outPath)
}

// writeSwatch builds a 640×480 PNG with three tier rows:
//
//	Row 0 (160px): 1 dark swatch centred horizontally
//	Row 1 (160px): 6 vivid hues (power-5 at [1..3], power-3 at [4..6])
//	Row 2 (160px): 6 pastel hues
//
// Each cell is padded by 4px on each side so colour boundaries are visible.
func writeSwatch(path string, palette []entry, pastels []entry) error {
	const (
		imgW    = 640
		imgH    = 480
		rows    = 3
		rowH    = imgH / rows // 160
		padding = 4
	)

	img := image.NewRGBA(image.Rect(0, 0, imgW, imgH))

	// Background: dark grey
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{R: 30, G: 30, B: 30, A: 255}), image.Point{}, draw.Src)

	fillRect := func(x, y, w, h int, c color.RGBA) {
		r := image.Rect(x+padding, y+padding, x+w-padding, y+h-padding)
		draw.Draw(img, r, image.NewUniform(c), image.Point{}, draw.Src)
	}

	// Row 0: single dark swatch centred.
	darkC := color.RGBA{R: palette[0].r, G: palette[0].g, B: palette[0].b, A: 255}
	cellW := imgW / 6 // same width as a vivid cell so it aligns visually
	startX := (imgW - cellW) / 2
	fillRect(startX, 0, cellW, rowH, darkC)

	// Row 1: 6 vivid hues (palette[1..6]).
	vivid := palette[1:7]
	cellWV := imgW / len(vivid)
	for i, e := range vivid {
		c := color.RGBA{R: e.r, G: e.g, B: e.b, A: 255}
		fillRect(i*cellWV, rowH, cellWV, rowH, c)
	}

	// Row 2: 6 pastel hues.
	cellWP := imgW / len(pastels)
	for i, e := range pastels {
		c := color.RGBA{R: e.r, G: e.g, B: e.b, A: 255}
		fillRect(i*cellWP, 2*rowH, cellWP, rowH, c)
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

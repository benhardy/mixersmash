//go:build ignore

// gen_palette generates the organism colour palette for the 11 animal/food pairs.
//
// Output: 22 color.RGBA literals printed to stdout (two per pair, same colour),
// suitable for pasting into the 22-entry palette in creatures.go.
// Optionally writes an 11-column x 1-row PNG swatch.
//
// Usage:
//
//	go run tools/gen_palette/main.go [output.png]
//
// 11 hues, one per pair (PairID 0..10), evenly spaced in OKLCh:
//
//	hue = baseHue + i*(360/11), baseHue = 20°, L = 0.72, C = 0.20
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

type pairColor struct {
	r, g, b uint8
	hue     float64
	clipped bool
}

// oklchToRGBA converts OKLCh to sRGB uint8 components, clamping out-of-gamut values.
// clipped reports whether any channel was out-of-gamut before clamping.
func oklchToRGBA(L, C, hDeg float64) (r, g, b uint8, clipped bool) {
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

	wasClipped := R < 0 || R > 1 || G < 0 || G > 1 || B < 0 || B > 1

	clampF := func(c float64) float64 {
		if c < 0 {
			return 0
		}
		if c > 1 {
			return 1
		}
		return c
	}
	R = clampF(R)
	G = clampF(G)
	B = clampF(B)

	return uint8(math.Round(R * 255)), uint8(math.Round(G * 255)), uint8(math.Round(B * 255)), wasClipped
}

func main() {
	const (
		numPairs = 11
		L        = 0.72
		C        = 0.20
		baseHue  = 20.0
		step     = 360.0 / numPairs
	)

	pairs := make([]pairColor, numPairs)
	for i := 0; i < numPairs; i++ {
		h := baseHue + float64(i)*step
		r, g, b, clip := oklchToRGBA(L, C, h)
		pairs[i] = pairColor{r, g, b, h, clip}
	}

	fmt.Println("// Organism colour palette — 22 entries (11 pairs, same colour per pair).")
	fmt.Println("// Paste into organismPalette in creatures.go.")
	fmt.Printf("// OKLCh parameters: L=%.2f, C=%.2f, baseHue=%.0f°, step=%.4f°\n", L, C, baseHue, step)
	fmt.Println()
	for i, p := range pairs {
		fmt.Printf("color.RGBA{%3d, %3d, %3d, 255}, // pair %2d animal (hue %.1f°)\n", p.r, p.g, p.b, i, p.hue)
		fmt.Printf("color.RGBA{%3d, %3d, %3d, 255}, // pair %2d food  (hue %.1f°)\n", p.r, p.g, p.b, i, p.hue)
	}

	// PNG swatch — only written when a path is supplied.
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "no output.png given; palette literals printed to stdout only")
		return
	}

	outPath := os.Args[1]
	if err := writeSwatch(outPath, pairs); err != nil {
		fmt.Fprintf(os.Stderr, "error writing swatch: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", outPath)
}

// writeSwatch builds an 11-column x 1-row PNG swatch, one cell per pair hue.
func writeSwatch(path string, pairs []pairColor) error {
	const (
		cellW   = 80
		cellH   = 80
		padding = 4
	)
	n := len(pairs)
	imgW := n * cellW
	imgH := cellH

	img := image.NewRGBA(image.Rect(0, 0, imgW, imgH))

	// Background: dark grey
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{R: 30, G: 30, B: 30, A: 255}), image.Point{}, draw.Src)

	for i, p := range pairs {
		c := color.RGBA{R: p.r, G: p.g, B: p.b, A: 255}
		x := i * cellW
		r := image.Rect(x+padding, padding, x+cellW-padding, cellH-padding)
		draw.Draw(img, r, image.NewUniform(c), image.Point{}, draw.Src)
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

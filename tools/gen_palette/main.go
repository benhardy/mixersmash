//go:build ignore

// gen_palette generates 23 OKLCh-derived sRGB colours for the creature palette.
//
// Parameters:
//   - 12 evenly-spaced hues starting at 15°: 15,45,75,105,135,165,195,225,255,285,315,345
//   - Chroma C = 0.13
//   - Lightness band L1 = 0.68  → 12 colours (one per hue)
//   - Lightness band L2 = 0.52  → 11 colours (drop hue index 11, i.e. 345°)
//     Total = 23 unique colours.
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
	"math"
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

func main() {
	const C = 0.13
	const L1 = 0.68
	const L2 = 0.52

	hues := [12]float64{15, 45, 75, 105, 135, 165, 195, 225, 255, 285, 315, 345}

	type colour struct {
		r, g, b uint8
		L, h    float64
	}

	var colours []colour

	// Band L1: all 12 hues
	for _, h := range hues {
		r, g, b := oklchToRGBA(L1, C, h)
		colours = append(colours, colour{r, g, b, L1, h})
	}

	// Band L2: 11 hues (drop hue index 11 = 345°)
	for i, h := range hues {
		if i == 11 {
			break
		}
		r, g, b := oklchToRGBA(L2, C, h)
		colours = append(colours, colour{r, g, b, L2, h})
	}

	fmt.Printf("Total colours: %d\n\n", len(colours))

	// Check for duplicates
	seen := make(map[[3]uint8]int)
	for i, c := range colours {
		key := [3]uint8{c.r, c.g, c.b}
		if prev, ok := seen[key]; ok {
			fmt.Printf("COLLISION: index %d and index %d both = {%d,%d,%d} L=%.2f h=%.0f\n",
				prev, i, c.r, c.g, c.b, c.L, c.h)
		}
		seen[key] = i
	}

	fmt.Println("// OKLCh-derived sRGB colours:")
	fmt.Println("// L1=0.68, L2=0.52, C=0.13")
	fmt.Println("// Hues (degrees): 15,45,75,105,135,165,195,225,255,285,315,345")
	fmt.Println("// Band L1 (indices 0-11): all 12 hues")
	fmt.Println("// Band L2 (indices 12-22): hues 15-315 (11 hues, 345° dropped)")
	fmt.Println()
	for i, c := range colours {
		hex := fmt.Sprintf("#%02X%02X%02X", c.r, c.g, c.b)
		fmt.Printf("// [%2d] L=%.2f h=%5.0f° %s  → color.RGBA{R: %3d, G: %3d, B: %3d, A: 255}\n",
			i, c.L, c.h, hex, c.r, c.g, c.b)
	}
}

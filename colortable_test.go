package main

import (
	"fmt"
	"image/color"
	"testing"
)

// hexRGB formats a color.RGBA as a 6-digit uppercase hex string (#RRGGBB).
func hexRGB(c color.RGBA) string {
	return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
}

// nearestOther returns the index of the palette entry whose colour is nearest
// to creaturePalette[idx] under the given distance function, excluding idx itself.
func nearestOther(idx int, distFn func(color.RGBA, color.RGBA) float32) int {
	target := creaturePalette[idx].Color
	best := -1
	var bestDist float32
	for i, e := range creaturePalette {
		if i == idx {
			continue
		}
		d := distFn(target, e.Color)
		if best < 0 || d < bestDist {
			bestDist = d
			best = i
		}
	}
	return best
}

// TestColorDistanceTables prints a side-by-side comparison of nearest-palette
// neighbours under colorDistanceRedmean and colorDistanceHSV for every entry
// in creaturePalette.  It is assertion-free and only produces output under -v,
// so it does not affect normal test runs or pre-commit hooks.
func TestColorDistanceTables(t *testing.T) {
	type row struct {
		name       string
		hex        string
		redmeanIdx int
		hsvIdx     int
	}

	rows := make([]row, len(creaturePalette))
	for i, e := range creaturePalette {
		rows[i] = row{
			name:       e.Name,
			hex:        hexRGB(e.Color),
			redmeanIdx: nearestOther(i, colorDistanceRedmean),
			hsvIdx:     nearestOther(i, colorDistanceHSV),
		}
	}

	// Print header.
	t.Logf("%-16s %-8s  %-16s %-8s  %-16s %-8s  %s",
		"CREATURE", "HEX",
		"REDMEAN-NEAR", "HEX",
		"HSV-NEAR", "HEX",
		"DIFFER",
	)
	t.Logf("%s", "--------------------------------------------------------------------------------")

	differCount := 0
	for _, r := range rows {
		rm := creaturePalette[r.redmeanIdx]
		hs := creaturePalette[r.hsvIdx]
		differ := ""
		if r.redmeanIdx != r.hsvIdx {
			differ = "DIFFER"
			differCount++
		}
		t.Logf("%-16s %-8s  %-16s %-8s  %-16s %-8s  %s",
			r.name, r.hex,
			rm.Name, hexRGB(rm.Color),
			hs.Name, hexRGB(hs.Color),
			differ,
		)
	}

	t.Logf("%s", "--------------------------------------------------------------------------------")
	t.Logf("Entries where redmean and HSV pick different nearest: %d / %d",
		differCount, len(creaturePalette))
}

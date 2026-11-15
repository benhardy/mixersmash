package main

import "math"

// screenToFrame converts a raw screen-pixel coordinate (sx, sy) to a frame
// coordinate (fx, fy), inverting the presentGeoM mapping applied in Draw.
//
// In landscape mode the frame is scaled uniformly to fit the screen with
// letterboxing (black bars).  In portrait mode the frame is additionally
// rotated 90° CCW so the landscape frame stands upright on a portrait screen.
//
// A tap that maps outside [0, fw] × [0, fh] lands in a letterbox bar; callers
// should treat out-of-range results as misses (the existing cell bounds check
// will reject them).
func screenToFrame(sx, sy, sw, sh, fw, fh float64, portrait bool) (float64, float64) {
	if !portrait {
		// Landscape: uniform fit-scale, centred.
		scale := math.Min(sw/fw, sh/fh)
		if scale <= 0 {
			return sx, sy
		}
		ox := (sw - fw*scale) / 2
		oy := (sh - fh*scale) / 2
		fx := (sx - ox) / scale
		fy := (sy - oy) / scale
		return fx, fy
	}
	// Portrait: frame rotated 90° CCW; rotated footprint is fh wide × fw tall.
	// Forward mapping: frame point (fx, fy) → screen point (ox + (fh-fy)*scale, oy + fx*scale).
	// Inverse: fx = (sy - oy) / scale; fy = fh - (sx - ox) / scale.
	scale := math.Min(sw/fh, sh/fw)
	if scale <= 0 {
		return sx, sy
	}
	ox := (sw - fh*scale) / 2
	oy := (sh - fw*scale) / 2
	fx := (sy - oy) / scale
	fy := fh - (sx-ox)/scale
	return fx, fy
}

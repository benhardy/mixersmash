package main

import (
	"math"
	"testing"
)

// forwardLandscape maps frame coords to screen coords under a landscape fit.
// Mirrors presentGeoM landscape: scale = min(sw/fw, sh/fh); ox, oy are offsets.
func forwardLandscape(fx, fy, sw, sh, fw, fh float64) (float64, float64) {
	scale := math.Min(sw/fw, sh/fh)
	ox := (sw - fw*scale) / 2
	oy := (sh - fh*scale) / 2
	return ox + fx*scale, oy + fy*scale
}

// forwardPortrait maps frame coords to screen coords under portrait 90° CCW rotation.
// Forward mapping: (fx, fy) → (ox + (fh-fy)*scale, oy + fx*scale).
func forwardPortrait(fx, fy, sw, sh, fw, fh float64) (float64, float64) {
	scale := math.Min(sw/fh, sh/fw)
	ox := (sw - fh*scale) / 2
	oy := (sh - fw*scale) / 2
	return ox + (fh-fy)*scale, oy + fx*scale
}

const epsilon = 0.5

func near(a, b float64) bool {
	return math.Abs(a-b) < epsilon
}

// TestScreenToFrameLandscameSameSize verifies that when screen and frame are the
// same size (no letterboxing, no rotation) the mapping is the identity.
func TestScreenToFrameLandscapeSameSize(t *testing.T) {
	const fw, fh = 800.0, 600.0
	cases := []struct {
		name   string
		sx, sy float64
		wfx    float64
		wfy    float64
	}{
		{"interior", 100, 200, 100, 200},
		{"top-left corner", 0, 0, 0, 0},
		{"bottom-right corner", fw, fh, fw, fh},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fx, fy := screenToFrame(tc.sx, tc.sy, fw, fh, fw, fh, false)
			if !near(fx, tc.wfx) || !near(fy, tc.wfy) {
				t.Errorf("screenToFrame(%v,%v, same-size landscape) = (%v,%v), want (%v,%v)",
					tc.sx, tc.sy, fx, fy, tc.wfx, tc.wfy)
			}
		})
	}
}

// TestScreenToFrameLandscapeLetterbox verifies that a 2× scaled landscape screen
// (1600×1200 showing an 800×600 frame) maps correctly to frame coords.
// Scale = min(1600/800, 1200/600) = 2.0; ox=(1600-800*2)/2=0; oy=(1200-600*2)/2=0.
// So screen == frame*2 exactly, no letterbox offset in this case.
func TestScreenToFrameLandscapeLetterbox(t *testing.T) {
	const fw, fh = 800.0, 600.0
	const sw, sh = 1600.0, 1200.0
	// scale=2, ox=0, oy=0 → screen (sx,sy) = frame (fx*2, fy*2).
	cases := []struct {
		name   string
		fx, fy float64 // frame coord to round-trip
	}{
		{"centre", fw / 2, fh / 2},
		{"top-left", 0, 0},
		{"top-right", fw, 0},
		{"bottom-right", fw, fh},
		{"bottom-left", 0, fh},
		{"interior", 100, 200},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Forward: frame → screen.
			sx, sy := forwardLandscape(tc.fx, tc.fy, sw, sh, fw, fh)
			// Inverse: screen → frame.
			gotFx, gotFy := screenToFrame(sx, sy, sw, sh, fw, fh, false)
			if !near(gotFx, tc.fx) || !near(gotFy, tc.fy) {
				t.Errorf("round-trip landscape (screen %v×%v frame %v×%v): frame (%v,%v) → screen (%v,%v) → frame (%v,%v), want (%v,%v)",
					sw, sh, fw, fh, tc.fx, tc.fy, sx, sy, gotFx, gotFy, tc.fx, tc.fy)
			}
		})
	}
}

// TestScreenToFramePortraitRoundTrip verifies that for a portrait screen (600×800)
// showing a landscape frame (800×600) rotated 90° CCW, the forward+inverse mapping
// round-trips to within epsilon for the frame centre and all four corners.
func TestScreenToFramePortraitRoundTrip(t *testing.T) {
	const fw, fh = 800.0, 600.0
	const sw, sh = 600.0, 800.0 // portrait: sw < sh

	// Verify the spec example: frame centre (400,300) round-trips.
	framePts := [][2]float64{
		{fw / 2, fh / 2}, // centre
		{0, 0},           // top-left
		{fw, 0},          // top-right
		{fw, fh},         // bottom-right
		{0, fh},          // bottom-left
	}
	names := []string{"centre", "top-left", "top-right", "bottom-right", "bottom-left"}

	for i, pt := range framePts {
		fx, fy := pt[0], pt[1]
		sx, sy := forwardPortrait(fx, fy, sw, sh, fw, fh)
		gotFx, gotFy := screenToFrame(sx, sy, sw, sh, fw, fh, true)
		if !near(gotFx, fx) || !near(gotFy, fy) {
			t.Errorf("portrait round-trip %s: frame (%v,%v) → screen (%v,%v) → frame (%v,%v), want (%v,%v)",
				names[i], fx, fy, sx, sy, gotFx, gotFy, fx, fy)
		}
	}
}

// TestScreenToFramePortraitCentrePoint verifies the specific example from the spec:
// frame centre (400,300) on a 600×800 portrait screen presenting an 800×600 frame.
func TestScreenToFramePortraitCentrePoint(t *testing.T) {
	const fw, fh = 800.0, 600.0
	const sw, sh = 600.0, 800.0

	// Compute where the frame centre lands on screen via forward mapping.
	sx, sy := forwardPortrait(fw/2, fh/2, sw, sh, fw, fh)
	// Invert back.
	gotFx, gotFy := screenToFrame(sx, sy, sw, sh, fw, fh, true)
	if !near(gotFx, fw/2) || !near(gotFy, fh/2) {
		t.Errorf("portrait centre: screen (%v,%v) → frame (%v,%v), want (~%v,~%v)",
			sx, sy, gotFx, gotFy, fw/2, fh/2)
	}
}

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

// TestScreenToBoardLandscape tests screenToBoard with landscape orientation,
// no scale (sw==fw, sh==fh) and hudDeviceW=160.
//
// Frame = hudDeviceW(160) + boardDeviceW(800) wide, 600 tall.
// In identity presentation the screen and frame overlap exactly.
// Board cell size = boardDeviceW(800) / gridCols(12) = 66 px.
func TestScreenToBoardLandscape(t *testing.T) {
	// Use a frame wide enough to hold hudDeviceW + boardDeviceW.
	const hudW = 160
	const dcs = 66 // 800/12 ≈ 66
	// Frame is hudW + 800 wide (960), 600 tall; screen same size (identity present).
	const fw, fh = 960.0, 600.0
	const sw, sh = 960.0, 600.0

	cases := []struct {
		name    string
		sx, sy  float64
		wantCol int
		wantRow int
	}{
		// Click at the board-local (0,0) cell centre: screen x = hudW + dcs/2, y = dcs/2.
		{"board cell (0,0)", float64(hudW) + float64(dcs)/2, float64(dcs) / 2, 0, 0},
		// Click at board-local (1,0): screen x = hudW + dcs + dcs/2.
		{"board cell (1,0)", float64(hudW) + float64(dcs) + float64(dcs)/2, float64(dcs) / 2, 1, 0},
		// Click inside HUD strip — should return (-1,-1).
		{"HUD strip", float64(hudW) / 2, float64(fh) / 2, -1, -1},
		// Click at screen x=0 — also in HUD strip.
		{"left edge", 0, float64(fh) / 2, -1, -1},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			col, row := screenToBoard(tc.sx, tc.sy, sw, sh, fw, fh, false, hudW, dcs)
			if col != tc.wantCol || row != tc.wantRow {
				t.Errorf("screenToBoard(%v,%v): got (%d,%d), want (%d,%d)",
					tc.sx, tc.sy, col, row, tc.wantCol, tc.wantRow)
			}
		})
	}
}

// TestScreenToBoardPortrait tests screenToBoard in portrait mode.
// Frame is 960×600 (hud 160 + board 800); portrait screen is 600×960.
// scale = min(sw/fh, sh/fw) = min(600/600, 960/960) = 1.0; ox=0, oy=0.
// Forward portrait: screen (ox + (fh-fy)*scale, oy + fx*scale) = (600-fy, fx).
// So for frame point (hudW + dcs/2, dcs/2):
//
//	sx = 600 - dcs/2, sy = hudW + dcs/2.
func TestScreenToBoardPortrait(t *testing.T) {
	const hudW = 160
	const dcs = 66
	const fw, fh = 960.0, 600.0
	// Portrait screen: sw=fh=600, sh=fw=960.
	const sw, sh = 600.0, 960.0

	// Board-local (0,0): frame (hudW+dcs/2, dcs/2).
	// Forward portrait: sx = fh - fy = 600 - dcs/2; sy = fx = hudW + dcs/2.
	sx0 := fh - float64(dcs)/2
	sy0 := float64(hudW) + float64(dcs)/2
	col, row := screenToBoard(sx0, sy0, sw, sh, fw, fh, true, hudW, dcs)
	if col != 0 || row != 0 {
		t.Errorf("portrait board cell (0,0): got (%d,%d), want (0,0)", col, row)
	}

	// A tap that forward-maps from frame (hudW/2, fh/2) lands in the HUD strip.
	// frame (hudW/2, fh/2): sx = fh - fh/2 = fh/2 = 300; sy = hudW/2 = 80.
	sxHUD := fh / 2
	syHUD := float64(hudW) / 2
	col2, row2 := screenToBoard(sxHUD, syHUD, sw, sh, fw, fh, true, hudW, dcs)
	if col2 != -1 || row2 != -1 {
		t.Errorf("portrait HUD tap: got (%d,%d), want (-1,-1)", col2, row2)
	}
}

// TestScreenToBoardOutOfBounds verifies that taps outside the frame return (-1,-1).
func TestScreenToBoardOutOfBounds(t *testing.T) {
	const hudW = 160
	const dcs = 66
	const fw, fh = 960.0, 600.0
	const sw, sh = 960.0, 600.0

	// Tap well outside the right edge.
	col, row := screenToBoard(sw+100, sh/2, sw, sh, fw, fh, false, hudW, dcs)
	if col != -1 || row != -1 {
		t.Errorf("out-of-bounds tap: got (%d,%d), want (-1,-1)", col, row)
	}
}

// TestConnectionThemeInstructions verifies that both themes implement
// connectionTheme and return non-empty instruction strings.
func TestConnectionThemeInstructions(t *testing.T) {
	themes := []connectionTheme{colourTheme{}, randomTheme{}}
	for _, th := range themes {
		th := th
		t.Run(th.name(), func(t *testing.T) {
			s := th.instructions()
			if s == "" {
				t.Errorf("%T.instructions() returned empty string", th)
			}
		})
	}
}

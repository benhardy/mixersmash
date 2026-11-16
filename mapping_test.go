package main

import (
	"math"
	"testing"
)

const epsilon = 0.5

func near(a, b float64) bool {
	return math.Abs(a-b) < epsilon
}

// nearF64 returns true when a and b are within tol of each other.
func nearF64(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

// ---------------------------------------------------------------------------
// frameScaleFor
// ---------------------------------------------------------------------------

// TestFrameScaleForLandscape verifies that in landscape orientation the
// returned k makes round(designW*k)×round(designH*k) ≈ the presented size,
// and that presentGeoM's fit-scale with those frame dims is ≈1.
func TestFrameScaleForLandscape(t *testing.T) {
	// Design: 1080×600 logical (hudWidth+windowWidth by windowHeight).
	const designW = hudWidth + windowWidth // 1080
	const designH = windowHeight           // 600

	cases := []struct {
		name   string
		sw, sh int
		wantFW int // round(designW*k)
		wantFH int // round(designH*k)
	}{
		{
			// 1080×600 screen, landscape: k = min(1080/1080, 600/600) = 1.0
			name: "screen exactly equals design",
			sw:   1080, sh: 600,
			wantFW: 1080, wantFH: 600,
		},
		{
			// 2160×1200 screen, landscape: k = min(2160/1080, 1200/600) = 2.0
			name: "2× landscape",
			sw:   2160, sh: 1200,
			wantFW: 2160, wantFH: 1200,
		},
		{
			// 1920×1080 screen, landscape: k = min(1920/1080, 1080/600) = min(1.777, 1.8) = 1.777
			// round(1080*1.777)=1920, round(600*1.777)=1066
			name: "1920×1080 widescreen landscape",
			sw:   1920, sh: 1080,
			wantFW: 1920, wantFH: 1067,
		},
		{
			// 3× DPI phone: sw=1080, sh=1920 is portrait — not tested here (portrait case below).
			// Landscape 3× proxy: sw=1800,sh=1800 → k=min(1800/1080,1800/600)=min(1.666,3.0)=1.666
			// round(1080*1.666)=1800, round(600*1.666)=1000
			name: "1800×1800 square landscape",
			sw:   1800, sh: 1800,
			wantFW: 1800, wantFH: 1000,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			k := frameScaleFor(tc.sw, tc.sh, false)
			if k <= 0 {
				t.Fatalf("frameScaleFor returned k=%v, want >0", k)
			}
			gotFW := int(math.Round(designW * k))
			gotFH := int(math.Round(designH * k))
			// Allow ±1 rounding tolerance.
			if abs(gotFW-tc.wantFW) > 1 || abs(gotFH-tc.wantFH) > 1 {
				t.Errorf("frame dims round(designW*k)×round(designH*k) = %d×%d, want ~%d×%d (k=%.4f)",
					gotFW, gotFH, tc.wantFW, tc.wantFH, k)
			}
			// Verify presentGeoM fit-scale ≈ 1 with these frame dims.
			fw := float64(gotFW)
			fh := float64(gotFH)
			sw := float64(tc.sw)
			sh := float64(tc.sh)
			fitScale := math.Min(sw/fw, sh/fh)
			if !nearF64(fitScale, 1.0, 0.02) {
				t.Errorf("presentGeoM fit-scale = %.4f, want ≈1.0 (frame %d×%d, screen %d×%d)",
					fitScale, gotFW, gotFH, tc.sw, tc.sh)
			}
		})
	}
}

// TestFrameScaleForPortrait verifies that in portrait orientation k makes the
// rotated design (designH wide × designW tall) fit into sw×sh, and that
// presentGeoM's portrait fit-scale with those frame dims is ≈1.
func TestFrameScaleForPortrait(t *testing.T) {
	const designW = hudWidth + windowWidth // 1080
	const designH = windowHeight           // 600

	cases := []struct {
		name   string
		sw, sh int
	}{
		{
			// Portrait phone at 3× DPI: 1080×1920 device px.
			// Rotated footprint = 600×1080; k = min(1080/600, 1920/1080) = min(1.8, 1.777) = 1.777.
			// frame = round(1080*1.777)×round(600*1.777) = 1920×1067.
			name: "portrait 1080×1920 at 3×",
			sw:   1080, sh: 1920,
		},
		{
			// Portrait phone at 2× DPI: 750×1334.
			// Rotated: k = min(750/600, 1334/1080) = min(1.25, 1.235) = 1.235.
			name: "portrait 750×1334 at 2×",
			sw:   750, sh: 1334,
		},
		{
			// Portrait tablet: 1536×2048 device px.
			// Rotated: k = min(1536/600, 2048/1080) = min(2.56, 1.896) = 1.896.
			name: "portrait 1536×2048 tablet",
			sw:   1536, sh: 2048,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			k := frameScaleFor(tc.sw, tc.sh, true)
			if k <= 0 {
				t.Fatalf("frameScaleFor portrait returned k=%v, want >0", k)
			}
			fw := float64(int(math.Round(designW * k)))
			fh := float64(int(math.Round(designH * k)))
			sw := float64(tc.sw)
			sh := float64(tc.sh)
			// In portrait, presentGeoM's scale = min(sw/fh, sh/fw).
			fitScale := math.Min(sw/fh, sh/fw)
			if !nearF64(fitScale, 1.0, 0.02) {
				t.Errorf("portrait presentGeoM fit-scale = %.4f, want ≈1.0 (frame %.0f×%.0f, screen %d×%d, k=%.4f)",
					fitScale, fw, fh, tc.sw, tc.sh, k)
			}
			// Frame area should be significantly smaller than design×s² for a 3× DPI device.
			// Just verify frame fits within the screen bounds in its rotated footprint.
			if fw > sh+1 || fh > sw+1 {
				t.Errorf("rotated frame %.0f×%.0f doesn't fit in screen %d×%d", fh, fw, tc.sw, tc.sh)
			}
		})
	}
}

// TestFrameScaleForPresentsAtScale1 is a combined parametric test: for
// landscape and portrait, the frame allocated at round(design*k) makes
// presentGeoM's scale ≈1, confirming over-resolution is eliminated.
func TestFrameScaleForPresentsAtScale1(t *testing.T) {
	const designW = float64(hudWidth + windowWidth) // 1080
	const designH = float64(windowHeight)           // 600

	cases := []struct {
		name     string
		sw, sh   int
		portrait bool
	}{
		{"landscape 1920×1080", 1920, 1080, false},
		{"landscape 2560×1440", 2560, 1440, false},
		{"portrait 1080×1920", 1080, 1920, true},
		{"portrait 768×1024", 768, 1024, true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			k := frameScaleFor(tc.sw, tc.sh, tc.portrait)
			fw := float64(int(math.Round(designW * k)))
			fh := float64(int(math.Round(designH * k)))
			sw := float64(tc.sw)
			sh := float64(tc.sh)
			var fitScale float64
			if !tc.portrait {
				fitScale = math.Min(sw/fw, sh/fh)
			} else {
				fitScale = math.Min(sw/fh, sh/fw)
			}
			if !nearF64(fitScale, 1.0, 0.02) {
				t.Errorf("%s: fit-scale=%.4f want ≈1.0 (k=%.4f, frame %.0f×%.0f, screen %d×%d)",
					tc.name, fitScale, k, fw, fh, tc.sw, tc.sh)
			}
		})
	}
}

// abs returns the absolute value of an int.
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// TestScreenToFrameLandscapeSameSize verifies that when screen and frame are the
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
// (1600×1200 showing an 800×600 frame) round-trips correctly using presentGeoM
// as the authoritative forward mapping.
// Scale = min(1600/800, 1200/600) = 2.0; ox=0, oy=0 (no letterbox in this case).
func TestScreenToFrameLandscapeLetterbox(t *testing.T) {
	const fw, fh = 800.0, 600.0
	const sw, sh = 1600.0, 1200.0

	cases := []struct {
		name   string
		fx, fy float64
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
			// Forward: frame → screen using the real GeoM.
			m := presentGeoM(false, sw, sh, fw, fh)
			sx, sy := m.Apply(tc.fx, tc.fy)
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
// showing a landscape frame (800×600) rotated 90° CCW, the round-trip through
// presentGeoM (forward) and screenToFrame (inverse) is within epsilon for the
// frame centre, all four corners, and two interior points.
func TestScreenToFramePortraitRoundTrip(t *testing.T) {
	const fw, fh = 800.0, 600.0
	const sw, sh = 600.0, 800.0 // portrait: sw < sh

	framePts := [][2]float64{
		{fw / 2, fh / 2}, // centre
		{0, 0},           // top-left
		{fw, 0},          // top-right
		{fw, fh},         // bottom-right
		{0, fh},          // bottom-left
		{100, 150},       // interior A
		{700, 400},       // interior B
	}
	names := []string{"centre", "top-left", "top-right", "bottom-right", "bottom-left", "interior-A", "interior-B"}

	for i, pt := range framePts {
		fx, fy := pt[0], pt[1]
		// Forward: use the real presentGeoM, not a hand-derived formula.
		m := presentGeoM(true, sw, sh, fw, fh)
		sx, sy := m.Apply(fx, fy)
		// Inverse: screenToFrame must invert the same GeoM.
		gotFx, gotFy := screenToFrame(sx, sy, sw, sh, fw, fh, true)
		if !near(gotFx, fx) || !near(gotFy, fy) {
			t.Errorf("portrait round-trip %s: frame (%v,%v) → screen (%v,%v) → frame (%v,%v), want (%v,%v)",
				names[i], fx, fy, sx, sy, gotFx, gotFy, fx, fy)
		}
	}
}

// TestScreenToFramePortraitCentrePoint verifies the specific example from the spec:
// frame centre (400,300) on a 600×800 portrait screen presenting an 800×600 frame,
// using presentGeoM as the authoritative forward.
func TestScreenToFramePortraitCentrePoint(t *testing.T) {
	const fw, fh = 800.0, 600.0
	const sw, sh = 600.0, 800.0

	m := presentGeoM(true, sw, sh, fw, fh)
	sx, sy := m.Apply(fw/2, fh/2)
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

// TestScreenToBoardPortrait tests screenToBoard in portrait mode, using
// presentGeoM to compute the forward screen coordinates so the test is
// anchored to the same transform that Drive() applies.
func TestScreenToBoardPortrait(t *testing.T) {
	const hudW = 160
	const dcs = 66
	const fw, fh = 960.0, 600.0
	// Portrait screen: sw=fh=600, sh=fw=960.
	const sw, sh = 600.0, 960.0

	// Board-local (0,0) centre: frame coords (hudW + dcs/2, dcs/2).
	fx0 := float64(hudW) + float64(dcs)/2
	fy0 := float64(dcs) / 2
	m := presentGeoM(true, sw, sh, fw, fh)
	sx0, sy0 := m.Apply(fx0, fy0)
	col, row := screenToBoard(sx0, sy0, sw, sh, fw, fh, true, hudW, dcs)
	if col != 0 || row != 0 {
		t.Errorf("portrait board cell (0,0): screen (%v,%v) → got (%d,%d), want (0,0)", sx0, sy0, col, row)
	}

	// A tap from frame coords in the HUD strip centre: (hudW/2, fh/2).
	fxHUD := float64(hudW) / 2
	fyHUD := fh / 2
	sxHUD, syHUD := m.Apply(fxHUD, fyHUD)
	col2, row2 := screenToBoard(sxHUD, syHUD, sw, sh, fw, fh, true, hudW, dcs)
	if col2 != -1 || row2 != -1 {
		t.Errorf("portrait HUD tap: screen (%v,%v) → got (%d,%d), want (-1,-1)", sxHUD, syHUD, col2, row2)
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

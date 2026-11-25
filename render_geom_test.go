package main

import (
	"math"
	"testing"
)

// geoMEpsilon is the tolerance for GeoM floating-point comparisons.
// It is tighter than the pixel-mapping epsilon in mapping_test.go (0.5).
const geoMEpsilon = 1e-9

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < geoMEpsilon
}

// TestSpriteTurnGeoMPivot verifies that the sprite-centre source point
// (radius, radius) always maps to the target screen centre (cx, cy),
// regardless of angle.  This confirms the pivot is correctly at the sprite centre.
func TestSpriteTurnGeoMPivot(t *testing.T) {
	t.Helper()

	const R = 10.0
	const cx = 50.0
	const cy = 80.0
	const scale = 1.0

	cases := []struct {
		name  string
		angle float64
	}{
		{"angle=0", 0},
		{"angle=pi/2", math.Pi / 2},
		{"angle=pi", math.Pi},
		{"angle=3pi/2", 3 * math.Pi / 2},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			m := spriteTurnGeoM(cx, cy, R, scale, tc.angle)
			// The sprite-centre source point is (radius, radius) because the
			// sprite image is sized 2R×2R (scale=1 means refSz=2R, so sprite
			// centre is at (R,R) in image space).
			gotX, gotY := m.Apply(R, R)
			if !approxEqual(gotX, cx) || !approxEqual(gotY, cy) {
				t.Errorf("angle=%.4f: pivot (%.0f,%.0f) mapped to (%.9f,%.9f); want (%.0f,%.0f)",
					tc.angle, R, R, gotX, gotY, cx, cy)
			}
		})
	}
}

// TestSpriteTurnGeoMDirection verifies that positive angle produces a clockwise
// rotation in screen-space (y-down).
//
// Setup: scale=1, radius=R=10, cx=cy=0.  The sprite image is treated as 2R×2R,
// so the right edge-centre in source image space is (2R, R).
//
// At angle=+pi/2 (clockwise 90°): the right edge should swing downward,
// landing at approximately (0, +R).
//
// At angle=-pi/2 (counter-clockwise 90°): the right edge should swing upward,
// landing at approximately (0, -R).
func TestSpriteTurnGeoMDirection(t *testing.T) {
	t.Helper()

	const R = 10.0
	// scale=1 means no scaling; radius=R; cx=cy=0 for simple expected values.
	const scale = 1.0
	const cx = 0.0
	const cy = 0.0

	// Source point: right edge-centre of the sprite image (2R, R).
	// This is the sprite's rightmost point (centre + (R, 0) in sprite local space).
	const srcX = 2 * R // = 20
	const srcY = R     // = 10

	cases := []struct {
		name    string
		angle   float64
		wantX   float64
		wantY   float64
		comment string
	}{
		{
			name:    "positive_pi_half_clockwise",
			angle:   math.Pi / 2,
			wantX:   0,
			wantY:   +R,
			comment: "right edge swings DOWN (clockwise)",
		},
		{
			name:    "negative_pi_half_anticlockwise",
			angle:   -math.Pi / 2,
			wantX:   0,
			wantY:   -R,
			comment: "right edge swings UP (counter-clockwise)",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			m := spriteTurnGeoM(cx, cy, R, scale, tc.angle)
			gotX, gotY := m.Apply(srcX, srcY)
			if !approxEqual(gotX, tc.wantX) || !approxEqual(gotY, tc.wantY) {
				t.Errorf("%s: source (%.0f,%.0f) angle=%.4f -> (%.9f,%.9f); want (%.4f,%.4f) [%s]",
					tc.name, srcX, srcY, tc.angle, gotX, gotY, tc.wantX, tc.wantY, tc.comment)
			}
		})
	}
}

// TestSpriteTurnGeoMPivotScale verifies pivot correctness with a non-unit scale.
// When scale != 1 the sprite image is scaled before the translate, so the
// source centre of the scaled sprite is still (radius, radius) in pre-Scale space.
func TestSpriteTurnGeoMPivotScale(t *testing.T) {
	t.Helper()

	const R = 10.0
	const cx = 100.0
	const cy = 200.0
	const scale = 0.5

	// Manually trace the transform for scale=0.5, R=10:
	// Scale(0.5,0.5): (R,R)=(10,10) -> (5,5)
	// Translate(-R,-R)=(-10,-10): (5,5) -> (-5,-5)
	// Rotate(0): no change -> (-5,-5)
	// Translate(cx,cy)=(100,200): (-5,-5) -> (95,195) ≠ (cx,cy).
	//
	// (R,R) does NOT land on (cx,cy) because Scale shrinks before the pivot translate.
	// The source point that maps to (cx,cy) is (R/scale, R/scale):
	// Scale(0.5): (20,20)->(10,10); Translate(-10,-10)->(0,0); Rotate->(0,0); Translate(100,200)->(100,200).
	//
	// The contract: source point (R/scale, R/scale) maps to (cx,cy) for any angle.
	srcX := R / scale // 20
	srcY := R / scale // 20

	cases := []struct {
		name  string
		angle float64
	}{
		{"angle=0", 0},
		{"angle=pi/2", math.Pi / 2},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			gm := spriteTurnGeoM(cx, cy, R, scale, tc.angle)
			gotX, gotY := gm.Apply(srcX, srcY)
			if !approxEqual(gotX, cx) || !approxEqual(gotY, cy) {
				t.Errorf("scale=%.2f angle=%.4f: pivot (%.0f,%.0f) mapped to (%.9f,%.9f); want (%.0f,%.0f)",
					scale, tc.angle, srcX, srcY, gotX, gotY, cx, cy)
			}
		})
	}
}

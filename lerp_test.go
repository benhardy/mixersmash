package main

import (
	"testing"
)

const lerpTol = float32(1e-6)

func approxEq(a, b float32) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff <= lerpTol
}

// TestLerp verifies lerp at t=0, t=1, t=0.5 (midpoint), and extrapolation.
func TestLerp(t *testing.T) {
	cases := []struct {
		name string
		a, b float32
		tVal float32
		want float32
	}{
		{"t=0 returns a", 3, 7, 0, 3},
		{"t=1 returns b", 3, 7, 1, 7},
		{"t=0.5 midpoint", 2, 8, 0.5, 5},
		{"extrapolate beyond 1", 0, 10, 1.5, 15},
		{"extrapolate below 0", 0, 10, -0.5, -5},
		{"equal endpoints", 4, 4, 0.3, 4},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := lerp(tc.a, tc.b, tc.tVal)
			if !approxEq(got, tc.want) {
				t.Errorf("lerp(%v, %v, %v) = %v, want %v", tc.a, tc.b, tc.tVal, got, tc.want)
			}
		})
	}
}

// TestLerp2 verifies that lerp2 applies lerp independently to both axes.
func TestLerp2(t *testing.T) {
	cases := []struct {
		name           string
		ax, ay, bx, by float32
		tVal           float32
		wantX, wantY   float32
	}{
		{"t=0 returns start", 1, 2, 5, 6, 0, 1, 2},
		{"t=1 returns end", 1, 2, 5, 6, 1, 5, 6},
		{"t=0.5 midpoint", 0, 0, 4, 8, 0.5, 2, 4},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			gx, gy := lerp2(tc.ax, tc.ay, tc.bx, tc.by, tc.tVal)
			if !approxEq(gx, tc.wantX) || !approxEq(gy, tc.wantY) {
				t.Errorf("lerp2(%v,%v,%v,%v,%v) = (%v,%v), want (%v,%v)",
					tc.ax, tc.ay, tc.bx, tc.by, tc.tVal, gx, gy, tc.wantX, tc.wantY)
			}
		})
	}
}

// TestQuadBezier verifies the 1D quadratic Bézier: t=0→p0, t=1→p1, and a
// known midpoint computed by hand: (0.5)^2*p0 + 2*0.5*0.5*c + (0.5)^2*p1
// = 0.25*p0 + 0.5*c + 0.25*p1.
func TestQuadBezier(t *testing.T) {
	p0, c, p1 := float32(0), float32(10), float32(4)
	// t=0.5: 0.25*0 + 0.5*10 + 0.25*4 = 0 + 5 + 1 = 6
	wantMid := float32(0.25*0 + 0.5*10 + 0.25*4)
	cases := []struct {
		name string
		tVal float32
		want float32
	}{
		{"t=0 returns p0", 0, p0},
		{"t=1 returns p1", 1, p1},
		{"t=0.5 known midpoint", 0.5, wantMid},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := quadBezier(p0, c, p1, tc.tVal)
			if !approxEq(got, tc.want) {
				t.Errorf("quadBezier(%v,%v,%v,%v) = %v, want %v", p0, c, p1, tc.tVal, got, tc.want)
			}
		})
	}
}

// TestQuadBezier2 verifies the 2D form returns the per-axis quadBezier values.
func TestQuadBezier2(t *testing.T) {
	p0x, p0y := float32(0), float32(0)
	cX, cY := float32(10), float32(20)
	p1x, p1y := float32(4), float32(8)
	// t=0.5 per axis: x=0.25*0+0.5*10+0.25*4=6, y=0.25*0+0.5*20+0.25*8=12
	cases := []struct {
		name         string
		tVal         float32
		wantX, wantY float32
	}{
		{"t=0 returns start", 0, p0x, p0y},
		{"t=1 returns end", 1, p1x, p1y},
		{"t=0.5 midpoint", 0.5, 6, 12},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			gx, gy := quadBezier2(p0x, p0y, cX, cY, p1x, p1y, tc.tVal)
			if !approxEq(gx, tc.wantX) || !approxEq(gy, tc.wantY) {
				t.Errorf("quadBezier2 at t=%v = (%v,%v), want (%v,%v)",
					tc.tVal, gx, gy, tc.wantX, tc.wantY)
			}
		})
	}
}

// TestEaseInQuad verifies t², boundary values, and a known interior point.
func TestEaseInQuad(t *testing.T) {
	cases := []struct {
		name string
		tVal float32
		want float32
	}{
		{"t=0 returns 0", 0, 0},
		{"t=1 returns 1", 1, 1},
		{"t=0.5 returns 0.25", 0.5, 0.25},
		{"t=0.25 returns 0.0625", 0.25, 0.0625},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := easeInQuad(tc.tVal)
			if !approxEq(got, tc.want) {
				t.Errorf("easeInQuad(%v) = %v, want %v", tc.tVal, got, tc.want)
			}
		})
	}
}

// TestEaseOutQuad verifies 1-(1-t)², boundary values, and a known interior point.
func TestEaseOutQuad(t *testing.T) {
	cases := []struct {
		name string
		tVal float32
		want float32
	}{
		{"t=0 returns 0", 0, 0},
		{"t=1 returns 1", 1, 1},
		{"t=0.5 returns 0.75", 0.5, 0.75},
		{"t=0.25 returns 0.4375", 0.25, 0.4375},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := easeOutQuad(tc.tVal)
			if !approxEq(got, tc.want) {
				t.Errorf("easeOutQuad(%v) = %v, want %v", tc.tVal, got, tc.want)
			}
		})
	}
}

// TestEaseInOutQuad verifies boundary values, the t=0.5 symmetry point, and
// that the function is symmetric about (0.5, 0.5).
func TestEaseInOutQuad(t *testing.T) {
	cases := []struct {
		name string
		tVal float32
		want float32
	}{
		{"t=0 returns 0", 0, 0},
		{"t=1 returns 1", 1, 1},
		{"t=0.5 returns 0.5", 0.5, 0.5},
		{"t=0.25 ease-in branch 2*0.25*0.25=0.125", 0.25, 0.125},
		{"t=0.75 ease-out branch 1-2*0.25*0.25=0.875", 0.75, 0.875},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := easeInOutQuad(tc.tVal)
			if !approxEq(got, tc.want) {
				t.Errorf("easeInOutQuad(%v) = %v, want %v", tc.tVal, got, tc.want)
			}
		})
	}

	// Symmetry: easeInOutQuad(t) + easeInOutQuad(1-t) == 1 for all t in [0,1].
	t.Run("symmetry", func(t *testing.T) {
		steps := 20
		for i := 0; i <= steps; i++ {
			tVal := float32(i) / float32(steps)
			a := easeInOutQuad(tVal)
			b := easeInOutQuad(1 - tVal)
			sum := a + b
			if !approxEq(sum, 1) {
				t.Errorf("symmetry failure at t=%v: easeInOutQuad(t)+easeInOutQuad(1-t) = %v, want 1",
					tVal, sum)
			}
		}
	})
}

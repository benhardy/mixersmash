package main

import (
	"math"
	"testing"
	"time"

	"mixersmash/internal/board"
)

// --- moveAnimDuration tests ---

func TestMoveAnimDuration(t *testing.T) {
	t.Helper()

	tests := []struct {
		state gameState
		want  time.Duration
	}{
		{stateShuffling, shuffleDuration},
		{stateWaiting, shuffleDuration},
		{stateVictimMoving, shuffleDuration},
		{stateDropping, dropDuration},
		{stateColumnCollapsing, dropDuration},
	}
	for _, tc := range tests {
		t.Run("", func(t *testing.T) {
			got := MoveAnimDuration(tc.state)
			if got != tc.want {
				t.Errorf("moveAnimDuration(%d) = %v; want %v", tc.state, got, tc.want)
			}
		})
	}
}

// --- moveAnimPos tests ---

const float32Eps = float32(1e-4)

func approxEqF32(a, b float32) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= float32Eps
}

func TestMoveAnimPos(t *testing.T) {
	t.Helper()

	anim := board.MoveAnim{
		FromX: 10, FromY: 20,
		ToX: 110, ToY: 220,
		CtrlX: 60, CtrlY: 320, // arc control point
	}

	t.Run("stateShuffling t=0 returns From", func(t *testing.T) {
		cx, cy := MoveAnimPos(anim, stateShuffling, 0)
		if !approxEqF32(cx, anim.FromX) || !approxEqF32(cy, anim.FromY) {
			t.Errorf("got (%.4f,%.4f); want (%.4f,%.4f)", cx, cy, anim.FromX, anim.FromY)
		}
	})

	t.Run("stateShuffling t=1 returns To", func(t *testing.T) {
		cx, cy := MoveAnimPos(anim, stateShuffling, 1)
		if !approxEqF32(cx, anim.ToX) || !approxEqF32(cy, anim.ToY) {
			t.Errorf("got (%.4f,%.4f); want (%.4f,%.4f)", cx, cy, anim.ToX, anim.ToY)
		}
	})

	t.Run("stateDropping t=0 returns From", func(t *testing.T) {
		cx, cy := MoveAnimPos(anim, stateDropping, 0)
		if !approxEqF32(cx, anim.FromX) || !approxEqF32(cy, anim.FromY) {
			t.Errorf("got (%.4f,%.4f); want (%.4f,%.4f)", cx, cy, anim.FromX, anim.FromY)
		}
	})

	t.Run("stateDropping t=1 returns To", func(t *testing.T) {
		cx, cy := MoveAnimPos(anim, stateDropping, 1)
		if !approxEqF32(cx, anim.ToX) || !approxEqF32(cy, anim.ToY) {
			t.Errorf("got (%.4f,%.4f); want (%.4f,%.4f)", cx, cy, anim.ToX, anim.ToY)
		}
	})

	t.Run("stateShuffling mid-t uses eased bezier", func(t *testing.T) {
		// At t=0.5 the easeInOutQuad returns exactly 0.5 (by definition),
		// so the Bézier at eased=0.5 equals quadBezier2 at 0.5.
		et := EaseInOutQuad(0.5)
		wantX, wantY := QuadBezier2(anim.FromX, anim.FromY, anim.CtrlX, anim.CtrlY, anim.ToX, anim.ToY, et)
		cx, cy := MoveAnimPos(anim, stateShuffling, 0.5)
		if !approxEqF32(cx, wantX) || !approxEqF32(cy, wantY) {
			t.Errorf("got (%.4f,%.4f); want (%.4f,%.4f)", cx, cy, wantX, wantY)
		}
	})

	t.Run("stateDropping mid-t uses eased lerp", func(t *testing.T) {
		posT := EaseInQuad(0.5)
		wantX, wantY := Lerp2(anim.FromX, anim.FromY, anim.ToX, anim.ToY, posT)
		cx, cy := MoveAnimPos(anim, stateDropping, 0.5)
		if !approxEqF32(cx, wantX) || !approxEqF32(cy, wantY) {
			t.Errorf("got (%.4f,%.4f); want (%.4f,%.4f)", cx, cy, wantX, wantY)
		}
	})
}

// --- victimAnimPos tests ---

func TestVictimAnimPos(t *testing.T) {
	t.Helper()

	t.Run("PathType 0 lerp t=0 returns From", func(t *testing.T) {
		v := board.MoveAnim{PathType: 0, FromX: 5, FromY: 15, ToX: 50, ToY: 150}
		cx, cy := VictimAnimPos(v, 0)
		if !approxEqF32(cx, v.FromX) || !approxEqF32(cy, v.FromY) {
			t.Errorf("got (%.4f,%.4f); want (%.4f,%.4f)", cx, cy, v.FromX, v.FromY)
		}
	})

	t.Run("PathType 0 lerp t=1 returns To", func(t *testing.T) {
		v := board.MoveAnim{PathType: 0, FromX: 5, FromY: 15, ToX: 50, ToY: 150}
		cx, cy := VictimAnimPos(v, 1)
		if !approxEqF32(cx, v.ToX) || !approxEqF32(cy, v.ToY) {
			t.Errorf("got (%.4f,%.4f); want (%.4f,%.4f)", cx, cy, v.ToX, v.ToY)
		}
	})

	t.Run("PathType 1 bezier t=0 returns From", func(t *testing.T) {
		v := board.MoveAnim{PathType: 1, FromX: 10, FromY: 20, CtrlX: 60, CtrlY: 300, ToX: 110, ToY: 220}
		cx, cy := VictimAnimPos(v, 0)
		if !approxEqF32(cx, v.FromX) || !approxEqF32(cy, v.FromY) {
			t.Errorf("got (%.4f,%.4f); want (%.4f,%.4f)", cx, cy, v.FromX, v.FromY)
		}
	})

	t.Run("PathType 1 bezier t=1 returns To", func(t *testing.T) {
		v := board.MoveAnim{PathType: 1, FromX: 10, FromY: 20, CtrlX: 60, CtrlY: 300, ToX: 110, ToY: 220}
		cx, cy := VictimAnimPos(v, 1)
		if !approxEqF32(cx, v.ToX) || !approxEqF32(cy, v.ToY) {
			t.Errorf("got (%.4f,%.4f); want (%.4f,%.4f)", cx, cy, v.ToX, v.ToY)
		}
	})

	t.Run("PathType 2 spiral t=0 returns From", func(t *testing.T) {
		v := board.MoveAnim{PathType: 2, FromX: 100, FromY: 200, ToX: 50, ToY: 100, SpiralTurns: 1.5}
		cx, cy := VictimAnimPos(v, 0)
		if !approxEqF32(cx, v.FromX) || !approxEqF32(cy, v.FromY) {
			t.Errorf("got (%.4f,%.4f); want (%.4f,%.4f)", cx, cy, v.FromX, v.FromY)
		}
	})

	t.Run("PathType 2 spiral t=1 returns To", func(t *testing.T) {
		v := board.MoveAnim{PathType: 2, FromX: 100, FromY: 200, ToX: 50, ToY: 100, SpiralTurns: 1.5}
		cx, cy := VictimAnimPos(v, 1)
		// At t=1: r = r0*(1-1) = 0, so cx = ToX + 0, cy = ToY + 0
		if !approxEqF32(cx, v.ToX) || !approxEqF32(cy, v.ToY) {
			t.Errorf("got (%.4f,%.4f); want (%.4f,%.4f)", cx, cy, v.ToX, v.ToY)
		}
	})

	t.Run("PathType 2 spiral From==To is stationary", func(t *testing.T) {
		// When From==To, r0=0 so cx=ToX, cy=ToY for all t.
		v := board.MoveAnim{PathType: 2, FromX: 64, FromY: 128, ToX: 64, ToY: 128, SpiralTurns: 2.0}
		for _, tval := range []float32{0, 0.25, 0.5, 0.75, 1.0} {
			cx, cy := VictimAnimPos(v, tval)
			if !approxEqF32(cx, v.ToX) || !approxEqF32(cy, v.ToY) {
				t.Errorf("t=%.2f: got (%.4f,%.4f); want (%.4f,%.4f)", tval, cx, cy, v.ToX, v.ToY)
			}
		}
	})

	t.Run("PathType 2 spiral mid-t matches manual formula", func(t *testing.T) {
		v := board.MoveAnim{PathType: 2, FromX: 100, FromY: 200, ToX: 50, ToY: 100, SpiralTurns: 1.0}
		tval := float32(0.5)
		ddx := v.FromX - v.ToX
		ddy := v.FromY - v.ToY
		r0 := float32(math.Sqrt(float64(ddx*ddx + ddy*ddy)))
		theta0 := float32(math.Atan2(float64(ddy), float64(ddx)))
		r := r0 * (1 - tval)
		theta := theta0 + v.SpiralTurns*2*float32(math.Pi)*tval
		wantX := v.ToX + r*float32(math.Cos(float64(theta)))
		wantY := v.ToY + r*float32(math.Sin(float64(theta)))

		cx, cy := VictimAnimPos(v, tval)
		if !approxEqF32(cx, wantX) || !approxEqF32(cy, wantY) {
			t.Errorf("got (%.4f,%.4f); want (%.4f,%.4f)", cx, cy, wantX, wantY)
		}
	})
}

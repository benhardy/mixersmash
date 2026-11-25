package anim

import (
	"math"
	"testing"

	"mixersmash/internal/board"
)

// --- MoveAnimDuration tests ---

func TestMoveAnimDuration(t *testing.T) {
	t.Helper()

	tests := []struct {
		name string
		kind StateKind
		want interface{}
	}{
		{"KindShuffle", KindShuffle, ShuffleDuration},
		{"KindOther", KindOther, ShuffleDuration},
		{"KindDrop", KindDrop, DropDuration},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := MoveAnimDuration(tc.kind)
			if got != tc.want {
				t.Errorf("MoveAnimDuration(%v) = %v; want %v", tc.kind, got, tc.want)
			}
		})
	}
}

// --- MoveAnimPos tests ---

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

	mv := board.NewShuffleAnim(10, 20, 110, 220, 60, 320, board.Cell{}, 0)

	t.Run("KindShuffle t=0 returns From", func(t *testing.T) {
		cx, cy := MoveAnimPos(mv, KindShuffle, 0)
		if !approxEqF32(cx, mv.FromX()) || !approxEqF32(cy, mv.FromY()) {
			t.Errorf("got (%.4f,%.4f); want (%.4f,%.4f)", cx, cy, mv.FromX(), mv.FromY())
		}
	})

	t.Run("KindShuffle t=1 returns To", func(t *testing.T) {
		cx, cy := MoveAnimPos(mv, KindShuffle, 1)
		if !approxEqF32(cx, mv.ToX()) || !approxEqF32(cy, mv.ToY()) {
			t.Errorf("got (%.4f,%.4f); want (%.4f,%.4f)", cx, cy, mv.ToX(), mv.ToY())
		}
	})

	t.Run("KindDrop t=0 returns From", func(t *testing.T) {
		cx, cy := MoveAnimPos(mv, KindDrop, 0)
		if !approxEqF32(cx, mv.FromX()) || !approxEqF32(cy, mv.FromY()) {
			t.Errorf("got (%.4f,%.4f); want (%.4f,%.4f)", cx, cy, mv.FromX(), mv.FromY())
		}
	})

	t.Run("KindDrop t=1 returns To", func(t *testing.T) {
		cx, cy := MoveAnimPos(mv, KindDrop, 1)
		if !approxEqF32(cx, mv.ToX()) || !approxEqF32(cy, mv.ToY()) {
			t.Errorf("got (%.4f,%.4f); want (%.4f,%.4f)", cx, cy, mv.ToX(), mv.ToY())
		}
	})

	t.Run("KindShuffle mid-t uses eased bezier", func(t *testing.T) {
		// At t=0.5 the EaseInOutQuad returns exactly 0.5 (by definition),
		// so the Bézier at eased=0.5 equals QuadBezier2 at 0.5.
		et := EaseInOutQuad(0.5)
		wantX, wantY := quadBezier2(mv.FromX(), mv.FromY(), mv.CtrlX(), mv.CtrlY(), mv.ToX(), mv.ToY(), et)
		cx, cy := MoveAnimPos(mv, KindShuffle, 0.5)
		if !approxEqF32(cx, wantX) || !approxEqF32(cy, wantY) {
			t.Errorf("got (%.4f,%.4f); want (%.4f,%.4f)", cx, cy, wantX, wantY)
		}
	})

	t.Run("KindDrop mid-t uses eased lerp", func(t *testing.T) {
		posT := EaseInQuad(0.5)
		wantX, wantY := lerp2(mv.FromX(), mv.FromY(), mv.ToX(), mv.ToY(), posT)
		cx, cy := MoveAnimPos(mv, KindDrop, 0.5)
		if !approxEqF32(cx, wantX) || !approxEqF32(cy, wantY) {
			t.Errorf("got (%.4f,%.4f); want (%.4f,%.4f)", cx, cy, wantX, wantY)
		}
	})
}

// --- VictimAnimPos tests ---

func TestVictimAnimPos(t *testing.T) {
	t.Helper()

	t.Run("PathType 0 lerp t=0 returns From", func(t *testing.T) {
		v := board.NewVictimSeedAnim(0, 0, board.Cell{}, 5, 15)
		cx, cy := VictimAnimPos(v, 0)
		if !approxEqF32(cx, v.FromX()) || !approxEqF32(cy, v.FromY()) {
			t.Errorf("got (%.4f,%.4f); want (%.4f,%.4f)", cx, cy, v.FromX(), v.FromY())
		}
	})

	t.Run("PathType 0 lerp t=1 returns To", func(t *testing.T) {
		v := board.NewVictimSeedAnim(0, 0, board.Cell{}, 5, 15)
		cx, cy := VictimAnimPos(v, 1)
		// VictimSeedAnim has To==From so both should equal FromX/FromY
		if !approxEqF32(cx, v.ToX()) || !approxEqF32(cy, v.ToY()) {
			t.Errorf("got (%.4f,%.4f); want (%.4f,%.4f)", cx, cy, v.ToX(), v.ToY())
		}
	})

	t.Run("PathType 0 lerp non-seed t=0 returns From", func(t *testing.T) {
		v := board.NewVictimAnim(0, 0, board.Cell{}, 5, 15, 50, 150, 0, 0, 0, 0)
		cx, cy := VictimAnimPos(v, 0)
		if !approxEqF32(cx, v.FromX()) || !approxEqF32(cy, v.FromY()) {
			t.Errorf("got (%.4f,%.4f); want (%.4f,%.4f)", cx, cy, v.FromX(), v.FromY())
		}
	})

	t.Run("PathType 0 lerp non-seed t=1 returns To", func(t *testing.T) {
		v := board.NewVictimAnim(0, 0, board.Cell{}, 5, 15, 50, 150, 0, 0, 0, 0)
		cx, cy := VictimAnimPos(v, 1)
		if !approxEqF32(cx, v.ToX()) || !approxEqF32(cy, v.ToY()) {
			t.Errorf("got (%.4f,%.4f); want (%.4f,%.4f)", cx, cy, v.ToX(), v.ToY())
		}
	})

	t.Run("PathType 1 bezier t=0 returns From", func(t *testing.T) {
		v := board.NewVictimAnim(0, 0, board.Cell{}, 10, 20, 110, 220, 1, 60, 300, 0)
		cx, cy := VictimAnimPos(v, 0)
		if !approxEqF32(cx, v.FromX()) || !approxEqF32(cy, v.FromY()) {
			t.Errorf("got (%.4f,%.4f); want (%.4f,%.4f)", cx, cy, v.FromX(), v.FromY())
		}
	})

	t.Run("PathType 1 bezier t=1 returns To", func(t *testing.T) {
		v := board.NewVictimAnim(0, 0, board.Cell{}, 10, 20, 110, 220, 1, 60, 300, 0)
		cx, cy := VictimAnimPos(v, 1)
		if !approxEqF32(cx, v.ToX()) || !approxEqF32(cy, v.ToY()) {
			t.Errorf("got (%.4f,%.4f); want (%.4f,%.4f)", cx, cy, v.ToX(), v.ToY())
		}
	})

	t.Run("PathType 2 spiral t=0 returns From", func(t *testing.T) {
		v := board.NewVictimAnim(0, 0, board.Cell{}, 100, 200, 50, 100, 2, 0, 0, 1.5)
		cx, cy := VictimAnimPos(v, 0)
		if !approxEqF32(cx, v.FromX()) || !approxEqF32(cy, v.FromY()) {
			t.Errorf("got (%.4f,%.4f); want (%.4f,%.4f)", cx, cy, v.FromX(), v.FromY())
		}
	})

	t.Run("PathType 2 spiral t=1 returns To", func(t *testing.T) {
		v := board.NewVictimAnim(0, 0, board.Cell{}, 100, 200, 50, 100, 2, 0, 0, 1.5)
		cx, cy := VictimAnimPos(v, 1)
		// At t=1: r = r0*(1-1) = 0, so cx = ToX + 0, cy = ToY + 0
		if !approxEqF32(cx, v.ToX()) || !approxEqF32(cy, v.ToY()) {
			t.Errorf("got (%.4f,%.4f); want (%.4f,%.4f)", cx, cy, v.ToX(), v.ToY())
		}
	})

	t.Run("PathType 2 spiral From==To is stationary", func(t *testing.T) {
		// When From==To, r0=0 so cx=ToX, cy=ToY for all t.
		v := board.NewVictimAnim(0, 0, board.Cell{}, 64, 128, 64, 128, 2, 0, 0, 2.0)
		for _, tval := range []float32{0, 0.25, 0.5, 0.75, 1.0} {
			cx, cy := VictimAnimPos(v, tval)
			if !approxEqF32(cx, v.ToX()) || !approxEqF32(cy, v.ToY()) {
				t.Errorf("t=%.2f: got (%.4f,%.4f); want (%.4f,%.4f)", tval, cx, cy, v.ToX(), v.ToY())
			}
		}
	})

	t.Run("PathType 2 spiral mid-t matches manual formula", func(t *testing.T) {
		v := board.NewVictimAnim(0, 0, board.Cell{}, 100, 200, 50, 100, 2, 0, 0, 1.0)
		tval := float32(0.5)
		ddx := v.FromX() - v.ToX()
		ddy := v.FromY() - v.ToY()
		r0 := float32(math.Sqrt(float64(ddx*ddx + ddy*ddy)))
		theta0 := float32(math.Atan2(float64(ddy), float64(ddx)))
		r := r0 * (1 - tval)
		theta := theta0 + v.SpiralTurns()*2*float32(math.Pi)*tval
		wantX := v.ToX() + r*float32(math.Cos(float64(theta)))
		wantY := v.ToY() + r*float32(math.Sin(float64(theta)))

		cx, cy := VictimAnimPos(v, tval)
		if !approxEqF32(cx, wantX) || !approxEqF32(cy, wantY) {
			t.Errorf("got (%.4f,%.4f); want (%.4f,%.4f)", cx, cy, wantX, wantY)
		}
	})
}

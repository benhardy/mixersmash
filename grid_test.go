package main

import (
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"mixersmash/internal/anim"
	"mixersmash/internal/board"
)

// ---------------------------------------------------------------------------
// State machine constants
// ---------------------------------------------------------------------------

// TestStateConstants documents the expected values of the state identifiers so
// that any accidental renumbering is caught immediately.
func TestStateConstants(t *testing.T) {
	if stateWaiting != 1 {
		t.Errorf("stateWaiting = %d, want 1", stateWaiting)
	}
	if stateShuffling != 2 {
		t.Errorf("stateShuffling = %d, want 2", stateShuffling)
	}
	if stateDropping != 3 {
		t.Errorf("stateDropping = %d, want 3", stateDropping)
	}
	if stateColumnCollapsing != 5 {
		t.Errorf("stateColumnCollapsing = %d, want 5", stateColumnCollapsing)
	}
}

// TestDurationConstants documents the wall-clock duration of each animation
// phase and verifies that they equal the original tick-based values at 60 TPS.
func TestDurationConstants(t *testing.T) {
	if anim.ShuffleDuration != 300*time.Millisecond {
		t.Errorf("anim.ShuffleDuration = %v, want 300ms", anim.ShuffleDuration)
	}
	if anim.DropDuration != 300*time.Millisecond {
		t.Errorf("anim.DropDuration = %v, want 300ms", anim.DropDuration)
	}
	if victimDuration != 400*time.Millisecond {
		t.Errorf("victimDuration = %v, want 400ms", victimDuration)
	}
	if targetAnimDuration != 250*time.Millisecond {
		t.Errorf("targetAnimDuration = %v, want 250ms", targetAnimDuration)
	}
}

// ---------------------------------------------------------------------------
// ElapsedFraction
// ---------------------------------------------------------------------------

// TestElapsedFraction verifies the 0→1 progress formula across representative
// inputs including the start, mid-point, and completion time, with clamping.
func TestElapsedFraction(t *testing.T) {
	base := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	dur := 400 * time.Millisecond

	cases := []struct {
		name    string
		elapsed time.Duration
		want    float32
	}{
		{"zero elapsed", 0, 0.0},
		{"half elapsed", 200 * time.Millisecond, 0.5},
		{"full elapsed", 400 * time.Millisecond, 1.0},
		{"over elapsed clamped", 800 * time.Millisecond, 1.0},
		{"one tenth in", 40 * time.Millisecond, 0.1},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := board.ElapsedFraction(base, base.Add(tc.elapsed), dur)
			if got != tc.want {
				t.Errorf("ElapsedFraction elapsed=%v dur=%v = %v, want %v",
					tc.elapsed, dur, got, tc.want)
			}
		})
	}

	// Sub-tick smoothness: two instants within the same 1/60s tick window must
	// produce different fractions, proving wall-clock resolution is used.
	t.Run("sub-tick smoothness", func(t *testing.T) {
		start := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		tickWindow := time.Second / 60 // ~16.67ms
		now1 := start.Add(tickWindow / 3)
		now2 := start.Add(2 * tickWindow / 3)
		f1 := board.ElapsedFraction(start, now1, anim.ShuffleDuration)
		f2 := board.ElapsedFraction(start, now2, anim.ShuffleDuration)
		if f1 == f2 {
			t.Errorf("sub-tick: ElapsedFraction at %v and %v within same tick window returned identical fraction %v; wall-clock not used",
				now1.Sub(start), now2.Sub(start), f1)
		}
		if f1 >= f2 {
			t.Errorf("sub-tick: fraction should increase with time: f1=%v f2=%v", f1, f2)
		}
	})
}

// ---------------------------------------------------------------------------
// newGameState — initial field values
// ---------------------------------------------------------------------------

// TestNewGameStateInitialFields verifies that newGameState produces a Game
// with the correct initial state machine values without requiring any ebiten
// context.
func TestNewGameStateInitialFields(t *testing.T) {
	grid := board.NewGrid(4, 4)
	g := newGameState(grid)

	if g.anim.currentState != stateWaiting {
		t.Errorf("currentState = %d, want stateWaiting (%d)", g.anim.currentState, stateWaiting)
	}
	if !g.anim.animStart.IsZero() {
		t.Errorf("animStart = %v, want zero time.Time", g.anim.animStart)
	}
	if g.anim.actionCol != -1 {
		t.Errorf("actionCol = %d, want -1", g.anim.actionCol)
	}
	if g.anim.actionRow != -1 {
		t.Errorf("actionRow = %d, want -1", g.anim.actionRow)
	}
	if g.view.canvas != nil {
		t.Errorf("canvas should be nil after newGameState, got non-nil")
	}
	if g.board.grid != grid {
		t.Errorf("grid pointer mismatch")
	}
}

// ---------------------------------------------------------------------------
// Health
// ---------------------------------------------------------------------------

// TestHealthConstants documents the expected values of the health magic numbers
// so that any accidental change to the keep-fraction or floor is caught immediately.
func TestHealthConstants(t *testing.T) {
	if healthDecrementKeep != float32(0.75) {
		t.Errorf("healthDecrementKeep = %v, want 0.75", healthDecrementKeep)
	}
	if healthMin != float32(0.562) {
		t.Errorf("healthMin = %v, want 0.562", healthMin)
	}
}

// TestHealthEpsilonConstant pins the value of healthEpsilon so that any
// accidental change to the zero-clamp threshold is caught immediately.
func TestHealthEpsilonConstant(t *testing.T) {
	if healthEpsilon != 0.001 {
		t.Errorf("healthEpsilon = %v, want 0.001", healthEpsilon)
	}
}

// ---------------------------------------------------------------------------
// findReleasedID
// ---------------------------------------------------------------------------

// TestFindReleasedID verifies the pure predicate across present/absent/empty cases.
func TestFindReleasedID(t *testing.T) {
	cases := []struct {
		name   string
		ids    []ebiten.TouchID
		target ebiten.TouchID
		want   bool
	}{
		{
			name:   "empty list returns false",
			ids:    nil,
			target: 1,
			want:   false,
		},
		{
			name:   "target present in singleton list",
			ids:    []ebiten.TouchID{3},
			target: 3,
			want:   true,
		},
		{
			name:   "target absent from list",
			ids:    []ebiten.TouchID{1, 2, 4},
			target: 3,
			want:   false,
		},
		{
			name:   "target present among multiple IDs",
			ids:    []ebiten.TouchID{1, 2, 3, 4},
			target: 2,
			want:   true,
		},
		{
			name:   "target is zero value and present",
			ids:    []ebiten.TouchID{0, 5},
			target: 0,
			want:   true,
		},
		{
			name:   "target is zero value and absent",
			ids:    []ebiten.TouchID{1, 2},
			target: 0,
			want:   false,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := findReleasedID(tc.ids, tc.target)
			if got != tc.want {
				t.Errorf("findReleasedID(%v, %v) = %v, want %v", tc.ids, tc.target, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// frameDims
// ---------------------------------------------------------------------------

// TestFrameDimsNilFrame verifies the fallback path when g.frame is nil.
// The nil case is fully reachable without an ebiten context.
func TestFrameDimsNilFrame(t *testing.T) {
	grid := board.NewGrid(4, 4)
	g := newGameState(grid)
	// g.frame is nil after newGameState — this exercises the fallback branch.
	frameW, frameH, hudDevW := g.frameDims()
	if frameW != float64(windowWidth) {
		t.Errorf("frameDims (nil frame) frameW = %v, want %v", frameW, float64(windowWidth))
	}
	if frameH != float64(windowHeight) {
		t.Errorf("frameDims (nil frame) frameH = %v, want %v", frameH, float64(windowHeight))
	}
	if hudDevW != 0 {
		t.Errorf("frameDims (nil frame) hudDevW = %v, want 0", hudDevW)
	}
}

// Note: testing the g.frame != nil branch requires an ebiten graphics context
// (ebiten.NewImage), which is unavailable in a headless unit test.  The
// nil/fallback branch above covers the constant path; the live path is exercised
// by the game at runtime.

// ---------------------------------------------------------------------------
// updateHover
// ---------------------------------------------------------------------------

// makeHoverGame builds a minimal Game with a cols×rows grid for updateHover tests.
// All cells are set alive (Health=1.0) with a palette creature.
// Callers may set specific cells dead or nil afterwards.
func makeHoverGame(cols, rows int) *Game {
	grid := board.NewGrid(cols, rows)
	// Assign creatures and set health so cells are alive.
	for c := 0; c < cols; c++ {
		for r := 0; r < rows; r++ {
			idx := (c*rows + r) % len(board.OrganismPalette)
			grid.Place(c, r, board.NewCell(&board.OrganismPalette[idx], 1.0))
		}
	}
	g := newGameState(grid)
	return g
}

// TestUpdateHoverDeadCellLeavesHoverUnchanged verifies that hovering a dead cell
// (Health == 0) leaves the previous hover state intact.
func TestUpdateHoverDeadCellLeavesHoverUnchanged(t *testing.T) {
	g := makeHoverGame(3, 3)
	// Pre-set hover to a known position.
	g.input.hoverCol = 0
	g.input.hoverRow = 0

	// Kill cell (1,1).
	g.board.grid.SetHealthZero(1, 1)

	g.updateHover(1, 1)

	if g.input.hoverCol != 0 || g.input.hoverRow != 0 {
		t.Errorf("updateHover on dead cell changed hover to (%d,%d), want (0,0)", g.input.hoverCol, g.input.hoverRow)
	}
}

// TestUpdateHoverOutOfBoundsLeavesHoverUnchanged verifies that an out-of-bounds
// (hc, hr) pair leaves the hover state unchanged.
func TestUpdateHoverOutOfBoundsLeavesHoverUnchanged(t *testing.T) {
	g := makeHoverGame(3, 3)
	g.input.hoverCol = 1
	g.input.hoverRow = 1

	// Negative col — out of bounds.
	g.updateHover(-1, 0)
	if g.input.hoverCol != 1 || g.input.hoverRow != 1 {
		t.Errorf("updateHover(-1,0) changed hover to (%d,%d), want (1,1)", g.input.hoverCol, g.input.hoverRow)
	}

	// Col beyond grid width.
	g.updateHover(3, 0)
	if g.input.hoverCol != 1 || g.input.hoverRow != 1 {
		t.Errorf("updateHover(3,0) changed hover to (%d,%d), want (1,1)", g.input.hoverCol, g.input.hoverRow)
	}

	// Row beyond grid height.
	g.updateHover(0, 3)
	if g.input.hoverCol != 1 || g.input.hoverRow != 1 {
		t.Errorf("updateHover(0,3) changed hover to (%d,%d), want (1,1)", g.input.hoverCol, g.input.hoverRow)
	}
}

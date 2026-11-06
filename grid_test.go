package main

import (
	"image/color"
	"testing"
)

// TestRandomColorAlpha verifies that randomColor returns a colour with A == 255.
func TestRandomColorAlpha(t *testing.T) {
	for i := 0; i < 100; i++ {
		c := randomColor()
		if c.A != 255 {
			t.Fatalf("randomColor() returned A=%d, want 255", c.A)
		}
	}
}

// TestRandomColorReturnsRGBA verifies that randomColor returns a color.RGBA value.
func TestRandomColorReturnsRGBA(t *testing.T) {
	var _ color.RGBA = randomColor()
}

// TestRecolorReturnsDifferentColor verifies that recolor returns a colour
// different from the one passed in. We try several times because there is a
// tiny probability that two random colours coincide.
func TestRecolorReturnsDifferentColor(t *testing.T) {
	current := color.RGBA{R: 128, G: 64, B: 32, A: 255}
	same := 0
	const trials = 50
	for i := 0; i < trials; i++ {
		next := recolor(current)
		if next.A != 255 {
			t.Fatalf("recolor() returned A=%d, want 255", next.A)
		}
		if next == current {
			same++
		}
	}
	// Collisions are acceptable occasionally but never on every trial.
	if same == trials {
		t.Fatalf("recolor() returned the same colour on all %d trials", trials)
	}
}

// TestCellFromPixel verifies that cellFromPixel maps pixel coordinates to the
// correct (col, row) grid cell.
func TestCellFromPixel(t *testing.T) {
	cases := []struct {
		px, py, cellSize int
		wantCol, wantRow int
	}{
		{0, 0, 32, 0, 0},
		{31, 31, 32, 0, 0},
		{32, 0, 32, 1, 0},
		{0, 32, 32, 0, 1},
		{64, 96, 32, 2, 3},
		{10, 20, 10, 1, 2},
	}
	for _, tc := range cases {
		col, row := cellFromPixel(tc.px, tc.py, tc.cellSize)
		if col != tc.wantCol || row != tc.wantRow {
			t.Errorf("cellFromPixel(%d, %d, %d) = (%d, %d), want (%d, %d)",
				tc.px, tc.py, tc.cellSize, col, row, tc.wantCol, tc.wantRow)
		}
	}
}

// TestGridInitialisation verifies that NewGrid returns a Grid whose Colors
// slice has dimensions [Cols][Rows] with every cell having A == 255.
func TestGridInitialisation(t *testing.T) {
	const cols, rows = 5, 4
	g := NewGrid(cols, rows)

	if g.Cols != cols {
		t.Errorf("Grid.Cols = %d, want %d", g.Cols, cols)
	}
	if g.Rows != rows {
		t.Errorf("Grid.Rows = %d, want %d", g.Rows, rows)
	}
	if len(g.Colors) != cols {
		t.Fatalf("len(Grid.Colors) = %d, want %d", len(g.Colors), cols)
	}
	for c := 0; c < cols; c++ {
		if len(g.Colors[c]) != rows {
			t.Fatalf("len(Grid.Colors[%d]) = %d, want %d", c, len(g.Colors[c]), rows)
		}
		for r := 0; r < rows; r++ {
			cell := g.Colors[c][r]
			if cell.A != 255 {
				t.Errorf("Grid.Colors[%d][%d].A = %d, want 255", c, r, cell.A)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// State machine constants
// ---------------------------------------------------------------------------

// TestStateConstants documents the expected values of the three state
// identifiers so that any accidental renumbering is caught immediately.
func TestStateConstants(t *testing.T) {
	if stateWaiting != 1 {
		t.Errorf("stateWaiting = %d, want 1", stateWaiting)
	}
	if stateCollapsing != 2 {
		t.Errorf("stateCollapsing = %d, want 2", stateCollapsing)
	}
	if stateSpawning != 3 {
		t.Errorf("stateSpawning = %d, want 3", stateSpawning)
	}
}

// TestTickConstants documents the duration of each animation phase in ticks.
func TestTickConstants(t *testing.T) {
	if collapseTicks != 5 {
		t.Errorf("collapseTicks = %d, want 5", collapseTicks)
	}
	if spawnTicks != 10 {
		t.Errorf("spawnTicks = %d, want 10", spawnTicks)
	}
}

// ---------------------------------------------------------------------------
// elapsedFraction
// ---------------------------------------------------------------------------

// TestElapsedFraction verifies the 0→1 progress formula across representative
// inputs including the start, mid-point, and completion tick.
func TestElapsedFraction(t *testing.T) {
	cases := []struct {
		name        string
		current     int64
		start       int64
		duration    int64
		want        float32
	}{
		{"zero elapsed", 10, 10, 5, 0.0},
		{"half elapsed", 12, 10, 4, 0.5},
		{"full elapsed", 15, 10, 5, 1.0},
		{"over elapsed", 20, 10, 5, 2.0},
		{"one tick in", 11, 10, 10, 0.1},
	}
	for _, tc := range cases {
		got := elapsedFraction(tc.current, tc.start, tc.duration)
		if got != tc.want {
			t.Errorf("%s: elapsedFraction(%d, %d, %d) = %v, want %v",
				tc.name, tc.current, tc.start, tc.duration, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// newGameState — initial field values
// ---------------------------------------------------------------------------

// TestNewGameStateInitialFields verifies that newGameState produces a Game
// with the correct initial state machine values without requiring any ebiten
// context.
func TestNewGameStateInitialFields(t *testing.T) {
	grid := NewGrid(4, 4)
	g := newGameState(grid)

	if g.currentState != stateWaiting {
		t.Errorf("currentState = %d, want stateWaiting (%d)", g.currentState, stateWaiting)
	}
	if g.startTick != 0 {
		t.Errorf("startTick = %d, want 0", g.startTick)
	}
	if g.actionCol != -1 {
		t.Errorf("actionCol = %d, want -1", g.actionCol)
	}
	if g.actionRow != -1 {
		t.Errorf("actionRow = %d, want -1", g.actionRow)
	}
	if g.canvas != nil {
		t.Errorf("canvas should be nil after newGameState, got non-nil")
	}
	if g.grid != grid {
		t.Errorf("grid pointer mismatch")
	}
}

// ---------------------------------------------------------------------------
// advanceState — state machine transitions
// ---------------------------------------------------------------------------

// newTestGame returns a Game wired up for testing: no canvas (nil is safe
// inside advanceState), a small grid, and the given state/tick.
func newTestGame(state int, startTick int64, col, row int) *Game {
	grid := NewGrid(4, 4)
	g := newGameState(grid)
	g.currentState = state
	g.startTick = startTick
	g.actionCol = col
	g.actionRow = row
	return g
}

// TestAdvanceStateCollapsingMidway confirms that mid-animation ticks keep the
// game in stateCollapsing.
func TestAdvanceStateCollapsingMidway(t *testing.T) {
	g := newTestGame(stateCollapsing, 100, 1, 1)
	// Advance by half of collapseTicks — should still be collapsing.
	g.advanceState(100 + collapseTicks/2)
	if g.currentState != stateCollapsing {
		t.Errorf("currentState = %d after mid-collapse tick, want stateCollapsing (%d)",
			g.currentState, stateCollapsing)
	}
}

// TestAdvanceStateCollapsingToSpawning verifies that reaching collapseTicks
// transitions from stateCollapsing to stateSpawning and resets startTick.
func TestAdvanceStateCollapsingToSpawning(t *testing.T) {
	const start int64 = 100
	g := newTestGame(stateCollapsing, start, 1, 1)
	completionTick := start + collapseTicks
	g.advanceState(completionTick)
	if g.currentState != stateSpawning {
		t.Errorf("currentState = %d after collapse complete, want stateSpawning (%d)",
			g.currentState, stateSpawning)
	}
	if g.startTick != completionTick {
		t.Errorf("startTick = %d after transition, want %d", g.startTick, completionTick)
	}
}

// TestAdvanceStateSpawningMidway confirms that mid-animation ticks keep the
// game in stateSpawning.
func TestAdvanceStateSpawningMidway(t *testing.T) {
	g := newTestGame(stateSpawning, 200, 2, 2)
	g.advanceState(200 + spawnTicks/2)
	if g.currentState != stateSpawning {
		t.Errorf("currentState = %d after mid-spawn tick, want stateSpawning (%d)",
			g.currentState, stateSpawning)
	}
}

// TestAdvanceStateSpawningToWaiting verifies that reaching spawnTicks
// transitions from stateSpawning to stateWaiting and resets startTick.
func TestAdvanceStateSpawningToWaiting(t *testing.T) {
	const start int64 = 200
	g := newTestGame(stateSpawning, start, 2, 2)
	completionTick := start + spawnTicks
	g.advanceState(completionTick)
	if g.currentState != stateWaiting {
		t.Errorf("currentState = %d after spawn complete, want stateWaiting (%d)",
			g.currentState, stateWaiting)
	}
	if g.startTick != completionTick {
		t.Errorf("startTick = %d after transition, want %d", g.startTick, completionTick)
	}
}

// TestAdvanceStateWaitingIsNoop verifies that calling advanceState while in
// stateWaiting does not alter any state fields.
func TestAdvanceStateWaitingIsNoop(t *testing.T) {
	const start int64 = 50
	g := newTestGame(stateWaiting, start, -1, -1)
	g.advanceState(start + 100)
	if g.currentState != stateWaiting {
		t.Errorf("currentState changed during stateWaiting; got %d", g.currentState)
	}
	if g.startTick != start {
		t.Errorf("startTick changed during stateWaiting; got %d, want %d", g.startTick, start)
	}
}

// ---------------------------------------------------------------------------
// Click gating
// ---------------------------------------------------------------------------

// TestClickGatingDuringCollapsing verifies that simulating the click-handling
// branch while in stateCollapsing does not alter currentState or actionCol/Row.
// advanceState is the pure path; we test that it ignores the stateWaiting click
// branch entirely (i.e. advanceState only handles non-waiting states).
func TestClickGatingDuringCollapsing(t *testing.T) {
	g := newTestGame(stateCollapsing, 100, 1, 1)
	// A direct call to advanceState (the path Update takes for non-waiting
	// states) must not change actionCol/Row or flip back to waiting.
	g.advanceState(101) // one tick in — far from completion
	if g.currentState != stateCollapsing {
		t.Errorf("state changed unexpectedly: got %d, want stateCollapsing (%d)",
			g.currentState, stateCollapsing)
	}
	if g.actionCol != 1 || g.actionRow != 1 {
		t.Errorf("actionCol/Row changed: got (%d, %d), want (1, 1)",
			g.actionCol, g.actionRow)
	}
}

// TestClickGatingDuringSpawning verifies the same gating for stateSpawning.
func TestClickGatingDuringSpawning(t *testing.T) {
	g := newTestGame(stateSpawning, 200, 3, 2)
	g.advanceState(201) // one tick in — far from completion
	if g.currentState != stateSpawning {
		t.Errorf("state changed unexpectedly: got %d, want stateSpawning (%d)",
			g.currentState, stateSpawning)
	}
	if g.actionCol != 3 || g.actionRow != 2 {
		t.Errorf("actionCol/Row changed: got (%d, %d), want (3, 2)",
			g.actionCol, g.actionRow)
	}
}

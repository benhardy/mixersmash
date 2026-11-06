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

// TestRecolorReturnsDifferentColor verifies that recolor mutates the cell to a
// colour different from the one it started with. We try several times because
// there is a tiny probability that two random colours coincide.
func TestRecolorReturnsDifferentColor(t *testing.T) {
	original := color.RGBA{R: 128, G: 64, B: 32, A: 255}
	same := 0
	const trials = 50
	for i := 0; i < trials; i++ {
		cell := &Cell{Color: original, Health: 1.0}
		recolor(cell)
		if cell.Color.A != 255 {
			t.Fatalf("recolor() produced A=%d, want 255", cell.Color.A)
		}
		if cell.Color == original {
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

// TestGridInitialisation verifies that NewGrid returns a Grid whose Cells
// slice has dimensions [Cols][Rows] with every cell having Color.A == 255.
func TestGridInitialisation(t *testing.T) {
	const cols, rows = 5, 4
	g := NewGrid(cols, rows)

	if g.Cols != cols {
		t.Errorf("Grid.Cols = %d, want %d", g.Cols, cols)
	}
	if g.Rows != rows {
		t.Errorf("Grid.Rows = %d, want %d", g.Rows, rows)
	}
	if len(g.Cells) != cols {
		t.Fatalf("len(Grid.Cells) = %d, want %d", len(g.Cells), cols)
	}
	for c := 0; c < cols; c++ {
		if len(g.Cells[c]) != rows {
			t.Fatalf("len(Grid.Cells[%d]) = %d, want %d", c, len(g.Cells[c]), rows)
		}
		for r := 0; r < rows; r++ {
			cell := g.Cells[c][r]
			if cell.Color.A != 255 {
				t.Errorf("Grid.Cells[%d][%d].Color.A = %d, want 255", c, r, cell.Color.A)
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

// ---------------------------------------------------------------------------
// Health
// ---------------------------------------------------------------------------

// TestHealthConstants documents the expected values of the health magic numbers
// so that any accidental change to the step or floor is caught immediately.
func TestHealthConstants(t *testing.T) {
	if healthDecrement != 0.2 {
		t.Errorf("healthDecrement = %v, want 0.2", healthDecrement)
	}
	if healthMin != 0.2 {
		t.Errorf("healthMin = %v, want 0.2", healthMin)
	}
}

// TestHealthEpsilonConstant pins the value of healthEpsilon so that any
// accidental change to the zero-clamp threshold is caught immediately.
func TestHealthEpsilonConstant(t *testing.T) {
	if healthEpsilon != 0.001 {
		t.Errorf("healthEpsilon = %v, want 0.001", healthEpsilon)
	}
}

// TestInitialCellHealth verifies that every cell produced by NewGrid starts
// with Health == 1.0.
func TestInitialCellHealth(t *testing.T) {
	const cols, rows = 3, 3
	g := NewGrid(cols, rows)
	for c := 0; c < cols; c++ {
		for r := 0; r < rows; r++ {
			if g.Cells[c][r].Health != 1.0 {
				t.Errorf("Cells[%d][%d].Health = %v, want 1.0", c, r, g.Cells[c][r].Health)
			}
		}
	}
}

// TestHealthDecrementsAfterCollapse verifies that completing a collapse
// (elapsed >= 1.0) decrements Health by healthDecrement when the cell has
// enough health remaining.
func TestHealthDecrementsAfterCollapse(t *testing.T) {
	const col, row = 1, 1
	g := newTestGame(stateCollapsing, 100, col, row)
	// Confirm starting health.
	if g.grid.Cells[col][row].Health != 1.0 {
		t.Fatalf("precondition: Health = %v, want 1.0", g.grid.Cells[col][row].Health)
	}
	// Drive the collapse to completion.
	g.advanceState(100 + collapseTicks)
	got := g.grid.Cells[col][row].Health
	want := float32(1.0) - healthDecrement
	if got != want {
		t.Errorf("Health after one collapse = %v, want %v", got, want)
	}
}

// TestHealthClampsToZeroOnFinalCollapse verifies that when Health is at or
// below healthMin before the final collapse, it clamps to 0.0 rather than
// going negative.
func TestHealthClampsToZeroOnFinalCollapse(t *testing.T) {
	const col, row = 2, 2
	g := newTestGame(stateCollapsing, 100, col, row)
	// Set health to healthMin — the threshold at which the clamp fires.
	g.grid.Cells[col][row].Health = healthMin
	g.advanceState(100 + collapseTicks)
	got := g.grid.Cells[col][row].Health
	if got != 0.0 {
		t.Errorf("Health after final collapse = %v, want 0.0 (clamped)", got)
	}
}

// TestAdvanceStateSkipsSpawningWhenHealthZero verifies that when a cell's
// Health reaches zero after a collapse completion the state machine transitions
// directly to stateWaiting, bypassing stateSpawning entirely.
func TestAdvanceStateSkipsSpawningWhenHealthZero(t *testing.T) {
	const col, row = 1, 2
	const startTick int64 = 50
	g := newGameState(NewGrid(4, 4))
	g.actionCol = col
	g.actionRow = row
	// healthMin triggers the clamp-to-zero branch, which then detects
	// Health < healthEpsilon and routes to stateWaiting instead of stateSpawning.
	g.grid.Cells[col][row].Health = healthMin
	g.currentState = stateCollapsing
	g.startTick = startTick

	g.advanceState(startTick + collapseTicks)

	if g.currentState != stateWaiting {
		t.Errorf("currentState = %d after zero-health collapse, want stateWaiting (%d)",
			g.currentState, stateWaiting)
	}
	if g.grid.Cells[col][row].Health != 0.0 {
		t.Errorf("Health = %v after zero-health collapse, want 0.0",
			g.grid.Cells[col][row].Health)
	}
}

// ---------------------------------------------------------------------------
// randomColor — white exclusion
// ---------------------------------------------------------------------------

// TestRandomColorNeverReturnsWhite verifies that randomColor never returns
// {R:255, G:255, B:255} across a large number of calls.
func TestRandomColorNeverReturnsWhite(t *testing.T) {
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	for i := 0; i < 10_000; i++ {
		c := randomColor()
		if c.R == white.R && c.G == white.G && c.B == white.B {
			t.Fatalf("randomColor() returned white {255,255,255} on iteration %d", i)
		}
	}
}

// ---------------------------------------------------------------------------
// cellNeighbours
// ---------------------------------------------------------------------------

// TestCellNeighboursCorner verifies that the corner cell (0,0) on a 25×18
// grid has exactly 2 valid cardinal neighbours: (1,0) and (0,1).
func TestCellNeighboursCorner(t *testing.T) {
	got := cellNeighbours(0, 0, 25, 18)
	if len(got) != 2 {
		t.Fatalf("cellNeighbours(0,0,25,18) returned %d neighbours, want 2: %v", len(got), got)
	}
	want := map[[2]int]bool{{1, 0}: true, {0, 1}: true}
	for _, n := range got {
		if !want[n] {
			t.Errorf("unexpected neighbour %v", n)
		}
	}
}

// TestCellNeighboursEdge verifies that an edge cell (0,5) on a 25×18 grid
// has exactly 3 valid cardinal neighbours.
func TestCellNeighboursEdge(t *testing.T) {
	got := cellNeighbours(0, 5, 25, 18)
	if len(got) != 3 {
		t.Fatalf("cellNeighbours(0,5,25,18) returned %d neighbours, want 3: %v", len(got), got)
	}
}

// TestCellNeighboursCentre verifies that an interior cell (5,5) on a 25×18
// grid has exactly 4 valid cardinal neighbours.
func TestCellNeighboursCentre(t *testing.T) {
	got := cellNeighbours(5, 5, 25, 18)
	if len(got) != 4 {
		t.Fatalf("cellNeighbours(5,5,25,18) returned %d neighbours, want 4: %v", len(got), got)
	}
}

// ---------------------------------------------------------------------------
// normaliseLink
// ---------------------------------------------------------------------------

// TestNormaliseLinkOrdering verifies that normaliseLink(3,2,1,4) and
// normaliseLink(1,4,3,2) return the same canonical value regardless of
// argument order.
func TestNormaliseLinkOrdering(t *testing.T) {
	a := normaliseLink(3, 2, 1, 4)
	b := normaliseLink(1, 4, 3, 2)
	if a != b {
		t.Errorf("normaliseLink not symmetric: normaliseLink(3,2,1,4)=%v, normaliseLink(1,4,3,2)=%v", a, b)
	}
}

// ---------------------------------------------------------------------------
// addLink
// ---------------------------------------------------------------------------

// TestAddLinkNoDuplicate verifies that adding the same (col,row) pair twice
// results in a slice of length 1 — duplicates are suppressed.
func TestAddLinkNoDuplicate(t *testing.T) {
	var links [][4]int
	links = addLink(links, 1, 1, 2, 2)
	links = addLink(links, 1, 1, 2, 2)
	if len(links) != 1 {
		t.Errorf("addLink with duplicate: got len=%d, want 1; links=%v", len(links), links)
	}
}

// ---------------------------------------------------------------------------
// removeLinksForCell
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// healthDrainPerCell
// ---------------------------------------------------------------------------

// TestHealthDrainPerCellNoLinks verifies that with zero linked neighbours the
// full totalDrain is returned (divided by 1).
func TestHealthDrainPerCellNoLinks(t *testing.T) {
	got := healthDrainPerCell(0.2, 0)
	if got != 0.2 {
		t.Errorf("healthDrainPerCell(0.2, 0) = %v, want 0.2", got)
	}
}

// TestHealthDrainPerCellOneLink verifies that with one linked neighbour the
// drain is split equally between the clicked cell and that neighbour (÷2).
func TestHealthDrainPerCellOneLink(t *testing.T) {
	got := healthDrainPerCell(0.2, 1)
	if got != 0.1 {
		t.Errorf("healthDrainPerCell(0.2, 1) = %v, want 0.1", got)
	}
}

// TestHealthDrainPerCellTwoLinks verifies that with two linked neighbours the
// drain is shared across three cells (clicked + 2 links), i.e. ≈ 0.2/3.
func TestHealthDrainPerCellTwoLinks(t *testing.T) {
	got := healthDrainPerCell(0.2, 2)
	want := float32(0.2) / float32(3)
	diff := got - want
	if diff < 0 {
		diff = -diff
	}
	if diff > 1e-6 {
		t.Errorf("healthDrainPerCell(0.2, 2) = %v, want ~%v (diff %v > 1e-6)", got, want, diff)
	}
}

// ---------------------------------------------------------------------------
// linkedCells
// ---------------------------------------------------------------------------

// TestLinkedCellsNone verifies that a cell not present in any link entry
// returns an empty (nil or zero-length) slice.
func TestLinkedCellsNone(t *testing.T) {
	links := [][4]int{
		{3, 3, 4, 3}, // neither endpoint is (1,1)
	}
	got := linkedCells(links, 1, 1)
	if len(got) != 0 {
		t.Errorf("linkedCells with no matching links: got %v, want empty", got)
	}
}

// TestLinkedCellsOne verifies that cell (1,1) linked to (2,1) returns a slice
// containing exactly {2, 1}.
func TestLinkedCellsOne(t *testing.T) {
	links := [][4]int{
		normaliseLink(1, 1, 2, 1),
	}
	got := linkedCells(links, 1, 1)
	if len(got) != 1 {
		t.Fatalf("linkedCells: got %d results, want 1; got=%v", len(got), got)
	}
	if got[0] != [2]int{2, 1} {
		t.Errorf("linkedCells: got[0] = %v, want {2, 1}", got[0])
	}
}

// TestLinkedCellsTwo verifies that cell (1,1) linked to both (2,1) and (1,2)
// returns both neighbours.
func TestLinkedCellsTwo(t *testing.T) {
	links := [][4]int{
		normaliseLink(1, 1, 2, 1),
		normaliseLink(1, 1, 1, 2),
	}
	got := linkedCells(links, 1, 1)
	if len(got) != 2 {
		t.Fatalf("linkedCells: got %d results, want 2; got=%v", len(got), got)
	}
	found := map[[2]int]bool{}
	for _, c := range got {
		found[c] = true
	}
	if !found[[2]int{2, 1}] {
		t.Errorf("linkedCells: missing {2, 1}; got=%v", got)
	}
	if !found[[2]int{1, 2}] {
		t.Errorf("linkedCells: missing {1, 2}; got=%v", got)
	}
}

// ---------------------------------------------------------------------------
// hasLink
// ---------------------------------------------------------------------------

// TestHasLink verifies that hasLink returns true when the link exists (in
// either direction) and false when it does not.
func TestHasLink(t *testing.T) {
	links := [][4]int{
		normaliseLink(1, 1, 2, 1), // link between (1,1) and (2,1)
		normaliseLink(3, 0, 3, 1), // link between (3,0) and (3,1)
	}

	// Present in forward direction.
	if !hasLink(links, 1, 1, 2, 1) {
		t.Error("hasLink(links, 1,1, 2,1) = false, want true (forward)")
	}
	// Present in reverse direction.
	if !hasLink(links, 2, 1, 1, 1) {
		t.Error("hasLink(links, 2,1, 1,1) = false, want true (reverse)")
	}
	// Second link, forward.
	if !hasLink(links, 3, 0, 3, 1) {
		t.Error("hasLink(links, 3,0, 3,1) = false, want true (second link forward)")
	}
	// Second link, reverse.
	if !hasLink(links, 3, 1, 3, 0) {
		t.Error("hasLink(links, 3,1, 3,0) = false, want true (second link reverse)")
	}
	// Not present.
	if hasLink(links, 0, 0, 1, 0) {
		t.Error("hasLink(links, 0,0, 1,0) = true, want false")
	}
	// Empty slice.
	if hasLink([][4]int{}, 1, 1, 2, 1) {
		t.Error("hasLink(empty, 1,1, 2,1) = true, want false")
	}
}

// TestRemoveLinksForCellRemovesBoth verifies that a link [1,1,2,2] is removed
// when queried by either endpoint: (col=1,row=1) or (col=2,row=2).
func TestRemoveLinksForCellRemovesBoth(t *testing.T) {
	// Build a slice with a single pre-normalised link between (1,1) and (2,2).
	initial := [][4]int{{1, 1, 2, 2}}

	// Remove by first endpoint.
	result := removeLinksForCell(initial, 1, 1)
	if len(result) != 0 {
		t.Errorf("removeLinksForCell with col=1,row=1: got %v, want empty slice", result)
	}

	// Remove by second endpoint.
	result = removeLinksForCell(initial, 2, 2)
	if len(result) != 0 {
		t.Errorf("removeLinksForCell with col=2,row=2: got %v, want empty slice", result)
	}
}

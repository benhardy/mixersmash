package main

import (
	"image/color"
	"sort"
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
}

// TestTickConstants documents the duration of each animation phase in ticks.
func TestTickConstants(t *testing.T) {
	if shuffleTicks != 18 {
		t.Errorf("shuffleTicks = %d, want 18", shuffleTicks)
	}
	if dropTicks != 18 {
		t.Errorf("dropTicks = %d, want 18", dropTicks)
	}
}

// ---------------------------------------------------------------------------
// elapsedFraction
// ---------------------------------------------------------------------------

// TestElapsedFraction verifies the 0→1 progress formula across representative
// inputs including the start, mid-point, and completion tick.
func TestElapsedFraction(t *testing.T) {
	cases := []struct {
		name     string
		current  int64
		start    int64
		duration int64
		want     float32
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
	if g.animStart != 0 {
		t.Errorf("animStart = %d, want 0", g.animStart)
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
// colorDistanceRedmean
// ---------------------------------------------------------------------------

// TestColorDistanceRedmeanSame verifies that a colour compared with itself is 0.
func TestColorDistanceRedmeanSame(t *testing.T) {
	c := color.RGBA{R: 100, G: 150, B: 200, A: 255}
	got := colorDistanceRedmean(c, c)
	if got != 0.0 {
		t.Errorf("colorDistanceRedmean(c, c) = %v, want 0.0", got)
	}
}

// TestColorDistanceRedmeanGreenWeighted verifies that a pure green shift produces
// a larger distance than an equal pure blue shift, since green is weighted 4× vs
// blue's ~2× in the redmean formula.
func TestColorDistanceRedmeanGreenWeighted(t *testing.T) {
	black := color.RGBA{R: 0, G: 0, B: 0, A: 255}
	green := color.RGBA{R: 0, G: 100, B: 0, A: 255}
	blue := color.RGBA{R: 0, G: 0, B: 100, A: 255}
	greenDist := colorDistanceRedmean(green, black)
	blueDist := colorDistanceRedmean(blue, black)
	if greenDist <= blueDist {
		t.Errorf("expected green distance (%v) > blue distance (%v)", greenDist, blueDist)
	}
}

// ---------------------------------------------------------------------------
// eightNeighbors
// ---------------------------------------------------------------------------

// sortPairs is a helper that sorts a [][2]int slice lexicographically so test
// comparisons are order-independent.
func sortPairs(pairs [][2]int) [][2]int {
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i][0] != pairs[j][0] {
			return pairs[i][0] < pairs[j][0]
		}
		return pairs[i][1] < pairs[j][1]
	})
	return pairs
}

// TestEightNeighborsCorner verifies that eightNeighbors(0,0,25,18) returns
// exactly 3 results: {1,0}, {0,1}, {1,1}.
func TestEightNeighborsCorner(t *testing.T) {
	got := eightNeighbors(0, 0, 25, 18)
	if len(got) != 3 {
		t.Fatalf("eightNeighbors(0,0,25,18) returned %d results, want 3: %v", len(got), got)
	}
	want := map[[2]int]bool{
		{1, 0}: true,
		{0, 1}: true,
		{1, 1}: true,
	}
	for _, n := range got {
		if !want[n] {
			t.Errorf("eightNeighbors(0,0,25,18): unexpected result %v", n)
		}
	}
}

// TestEightNeighborsEdge verifies that eightNeighbors(0,5,25,18) returns
// exactly 5 results (left edge, non-corner).
func TestEightNeighborsEdge(t *testing.T) {
	got := eightNeighbors(0, 5, 25, 18)
	if len(got) != 5 {
		t.Fatalf("eightNeighbors(0,5,25,18) returned %d results, want 5: %v", len(got), got)
	}
}

// TestEightNeighborsCentre verifies that eightNeighbors(5,5,25,18) returns
// exactly 8 results for an interior cell.
func TestEightNeighborsCentre(t *testing.T) {
	got := eightNeighbors(5, 5, 25, 18)
	if len(got) != 8 {
		t.Fatalf("eightNeighbors(5,5,25,18) returned %d results, want 8: %v", len(got), got)
	}
}

// ---------------------------------------------------------------------------
// closestColorNeighbor
// ---------------------------------------------------------------------------

// TestClosestColorNeighborFindsNearest sets up a 3×3 grid with known colours
// and asserts that closestColorNeighbor returns the neighbour whose colour is
// closest to the centre cell.
func TestClosestColorNeighborFindsNearest(t *testing.T) {
	// Build a 3×3 grid: all cells healthy.
	cells := make([][]Cell, 3)
	for c := range cells {
		cells[c] = make([]Cell, 3)
		for r := range cells[c] {
			cells[c][r] = Cell{Color: color.RGBA{128, 128, 128, 255}, Health: 1.0}
		}
	}

	// Centre cell at (1,1) is pure red.
	cells[1][1].Color = color.RGBA{R: 200, G: 0, B: 0, A: 255}

	// Neighbour at (0,0): distant colour (blue).
	cells[0][0].Color = color.RGBA{R: 0, G: 0, B: 200, A: 255}
	// Neighbour at (2,2): closest colour to red (nearly red).
	cells[2][2].Color = color.RGBA{R: 195, G: 5, B: 5, A: 255}
	// Neighbour at (1,0): medium colour (green).
	cells[1][0].Color = color.RGBA{R: 0, G: 200, B: 0, A: 255}

	col, row, found := closestColorNeighbor(cells, 1, 1, 3, 3)
	if !found {
		t.Fatal("closestColorNeighbor returned found=false, want true")
	}
	if col != 2 || row != 2 {
		t.Errorf("closestColorNeighbor returned (%d, %d), want (2, 2)", col, row)
	}
}

// ---------------------------------------------------------------------------
// gravityMoves
// ---------------------------------------------------------------------------

// TestGravityMovesNoHoles verifies that gravityMoves with no holes returns an
// empty slice — no cell changes row when nothing is removed.
func TestGravityMovesNoHoles(t *testing.T) {
	got := gravityMoves(18, nil)
	if len(got) != 0 {
		t.Errorf("gravityMoves(18, nil) = %v, want empty slice", got)
	}
}

// TestGravityMovesSingleHoleMiddle verifies that removing the middle row of a
// 5-row column compacts rows 0 and 1 downward by one; rows 3 and 4 are already
// in their correct positions.
// alive=[0,1,3,4], newCount=1 → 0→1, 1→2; 3→3(stay), 4→4(stay)
func TestGravityMovesSingleHoleMiddle(t *testing.T) {
	got := gravityMoves(5, []int{2})
	want := [][2]int{{0, 1}, {1, 2}}
	if len(got) != len(want) {
		t.Fatalf("gravityMoves(5, [2]) = %v, want %v", got, want)
	}
	for i, m := range got {
		if m != want[i] {
			t.Errorf("gravityMoves(5, [2])[%d] = %v, want %v", i, m, want[i])
		}
	}
}

// TestGravityMovesSingleHoleTop verifies that removing the top row of a 5-row
// column results in no moves — alive rows [1,2,3,4] land at positions [1,2,3,4].
// alive=[1,2,3,4], newCount=1 → 1→1(stay), 2→2(stay), 3→3(stay), 4→4(stay)
func TestGravityMovesSingleHoleTop(t *testing.T) {
	got := gravityMoves(5, []int{0})
	if len(got) != 0 {
		t.Errorf("gravityMoves(5, [0]) = %v, want empty slice", got)
	}
}

// TestGravityMovesSingleHoleBottom verifies that removing the bottom row of a
// 5-row column compacts all remaining rows downward by one.
// alive=[0,1,2,3], newCount=1 → 0→1, 1→2, 2→3, 3→4
func TestGravityMovesSingleHoleBottom(t *testing.T) {
	got := gravityMoves(5, []int{4})
	want := [][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 4}}
	if len(got) != len(want) {
		t.Fatalf("gravityMoves(5, [4]) = %v, want %v", got, want)
	}
	for i, m := range got {
		if m != want[i] {
			t.Errorf("gravityMoves(5, [4])[%d] = %v, want %v", i, m, want[i])
		}
	}
}

// TestGravityMovesTwoHolesSameColumn verifies two non-adjacent holes in a
// 6-row column.
// alive=[0,2,4,5], newCount=2 → 0→2, 2→3; 4→4(stay), 5→5(stay)
func TestGravityMovesTwoHolesSameColumn(t *testing.T) {
	got := gravityMoves(6, []int{1, 3})
	want := [][2]int{{0, 2}, {2, 3}}
	if len(got) != len(want) {
		t.Fatalf("gravityMoves(6, [1,3]) = %v, want %v", got, want)
	}
	for i, m := range got {
		if m != want[i] {
			t.Errorf("gravityMoves(6, [1,3])[%d] = %v, want %v", i, m, want[i])
		}
	}
}

// TestGravityMovesAllHoles verifies that removing every row in a 3-row column
// returns an empty slice — no alive rows, no moves.
func TestGravityMovesAllHoles(t *testing.T) {
	got := gravityMoves(3, []int{0, 1, 2})
	if len(got) != 0 {
		t.Errorf("gravityMoves(3, [0,1,2]) = %v, want empty slice", got)
	}
}

// TestClosestColorNeighborSkipsDead verifies that a neighbour with Health==0
// is not returned even if it would be the closest by colour.
func TestClosestColorNeighborSkipsDead(t *testing.T) {
	// Build a 3×3 grid: all cells healthy.
	cells := make([][]Cell, 3)
	for c := range cells {
		cells[c] = make([]Cell, 3)
		for r := range cells[c] {
			cells[c][r] = Cell{Color: color.RGBA{128, 128, 128, 255}, Health: 1.0}
		}
	}

	// Centre cell at (1,1) is pure red.
	cells[1][1].Color = color.RGBA{R: 200, G: 0, B: 0, A: 255}

	// Dead neighbour at (2,2): perfect colour match but Health=0.
	cells[2][2].Color = color.RGBA{R: 200, G: 0, B: 0, A: 255}
	cells[2][2].Health = 0.0

	// Live neighbour at (0,0): worse colour match but alive.
	cells[0][0].Color = color.RGBA{R: 150, G: 50, B: 50, A: 255}

	// All other neighbours use the neutral grey already set.

	col, row, found := closestColorNeighbor(cells, 1, 1, 3, 3)
	if !found {
		t.Fatal("closestColorNeighbor returned found=false with live neighbours present")
	}
	// The dead cell (2,2) must not be returned.
	if col == 2 && row == 2 {
		t.Errorf("closestColorNeighbor returned dead cell (2,2)")
	}
	// Suppress unused variable warning in case the compiler complains.
	_ = col
	_ = row
}

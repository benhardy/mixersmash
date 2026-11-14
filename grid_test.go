package main

import (
	"image/color"
	"sort"
	"strings"
	"testing"
)

// cellWithColor returns a Cell with the given colour wrapped in a Creature.
// Health is left at its zero value; callers that need Health=1.0 must set it explicitly.
func cellWithColor(c color.RGBA) Cell {
	cr := &Creature{Color: c}
	return Cell{Creature: cr}
}

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
			if cell.Color().A != 255 {
				t.Errorf("Grid.Cells[%d][%d].Color().A = %d, want 255", c, r, cell.Color().A)
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
	if stateColumnCollapsing != 5 {
		t.Errorf("stateColumnCollapsing = %d, want 5", stateColumnCollapsing)
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
	if healthDecrement != float32(1.0/3.0) {
		t.Errorf("healthDecrement = %v, want %v (1/3)", healthDecrement, float32(1.0/3.0))
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
			cells[c][r] = cellWithColor(color.RGBA{128, 128, 128, 255})
			cells[c][r].Health = 1.0
		}
	}

	// Centre cell at (1,1) is pure red.
	cells[1][1].Creature = &Creature{Color: color.RGBA{R: 200, G: 0, B: 0, A: 255}}

	// Neighbour at (0,0): distant colour (blue).
	cells[0][0].Creature = &Creature{Color: color.RGBA{R: 0, G: 0, B: 200, A: 255}}
	// Neighbour at (2,2): closest colour to red (nearly red).
	cells[2][2].Creature = &Creature{Color: color.RGBA{R: 195, G: 5, B: 5, A: 255}}
	// Neighbour at (1,0): medium colour (green).
	cells[1][0].Creature = &Creature{Color: color.RGBA{R: 0, G: 200, B: 0, A: 255}}

	col, row, found := closestColorNeighbor(cells, 1, 1, 3, 3)
	if !found {
		t.Fatal("closestColorNeighbor returned found=false, want true")
	}
	if col != 2 || row != 2 {
		t.Errorf("closestColorNeighbor returned (%d, %d), want (2, 2)", col, row)
	}
}

// ---------------------------------------------------------------------------
// closestColorNeighbors (plural)
// ---------------------------------------------------------------------------

// makeUniformGrid returns a cols×rows grid of cells with the given colour and
// Health 1.0.  Callers override specific cells after calling this helper.
func makeUniformGrid(t *testing.T, cols, rows int, c color.RGBA) [][]Cell {
	t.Helper()
	cells := make([][]Cell, cols)
	for col := range cells {
		cells[col] = make([]Cell, rows)
		for row := range cells[col] {
			cells[col][row] = cellWithColor(c)
			cells[col][row].Health = 1.0
		}
	}
	return cells
}

// pairsAsSet converts a [][2]int slice into a map keyed by [2]int for
// order-independent set comparisons.
func pairsAsSet(pairs [][2]int) map[[2]int]bool {
	s := make(map[[2]int]bool, len(pairs))
	for _, p := range pairs {
		s[p] = true
	}
	return s
}

// TestClosestColorNeighborsSingleNearest verifies that when one neighbour is
// strictly closer than all others, the plural function returns exactly that one.
func TestClosestColorNeighborsSingleNearest(t *testing.T) {
	cells := makeUniformGrid(t, 3, 3, color.RGBA{128, 128, 128, 255})
	// Kill all cells; selectively revive only the target and three neighbours.
	for c := 0; c < 3; c++ {
		for r := 0; r < 3; r++ {
			cells[c][r].Health = 0
		}
	}

	// Target at (1,1): pure red.
	cells[1][1].Creature = &Creature{Color: color.RGBA{R: 200, G: 0, B: 0, A: 255}}
	cells[1][1].Health = 1.0

	// Strictly closest: nearly-red at (2,2).
	cells[2][2].Creature = &Creature{Color: color.RGBA{R: 195, G: 5, B: 5, A: 255}}
	cells[2][2].Health = 1.0
	// Farther: blue at (0,0).
	cells[0][0].Creature = &Creature{Color: color.RGBA{R: 0, G: 0, B: 200, A: 255}}
	cells[0][0].Health = 1.0
	// Farther: green at (1,0).
	cells[1][0].Creature = &Creature{Color: color.RGBA{R: 0, G: 200, B: 0, A: 255}}
	cells[1][0].Health = 1.0

	got := closestColorNeighbors(cells, 1, 1, 3, 3)
	if len(got) != 1 {
		t.Fatalf("want exactly 1 result, got %d: %v", len(got), got)
	}
	if got[0] != [2]int{2, 2} {
		t.Errorf("want {2,2}, got %v", got[0])
	}
}

// TestClosestColorNeighborsTwoWayTie verifies that two neighbours sharing the
// exact minimum redmean distance are both returned.
func TestClosestColorNeighborsTwoWayTie(t *testing.T) {
	// 3×3 grid with dead cells everywhere except the two tied neighbours and
	// the target, so that no other live cell can influence the result.
	cells := makeUniformGrid(t, 3, 3, color.RGBA{128, 128, 128, 255})
	// Kill all neighbours first.
	for c := 0; c < 3; c++ {
		for r := 0; r < 3; r++ {
			cells[c][r].Health = 0
		}
	}

	// Target at (1,1).
	cells[1][1].Creature = &Creature{Color: color.RGBA{R: 200, G: 100, B: 100, A: 255}}
	cells[1][1].Health = 1.0

	// Build two neighbours with identical colour so their distance to target
	// is bitwise-equal (same float32).
	tiedColor := color.RGBA{R: 190, G: 110, B: 90, A: 255}
	cells[0][0].Creature = &Creature{Color: tiedColor}
	cells[0][0].Health = 1.0
	cells[2][0].Creature = &Creature{Color: tiedColor}
	cells[2][0].Health = 1.0

	got := closestColorNeighbors(cells, 1, 1, 3, 3)
	if len(got) != 2 {
		t.Fatalf("want 2 results (tied pair), got %d: %v", len(got), got)
	}
	want := pairsAsSet([][2]int{{0, 0}, {2, 0}})
	for _, p := range got {
		if !want[p] {
			t.Errorf("unexpected result %v in tied pair", p)
		}
	}
}

// TestClosestColorNeighborsThreeWayTie verifies that three neighbours at the
// exact same minimum distance are all returned.
func TestClosestColorNeighborsThreeWayTie(t *testing.T) {
	cells := makeUniformGrid(t, 3, 3, color.RGBA{128, 128, 128, 255})
	// Kill all neighbours.
	for c := 0; c < 3; c++ {
		for r := 0; r < 3; r++ {
			cells[c][r].Health = 0
		}
	}

	// Target at (1,1).
	cells[1][1].Creature = &Creature{Color: color.RGBA{R: 200, G: 100, B: 100, A: 255}}
	cells[1][1].Health = 1.0

	tiedColor := color.RGBA{R: 185, G: 115, B: 85, A: 255}
	cells[0][0].Creature = &Creature{Color: tiedColor}
	cells[0][0].Health = 1.0
	cells[2][0].Creature = &Creature{Color: tiedColor}
	cells[2][0].Health = 1.0
	cells[0][2].Creature = &Creature{Color: tiedColor}
	cells[0][2].Health = 1.0

	got := closestColorNeighbors(cells, 1, 1, 3, 3)
	if len(got) != 3 {
		t.Fatalf("want 3 results (three-way tie), got %d: %v", len(got), got)
	}
	want := pairsAsSet([][2]int{{0, 0}, {2, 0}, {0, 2}})
	for _, p := range got {
		if !want[p] {
			t.Errorf("unexpected result %v in three-way tie", p)
		}
	}
}

// TestClosestColorNeighborsTieNotAtMinimum verifies that when two neighbours tie
// each other but a third is strictly closer, only the strictly-closest is returned.
func TestClosestColorNeighborsTieNotAtMinimum(t *testing.T) {
	cells := makeUniformGrid(t, 3, 3, color.RGBA{128, 128, 128, 255})
	for c := 0; c < 3; c++ {
		for r := 0; r < 3; r++ {
			cells[c][r].Health = 0
		}
	}

	// Target at (1,1): pure red.
	cells[1][1].Creature = &Creature{Color: color.RGBA{R: 200, G: 0, B: 0, A: 255}}
	cells[1][1].Health = 1.0

	// Strictly closest: nearly red at (2,2).
	cells[2][2].Creature = &Creature{Color: color.RGBA{R: 198, G: 2, B: 2, A: 255}}
	cells[2][2].Health = 1.0

	// Two neighbours that tie each other but are farther than (2,2).
	tiedFarColor := color.RGBA{R: 100, G: 100, B: 100, A: 255}
	cells[0][0].Creature = &Creature{Color: tiedFarColor}
	cells[0][0].Health = 1.0
	cells[2][0].Creature = &Creature{Color: tiedFarColor}
	cells[2][0].Health = 1.0

	got := closestColorNeighbors(cells, 1, 1, 3, 3)
	if len(got) != 1 {
		t.Fatalf("want exactly 1 result (strictly closest), got %d: %v", len(got), got)
	}
	if got[0] != [2]int{2, 2} {
		t.Errorf("want {2,2}, got %v", got[0])
	}
}

// TestClosestColorNeighborsSkipsDead verifies that neighbours with Health <= 0
// are excluded even if their colour would tie or win.
func TestClosestColorNeighborsSkipsDead(t *testing.T) {
	cells := makeUniformGrid(t, 3, 3, color.RGBA{128, 128, 128, 255})
	for c := 0; c < 3; c++ {
		for r := 0; r < 3; r++ {
			cells[c][r].Health = 0
		}
	}

	// Target at (1,1): pure red.
	cells[1][1].Creature = &Creature{Color: color.RGBA{R: 200, G: 0, B: 0, A: 255}}
	cells[1][1].Health = 1.0

	// Dead neighbour: perfect colour match, Health=0.
	cells[2][2].Creature = &Creature{Color: color.RGBA{R: 200, G: 0, B: 0, A: 255}}
	cells[2][2].Health = 0.0

	// Also dead but negative health, same colour.
	cells[0][0].Creature = &Creature{Color: color.RGBA{R: 200, G: 0, B: 0, A: 255}}
	cells[0][0].Health = -1.0

	// One live neighbour with a less-close colour.
	cells[1][0].Creature = &Creature{Color: color.RGBA{R: 150, G: 50, B: 50, A: 255}}
	cells[1][0].Health = 1.0

	got := closestColorNeighbors(cells, 1, 1, 3, 3)
	if len(got) != 1 {
		t.Fatalf("want 1 live result, got %d: %v", len(got), got)
	}
	if got[0] == [2]int{2, 2} || got[0] == [2]int{0, 0} {
		t.Errorf("dead cell returned: %v", got[0])
	}
	if got[0] != [2]int{1, 0} {
		t.Errorf("want {1,0}, got %v", got[0])
	}
}

// TestClosestColorNeighborsNoAlive verifies that an empty slice is returned
// when no neighbour has Health > 0.
func TestClosestColorNeighborsNoAlive(t *testing.T) {
	cells := makeUniformGrid(t, 3, 3, color.RGBA{128, 128, 128, 255})
	// Kill all cells including potential neighbours.
	for c := 0; c < 3; c++ {
		for r := 0; r < 3; r++ {
			cells[c][r].Health = 0
		}
	}
	// Keep only the target alive.
	cells[1][1].Health = 1.0

	got := closestColorNeighbors(cells, 1, 1, 3, 3)
	if len(got) != 0 {
		t.Fatalf("want empty slice, got %d results: %v", len(got), got)
	}
}

// TestClosestColorNeighborsCornerTarget verifies that a corner target (0,0)
// with only 3 valid neighbours still returns a correct result.
func TestClosestColorNeighborsCornerTarget(t *testing.T) {
	cells := makeUniformGrid(t, 5, 5, color.RGBA{128, 128, 128, 255})
	// Kill all cells; selectively revive only the target and its 3 corner neighbours.
	for c := 0; c < 5; c++ {
		for r := 0; r < 5; r++ {
			cells[c][r].Health = 0
		}
	}

	// Target at (0,0): pure red.
	cells[0][0].Creature = &Creature{Color: color.RGBA{R: 200, G: 0, B: 0, A: 255}}
	cells[0][0].Health = 1.0

	// Among the 3 corner neighbours {(1,0),(0,1),(1,1)}, make (1,1) closest.
	cells[1][1].Creature = &Creature{Color: color.RGBA{R: 195, G: 5, B: 5, A: 255}}
	cells[1][1].Health = 1.0
	cells[1][0].Creature = &Creature{Color: color.RGBA{R: 0, G: 200, B: 0, A: 255}}
	cells[1][0].Health = 1.0
	cells[0][1].Creature = &Creature{Color: color.RGBA{R: 0, G: 0, B: 200, A: 255}}
	cells[0][1].Health = 1.0

	got := closestColorNeighbors(cells, 0, 0, 5, 5)
	if len(got) != 1 {
		t.Fatalf("want 1 result at corner, got %d: %v", len(got), got)
	}
	if got[0] != [2]int{1, 1} {
		t.Errorf("want {1,1}, got %v", got[0])
	}
}

// TestClosestColorNeighborsOrderIndependence verifies that the result set is
// correct regardless of the internal neighbour scan order by checking set
// membership rather than slice position.
func TestClosestColorNeighborsOrderIndependence(t *testing.T) {
	// 4×4 grid so the target at (1,1) has a full 8-neighbour ring.
	cells := makeUniformGrid(t, 4, 4, color.RGBA{128, 128, 128, 255})
	for c := 0; c < 4; c++ {
		for r := 0; r < 4; r++ {
			cells[c][r].Health = 0
		}
	}

	// Target at (1,1).
	cells[1][1].Creature = &Creature{Color: color.RGBA{R: 200, G: 100, B: 50, A: 255}}
	cells[1][1].Health = 1.0

	// Three tied winners placed at non-adjacent positions.
	tiedColor := color.RGBA{R: 195, G: 105, B: 45, A: 255}
	winners := [][2]int{{0, 0}, {2, 2}, {0, 2}}
	for _, w := range winners {
		cells[w[0]][w[1]].Creature = &Creature{Color: tiedColor}
		cells[w[0]][w[1]].Health = 1.0
	}

	// One farther live cell.
	cells[2][0].Creature = &Creature{Color: color.RGBA{R: 0, G: 0, B: 200, A: 255}}
	cells[2][0].Health = 1.0

	got := closestColorNeighbors(cells, 1, 1, 4, 4)
	if len(got) != 3 {
		t.Fatalf("want 3 tied winners, got %d: %v", len(got), got)
	}
	wantSet := pairsAsSet(winners)
	for _, p := range got {
		if !wantSet[p] {
			t.Errorf("unexpected result %v; expected one of %v", p, winners)
		}
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

// ---------------------------------------------------------------------------
// scoreForKill
// ---------------------------------------------------------------------------

// TestScoreForKill verifies the pure scoring function across the full
// multi-victim matrix.
//
// Formula:
//
//	N   = len(victimColors)
//	base       = 5
//	additional = max(0, N-1) * 20
//	sameColour = 30 * N  if N>=2 and all victims share the exact same color.RGBA, else 0
//	targetColour = 40 * M  where M = count of victims whose color == targetColor
//	total = base + additional + sameColour + targetColour
func TestScoreForKill(t *testing.T) {
	red := color.RGBA{R: 200, G: 0, B: 0, A: 255}
	blue := color.RGBA{R: 0, G: 0, B: 200, A: 255}
	green := color.RGBA{R: 0, G: 200, B: 0, A: 255}
	target := red

	cases := []struct {
		name         string
		victimColors []color.RGBA
		targetColor  color.RGBA
		want         int
	}{
		{
			// N=0: base=5, additional=0, sameColour=0, targetColour=0 → 5
			name:         "N=0 no victims",
			victimColors: []color.RGBA{},
			targetColor:  target,
			want:         5,
		},
		{
			// N=1 victim != target: base=5, additional=0, sameColour=0, targetColour=0 → 5
			name:         "N=1 victim colour != target",
			victimColors: []color.RGBA{blue},
			targetColor:  target,
			want:         5,
		},
		{
			// N=1 victim == target: base=5, additional=0, sameColour=0, targetColour=40 → 45
			name:         "N=1 victim colour == target",
			victimColors: []color.RGBA{red},
			targetColor:  target,
			want:         45,
		},
		{
			// N=2 distinct colours, neither == target: base=5, additional=20, sameColour=0, targetColour=0 → 25
			name:         "N=2 distinct colours neither == target",
			victimColors: []color.RGBA{blue, green},
			targetColor:  target,
			want:         25,
		},
		{
			// N=3 distinct colours, none == target: base=5, additional=40, sameColour=0, targetColour=0 → 45
			name:         "N=3 distinct colours none == target",
			victimColors: []color.RGBA{blue, green, color.RGBA{R: 100, G: 100, B: 100, A: 255}},
			targetColor:  target,
			want:         45,
		},
		{
			// N=2 both same colour != target: base=5, additional=20, sameColour=60, targetColour=0 → 85
			name:         "N=2 both same colour != target",
			victimColors: []color.RGBA{blue, blue},
			targetColor:  target,
			want:         85,
		},
		{
			// N=3 all same colour != target: base=5, additional=40, sameColour=90, targetColour=0 → 135
			name:         "N=3 all same colour != target",
			victimColors: []color.RGBA{blue, blue, blue},
			targetColor:  target,
			want:         135,
		},
		{
			// N=3 all same colour AND == target: base=5, additional=40, sameColour=90, targetColour=120 → 255
			name:         "N=3 all same colour and == target (worked example)",
			victimColors: []color.RGBA{red, red, red},
			targetColor:  target,
			want:         255,
		},
		{
			// N=2 same colour, both == target: base=5, additional=20, sameColour=60, targetColour=80 → 165
			name:         "N=2 same colour both == target",
			victimColors: []color.RGBA{red, red},
			targetColor:  target,
			want:         165,
		},
		{
			// N=3 mixed: two == target, one different, not all-same: base=5, additional=40, sameColour=0, targetColour=80 → 125
			name:         "N=3 mixed two == target one different not all-same",
			victimColors: []color.RGBA{red, red, blue},
			targetColor:  target,
			want:         125,
		},
		{
			// N=2 distinct, exactly one == target: base=5, additional=20, sameColour=0, targetColour=40 → 65
			name:         "N=2 distinct exactly one == target",
			victimColors: []color.RGBA{red, blue},
			targetColor:  target,
			want:         65,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := scoreForKill(tc.victimColors, tc.targetColor)
			if got != tc.want {
				t.Errorf("scoreForKill(%v, %v) = %d, want %d",
					tc.victimColors, tc.targetColor, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// creaturePalette — structure and palette-level invariants
// ---------------------------------------------------------------------------

// TestCreaturePaletteMinLength verifies the palette has at least 64 entries.
func TestCreaturePaletteMinLength(t *testing.T) {
	if len(creaturePalette) < 64 {
		t.Errorf("creaturePalette has %d entries, want at least 64", len(creaturePalette))
	}
}

// TestCreaturePaletteColorsUnique verifies that no two entries share the same
// color.RGBA value.
func TestCreaturePaletteColorsUnique(t *testing.T) {
	seen := make(map[color.RGBA]int)
	for i, e := range creaturePalette {
		if prev, ok := seen[e.Color]; ok {
			t.Errorf("creaturePalette[%d] and creaturePalette[%d] share color %v",
				prev, i, e.Color)
		}
		seen[e.Color] = i
	}
}

// TestCreaturePaletteGlyphsUniqueAndNonZero verifies that every entry has a
// distinct, non-zero glyph rune.
func TestCreaturePaletteGlyphsUniqueAndNonZero(t *testing.T) {
	seen := make(map[rune]int)
	for i, e := range creaturePalette {
		if e.Glyph == 0 {
			t.Errorf("creaturePalette[%d] (%q) has zero Glyph", i, e.Name)
		}
		if prev, ok := seen[e.Glyph]; ok {
			t.Errorf("creaturePalette[%d] and creaturePalette[%d] share glyph %q",
				prev, i, e.Glyph)
		}
		seen[e.Glyph] = i
	}
}

// TestCreaturePaletteChannelValues verifies that every R, G, B channel value of
// every palette entry is a member of the 5-level set {102, 140, 178, 217, 255}.
func TestCreaturePaletteChannelValues(t *testing.T) {
	allowed := map[uint8]bool{102: true, 140: true, 178: true, 217: true, 255: true}
	for i, e := range creaturePalette {
		for _, ch := range []struct {
			name string
			val  uint8
		}{
			{"R", e.Color.R},
			{"G", e.Color.G},
			{"B", e.Color.B},
		} {
			if !allowed[ch.val] {
				t.Errorf("creaturePalette[%d] (%q) channel %s = %d, not in {102,140,178,217,255}",
					i, e.Name, ch.name, ch.val)
			}
		}
	}
}

// TestCreaturePaletteChannelRange verifies that the minimum channel value used
// across all palette entries is exactly 102 (floor preserved) and that 255 is
// also present (full range exercised).
func TestCreaturePaletteChannelRange(t *testing.T) {
	var minVal uint8 = 255
	has255 := false
	for _, e := range creaturePalette {
		for _, ch := range []uint8{e.Color.R, e.Color.G, e.Color.B} {
			if ch < minVal {
				minVal = ch
			}
			if ch == 255 {
				has255 = true
			}
		}
	}
	if minVal != 102 {
		t.Errorf("minimum channel value across palette = %d, want exactly 102", minVal)
	}
	if !has255 {
		t.Errorf("no palette entry uses channel value 255; full 5-level range not exercised")
	}
}

// TestCreaturePaletteRequiredNames verifies that the palette contains at least
// one entry whose Name contains each required creature name (case-insensitive).
func TestCreaturePaletteRequiredNames(t *testing.T) {
	required := []string{
		"kangaroo", "koala", "parrot", "hedgehog", "pine", "fern", "flower",
		"cat", "wolf", "goat", "horse", "butterfly", "cherry", "banana",
		"snake", "dragon", "rat", "ghost", "rooster", "fish", "dolphin",
		"octopus", "palm", "monkey", "bat", "jellyfish", "mosquito", "wheat", "pig",
	}
	for _, req := range required {
		found := false
		for _, e := range creaturePalette {
			if strings.Contains(strings.ToLower(e.Name), req) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("creaturePalette has no entry whose Name contains %q", req)
		}
	}
}

// ---------------------------------------------------------------------------
// darkerShade
// ---------------------------------------------------------------------------

// TestDarkerShade verifies the floor(channel*0.35) formula and that alpha is
// always forced to 255 regardless of input alpha.
func TestDarkerShade(t *testing.T) {
	cases := []struct {
		name  string
		input color.RGBA
		want  color.RGBA
	}{
		{
			name:  "mixed channels",
			input: color.RGBA{R: 200, G: 100, B: 50, A: 255},
			want:  color.RGBA{R: 70, G: 35, B: 17, A: 255},
		},
		{
			name:  "all 255",
			input: color.RGBA{R: 255, G: 255, B: 255, A: 255},
			want:  color.RGBA{R: 89, G: 89, B: 89, A: 255},
		},
		{
			name:  "all zero",
			input: color.RGBA{R: 0, G: 0, B: 0, A: 255},
			want:  color.RGBA{R: 0, G: 0, B: 0, A: 255},
		},
		{
			name:  "alpha forced to 255 when input alpha differs",
			input: color.RGBA{R: 200, G: 100, B: 50, A: 100},
			want:  color.RGBA{R: 70, G: 35, B: 17, A: 255},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := darkerShade(tc.input)
			if got != tc.want {
				t.Errorf("darkerShade(%v) = %v, want %v", tc.input, got, tc.want)
			}
		})
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
			cells[c][r] = cellWithColor(color.RGBA{128, 128, 128, 255})
			cells[c][r].Health = 1.0
		}
	}

	// Centre cell at (1,1) is pure red.
	cells[1][1].Creature = &Creature{Color: color.RGBA{R: 200, G: 0, B: 0, A: 255}}

	// Dead neighbour at (2,2): perfect colour match but Health=0.
	cells[2][2].Creature = &Creature{Color: color.RGBA{R: 200, G: 0, B: 0, A: 255}}
	cells[2][2].Health = 0.0

	// Live neighbour at (0,0): worse colour match but alive.
	cells[0][0].Creature = &Creature{Color: color.RGBA{R: 150, G: 50, B: 50, A: 255}}

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

// ---------------------------------------------------------------------------
// horizontalCollapseMoves
// ---------------------------------------------------------------------------

// movesAsSet converts a [][2]int slice into a map keyed by [2]int for
// order-independent set comparisons.
func movesAsSet(t *testing.T, moves [][2]int) map[[2]int]bool {
	t.Helper()
	s := make(map[[2]int]bool, len(moves))
	for _, m := range moves {
		s[m] = true
	}
	return s
}

// assertMoveSetsEqual fails the test if got and want differ as sets.
func assertMoveSetsEqual(t *testing.T, got, want [][2]int) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("move count: got %d, want %d; got=%v want=%v", len(got), len(want), got, want)
		return
	}
	wantSet := movesAsSet(t, want)
	for _, m := range got {
		if !wantSet[m] {
			t.Errorf("unexpected move %v in result %v; want set %v", m, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// applyColumnCollapse
// ---------------------------------------------------------------------------

// makeSnapshot builds a [][]Cell snapshot where each column is identifiable by
// a unique Health value: column c gets Health = float32(c+1)*0.1 and a fixed
// non-zero Color so live cells are distinguishable.  numRows rows per column.
func makeSnapshot(numCols, numRows int) [][]Cell {
	snapshot := make([][]Cell, numCols)
	for c := 0; c < numCols; c++ {
		snapshot[c] = make([]Cell, numRows)
		for r := 0; r < numRows; r++ {
			snapshot[c][r] = cellWithColor(color.RGBA{R: uint8(c*10 + 10), G: uint8(r*10 + 10), B: 50, A: 255})
			snapshot[c][r].Health = float32(c+1) * 0.1
		}
	}
	return snapshot
}

// colIdentical returns true when every cell in result[col] matches src[col].
func colIdentical(result, src [][]Cell, col int) bool {
	if len(result[col]) != len(src[col]) {
		return false
	}
	for r := range result[col] {
		if result[col][r] != src[col][r] {
			return false
		}
	}
	return true
}

// colZero returns true when every cell in result[col] is the zero Cell{}.
func colZero(result [][]Cell, col int) bool {
	for _, cell := range result[col] {
		if cell != (Cell{}) {
			return false
		}
	}
	return true
}

// TestApplyColumnCollapse is the regression guard for the column-collapse mutation.
// It verifies three invariants for each scenario:
//  1. Destination columns hold the correct source column's cells.
//  2. Vacated columns (oldCols not in newCols) are zeroed.
//  3. Unchanged columns (neither source nor destination) are preserved intact.
func TestApplyColumnCollapse(t *testing.T) {
	const numRows = 3

	cases := []struct {
		name          string
		numCols       int
		moves         [][2]int    // {oldCol, newCol}
		wantDst       map[int]int // dstCol → expectedSrcCol
		wantVacated   []int       // must be all-zero after apply
		wantUnchanged []int       // must equal snapshot after apply
	}{
		{
			// numCols=6, emptyCols=[2]: horizontalCollapseMoves gives {3→2,4→3,5→4}
			// (leftEmpties=0, nonEmpty=[0,1,3,4,5] → map to [0,1,2,3,4]).
			// Destinations: {2,3,4}. Vacated: {3,4,5} minus {2,3,4} = {5}.
			// Unchanged: {0,1}.
			name:          "left-side empty col 2 of 6",
			numCols:       6,
			moves:         [][2]int{{3, 2}, {4, 3}, {5, 4}},
			wantDst:       map[int]int{2: 3, 3: 4, 4: 5},
			wantVacated:   []int{5},
			wantUnchanged: []int{0, 1},
		},
		{
			// numCols=6, emptyCols=[3]: horizontalCollapseMoves gives {0→1} (leftEmpties=1,
			// nonEmpty=[0,1,2,4,5] → map to [1,2,3,4,5]: only 0→1 differs).
			// Destinations: {1}. Vacated: {0} minus {1} = {0}.
			// Unchanged: {2,3,4,5} (they map to themselves so no move entry exists).
			name:          "right-side empty col 3 of 6",
			numCols:       6,
			moves:         [][2]int{{0, 1}},
			wantDst:       map[int]int{1: 0},
			wantVacated:   []int{0},
			wantUnchanged: []int{2, 3, 4, 5},
		},
		{
			// numCols=6, emptyCols=[1,4]: moves = {0→1, 5→4} (leftEmpties=1,
			// nonEmpty=[0,2,3,5] → [1,2,3,4]: 0→1 and 5→4 differ).
			// Destinations: {1,4}. Vacated: {0,5} minus {1,4} = {0,5}.
			// Unchanged: {2,3}.
			name:          "one empty each side cols 1 and 4 of 6",
			numCols:       6,
			moves:         [][2]int{{0, 1}, {5, 4}},
			wantDst:       map[int]int{1: 0, 4: 5},
			wantVacated:   []int{0, 5},
			wantUnchanged: []int{2, 3},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			snapshot := makeSnapshot(tc.numCols, numRows)
			result := applyColumnCollapse(snapshot, tc.moves)

			// 1. Destination columns must hold the correct source data.
			for dstCol, srcCol := range tc.wantDst {
				for r := 0; r < numRows; r++ {
					if result[dstCol][r] != snapshot[srcCol][r] {
						t.Errorf("dst col %d row %d = %+v, want snapshot[%d][%d] = %+v",
							dstCol, r, result[dstCol][r], srcCol, r, snapshot[srcCol][r])
					}
				}
			}

			// 2. Vacated columns must be all-zero.
			for _, col := range tc.wantVacated {
				if !colZero(result, col) {
					t.Errorf("vacated col %d is not zero: %+v", col, result[col])
				}
			}

			// 3. Unchanged columns must be identical to snapshot.
			for _, col := range tc.wantUnchanged {
				if !colIdentical(result, snapshot, col) {
					t.Errorf("unchanged col %d was modified: got %+v, want %+v",
						col, result[col], snapshot[col])
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// horizontalCollapseMoves
// ---------------------------------------------------------------------------

// TestHorizontalCollapseMoves verifies the pure-function mapping from (numCols,
// emptyCols) to the [oldCol,newCol] pairs for columns that change position.
//
// Formula recap:
//
//	nonEmpty     = columns not in emptyCols, ascending order
//	leftEmpties  = count of emptyCols entries with index < numCols/2
//	nonEmpty[i]  maps to (leftEmpties + i)
//	emit {nonEmpty[i], leftEmpties+i} only when they differ
func TestHorizontalCollapseMoves(t *testing.T) {
	cases := []struct {
		name      string
		numCols   int
		emptyCols []int
		want      [][2]int
	}{
		{
			// No empty columns → nonEmpty=[0..11], leftEmpties=0, all map to
			// themselves → no moves.
			name:      "no empty cols",
			numCols:   12,
			emptyCols: nil,
			want:      nil,
		},
		{
			// emptyCols=[11] (right edge, 11>=6) → leftEmpties=0,
			// nonEmpty=[0..10] → map to [0..10] → no changes.
			name:      "right edge empty col 11 of 12",
			numCols:   12,
			emptyCols: []int{11},
			want:      nil,
		},
		{
			// emptyCols=[0] (left edge, 0<6) → leftEmpties=1,
			// nonEmpty=[1..11] → map to [1..11] (leftEmpties+0=1, +1=2, … +10=11) → no changes.
			name:      "left edge empty col 0 of 12",
			numCols:   12,
			emptyCols: []int{0},
			want:      nil,
		},
		{
			// numCols=4, emptyCols=[1] (1<2 → leftEmpties=1),
			// nonEmpty=[0,2,3] → map to [1,2,3]: 0→1 differs; 2→2 same; 3→3 same.
			// Exactly one move: {0,1}.
			name:      "left-half empty col 1 of 4",
			numCols:   4,
			emptyCols: []int{1},
			want:      [][2]int{{0, 1}},
		},
		{
			// numCols=4, emptyCols=[2] (2>=2 → leftEmpties=0),
			// nonEmpty=[0,1,3] → map to [0,1,2]: 0→0 same; 1→1 same; 3→2 differs.
			// Exactly one move: {3,2}.
			name:      "right-half empty col 2 of 4",
			numCols:   4,
			emptyCols: []int{2},
			want:      [][2]int{{3, 2}},
		},
		{
			// numCols=6, emptyCols=[1,4] (1<3 → 1 left; 4>=3 → 0 left; leftEmpties=1),
			// nonEmpty=[0,2,3,5] → map to [1,2,3,4]: 0→1 differs; 2→2 same; 3→3 same; 5→4 differs.
			// Moves: {0,1} and {5,4}.
			name:      "one empty each side cols 1 and 4 of 6",
			numCols:   6,
			emptyCols: []int{1, 4},
			want:      [][2]int{{0, 1}, {5, 4}},
		},
		{
			// numCols=4, all columns empty → nonEmpty=[] → no moves.
			name:      "all cols empty",
			numCols:   4,
			emptyCols: []int{0, 1, 2, 3},
			want:      nil,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := horizontalCollapseMoves(tc.numCols, tc.emptyCols)
			// Normalise nil vs empty for length check.
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			assertMoveSetsEqual(t, got, tc.want)
		})
	}
}

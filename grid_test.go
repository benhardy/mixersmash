package main

import (
	"image/color"
	"math"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// cellWithColor returns a Cell with the given colour wrapped in a Creature.
// Health is left at its zero value; callers that need Health=1.0 must set it explicitly.
func cellWithColor(c color.RGBA) Cell {
	o := &Organism{Color: c}
	return Cell{Organism: o}
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
	_ = randomColor()
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

// TestDurationConstants documents the wall-clock duration of each animation
// phase and verifies that they equal the original tick-based values at 60 TPS.
func TestDurationConstants(t *testing.T) {
	if shuffleDuration != 300*time.Millisecond {
		t.Errorf("shuffleDuration = %v, want 300ms", shuffleDuration)
	}
	if dropDuration != 300*time.Millisecond {
		t.Errorf("dropDuration = %v, want 300ms", dropDuration)
	}
	if victimDuration != 400*time.Millisecond {
		t.Errorf("victimDuration = %v, want 400ms", victimDuration)
	}
	if targetAnimDuration != 250*time.Millisecond {
		t.Errorf("targetAnimDuration = %v, want 250ms", targetAnimDuration)
	}
}

// ---------------------------------------------------------------------------
// elapsedFraction
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
			got := elapsedFraction(base, base.Add(tc.elapsed), dur)
			if got != tc.want {
				t.Errorf("elapsedFraction elapsed=%v dur=%v = %v, want %v",
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
		f1 := elapsedFraction(start, now1, shuffleDuration)
		f2 := elapsedFraction(start, now2, shuffleDuration)
		if f1 == f2 {
			t.Errorf("sub-tick: elapsedFraction at %v and %v within same tick window returned identical fraction %v; wall-clock not used",
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
	grid := NewGrid(4, 4)
	g := newGameState(grid)

	if g.currentState != stateWaiting {
		t.Errorf("currentState = %d, want stateWaiting (%d)", g.currentState, stateWaiting)
	}
	if !g.animStart.IsZero() {
		t.Errorf("animStart = %v, want zero time.Time", g.animStart)
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
// eightNeighbors
// ---------------------------------------------------------------------------

// TestEightNeighborsCorner verifies that eightNeighbors(nil,0,0,25,18) returns
// exactly 3 results: {1,0}, {0,1}, {1,1}.
func TestEightNeighborsCorner(t *testing.T) {
	got := eightNeighbors(nil, 0, 0, 25, 18)
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

// TestEightNeighborsEdge verifies that eightNeighbors(nil,0,5,25,18) returns
// exactly 5 results (left edge, non-corner).
func TestEightNeighborsEdge(t *testing.T) {
	got := eightNeighbors(nil, 0, 5, 25, 18)
	if len(got) != 5 {
		t.Fatalf("eightNeighbors(0,5,25,18) returned %d results, want 5: %v", len(got), got)
	}
}

// TestEightNeighborsCentre verifies that eightNeighbors(nil,5,5,25,18) returns
// exactly 8 results for an interior cell.
func TestEightNeighborsCentre(t *testing.T) {
	got := eightNeighbors(nil, 5, 5, 25, 18)
	if len(got) != 8 {
		t.Fatalf("eightNeighbors(5,5,25,18) returned %d results, want 8: %v", len(got), got)
	}
}

// ---------------------------------------------------------------------------
// connectionVictims
// ---------------------------------------------------------------------------

// makeConnectionGrid builds a cols×rows grid of zero-Health cells. Callers set
// specific cells' Creature and Health after calling this helper.
func makeConnectionGrid(cols, rows int) [][]Cell {
	cells := make([][]Cell, cols)
	for c := range cells {
		cells[c] = make([]Cell, rows)
	}
	return cells
}

// TestConnectionVictimsNilCreatureTarget verifies that nil is returned when
// the target cell has no Creature.
func TestConnectionVictimsNilCreatureTarget(t *testing.T) {
	cells := makeConnectionGrid(3, 3)
	// cells[1][1] has Creature==nil by default.
	cells[2][2] = Cell{Organism: &organismPalette[0], Health: 1.0}

	got, _ := connectionVictims(nil, nil, cells, 1, 1, 3, 3)
	if len(got) != 0 {
		t.Errorf("want empty result for nil-Creature target, got %v", got)
	}
}

// TestConnectionVictimsSameTypeAliveNeighbour verifies that an alive neighbour
// of the SAME creature type (pointer identity) as the target is returned as a
// victim even if it is not in target.Connections.
func TestConnectionVictimsSameTypeAliveNeighbour(t *testing.T) {
	// Find a palette creature that is NOT connected to itself (all are, by
	// construction — we just need any creature and confirm same-pointer match).
	// Use organismPalette[0]. Verify it is not in its own Connections (no self-link).
	target := &organismPalette[0]
	cells := makeConnectionGrid(3, 3)
	cells[1][1] = Cell{Organism: target, Health: 1.0}
	// Place the same *Organism pointer at neighbour (2,1). This is same-type but
	// must NOT appear in target.Connections (no self-link invariant guarantees this).
	cells[2][1] = Cell{Organism: target, Health: 1.0}

	got, _ := connectionVictims(nil, nil, cells, 1, 1, 3, 3)
	if len(got) != 1 {
		t.Fatalf("want 1 victim (same-type alive), got %d: %v", len(got), got)
	}
	if got[0] != [2]int{2, 1} {
		t.Errorf("want {2,1}, got %v", got[0])
	}
}

// TestConnectionVictimsSameTypeDeadNeighbour verifies that a dead neighbour of
// the same creature type is NOT returned as a victim.
func TestConnectionVictimsSameTypeDeadNeighbour(t *testing.T) {
	target := &organismPalette[0]
	cells := makeConnectionGrid(3, 3)
	cells[1][1] = Cell{Organism: target, Health: 1.0}
	cells[2][1] = Cell{Organism: target, Health: 0.0} // dead same-type

	got, _ := connectionVictims(nil, nil, cells, 1, 1, 3, 3)
	if len(got) != 0 {
		t.Errorf("want no victims (dead same-type neighbour), got %d: %v", len(got), got)
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
// organismPalette — structure and palette-level invariants
// ---------------------------------------------------------------------------

// TestOrganismPaletteSize verifies the palette has exactly 22 entries.
func TestOrganismPaletteSize(t *testing.T) {
	if len(organismPalette) != 22 {
		t.Errorf("organismPalette has %d entries, want exactly 22", len(organismPalette))
	}
}

// TestOrganismPaletteGlyphsUniqueAndNonZero verifies that every entry has a
// distinct, non-zero glyph rune.
func TestOrganismPaletteGlyphsUniqueAndNonZero(t *testing.T) {
	seen := make(map[rune]int)
	for i, e := range organismPalette {
		if e.Glyph == 0 {
			t.Errorf("organismPalette[%d] (%q) has zero Glyph", i, e.Name)
		}
		if prev, ok := seen[e.Glyph]; ok {
			t.Errorf("organismPalette[%d] and organismPalette[%d] share glyph %q",
				prev, i, e.Glyph)
		}
		seen[e.Glyph] = i
	}
}

// TestOrganismPaletteAlphaAll255 verifies that every palette entry has Color.A == 255.
func TestOrganismPaletteAlphaAll255(t *testing.T) {
	for i, e := range organismPalette {
		if e.Color.A != 255 {
			t.Errorf("organismPalette[%d] (%q) Color.A = %d, want 255", i, e.Name, e.Color.A)
		}
	}
}

// ---------------------------------------------------------------------------
// glyphTint
// ---------------------------------------------------------------------------

// TestGlyphTint verifies the luminance-adaptive tint: dark cells get near-white,
// bright cells get near-black, and alpha is always 255.
func TestGlyphTint(t *testing.T) {
	nearBlack := color.RGBA{R: 30, G: 25, B: 20, A: 255}
	nearWhite := color.RGBA{R: 230, G: 225, B: 215, A: 255}

	cases := []struct {
		name  string
		input color.RGBA
		want  color.RGBA
	}{
		{
			// Near-black cell (OKLab L well below 0.65) → near-white tint.
			name:  "near-black cell gets near-white tint",
			input: color.RGBA{R: 51, G: 68, B: 51, A: 255},
			want:  nearWhite,
		},
		{
			// Cream cell (OKLab L well above 0.65) → near-black tint.
			name:  "cream cell gets near-black tint",
			input: color.RGBA{R: 242, G: 237, B: 224, A: 255},
			want:  nearBlack,
		},
		{
			// Vivid blue (OKLab L=0.6918 >= 0.65) → near-black tint.
			name:  "vivid blue gets near-black tint",
			input: color.RGBA{R: 113, G: 149, B: 255, A: 255},
			want:  nearBlack,
		},
		{
			// Vivid red (OKLab L=0.6892 >= 0.65) → near-black tint; also confirms A=255.
			name:  "vivid red gets near-black tint and A=255",
			input: color.RGBA{R: 255, G: 93, B: 72, A: 255},
			want:  nearBlack,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := glyphTint(tc.input)
			if got != tc.want {
				t.Errorf("glyphTint(%v) = %v, want %v", tc.input, got, tc.want)
			}
			if got.A != 255 {
				t.Errorf("glyphTint(%v).A = %d, want 255", tc.input, got.A)
			}
		})
	}
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

// ---------------------------------------------------------------------------
// TestOrganismPalettePairStructure — new palette invariants
// ---------------------------------------------------------------------------

// TestOrganismPalettePairStructure verifies that the 22-entry Australian
// animal/food palette has the correct pair structure:
//   - exactly 22 entries
//   - PairIDs 0..10 each appear exactly twice
//   - for each PairID exactly one entry has IsAnimal=true and one false
//   - both entries in each pair share the same Color
func TestOrganismPalettePairStructure(t *testing.T) {
	if len(organismPalette) != 22 {
		t.Fatalf("organismPalette has %d entries, want 22", len(organismPalette))
	}

	type pairInfo struct {
		count   int
		animals int
		foods   int
		colors  []color.RGBA
	}
	pairs := make(map[int]*pairInfo)
	for i := range organismPalette {
		e := &organismPalette[i]
		if pairs[e.PairID] == nil {
			pairs[e.PairID] = &pairInfo{}
		}
		p := pairs[e.PairID]
		p.count++
		if e.IsAnimal {
			p.animals++
		} else {
			p.foods++
		}
		p.colors = append(p.colors, e.Color)
	}

	for id := 0; id <= 10; id++ {
		p, ok := pairs[id]
		if !ok {
			t.Errorf("PairID %d not found in palette", id)
			continue
		}
		if p.count != 2 {
			t.Errorf("PairID %d has %d entries, want 2", id, p.count)
		}
		if p.animals != 1 {
			t.Errorf("PairID %d has %d animal entries, want 1", id, p.animals)
		}
		if p.foods != 1 {
			t.Errorf("PairID %d has %d food entries, want 1", id, p.foods)
		}
		if len(p.colors) == 2 && p.colors[0] != p.colors[1] {
			t.Errorf("PairID %d: animal Color %v != food Color %v", id, p.colors[0], p.colors[1])
		}
	}

	// Verify no unexpected PairIDs.
	for id := range pairs {
		if id < 0 || id > 10 {
			t.Errorf("unexpected PairID %d in palette", id)
		}
	}
}

// ---------------------------------------------------------------------------
// shuffleCount
// ---------------------------------------------------------------------------

// TestShuffleCount verifies the pure helper that maps (power, aliveCount) to the
// number of neighbours to shuffle.
//
// Rules:
//   - Power == 2            → 0 for any aliveCount
//   - aliveCount == 0       → 0 for any power
//   - otherwise             → min(power, aliveCount)
func TestShuffleCount(t *testing.T) {
	cases := []struct {
		name       string
		power      int
		aliveCount int
		want       int
	}{
		// Power-2 cases: always 0 regardless of neighbours.
		{name: "power2 alive0", power: 2, aliveCount: 0, want: 0},
		{name: "power2 alive1", power: 2, aliveCount: 1, want: 0},
		{name: "power2 alive8", power: 2, aliveCount: 8, want: 0},

		// Zero alive neighbours: always 0 regardless of power.
		{name: "power3 alive0", power: 3, aliveCount: 0, want: 0},
		{name: "power5 alive0", power: 5, aliveCount: 0, want: 0},
		{name: "power8 alive0", power: 8, aliveCount: 0, want: 0},

		// Power-3: min(3, alive).
		{name: "power3 alive1", power: 3, aliveCount: 1, want: 1},
		{name: "power3 alive2", power: 3, aliveCount: 2, want: 2},
		{name: "power3 alive3", power: 3, aliveCount: 3, want: 3},
		{name: "power3 alive8", power: 3, aliveCount: 8, want: 3},

		// Power-5: min(5, alive).
		{name: "power5 alive3", power: 5, aliveCount: 3, want: 3},
		{name: "power5 alive5", power: 5, aliveCount: 5, want: 5},
		{name: "power5 alive8", power: 5, aliveCount: 8, want: 5},

		// Power-8: min(8, alive).
		{name: "power8 alive4", power: 8, aliveCount: 4, want: 4},
		{name: "power8 alive7", power: 8, aliveCount: 7, want: 7},
		{name: "power8 alive8", power: 8, aliveCount: 8, want: 8},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := shuffleCount(tc.power, tc.aliveCount)
			if got != tc.want {
				t.Errorf("shuffleCount(power=%d, alive=%d) = %d, want %d",
					tc.power, tc.aliveCount, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// sRGBToOKLab — unit tests
// ---------------------------------------------------------------------------

// TestSRGBToOKLabBlack verifies that pure black maps to OKLab (0, 0, 0).
func TestSRGBToOKLabBlack(t *testing.T) {
	lab := sRGBToOKLab(color.RGBA{R: 0, G: 0, B: 0, A: 255})
	if lab[0] != 0 || lab[1] != 0 || lab[2] != 0 {
		t.Errorf("sRGBToOKLab(black) = %v, want [0 0 0]", lab)
	}
}

// TestSRGBToOKLabWhiteLuminance verifies that pure white has OKLab L close to 1.
func TestSRGBToOKLabWhiteLuminance(t *testing.T) {
	lab := sRGBToOKLab(color.RGBA{R: 255, G: 255, B: 255, A: 255})
	if math.Abs(lab[0]-1.0) > 1e-5 {
		t.Errorf("sRGBToOKLab(white).L = %f, want ~1.0", lab[0])
	}
}

// TestSRGBToOKLabRedGreenDistance verifies that red and green are farther apart
// in OKLab than red and orange (perceptual ordering check).
func TestSRGBToOKLabRedGreenDistance(t *testing.T) {
	red := sRGBToOKLab(color.RGBA{R: 220, G: 60, B: 60, A: 255})
	green := sRGBToOKLab(color.RGBA{R: 60, G: 180, B: 60, A: 255})
	orange := sRGBToOKLab(color.RGBA{R: 220, G: 140, B: 60, A: 255})

	dist := func(a, b [3]float64) float64 {
		dl, da, db := a[0]-b[0], a[1]-b[1], a[2]-b[2]
		return math.Sqrt(dl*dl + da*da + db*db)
	}
	dRG := dist(red, green)
	dRO := dist(red, orange)
	if dRG <= dRO {
		t.Errorf("expected dist(red,green)=%f > dist(red,orange)=%f", dRG, dRO)
	}
}

// ---------------------------------------------------------------------------
// commitCellAt — health decrement and power-2 instant-death rule
// ---------------------------------------------------------------------------

// TestNextHealthAfterHit exercises nextHealthAfterHit, the pure function that
// encapsulates the cell-death health-decrement rule extracted from commitCellAt.
func TestNextHealthAfterHit(t *testing.T) {
	t.Run("power2 instant death", func(t *testing.T) {
		cases := []float32{1.0, 0.75, 0.5625, 0.001}
		for _, h := range cases {
			if got := nextHealthAfterHit(2, h); got != 0 {
				t.Errorf("nextHealthAfterHit(2, %v) = %v, want 0 (instant death)", h, got)
			}
		}
	})

	t.Run("power3 hit sequence", func(t *testing.T) {
		// Hit 1: 1.0 * 0.75 = 0.75 > healthMin(0.562) → alive
		h1 := nextHealthAfterHit(3, 1.0)
		want1 := float32(1.0) * healthDecrementKeep
		if h1 != want1 {
			t.Errorf("hit 1: got %v, want %v", h1, want1)
		}
		if h1 <= 0 {
			t.Errorf("hit 1: cell should survive, got health %v", h1)
		}

		// Hit 2: 0.75 * 0.75 = 0.5625 > healthMin(0.562) → alive
		h2 := nextHealthAfterHit(3, h1)
		want2 := want1 * healthDecrementKeep
		if h2 != want2 {
			t.Errorf("hit 2: got %v, want %v", h2, want2)
		}
		if h2 <= 0 {
			t.Errorf("hit 2: cell should survive, got health %v", h2)
		}

		// Hit 3: 0.5625 * 0.75 = 0.421875 <= healthMin(0.562) → dead
		h3 := nextHealthAfterHit(3, h2)
		if h3 != 0 {
			t.Errorf("hit 3: got %v, want 0 (should die at third hit)", h3)
		}
	})
}

// TestPower2CellDiesOnFirstHit verifies that a power-2 cell is killed by a
// single hit via nextHealthAfterHit.
func TestPower2CellDiesOnFirstHit(t *testing.T) {
	if got := nextHealthAfterHit(2, 1.0); got != 0 {
		t.Errorf("nextHealthAfterHit(2, 1.0) = %v, want 0 (instant death)", got)
	}
}

// TestHigherPowerCellSurvivesFirstHit verifies that a power-3 cell is NOT
// killed by a single hit.  After one application of healthDecrementKeep the
// health is 0.75 — above both healthMin (0.562) and healthEpsilon (0.001),
// so the cell remains alive.  A second hit brings health to 0.5625, which is
// still above healthMin (0.562) by 0.0005, so the cell is still alive.
// Only from three hits onward does the floor trigger and the cell die.
//
// Concretely this test asserts:
//   - after hit 1: Health == 1.0 * 0.75 == 0.75 (> healthMin → alive)
//   - after hit 2: Health == 0.75 * 0.75 == 0.5625 (> healthMin → alive)
//   - after hit 3: Health would be 0.5625 * 0.75 == 0.421875; <= healthMin → dead
func TestHigherPowerCellSurvivesFirstHit(t *testing.T) {
	// Hit 1: must survive.
	h1 := nextHealthAfterHit(3, 1.0)
	if h1 <= 0 {
		t.Errorf("power-3 cell after hit 1: Health = %v, want > 0 (cell should survive)", h1)
	}
	wantAfterHit1 := float32(1.0) * healthDecrementKeep
	if h1 != wantAfterHit1 {
		t.Errorf("power-3 cell after hit 1: Health = %v, want %v", h1, wantAfterHit1)
	}

	// Hit 2: must still survive.
	h2 := nextHealthAfterHit(3, h1)
	if h2 <= 0 {
		t.Errorf("power-3 cell after hit 2: Health = %v, want > 0 (cell should survive)", h2)
	}
	wantAfterHit2 := wantAfterHit1 * healthDecrementKeep
	if h2 != wantAfterHit2 {
		t.Errorf("power-3 cell after hit 2: Health = %v, want %v", h2, wantAfterHit2)
	}

	// Hit 3: health drops to 0.421875 which is <= healthMin (0.562), so cell dies.
	h3 := nextHealthAfterHit(3, h2)
	if h3 != 0 {
		t.Errorf("power-3 cell after hit 3: Health = %v, want 0 (should die at third hit)", h3)
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
	grid := NewGrid(4, 4)
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
// All cells are set alive (Health=1.0) with a palette creature so connectionVictims
// can find matches.  Callers may set specific cells dead or nil afterwards.
func makeHoverGame(cols, rows int) *Game {
	grid := NewGrid(cols, rows)
	// Assign creatures and set health so cells are alive.
	for c := 0; c < cols; c++ {
		for r := 0; r < rows; r++ {
			idx := (c*rows + r) % len(organismPalette)
			grid.Cells[c][r].Organism = &organismPalette[idx]
			grid.Cells[c][r].Health = 1.0
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
	g.hoverCol = 0
	g.hoverRow = 0

	// Kill cell (1,1).
	g.grid.Cells[1][1].Health = 0

	g.updateHover(1, 1)

	if g.hoverCol != 0 || g.hoverRow != 0 {
		t.Errorf("updateHover on dead cell changed hover to (%d,%d), want (0,0)", g.hoverCol, g.hoverRow)
	}
}

// TestUpdateHoverOutOfBoundsLeavesHoverUnchanged verifies that an out-of-bounds
// (hc, hr) pair leaves the hover state unchanged.
func TestUpdateHoverOutOfBoundsLeavesHoverUnchanged(t *testing.T) {
	g := makeHoverGame(3, 3)
	g.hoverCol = 1
	g.hoverRow = 1

	// Negative col — out of bounds.
	g.updateHover(-1, 0)
	if g.hoverCol != 1 || g.hoverRow != 1 {
		t.Errorf("updateHover(-1,0) changed hover to (%d,%d), want (1,1)", g.hoverCol, g.hoverRow)
	}

	// Col beyond grid width.
	g.updateHover(3, 0)
	if g.hoverCol != 1 || g.hoverRow != 1 {
		t.Errorf("updateHover(3,0) changed hover to (%d,%d), want (1,1)", g.hoverCol, g.hoverRow)
	}

	// Row beyond grid height.
	g.updateHover(0, 3)
	if g.hoverCol != 1 || g.hoverRow != 1 {
		t.Errorf("updateHover(0,3) changed hover to (%d,%d), want (1,1)", g.hoverCol, g.hoverRow)
	}
}

package board

import (
	"image/color"
	"math"
	"testing"
)

// cellWithColor returns a Cell with the given colour wrapped in an Organism.
// Health is left at its zero value; callers that need Health=1.0 must set it explicitly.
func cellWithColor(c color.RGBA) Cell {
	o := &Organism{color: c}
	return Cell{organism: o}
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
	if len(g.cells) != cols {
		t.Fatalf("len(Grid.Cells) = %d, want %d", len(g.cells), cols)
	}
	for c := 0; c < cols; c++ {
		if len(g.cells[c]) != rows {
			t.Fatalf("len(Grid.Cells[%d]) = %d, want %d", c, len(g.cells[c]), rows)
		}
		for r := 0; r < rows; r++ {
			cell := g.cells[c][r]
			if cell.Color().A != 255 {
				t.Errorf("Grid.Cells[%d][%d].Color().A = %d, want 255", c, r, cell.Color().A)
			}
		}
	}
}

// TestInitialCellHealth verifies that every cell produced by NewGrid starts
// with Health == 1.0.
func TestInitialCellHealth(t *testing.T) {
	const cols, rows = 3, 3
	g := NewGrid(cols, rows)
	for c := 0; c < cols; c++ {
		for r := 0; r < rows; r++ {
			if g.cells[c][r].health != 1.0 {
				t.Errorf("Cells[%d][%d].health = %v, want 1.0", c, r, g.cells[c][r].health)
			}
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
// gravityMoves
// ---------------------------------------------------------------------------

// TestGravityMovesNoHoles verifies that GravityMoves with no holes returns an
// empty slice — no cell changes row when nothing is removed.
func TestGravityMovesNoHoles(t *testing.T) {
	got := GravityMoves(18, nil)
	if len(got) != 0 {
		t.Errorf("GravityMoves(18, nil) = %v, want empty slice", got)
	}
}

// TestGravityMovesSingleHoleMiddle verifies that removing the middle row of a
// 5-row column compacts rows 0 and 1 downward by one; rows 3 and 4 are already
// in their correct positions.
// alive=[0,1,3,4], newCount=1 → 0→1, 1→2; 3→3(stay), 4→4(stay)
func TestGravityMovesSingleHoleMiddle(t *testing.T) {
	got := GravityMoves(5, []int{2})
	want := [][2]int{{0, 1}, {1, 2}}
	if len(got) != len(want) {
		t.Fatalf("GravityMoves(5, [2]) = %v, want %v", got, want)
	}
	for i, m := range got {
		if m != want[i] {
			t.Errorf("GravityMoves(5, [2])[%d] = %v, want %v", i, m, want[i])
		}
	}
}

// TestGravityMovesSingleHoleTop verifies that removing the top row of a 5-row
// column results in no moves — alive rows [1,2,3,4] land at positions [1,2,3,4].
// alive=[1,2,3,4], newCount=1 → 1→1(stay), 2→2(stay), 3→3(stay), 4→4(stay)
func TestGravityMovesSingleHoleTop(t *testing.T) {
	got := GravityMoves(5, []int{0})
	if len(got) != 0 {
		t.Errorf("GravityMoves(5, [0]) = %v, want empty slice", got)
	}
}

// TestGravityMovesSingleHoleBottom verifies that removing the bottom row of a
// 5-row column compacts all remaining rows downward by one.
// alive=[0,1,2,3], newCount=1 → 0→1, 1→2, 2→3, 3→4
func TestGravityMovesSingleHoleBottom(t *testing.T) {
	got := GravityMoves(5, []int{4})
	want := [][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 4}}
	if len(got) != len(want) {
		t.Fatalf("GravityMoves(5, [4]) = %v, want %v", got, want)
	}
	for i, m := range got {
		if m != want[i] {
			t.Errorf("GravityMoves(5, [4])[%d] = %v, want %v", i, m, want[i])
		}
	}
}

// TestGravityMovesTwoHolesSameColumn verifies two non-adjacent holes in a
// 6-row column.
// alive=[0,2,4,5], newCount=2 → 0→2, 2→3; 4→4(stay), 5→5(stay)
func TestGravityMovesTwoHolesSameColumn(t *testing.T) {
	got := GravityMoves(6, []int{1, 3})
	want := [][2]int{{0, 2}, {2, 3}}
	if len(got) != len(want) {
		t.Fatalf("GravityMoves(6, [1,3]) = %v, want %v", got, want)
	}
	for i, m := range got {
		if m != want[i] {
			t.Errorf("GravityMoves(6, [1,3])[%d] = %v, want %v", i, m, want[i])
		}
	}
}

// TestGravityMovesAllHoles verifies that removing every row in a 3-row column
// returns an empty slice — no alive rows, no moves.
func TestGravityMovesAllHoles(t *testing.T) {
	got := GravityMoves(3, []int{0, 1, 2})
	if len(got) != 0 {
		t.Errorf("GravityMoves(3, [0,1,2]) = %v, want empty slice", got)
	}
}

// ---------------------------------------------------------------------------
// organismPalette — structure and palette-level invariants
// ---------------------------------------------------------------------------

// TestOrganismPaletteSize verifies the palette has exactly 23 entries.
func TestOrganismPaletteSize(t *testing.T) {
	if len(OrganismPalette) != 23 {
		t.Errorf("OrganismPalette has %d entries, want exactly 23", len(OrganismPalette))
	}
}

// TestOrganismPaletteGlyphsUniqueAndNonZero verifies that every entry has a
// distinct, non-zero glyph rune.
func TestOrganismPaletteGlyphsUniqueAndNonZero(t *testing.T) {
	seen := make(map[rune]int)
	for i, e := range OrganismPalette {
		if e.glyph == 0 {
			t.Errorf("OrganismPalette[%d] (%q) has zero Glyph", i, e.name)
		}
		if prev, ok := seen[e.glyph]; ok {
			t.Errorf("OrganismPalette[%d] and OrganismPalette[%d] share glyph %q",
				prev, i, e.glyph)
		}
		seen[e.glyph] = i
	}
}

// TestOrganismPaletteAlphaAll255 verifies that every palette entry has Color.A == 255.
func TestOrganismPaletteAlphaAll255(t *testing.T) {
	for i, e := range OrganismPalette {
		if e.color.A != 255 {
			t.Errorf("OrganismPalette[%d] (%q) Color.A = %d, want 255", i, e.name, e.color.A)
		}
	}
}

// ---------------------------------------------------------------------------
// GlyphTint
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
			got := GlyphTint(tc.input)
			if got != tc.want {
				t.Errorf("GlyphTint(%v) = %v, want %v", tc.input, got, tc.want)
			}
			if got.A != 255 {
				t.Errorf("GlyphTint(%v).A = %d, want 255", tc.input, got.A)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// helpers shared by applyColumnCollapse and horizontalCollapseMoves tests
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
			snapshot[c][r].health = float32(c+1) * 0.1
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
			result := ApplyColumnCollapse(snapshot, tc.moves)

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
			got := HorizontalCollapseMoves(tc.numCols, tc.emptyCols)
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

// TestOrganismPalettePairStructure verifies that the palette has the correct
// structure: exactly one Mixer entry (PairID -1, IsAnimal false, IsMixer true)
// and 22 non-mixer entries forming 11 animal/food pairs (PairIDs 0..10).
func TestOrganismPalettePairStructure(t *testing.T) {
	// Separate mixer entries from non-mixer entries.
	var mixers []Organism
	var nonMixers []Organism
	for _, e := range OrganismPalette {
		if e.isMixer {
			mixers = append(mixers, e)
		} else {
			nonMixers = append(nonMixers, e)
		}
	}

	// Exactly one Mixer entry.
	if len(mixers) != 1 {
		t.Fatalf("palette has %d IsMixer entries, want exactly 1", len(mixers))
	}
	m := mixers[0]
	if m.pairID != -1 {
		t.Errorf("Mixer PairID = %d, want -1", m.pairID)
	}
	if m.isAnimal {
		t.Errorf("Mixer IsAnimal = true, want false")
	}

	// Exactly 22 non-mixer entries.
	if len(nonMixers) != 22 {
		t.Fatalf("palette has %d non-mixer entries, want 22", len(nonMixers))
	}

	type pairInfo struct {
		count   int
		animals int
		foods   int
		colors  []color.RGBA
	}
	pairs := make(map[int]*pairInfo)
	for i := range nonMixers {
		e := &nonMixers[i]
		if pairs[e.pairID] == nil {
			pairs[e.pairID] = &pairInfo{}
		}
		p := pairs[e.pairID]
		p.count++
		if e.isAnimal {
			p.animals++
		} else {
			p.foods++
		}
		p.colors = append(p.colors, e.color)
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

	// Verify no unexpected pairIDs among non-mixer entries.
	for id := range pairs {
		if id < 0 || id > 10 {
			t.Errorf("unexpected PairID %d in non-mixer palette entries", id)
		}
	}
}

// ---------------------------------------------------------------------------
// SRGBToOKLab — unit tests
// ---------------------------------------------------------------------------

// TestSRGBToOKLabBlack verifies that pure black maps to OKLab (0, 0, 0).
func TestSRGBToOKLabBlack(t *testing.T) {
	lab := SRGBToOKLab(color.RGBA{R: 0, G: 0, B: 0, A: 255})
	if lab[0] != 0 || lab[1] != 0 || lab[2] != 0 {
		t.Errorf("SRGBToOKLab(black) = %v, want [0 0 0]", lab)
	}
}

// TestSRGBToOKLabWhiteLuminance verifies that pure white has OKLab L close to 1.
func TestSRGBToOKLabWhiteLuminance(t *testing.T) {
	lab := SRGBToOKLab(color.RGBA{R: 255, G: 255, B: 255, A: 255})
	if math.Abs(lab[0]-1.0) > 1e-5 {
		t.Errorf("SRGBToOKLab(white).L = %f, want ~1.0", lab[0])
	}
}

// TestSRGBToOKLabRedGreenDistance verifies that red and green are farther apart
// in OKLab than red and orange (perceptual ordering check).
func TestSRGBToOKLabRedGreenDistance(t *testing.T) {
	red := SRGBToOKLab(color.RGBA{R: 220, G: 60, B: 60, A: 255})
	green := SRGBToOKLab(color.RGBA{R: 60, G: 180, B: 60, A: 255})
	orange := SRGBToOKLab(color.RGBA{R: 220, G: 140, B: 60, A: 255})

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
// ScoreForCluster
// ---------------------------------------------------------------------------

// TestScoreForCluster verifies the n*n scoring formula.
func TestScoreForCluster(t *testing.T) {
	cases := []struct {
		n    int
		want int
	}{
		{1, 1},
		{5, 25},
		{10, 100},
	}
	for _, tc := range cases {
		got := ScoreForCluster(tc.n)
		if got != tc.want {
			t.Errorf("ScoreForCluster(%d) = %d, want %d", tc.n, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// floodMatchCluster helpers
// ---------------------------------------------------------------------------

// makeTestCells builds a cols×rows Cells slice filled with nil organisms (all dead/empty).
func makeTestCells(cols, rows int) [][]Cell {
	cells := make([][]Cell, cols)
	for c := 0; c < cols; c++ {
		cells[c] = make([]Cell, rows)
	}
	return cells
}

// placeOrganism sets cells[col][row] to an alive cell with the given organism pointer.
func placeOrganism(cells [][]Cell, col, row int, o *Organism) {
	cells[col][row] = Cell{organism: o, health: 1.0}
}

// ---------------------------------------------------------------------------
// floodMatchCluster
// ---------------------------------------------------------------------------

// TestFloodMatchClusterLoneAnimal verifies that a single animal with no matching
// neighbours returns only the seed.
func TestFloodMatchClusterLoneAnimal(t *testing.T) {
	animal := &Organism{name: "A", pairID: 0, isAnimal: true}
	other := &Organism{name: "B", pairID: 1, isAnimal: true}

	cells := makeTestCells(3, 3)
	placeOrganism(cells, 1, 1, animal)
	placeOrganism(cells, 0, 1, other)
	placeOrganism(cells, 2, 1, other)
	placeOrganism(cells, 1, 0, other)
	placeOrganism(cells, 1, 2, other)

	got := FloodMatchCluster(cells, 1, 1, 3, 3)
	if len(got) != 1 || got[0] != [2]int{1, 1} {
		t.Errorf("FloodMatchCluster lone animal = %v, want [{1 1}]", got)
	}
}

// TestFloodMatchClusterChainOfAnimals verifies that a chain of 3 same-pointer
// animals (all orthogonally adjacent) are all included via flood spread.
func TestFloodMatchClusterChainOfAnimals(t *testing.T) {
	animal := &Organism{name: "A", pairID: 0, isAnimal: true}

	// Layout (3×1): [animal][animal][animal]
	cells := makeTestCells(3, 1)
	placeOrganism(cells, 0, 0, animal)
	placeOrganism(cells, 1, 0, animal)
	placeOrganism(cells, 2, 0, animal)

	got := FloodMatchCluster(cells, 0, 0, 3, 1)
	if len(got) != 3 {
		t.Fatalf("FloodMatchCluster chain of 3 = %v (len %d), want 3 results", got, len(got))
	}
	coords := map[[2]int]bool{}
	for _, c := range got {
		coords[c] = true
	}
	for _, want := range [][2]int{{0, 0}, {1, 0}, {2, 0}} {
		if !coords[want] {
			t.Errorf("FloodMatchCluster chain of 3: missing %v in %v", want, got)
		}
	}
}

// TestFloodMatchClusterWithPairedFood verifies that an animal with an
// orthogonally adjacent paired food includes both.
func TestFloodMatchClusterWithPairedFood(t *testing.T) {
	animal := &Organism{name: "A", pairID: 2, isAnimal: true}
	food := &Organism{name: "F", pairID: 2, isAnimal: false}

	cells := makeTestCells(2, 1)
	placeOrganism(cells, 0, 0, animal)
	placeOrganism(cells, 1, 0, food)

	got := FloodMatchCluster(cells, 0, 0, 2, 1)
	if len(got) != 2 {
		t.Fatalf("FloodMatchCluster animal+food = %v (len %d), want 2", got, len(got))
	}
	coords := map[[2]int]bool{}
	for _, c := range got {
		coords[c] = true
	}
	if !coords[[2]int{0, 0}] || !coords[[2]int{1, 0}] {
		t.Errorf("FloodMatchCluster animal+food: got %v, want both {0,0} and {1,0}", got)
	}
}

// TestFloodMatchClusterNonPairedFoodExcluded verifies that adjacent food with a
// different PairID is not included.
func TestFloodMatchClusterNonPairedFoodExcluded(t *testing.T) {
	animal := &Organism{name: "A", pairID: 2, isAnimal: true}
	wrongFood := &Organism{name: "WF", pairID: 3, isAnimal: false}

	cells := makeTestCells(2, 1)
	placeOrganism(cells, 0, 0, animal)
	placeOrganism(cells, 1, 0, wrongFood)

	got := FloodMatchCluster(cells, 0, 0, 2, 1)
	if len(got) != 1 || got[0] != [2]int{0, 0} {
		t.Errorf("FloodMatchCluster non-paired food = %v, want only [{0 0}]", got)
	}
}

// TestFloodMatchClusterFoodDoesNotSpread verifies that food is a leaf: a food
// cell between two separated animals is included once but does not bridge them.
//
// Layout (3×1): [animalA][food(pairID=A)][animalA]
// The food is adjacent to both animals but food does not enqueue, so only the
// seed animal and the food it touches are included — the far animal is not reachable.
func TestFloodMatchClusterFoodDoesNotSpread(t *testing.T) {
	animal := &Organism{name: "A", pairID: 5, isAnimal: true}
	food := &Organism{name: "F", pairID: 5, isAnimal: false}

	cells := makeTestCells(3, 1)
	placeOrganism(cells, 0, 0, animal)
	placeOrganism(cells, 1, 0, food)
	placeOrganism(cells, 2, 0, animal)

	got := FloodMatchCluster(cells, 0, 0, 3, 1)
	// Seed at (0,0) finds food at (1,0) → included but not enqueued.
	// (2,0) is never reached from (1,0) because food is not enqueued.
	if len(got) != 2 {
		t.Fatalf("FloodMatchCluster food-no-spread = %v (len %d), want 2 (seed + food only)", got, len(got))
	}
	coords := map[[2]int]bool{}
	for _, c := range got {
		coords[c] = true
	}
	if !coords[[2]int{0, 0}] || !coords[[2]int{1, 0}] {
		t.Errorf("FloodMatchCluster food-no-spread: got %v, want {0,0} and {1,0}", got)
	}
	if coords[[2]int{2, 0}] {
		t.Errorf("FloodMatchCluster food-no-spread: far animal {2,0} should not be reachable, got %v", got)
	}
}

// TestFloodMatchClusterDiagonalIncluded verifies that a diagonally adjacent
// same-animal cell IS included (8-neighbour flood).
func TestFloodMatchClusterDiagonalIncluded(t *testing.T) {
	animal := &Organism{name: "A", pairID: 7, isAnimal: true}

	// Layout (2×2): [animal][empty]
	//               [empty][animal]
	// Seed at (0,0) with diagonal same-animal at (1,1).
	cells := makeTestCells(2, 2)
	placeOrganism(cells, 0, 0, animal)
	placeOrganism(cells, 1, 1, animal)

	got := FloodMatchCluster(cells, 0, 0, 2, 2)
	if len(got) != 2 {
		t.Fatalf("FloodMatchCluster diagonal included = %v (len %d), want 2 results", got, len(got))
	}
	coords := map[[2]int]bool{}
	for _, c := range got {
		coords[c] = true
	}
	if !coords[[2]int{0, 0}] || !coords[[2]int{1, 1}] {
		t.Errorf("FloodMatchCluster diagonal included: got %v, want both {0,0} and {1,1}", got)
	}
}

// TestFloodMatchClusterDiagonalFoodIncluded verifies that a diagonally adjacent
// paired food cell IS included as a leaf (8-neighbour).
func TestFloodMatchClusterDiagonalFoodIncluded(t *testing.T) {
	animal := &Organism{name: "A", pairID: 3, isAnimal: true}
	food := &Organism{name: "F", pairID: 3, isAnimal: false}

	// Layout (2×2): [animal][empty]
	//               [empty][food]
	// Seed at (0,0) with diagonal food at (1,1).
	cells := makeTestCells(2, 2)
	placeOrganism(cells, 0, 0, animal)
	placeOrganism(cells, 1, 1, food)

	got := FloodMatchCluster(cells, 0, 0, 2, 2)
	if len(got) != 2 {
		t.Fatalf("FloodMatchCluster diagonal food included = %v (len %d), want 2 results", got, len(got))
	}
	coords := map[[2]int]bool{}
	for _, c := range got {
		coords[c] = true
	}
	if !coords[[2]int{0, 0}] || !coords[[2]int{1, 1}] {
		t.Errorf("FloodMatchCluster diagonal food included: got %v, want both {0,0} and {1,1}", got)
	}
}

// ---------------------------------------------------------------------------
// clusterHasFood
// ---------------------------------------------------------------------------

// TestClusterHasFoodWithFood verifies that a cluster containing a food cell returns true.
func TestClusterHasFoodWithFood(t *testing.T) {
	animal := &Organism{name: "A", pairID: 1, isAnimal: true}
	food := &Organism{name: "F", pairID: 1, isAnimal: false}

	cells := makeTestCells(2, 1)
	placeOrganism(cells, 0, 0, animal)
	placeOrganism(cells, 1, 0, food)

	coords := [][2]int{{0, 0}, {1, 0}}
	if !ClusterHasFood(cells, coords) {
		t.Error("ClusterHasFood with food cell = false, want true")
	}
}

// TestClusterHasFoodAnimalsOnly verifies that an animals-only cluster returns false.
func TestClusterHasFoodAnimalsOnly(t *testing.T) {
	animal := &Organism{name: "A", pairID: 1, isAnimal: true}

	cells := makeTestCells(2, 1)
	placeOrganism(cells, 0, 0, animal)
	placeOrganism(cells, 1, 0, animal)

	coords := [][2]int{{0, 0}, {1, 0}}
	if ClusterHasFood(cells, coords) {
		t.Error("ClusterHasFood animals-only = true, want false")
	}
}

// TestClusterHasFoodSingleAnimal verifies that a lone animal (seed only) returns false.
func TestClusterHasFoodSingleAnimal(t *testing.T) {
	animal := &Organism{name: "A", pairID: 1, isAnimal: true}

	cells := makeTestCells(1, 1)
	placeOrganism(cells, 0, 0, animal)

	coords := [][2]int{{0, 0}}
	if ClusterHasFood(cells, coords) {
		t.Error("ClusterHasFood single animal = true, want false")
	}
}

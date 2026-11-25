package main

import (
	"testing"

	"mixersmash/internal/board"
)

// TestBuildDropMoveAnims verifies the MoveAnim entries produced by
// buildDropMoveAnims for a small grid column. The grid is set up in its
// post-applyDropAndClear state (cells already gravity-compacted), and the
// helper is called with the same moves slice that applyDropAndClear would have
// used. Only from/to coordinates and count are checked — Cell fields are set
// by the caller in real usage and are not under test here.
func TestBuildDropMoveAnims(t *testing.T) {
	// 4-row grid, 1 column.  Row 2 is the hole; rows 0, 1, 3 are alive.
	// gravityMoves(4, [2]) yields [[0,1],[1,2]] — row 3 stays in place.
	// After applyDropAndClear the grid is:
	//   row 0: Cell{}  (zeroed — no respawn)
	//   row 1: original row-0 cell
	//   row 2: original row-1 cell
	//   row 3: original row-3 cell (unchanged)
	const dcs = 10
	const cols = 1
	const rows = 4

	cr := func() *board.Organism { return &board.Organism{} }

	cells := make([][]board.Cell, cols)
	cells[0] = []board.Cell{
		{Organism: nil, Health: 0},    // row 0: vacated slot
		{Organism: cr(), Health: 1.0}, // row 1: compacted from row 0
		{Organism: cr(), Health: 1.0}, // row 2: compacted from row 1
		{Organism: cr(), Health: 1.0}, // row 3: unchanged
	}

	g := &Game{
		board: boardState{grid: &board.Grid{Cols: cols, Rows: rows, Cells: cells}},
	}

	holeRows := []int{2}
	moves := board.GravityMoves(rows, holeRows) // [[0,1],[1,2]]

	g.buildDropMoveAnims(0, holeRows, moves, dcs)

	// respawnCells is false (compile-time constant), so only moved-cell anims are produced.
	wantCount := len(moves) // 2
	if len(g.anim.moveAnims) != wantCount {
		t.Fatalf("moveAnims count = %d; want %d", len(g.anim.moveAnims), wantCount)
	}

	tests := []struct {
		name         string
		fromX, fromY float32
		toX, toY     float32
	}{
		{
			name:  "move [0->1]",
			fromX: float32(0*dcs + dcs/2), fromY: float32(0*dcs + dcs/2), // (5,5)
			toX: float32(0*dcs + dcs/2), toY: float32(1*dcs + dcs/2), // (5,15)
		},
		{
			name:  "move [1->2]",
			fromX: float32(0*dcs + dcs/2), fromY: float32(1*dcs + dcs/2), // (5,15)
			toX: float32(0*dcs + dcs/2), toY: float32(2*dcs + dcs/2), // (5,25)
		},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := g.anim.moveAnims[i]
			if a.FromX != tc.fromX || a.FromY != tc.fromY {
				t.Errorf("anim[%d] From = (%.1f,%.1f); want (%.1f,%.1f)",
					i, a.FromX, a.FromY, tc.fromX, tc.fromY)
			}
			if a.ToX != tc.toX || a.ToY != tc.toY {
				t.Errorf("anim[%d] To = (%.1f,%.1f); want (%.1f,%.1f)",
					i, a.ToX, a.ToY, tc.toX, tc.toY)
			}
		})
	}
}

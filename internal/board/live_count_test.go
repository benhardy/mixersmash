package board

import (
	"testing"
)

// liveCell returns a Cell with Health=1.0 and a non-nil Organism.
func liveCell() Cell {
	return Cell{Organism: &Organism{}, Health: 1.0}
}

// deadCell returns a Cell with Health=0 and a non-nil Organism.
func deadCell() Cell {
	return Cell{Organism: &Organism{}, Health: 0}
}

// ---------------------------------------------------------------------------
// NewGrid live count
// ---------------------------------------------------------------------------

// TestNewGridLiveCount verifies that NewGrid(3,3) seeds Live()==9.
func TestNewGridLiveCount(t *testing.T) {
	g := NewGrid(3, 3)
	if got := g.Live(); got != 9 {
		t.Errorf("NewGrid(3,3).Live() = %d, want 9", got)
	}
}

// ---------------------------------------------------------------------------
// Place
// ---------------------------------------------------------------------------

// TestPlaceLiveDeltas verifies the four placement cases.
func TestPlaceLiveDeltas(t *testing.T) {
	cases := []struct {
		name    string
		initial Cell
		placed  Cell
		want    int // expected Live() after Place (starting from Live()==1 if initial is live, else 0)
	}{
		{
			name:    "alive-over-alive: unchanged",
			initial: liveCell(),
			placed:  liveCell(),
			want:    1,
		},
		{
			name:    "alive-over-dead: +1",
			initial: deadCell(),
			placed:  liveCell(),
			want:    1,
		},
		{
			name:    "dead-over-alive: -1",
			initial: liveCell(),
			placed:  Cell{},
			want:    0,
		},
		{
			name:    "dead-over-dead: unchanged",
			initial: deadCell(),
			placed:  Cell{},
			want:    0,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Build a minimal 1x1 grid, bypass NewGrid to control initial state.
			g := &Grid{
				Cols:  1,
				Rows:  1,
				Cells: [][]Cell{{{}}},
			}
			// Seed the cell directly and set live accordingly.
			g.Cells[0][0] = tc.initial
			if tc.initial.Health > 0 {
				g.live = 1
			}

			g.Place(0, 0, tc.placed)
			if got := g.Live(); got != tc.want {
				t.Errorf("%s: Live() = %d, want %d", tc.name, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// SetHealthZero
// ---------------------------------------------------------------------------

// TestSetHealthZeroDeltas verifies decrement on live and idempotency on dead.
func TestSetHealthZeroDeltas(t *testing.T) {
	t.Run("live cell decrements live by 1", func(t *testing.T) {
		g := &Grid{
			Cols:  1,
			Rows:  1,
			Cells: [][]Cell{{liveCell()}},
			live:  1,
		}
		g.SetHealthZero(0, 0)
		if got := g.Live(); got != 0 {
			t.Errorf("SetHealthZero on live: Live() = %d, want 0", got)
		}
	})

	t.Run("calling again on same cell: unchanged", func(t *testing.T) {
		g := &Grid{
			Cols:  1,
			Rows:  1,
			Cells: [][]Cell{{liveCell()}},
			live:  1,
		}
		g.SetHealthZero(0, 0)
		g.SetHealthZero(0, 0) // idempotent
		if got := g.Live(); got != 0 {
			t.Errorf("SetHealthZero idempotent: Live() = %d, want 0", got)
		}
	})
}

// ---------------------------------------------------------------------------
// BumpHealthAt
// ---------------------------------------------------------------------------

// TestBumpHealthAtLivenessAndReturn verifies delta crossing, non-crossing, and old-health return.
func TestBumpHealthAtLivenessAndReturn(t *testing.T) {
	t.Run("crosses to <=0: live decrements, returns old health", func(t *testing.T) {
		g := &Grid{
			Cols:  1,
			Rows:  1,
			Cells: [][]Cell{{{Health: 0.5}}},
			live:  1,
		}
		oldH := g.BumpHealthAt(0, 0, -0.6) // 0.5 + (-0.6) = -0.1 → clamped to 0
		if oldH != 0.5 {
			t.Errorf("BumpHealthAt returns old health: got %v, want 0.5", oldH)
		}
		if got := g.Live(); got != 0 {
			t.Errorf("BumpHealthAt crosses to 0: Live() = %d, want 0", got)
		}
	})

	t.Run("stays >0: live unchanged", func(t *testing.T) {
		g := &Grid{
			Cols:  1,
			Rows:  1,
			Cells: [][]Cell{{{Health: 0.5}}},
			live:  1,
		}
		oldH := g.BumpHealthAt(0, 0, -0.3) // 0.5 - 0.3 = 0.2 > 0
		if oldH != 0.5 {
			t.Errorf("BumpHealthAt stays >0 returns old health: got %v, want 0.5", oldH)
		}
		if got := g.Live(); got != 1 {
			t.Errorf("BumpHealthAt stays >0: Live() = %d, want 1", got)
		}
	})

	t.Run("dead bumps to alive: live increments", func(t *testing.T) {
		g := &Grid{
			Cols:  1,
			Rows:  1,
			Cells: [][]Cell{{{Health: 0}}},
			live:  0,
		}
		g.BumpHealthAt(0, 0, 0.5)
		if got := g.Live(); got != 1 {
			t.Errorf("BumpHealthAt dead->alive: Live() = %d, want 1", got)
		}
	})
}

// ---------------------------------------------------------------------------
// ReplaceColumn
// ---------------------------------------------------------------------------

// TestReplaceColumnLiveDeltas verifies that Live() reflects the net delta after column replacement.
func TestReplaceColumnLiveDeltas(t *testing.T) {
	t.Run("fewer live cells in src: live decreases", func(t *testing.T) {
		// 3-row column, all live initially → Live()==3
		g := &Grid{
			Cols: 1,
			Rows: 3,
			Cells: [][]Cell{{
				liveCell(),
				liveCell(),
				liveCell(),
			}},
			live: 3,
		}
		src := []Cell{liveCell(), deadCell(), deadCell()}
		g.ReplaceColumn(0, src)
		if got := g.Live(); got != 1 {
			t.Errorf("ReplaceColumn fewer live: Live() = %d, want 1", got)
		}
	})

	t.Run("more live cells in src: live increases", func(t *testing.T) {
		// 3-row column, all dead initially → Live()==0
		g := &Grid{
			Cols: 1,
			Rows: 3,
			Cells: [][]Cell{{
				deadCell(),
				deadCell(),
				deadCell(),
			}},
			live: 0,
		}
		src := []Cell{liveCell(), liveCell(), deadCell()}
		g.ReplaceColumn(0, src)
		if got := g.Live(); got != 2 {
			t.Errorf("ReplaceColumn more live: Live() = %d, want 2", got)
		}
	})
}

// ---------------------------------------------------------------------------
// Gravity-like sequence
// ---------------------------------------------------------------------------

// TestGravityLikeSequence verifies a gravity simulation: take a column snapshot,
// compact live cells downward (Place moved cells + zero the vacated slots),
// and confirm Live() equals the remaining live count.
func TestGravityLikeSequence(t *testing.T) {
	// 1-column, 4-row grid: rows 0,1,2 are live; row 3 is dead (simulating a gap).
	// After gravity the two cells originally at row 0 and row 1 should fall into
	// row 2 and row 3; row 1 vacated stays live (cell moved), row 0 vacated becomes empty.
	// Net: still 3 live cells → Live() == 3.
	g := &Grid{
		Cols: 1,
		Rows: 4,
		Cells: [][]Cell{{
			liveCell(), // row 0
			liveCell(), // row 1
			liveCell(), // row 2
			{},         // row 3: dead/empty
		}},
		live: 3,
	}

	// Snapshot the column before mutation.
	snap := g.Column(0)

	// Gravity: hole at row 3 (empty), compact rows 0,1,2 → they stay in place
	// because GravityMoves(4, []int{3}) returns empty (the top row fills the gap).
	// Instead simulate: hole at row 1 (remove it), compact 0,2 downward.
	// GravityMoves(4, []int{1}): alive=[0,2,3], newCount=1 → 0→1, 2→2(stay), 3→3(stay).
	// But row 3 was dead. After move: row 1 gets snap[0], row 0 gets zeroed.
	g.Place(0, 1, snap[0]) // move row 0 → row 1
	g.Place(0, 0, Cell{})  // vacate old row 0

	// Live cells: row 0 is now empty (-1), row 1 has snap[0] (was live, still live, net 0),
	// row 2 is still live, row 3 still dead. Net live: 2.
	want := 2
	if got := g.Live(); got != want {
		t.Errorf("gravity-like sequence: Live() = %d, want %d", got, want)
	}
}

// ---------------------------------------------------------------------------
// Cluster kill
// ---------------------------------------------------------------------------

// TestClusterKill verifies that zeroing N distinct live cells decrements Live() by N,
// and that zeroing an already-dead cell does not double-count.
func TestClusterKill(t *testing.T) {
	// 3x3 grid, all live (Live()==9 from NewGrid).
	g := NewGrid(3, 3)

	// Zero 4 distinct live cells.
	g.SetHealthZero(0, 0)
	g.SetHealthZero(0, 1)
	g.SetHealthZero(1, 0)
	g.SetHealthZero(1, 1)
	if got := g.Live(); got != 5 {
		t.Errorf("cluster kill 4 live cells: Live() = %d, want 5", got)
	}

	// Zero one already-dead cell: live must not change.
	g.SetHealthZero(0, 0)
	if got := g.Live(); got != 5 {
		t.Errorf("cluster kill already-dead cell: Live() = %d, want 5 (unchanged)", got)
	}
}

// ---------------------------------------------------------------------------
// Round-trip birth → death → birth
// ---------------------------------------------------------------------------

// TestRoundTripBirthDeathBirth verifies net +1,-1,+1 liveness changes on one cell.
func TestRoundTripBirthDeathBirth(t *testing.T) {
	// Start with a dead cell.
	g := &Grid{
		Cols:  1,
		Rows:  1,
		Cells: [][]Cell{{deadCell()}},
		live:  0,
	}

	// Birth: place a live cell.
	g.Place(0, 0, liveCell())
	if got := g.Live(); got != 1 {
		t.Errorf("after birth: Live() = %d, want 1", got)
	}

	// Death: zero the health.
	g.SetHealthZero(0, 0)
	if got := g.Live(); got != 0 {
		t.Errorf("after death: Live() = %d, want 0", got)
	}

	// Rebirth: bump health back up.
	g.BumpHealthAt(0, 0, 1.0)
	if got := g.Live(); got != 1 {
		t.Errorf("after rebirth: Live() = %d, want 1", got)
	}
}

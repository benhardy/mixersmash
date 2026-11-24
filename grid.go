package main

import (
	"image/color"
	"time"
)

// Cell holds a grid cell's organism reference and health (1.0 = full, 0.0 = dead).
// A nil Organism means the cell is empty/dead.
type Cell struct {
	Organism *Organism
	Health   float32
}

// Kills a cell stone dead, returning its old health. The caller must redraw the cell after calling Die().
func (c *Cell) Die() float32 {
	oldHealth := c.Health
	c.Health = 0
	return oldHealth
}

func (c *Cell) BumpHealth(delta float32) float32 {
	oldHealth := c.Health
	c.Health = max(c.Health+delta, 0)
	return oldHealth
}

func (c *Cell) IsDead() bool {
	return c.Health <= 0
}

// Color returns the palette colour for this cell's organism, or a zero color.RGBA
// (A=0) if the cell is empty (Organism == nil). All palette colours have A=255,
// so A=0 is a safe sentinel for an empty cell.
func (c Cell) Color() color.RGBA {
	if c.Organism == nil {
		return color.RGBA{}
	}
	return c.Organism.Color
}

// MoveAnim describes the animated movement of a cell from one pixel position to
// another during shuffle, drop, or victim-move phases. FromX/FromY and ToX/ToY
// are pixel centres. CtrlX/CtrlY is the quadratic Bézier control point used
// during shuffle and arc-path victim moves; zero value falls back to straight-line
// interpolation (used for drops).
//
// Col, Row, PathType, and SpiralTurns are used only by the victim-move phase.
// PathType: 0 = straight line, 1 = quadratic Bézier arc, 2 = inward spiral.
// SpiralTurns: signed — positive = CCW, negative = CW.
//
// NewHealth is used only for the Mixer's stationary shuffle entry: it holds the
// post-click health so the bounce animation can interpolate from Cell.Health
// (pre-click) to NewHealth (post-click). Zero for all other entries.
type MoveAnim struct {
	FromX, FromY float32
	ToX, ToY     float32
	CtrlX, CtrlY float32 // quadratic Bézier control point; zero = straight line
	Cell         Cell
	Col          int     // victim-move phase: original grid column
	Row          int     // victim-move phase: original grid row
	PathType     int     // victim-move phase: 0=straight, 1=arc, 2=spiral
	SpiralTurns  float32 // victim-move phase: signed turn count
	Rotations    float32 // victim-move phase: signed rotation count
	NewHealth    float32 // Mixer shuffle entry only: post-click health (zero for neighbours)
}

// Grid holds the grid dimensions and per-cell colours.
type Grid struct {
	Cols  int
	Rows  int
	Cells [][]Cell
}

// NewGrid initialises a Grid with random opaque colours in every cell.
func NewGrid(cols, rows int) *Grid {
	cells := make([][]Cell, cols)
	for c := 0; c < cols; c++ {
		cells[c] = make([]Cell, rows)
		for r := 0; r < rows; r++ {
			cells[c][r] = Cell{
				Organism: PaletteOrganism(),
				Health:   1.0,
			}
		}
	}
	return &Grid{
		Cols:  cols,
		Rows:  rows,
		Cells: cells,
	}
}

// eightNeighbors appends all in-bounds (col+dx, row+dy) for dx,dy in
// {-1,0,1}×{-1,0,1} excluding (0,0) into buf (reset to [:0] first) and
// returns the updated slice. The caller owns buf; pass a pre-allocated slice
// to avoid per-call allocation once capacity stabilises at 8.
func eightNeighbors(buf [][2]int, col, row, cols, rows int) [][2]int {
	buf = buf[:0]
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			nc := col + dx
			nr := row + dy
			if nc >= 0 && nc < cols && nr >= 0 && nr < rows {
				buf = append(buf, [2]int{nc, nr})
			}
		}
	}
	return buf
}

// GravityMoves returns the [oldRow, newRow] mapping for cells in a column of numRows
// when holes appear at holeRows. Only returns entries for cells that actually change row.
// Cells not in holeRows are "alive" and compact downward; holeRows are removed.
func GravityMoves(numRows int, holeRows []int) [][2]int {
	// Build a set of hole rows for fast lookup.
	holeSet := make(map[int]bool, len(holeRows))
	for _, r := range holeRows {
		holeSet[r] = true
	}
	// Collect alive rows top-to-bottom
	var alive []int
	for r := 0; r < numRows; r++ {
		if !holeSet[r] {
			alive = append(alive, r)
		}
	}
	newCount := numRows - len(alive) // number of new cells needed at top
	var moves [][2]int
	for i, oldRow := range alive {
		newRow := newCount + i
		if oldRow != newRow {
			moves = append(moves, [2]int{oldRow, newRow})
		}
	}
	return moves
}

// HorizontalCollapseMoves returns the [oldCol, newCol] mapping for non-empty
// columns when empty columns are removed and the remaining columns are packed
// toward the centre.
//
// nonEmpty = columns in [0,numCols) not in emptyCols, ascending.
// leftEmpties = count of emptyCols entries with index < numCols/2.
// nonEmpty[i] maps to (leftEmpties + i).
// Only pairs where oldCol != newCol are returned.
func HorizontalCollapseMoves(numCols int, emptyCols []int) [][2]int {
	emptySet := make(map[int]bool, len(emptyCols))
	for _, c := range emptyCols {
		emptySet[c] = true
	}
	var nonEmpty []int
	for c := 0; c < numCols; c++ {
		if !emptySet[c] {
			nonEmpty = append(nonEmpty, c)
		}
	}
	half := numCols / 2
	leftEmpties := 0
	for _, c := range emptyCols {
		if c < half {
			leftEmpties++
		}
	}
	var moves [][2]int
	for i, oldCol := range nonEmpty {
		newCol := leftEmpties + i
		if oldCol != newCol {
			moves = append(moves, [2]int{oldCol, newCol})
		}
	}
	return moves
}

// ApplyColumnCollapse returns a new column layout derived from snapshot by
// applying the given moves. It is a pure function with no ebiten dependency.
//
// Algorithm:
//  1. Start with every column copied from snapshot (preserves unchanged columns).
//  2. For each move {oldCol, newCol}, set result[newCol] = snapshot[oldCol].
//  3. Zero only the vacated columns: {oldCol for each move} MINUS {newCol for
//     each move}. These are the edge columns that lost their content and were
//     not themselves targeted as a destination.
//
// Columns that neither moved nor were landed on are preserved unchanged.
func ApplyColumnCollapse(snapshot [][]Cell, moves [][2]int) [][]Cell {
	numCols := len(snapshot)
	result := make([][]Cell, numCols)
	for c := 0; c < numCols; c++ {
		col := make([]Cell, len(snapshot[c]))
		copy(col, snapshot[c])
		result[c] = col
	}

	// Collect newCol values so we can compute the vacated set.
	newColSet := make(map[int]bool, len(moves))
	for _, m := range moves {
		newColSet[m[1]] = true
	}

	// Apply moves: destination gets source column's data.
	for _, m := range moves {
		copy(result[m[1]], snapshot[m[0]])
	}

	// Zero vacated columns: sources that are not also destinations.
	for _, m := range moves {
		if !newColSet[m[0]] {
			for r := range result[m[0]] {
				result[m[0]][r] = Cell{}
			}
		}
	}

	return result
}

// FloodMatchCluster returns all grid coordinates that belong to the flood-match
// cluster seeded at (col, row). The seed cell must hold an animal Organism.
//
// Rules (BFS over 8-neighbours, pure — no mutation):
//   - Same animal pointer as the seed: included and enqueued (flood spreads).
//   - Different organism whose PairID equals the seed's PairID and IsAnimal==false
//     (the paired food): included but NOT enqueued (food is a leaf, does not spread).
//   - Everything else: ignored.
//
// Returns a [][2]int of (col, row) pairs including the seed. A lone animal with no
// matching neighbours returns a single-element slice.
func FloodMatchCluster(cells [][]Cell, col, row, cols, rows int) [][2]int {
	seed := cells[col][row].Organism
	if seed == nil || !seed.IsAnimal {
		return nil
	}

	visited := make([][]bool, cols)
	for i := range visited {
		visited[i] = make([]bool, rows)
	}
	visited[col][row] = true

	result := [][2]int{{col, row}}
	queue := [][2]int{{col, row}}

	var nbuf [][2]int
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		nbuf = eightNeighbors(nbuf, cur[0], cur[1], cols, rows)
		for _, n := range nbuf {
			nc, nr := n[0], n[1]
			if visited[nc][nr] {
				continue
			}
			o := cells[nc][nr].Organism
			if o == nil {
				continue
			}
			if o == seed {
				// Same animal pointer: add and enqueue.
				visited[nc][nr] = true
				result = append(result, [2]int{nc, nr})
				queue = append(queue, [2]int{nc, nr})
			} else if o.PairID == seed.PairID && !o.IsAnimal {
				// Paired food: add but do NOT enqueue.
				visited[nc][nr] = true
				result = append(result, [2]int{nc, nr})
			}
		}
	}
	return result
}

// ClusterHasFood reports whether any cell in coords is a food cell
// (Organism != nil and IsAnimal == false). Used by commitCellAt to gate the
// no-food no-op rule: a cluster with no paired food must not clear.
func ClusterHasFood(cells [][]Cell, coords [][2]int) bool {
	for _, coord := range coords {
		o := cells[coord[0]][coord[1]].Organism
		if o != nil && !o.IsAnimal {
			return true
		}
	}
	return false
}

// ScoreForCluster returns the score for clearing a cluster of n cells.
// Score is super-linear: n*n.
func ScoreForCluster(n int) int {
	return n * n
}

// ElapsedFraction returns how far through a phase we are as a 0→1 float32,
// clamped to [0, 1].  start is the wall-clock time when the phase began,
// now is the current wall-clock time, and duration is the total phase length.
// Using wall-clock time (rather than game ticks) allows smooth interpolation
// at display refresh rates above 60 Hz.
func ElapsedFraction(start, now time.Time, duration time.Duration) float32 {
	f := float32(now.Sub(start)) / float32(duration)
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

// DeviceCellCenter returns the device-pixel centre of a grid cell for the given
// device cell size dcs.  Used wherever pixel coordinates must map to the
// device-resolution canvas or screen.
func DeviceCellCenter(col, row, dcs int) (float32, float32) {
	return float32(col*dcs + dcs/2), float32(row*dcs + dcs/2)
}

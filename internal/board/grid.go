package board

import (
	"image/color"
	"time"
)

// Cell holds a grid cell's organism reference and health (1.0 = full, 0.0 = dead).
// A nil organism means the cell is empty/dead.
type Cell struct {
	organism *Organism
	health   float32
}

// NewCell constructs a Cell with the given organism pointer and health value.
// Use this from outside package board where struct-literal construction is not possible.
func NewCell(o *Organism, health float32) Cell {
	return Cell{organism: o, health: health}
}

// Organism returns the cell's organism pointer (may be nil for an empty cell).
func (c Cell) Organism() *Organism {
	return c.organism
}

// Health returns the cell's current health value (1.0 = full, 0.0 = dead).
func (c Cell) Health() float32 {
	return c.health
}

// BumpHealth adjusts the cell's health by delta (clamped to >= 0) and returns the old health.
func (c *Cell) BumpHealth(delta float32) float32 {
	oldHealth := c.health
	c.health = max(c.health+delta, 0)
	return oldHealth
}

// IsDead reports whether the cell's health is at or below zero.
func (c Cell) IsDead() bool {
	return c.health <= 0
}

// Color returns the palette colour for this cell's organism, or a zero color.RGBA
// (A=0) if the cell is empty (organism == nil). All palette colours have A=255,
// so A=0 is a safe sentinel for an empty cell.
func (c Cell) Color() color.RGBA {
	if c.organism == nil {
		return color.RGBA{}
	}
	return c.organism.color
}

// MoveAnim describes the animated movement of a cell from one pixel position to
// another during shuffle, drop, or victim-move phases. fromX/fromY and toX/toY
// are pixel centres. ctrlX/ctrlY is the quadratic Bézier control point used
// during shuffle and arc-path victim moves; zero value falls back to straight-line
// interpolation (used for drops).
//
// col, row, pathType, and spiralTurns are used only by the victim-move phase.
// pathType: 0 = straight line, 1 = quadratic Bézier arc, 2 = inward spiral.
// spiralTurns: signed — positive = CCW, negative = CW.
//
// newHealth is used only for the Mixer's stationary shuffle entry: it holds the
// post-click health so the bounce animation can interpolate from cell.Health
// (pre-click) to newHealth (post-click). Zero for all other entries.
//
// rotations is reserved for a planned feature (spin animation). The mixer
// constructor sets it to 1; render uses it as a non-zero sentinel.
type MoveAnim struct {
	fromX, fromY float32
	toX, toY     float32
	ctrlX, ctrlY float32 // quadratic Bézier control point; zero = straight line
	cell         Cell
	col          int     // victim-move phase: original grid column
	row          int     // victim-move phase: original grid row
	pathType     int     // victim-move phase: 0=straight, 1=arc, 2=spiral
	spiralTurns  float32 // victim-move phase: signed turn count
	rotations    float32 // reserved: signed rotation count (mixer sets to 1)
	newHealth    float32 // Mixer shuffle entry only: post-click health (zero for neighbours)
}

// NewMoveAnim constructs a plain move animation (used for drops and horizontal
// column-collapse slides). No Bézier control, no victim fields.
func NewMoveAnim(fromX, fromY, toX, toY float32, cell Cell) MoveAnim {
	return MoveAnim{fromX: fromX, fromY: fromY, toX: toX, toY: toY, cell: cell}
}

// NewShuffleAnim constructs a shuffle-neighbour animation with a quadratic
// Bézier arc from (fromX,fromY) to (toX,toY) through (ctrlX,ctrlY).
func NewShuffleAnim(fromX, fromY, toX, toY, ctrlX, ctrlY float32, cell Cell) MoveAnim {
	return MoveAnim{
		fromX: fromX, fromY: fromY,
		toX: toX, toY: toY,
		ctrlX: ctrlX, ctrlY: ctrlY,
		cell: cell,
	}
}

// NewMixerAnim constructs the stationary mixer-click animation entry. The
// position is a fixed point (fromX,fromY) == (toX,toY) == (ctrlX,ctrlY).
// rotations is set to 1 as a non-zero sentinel used by render to identify this
// entry and apply the bounce-radius interpolation. newHealth is the post-click
// health for the radius interpolation.
func NewMixerAnim(fromX, fromY float32, cell Cell, newHealth float32) MoveAnim {
	return MoveAnim{
		fromX: fromX, fromY: fromY,
		toX: fromX, toY: fromY,
		ctrlX: fromX, ctrlY: fromY,
		cell:      cell,
		newHealth: newHealth,
		pathType:  0,
		rotations: 1,
	}
}

// NewVictimSeedAnim constructs the stationary victim-seed animation entry
// (the clicked cell, which shrinks in place).
func NewVictimSeedAnim(col, row int, cell Cell, fromX, fromY float32) MoveAnim {
	return MoveAnim{
		col: col, row: row,
		cell:  cell,
		fromX: fromX, fromY: fromY,
		toX: fromX, toY: fromY,
		pathType:  0,
		rotations: 0,
	}
}

// NewVictimAnim constructs a non-seed victim animation entry that travels
// from (fromX,fromY) toward the seed centre using the given path parameters.
func NewVictimAnim(col, row int, cell Cell, fromX, fromY, toX, toY float32, pathType int, ctrlX, ctrlY, spiralTurns float32) MoveAnim {
	return MoveAnim{
		col: col, row: row,
		cell:  cell,
		fromX: fromX, fromY: fromY,
		toX: toX, toY: toY,
		pathType:    pathType,
		ctrlX:       ctrlX,
		ctrlY:       ctrlY,
		spiralTurns: spiralTurns,
		rotations:   0,
	}
}

// FromX returns the animation start pixel X coordinate.
func (m MoveAnim) FromX() float32 { return m.fromX }

// FromY returns the animation start pixel Y coordinate.
func (m MoveAnim) FromY() float32 { return m.fromY }

// ToX returns the animation end pixel X coordinate.
func (m MoveAnim) ToX() float32 { return m.toX }

// ToY returns the animation end pixel Y coordinate.
func (m MoveAnim) ToY() float32 { return m.toY }

// CtrlX returns the quadratic Bézier control point X coordinate.
func (m MoveAnim) CtrlX() float32 { return m.ctrlX }

// CtrlY returns the quadratic Bézier control point Y coordinate.
func (m MoveAnim) CtrlY() float32 { return m.ctrlY }

// Cell returns the cell being animated.
func (m MoveAnim) Cell() Cell { return m.cell }

// Col returns the original grid column (victim-move phase only).
func (m MoveAnim) Col() int { return m.col }

// Row returns the original grid row (victim-move phase only).
func (m MoveAnim) Row() int { return m.row }

// PathType returns the path type for victim animations
// (0=straight, 1=Bézier arc, 2=inward spiral).
func (m MoveAnim) PathType() int { return m.pathType }

// SpiralTurns returns the signed spiral turn count (victim spiral path only).
func (m MoveAnim) SpiralTurns() float32 { return m.spiralTurns }

// Rotations returns the signed rotation count. Non-zero identifies the mixer
// entry in the shuffle animation; reserved for a future spin feature.
func (m MoveAnim) Rotations() float32 { return m.rotations }

// NewHealth returns the post-click health for the mixer bounce animation.
// Zero for all non-mixer entries.
func (m MoveAnim) NewHealth() float32 { return m.newHealth }

// Grid holds the grid dimensions and per-cell colours.
type Grid struct {
	Cols  int
	Rows  int
	cells [][]Cell
	live  int // count of cells whose Health > 0
}

// NewGrid initialises a Grid with random opaque colours in every cell.
// Every cell starts with Health=1.0, so live is seeded as cols*rows.
func NewGrid(cols, rows int) *Grid {
	cells := make([][]Cell, cols)
	for c := 0; c < cols; c++ {
		cells[c] = make([]Cell, rows)
		for r := 0; r < rows; r++ {
			cells[c][r] = Cell{
				organism: PaletteOrganism(),
				health:   1.0,
			}
		}
	}
	return &Grid{
		Cols:  cols,
		Rows:  rows,
		cells: cells,
		live:  cols * rows,
	}
}

// Live returns the number of cells whose Health is greater than zero.
func (g *Grid) Live() int {
	return g.live
}

// At returns a copy of the cell at (col, row).
func (g *Grid) At(col, row int) Cell {
	return g.cells[col][row]
}

// Column returns a fresh copy slice of all cells in the given column.
func (g *Grid) Column(col int) []Cell {
	src := g.cells[col]
	dst := make([]Cell, len(src))
	copy(dst, src)
	return dst
}

// Snapshot returns a full deep copy of all grid cells as a [cols][rows] slice.
func (g *Grid) Snapshot() [][]Cell {
	result := make([][]Cell, g.Cols)
	for c := 0; c < g.Cols; c++ {
		col := make([]Cell, g.Rows)
		copy(col, g.cells[c])
		result[c] = col
	}
	return result
}

// Place assigns c to the cell at (col, row) and updates the live count by the
// liveness delta (liveness defined as Health > 0).
func (g *Grid) Place(col, row int, c Cell) {
	old := g.cells[col][row]
	g.cells[col][row] = c
	wasAlive := 0
	if old.health > 0 {
		wasAlive = 1
	}
	isAlive := 0
	if c.health > 0 {
		isAlive = 1
	}
	g.live += isAlive - wasAlive
}

// BumpHealthAt applies BumpHealth to the cell at (col, row) and returns the
// old health. g.live is updated by the liveness delta.
func (g *Grid) BumpHealthAt(col, row int, delta float32) float32 {
	wasAlive := g.cells[col][row].health > 0
	oldHealth := g.cells[col][row].BumpHealth(delta)
	isAlive := g.cells[col][row].health > 0
	if wasAlive && !isAlive {
		g.live--
	} else if !wasAlive && isAlive {
		g.live++
	}
	return oldHealth
}

// SetHealthZero sets the Health of the cell at (col, row) to zero.
// g.live is decremented only if the cell was previously alive (Health > 0).
func (g *Grid) SetHealthZero(col, row int) {
	if g.cells[col][row].health > 0 {
		g.cells[col][row].health = 0
		g.live--
	}
}

// ReplaceColumn copies src into column col cell-by-cell, accumulating the
// total liveness delta for the column and applying it once to g.live.
func (g *Grid) ReplaceColumn(col int, src []Cell) {
	delta := 0
	for r, c := range src {
		old := g.cells[col][r]
		g.cells[col][r] = c
		wasAlive := 0
		if old.health > 0 {
			wasAlive = 1
		}
		isAlive := 0
		if c.health > 0 {
			isAlive = 1
		}
		delta += isAlive - wasAlive
	}
	g.live += delta
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

// EightNeighbors returns the in-bounds 8-neighbours of (col, row) in this Grid,
// appending into buf (reset to [:0] first). The caller owns buf; pass a
// pre-allocated slice to avoid per-call allocation once capacity stabilises at 8.
func (g *Grid) EightNeighbors(buf [][2]int, col, row int) [][2]int {
	return eightNeighbors(buf, col, row, g.Cols, g.Rows)
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

// floodMatchCluster returns all grid coordinates that belong to the flood-match
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
func floodMatchCluster(cells [][]Cell, col, row, cols, rows int) [][2]int {
	seed := cells[col][row].organism
	if seed == nil || !seed.isAnimal {
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
			o := cells[nc][nr].organism
			if o == nil {
				continue
			}
			if o == seed {
				// Same animal pointer: add and enqueue.
				visited[nc][nr] = true
				result = append(result, [2]int{nc, nr})
				queue = append(queue, [2]int{nc, nr})
			} else if o.pairID == seed.pairID && !o.isAnimal {
				// Paired food: add but do NOT enqueue.
				visited[nc][nr] = true
				result = append(result, [2]int{nc, nr})
			}
		}
	}
	return result
}

// clusterHasFood reports whether any cell in coords is a food cell
// (Organism != nil and IsAnimal == false). Used by commitCellAt to gate the
// no-food no-op rule: a cluster with no paired food must not clear.
func clusterHasFood(cells [][]Cell, coords [][2]int) bool {
	for _, coord := range coords {
		o := cells[coord[0]][coord[1]].organism
		if o != nil && !o.isAnimal {
			return true
		}
	}
	return false
}

// FloodMatchCluster returns all grid coordinates that belong to the flood-match
// cluster seeded at (col, row). See floodMatchCluster for the BFS rules.
func (g *Grid) FloodMatchCluster(col, row int) [][2]int {
	return floodMatchCluster(g.cells, col, row, g.Cols, g.Rows)
}

// ClusterHasFood reports whether any cell in coords is a food cell
// (Organism != nil and IsAnimal == false).
func (g *Grid) ClusterHasFood(coords [][2]int) bool {
	return clusterHasFood(g.cells, coords)
}

// ScoreForCluster returns the score for clearing a cluster of n cells.
// Score is super-linear: n*n.
func ScoreForCluster(n int) int {
	return n * n
}

// CountLiveCells returns the number of cells in the grid whose Health is
// greater than zero. It iterates every column and row without allocating.
func (g *Grid) CountLiveCells() int {
	n := 0
	for c := 0; c < g.Cols; c++ {
		for r := 0; r < g.Rows; r++ {
			if g.cells[c][r].health > 0 {
				n++
			}
		}
	}
	return n
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

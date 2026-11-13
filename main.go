package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"math/rand"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	windowWidth  = 800
	windowHeight = 600
	cellSize     = 64
	gridCols     = windowWidth / cellSize  // 25
	gridRows     = windowHeight / cellSize // 18

	stateWaiting      = 1
	stateShuffling    = 2
	stateDropping     = 3
	stateVictimMoving = 4

	shuffleTicks    = 18        // 0.3s at 60tps
	dropTicks       = 18        // 0.3s at 60tps
	victimTicks     = int64(24) // 0.4s at 60tps
	targetAnimTicks = int64(15) // 0.25s at 60 TPS

	healthDecrement = float32(0.2)
	healthMin       = float32(0.2)
	healthEpsilon   = float32(0.001)

	respawnCells = false // set false to leave gaps at top instead of spawning new cells
)

const glowShaderSrc = `//kage:unit pixels
package main

var Center vec2
var InnerRadius float
var OuterRadius float

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
    dist := length(dstPos.xy - Center)
    alpha := 1.0 - (dist-InnerRadius)/(OuterRadius-InnerRadius)
    alpha *= clamp(dist-(InnerRadius-1.0), 0.0, 1.0)
    alpha = clamp(alpha, 0.0, 1.0)
    return vec4(alpha, alpha, alpha, alpha) // RGB values premultiplied by alpha
}
`

// colorDistanceRedmean returns a perceptually-weighted colour distance using the
// "redmean" approximation, which adapts RGB channel weights based on average redness.
func colorDistanceRedmean(a, b color.RGBA) float32 {
	rBar := (float32(a.R) + float32(b.R)) / 2
	dR := float32(a.R) - float32(b.R)
	dG := float32(a.G) - float32(b.G)
	dB := float32(a.B) - float32(b.B)
	return float32(math.Sqrt(float64(
		(2+rBar/256)*dR*dR + 4*dG*dG + (2+(255-rBar)/256)*dB*dB,
	)))
}

// randomColor returns a random opaque colour (A=255).
// will be a subset of web-safe colours
func randomColor() color.RGBA {
	for {
		col := color.RGBA{
			R: uint8(51 * (rand.Intn(4) + 2)),
			G: uint8(51 * (rand.Intn(4) + 2)),
			B: uint8(51 * (rand.Intn(4) + 2)),
			A: 255,
		}
		if col.R == 255 && col.G == 255 && col.B == 255 {
			continue // don't use white
		}
		if col.R < 102 || col.G < 102 || col.B < 102 {
			continue // don't use too-dark colours
		}
		return col
	}
}

// cellFromPixel converts pixel coordinates to (col, row) grid indices.
// No bounds clamping is applied.
func cellFromPixel(px, py, cellSize int) (int, int) {
	return px / cellSize, py / cellSize
}

// eightNeighbors returns all in-bounds (col+dx, row+dy) for dx,dy in
// {-1,0,1}×{-1,0,1} excluding (0,0).
func eightNeighbors(col, row, cols, rows int) [][2]int {
	var result [][2]int
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			nc := col + dx
			nr := row + dy
			if nc >= 0 && nc < cols && nr >= 0 && nr < rows {
				result = append(result, [2]int{nc, nr})
			}
		}
	}
	return result
}

// closestColorNeighbor returns the (col, row) of the 8-neighbour of (col,row)
// with Health > 0 that has minimum colorDistanceRedmean distance to
// cells[col][row].Color. Returns found=false if no healthy neighbour exists.
func closestColorNeighbor(cells [][]Cell, col, row, cols, rows int) (int, int, bool) {
	neighbors := eightNeighbors(col, row, cols, rows)
	target := cells[col][row].Color
	bestDist := float32(-1)
	bestCol, bestRow := 0, 0
	found := false
	for _, n := range neighbors {
		nc, nr := n[0], n[1]
		if cells[nc][nr].Health <= 0 {
			continue
		}
		dist := colorDistanceRedmean(target, cells[nc][nr].Color)
		if !found || dist < bestDist {
			bestDist = dist
			bestCol = nc
			bestRow = nr
			found = true
		}
	}
	return bestCol, bestRow, found
}

// closestColorNeighbors returns all 8-neighbours of (col,row) with Health > 0
// that share the exact minimum colorDistanceRedmean distance to
// cells[col][row].Color. Tie is determined by exact float32 equality.
// Returns an empty slice if no healthy neighbour exists.
func closestColorNeighbors(cells [][]Cell, col, row, cols, rows int) [][2]int {
	neighbors := eightNeighbors(col, row, cols, rows)
	target := cells[col][row].Color
	bestDist := float32(-1)
	found := false
	var result [][2]int
	for _, n := range neighbors {
		nc, nr := n[0], n[1]
		if cells[nc][nr].Health <= 0 {
			continue
		}
		dist := colorDistanceRedmean(target, cells[nc][nr].Color)
		if !found || dist < bestDist {
			bestDist = dist
			result = result[:0]
			result = append(result, [2]int{nc, nr})
			found = true
		} else if dist == bestDist {
			result = append(result, [2]int{nc, nr})
		}
	}
	return result
}

type Cell struct {
	Color  color.RGBA
	Health float32
}

// MoveAnim describes the animated movement of a cell from one pixel position to
// another during shuffle or drop phases. FromX/FromY and ToX/ToY are pixel centres.
// CtrlX/CtrlY is the quadratic Bézier control point used during shuffle; zero
// value falls back to straight-line interpolation (used for drops).
type MoveAnim struct {
	FromX, FromY float32
	ToX, ToY     float32
	CtrlX, CtrlY float32 // quadratic Bézier control point; zero = straight line
	Cell         Cell
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
				Color:  randomColor(),
				Health: 1.0,
			}
		}
	}
	return &Grid{
		Cols:  cols,
		Rows:  rows,
		Cells: cells,
	}
}

// VictimAnim holds per-victim animation state for the stateVictimMoving phase.
// All victims converge on the same target cell centre (toX, toY).
type VictimAnim struct {
	col, row     int     // victim's original grid position
	cell         Cell    // snapshot of the victim cell before animation
	fromX, fromY float32 // pixel start (victim cell centre)
	toX, toY     float32 // pixel end (target cell centre, same for all victims)
	pathType     int     // 0=straight, 1=arc, 2=spiral
	ctrlX, ctrlY float32 // Bezier control point for arc path
	spiralTurns  float32 // signed: positive=CCW, negative=CW
}

// Game is the Ebitengine game state.
type Game struct {
	grid         *Grid
	canvas       *ebiten.Image
	currentState int        // which state are we in?
	animStart    int64      // when did the current animation phase start?
	actionCol    int        // the column an action is happening
	actionRow    int        // the row any given action is happening
	moveAnims    []MoveAnim // used for both shuffle and drop phases
	glowShader   *ebiten.Shader
	cellCount    int // number of live cells currently in the grid

	// hover hint fields
	hoverCol, hoverRow int      // cell under cursor (-1 = none)
	victimHints        [][2]int // closest-colour neighbours of hovered cell (empty = none)

	// victim movement animation fields (one entry per simultaneous victim)
	victims []VictimAnim

	// target cell bounce animation fields
	targetAnimating  bool
	targetAnimStart  int64
	targetAnimStartR float32
	targetAnimPeakR  float32
	targetAnimEndR   float32
	targetAnimColor  color.RGBA
}

// newGameState returns a Game with data fields initialised but no ebiten.Image
// allocated.  Callers that need a canvas (i.e. main) must set g.canvas after
// calling this.
func newGameState(grid *Grid) *Game {
	return &Game{
		grid:            grid,
		currentState:    stateWaiting,
		animStart:       0,
		actionCol:       -1,
		actionRow:       -1,
		cellCount:       grid.Cols * grid.Rows,
		hoverCol:        -1,
		hoverRow:        -1,
		victimHints:     nil,
		victims:         nil,
		targetAnimating: false,
	}
}

// elapsedFraction returns how far through a phase we are as a 0→1 float32.
// currentTick is the game tick at the time of evaluation, startTick is when
// the phase began, and durationTicks is the total length of the phase.
func elapsedFraction(currentTick, startTick, durationTicks int64) float32 {
	return float32(currentTick-startTick) / float32(durationTicks)
}

// cellCenter returns the pixel centre of a grid cell.
func cellCenter(col, row int) (float32, float32) {
	return float32(col*cellSize + cellSize/2), float32(row*cellSize + cellSize/2)
}

// drawCell erases the cell's bounding rectangle on canvas with black and
// redraws the antialiased circle for (col, row).  No glow is drawn here;
// glow is composited onto the screen each frame in Draw().
func (g *Game) drawCell(col, row int, cell *Cell, cs int, cellFullness float32) {
	csFloat := float32(cs)

	// Clear exactly the cell rect — full pixel replacement, not a blend.
	x0 := col * cs
	y0 := row * cs
	g.canvas.SubImage(image.Rect(x0, y0, x0+cs, y0+cs)).(*ebiten.Image).Clear()

	if cell.Health <= 0 {
		return
	}
	halfCs := csFloat / 2
	cx := float32(col*cs) + halfCs
	cy := float32(row*cs) + halfCs
	maxRadius := halfCs - 2
	radius := maxRadius * cellFullness * cell.Health
	clr := cell.Color
	vector.DrawFilledCircle(g.canvas, cx, cy, radius, clr, true)
}

// gravityMoves returns the [oldRow, newRow] mapping for cells in a column of numRows
// when holes appear at holeRows. Only returns entries for cells that actually change row.
// Cells not in holeRows are "alive" and compact downward; holeRows are removed.
func gravityMoves(numRows int, holeRows []int) [][2]int {
	// Build a set of hole rows for fast lookup
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

// rowCell is a helper pairing a row index with its cell, used in buildDropAnims.
type rowCell struct {
	row  int
	cell Cell
}

// buildDropAnims computes MoveAnim entries for each affected column: alive cells
// compact downward, and new random cells fall from above the screen top.
// Only canvas positions that cells are vacating are cleared — not the whole column.
// Returns the number of new cells spawned (non-zero only when respawnCells is true).
func (g *Game) buildDropAnims(removedCols []int) int {
	if cap(g.moveAnims) > 0 {
		g.moveAnims = g.moveAnims[:0]
	}
	spawned := 0
	seen := make(map[int]bool)
	for _, col := range removedCols {
		if seen[col] {
			continue
		}
		seen[col] = true

		// Find which rows in this column are dead (holes just created this round).
		var holeRows []int
		for row := 0; row < g.grid.Rows; row++ {
			if g.grid.Cells[col][row].Health <= 0 {
				holeRows = append(holeRows, row)
			}
		}
		if len(holeRows) == 0 {
			continue
		}

		moves := gravityMoves(g.grid.Rows, holeRows)

		// Apply grid moves using a snapshot to avoid aliasing.
		snapshot := make([]Cell, g.grid.Rows)
		for row := 0; row < g.grid.Rows; row++ {
			snapshot[row] = g.grid.Cells[col][row]
		}

		// Write moved alive cells to their new positions.
		for _, m := range moves {
			g.grid.Cells[col][m[1]] = snapshot[m[0]]
		}

		if !respawnCells {
			// Zero the top slots that were vacated and are not being filled.
			for i := 0; i < len(holeRows); i++ {
				g.grid.Cells[col][i] = Cell{} // Health=0, invisible; canvas already cleared by vacating moves
			}
		}

		if respawnCells {
			// Write new random cells into the top slots (0..len(holeRows)-1).
			for i := 0; i < len(holeRows); i++ {
				g.grid.Cells[col][i] = Cell{Color: randomColor(), Health: 1.0}
				spawned++
			}
		}

		// Create MoveAnims for moved alive cells — clear only their old (vacated) positions.
		for _, m := range moves {
			cell := g.grid.Cells[col][m[1]]
			fx, fy := cellCenter(col, m[0])
			tx, ty := cellCenter(col, m[1])
			g.moveAnims = append(g.moveAnims, MoveAnim{fx, fy, tx, ty, 0, 0, cell})
			// Clear the vacated canvas position.
			x0, y0 := col*cellSize, m[0]*cellSize
			g.canvas.SubImage(image.Rect(x0, y0, x0+cellSize, y0+cellSize)).(*ebiten.Image).Clear()
		}

		if respawnCells {
			// Create MoveAnims for new cells animating in from above screen.
			for i := 0; i < len(holeRows); i++ {
				cell := g.grid.Cells[col][i]
				fx := float32(col*cellSize + cellSize/2)
				fy := float32(-cellSize / 2)
				tx, ty := cellCenter(col, i)
				g.moveAnims = append(g.moveAnims, MoveAnim{fx, fy, tx, ty, 0, 0, cell})
				// The canvas position for row i was vacated by the moved cell above — already cleared.
			}
		}
	}
	return spawned
}

// updateWaiting handles hover recompute and click processing while in stateWaiting.
func (g *Game) updateWaiting() error {
	// Hover recompute.
	mx, my := ebiten.CursorPosition()
	hc := mx / cellSize
	hr := my / cellSize
	if hc >= 0 && hc < g.grid.Cols && hr >= 0 && hr < g.grid.Rows {
		if g.grid.Cells[hc][hr].Health > 0 {
			g.hoverCol, g.hoverRow = hc, hr
			g.victimHints = closestColorNeighbors(g.grid.Cells, hc, hr, g.grid.Cols, g.grid.Rows)
		}
	}

	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		// nothing to do
		return nil
	}
	// Click handling.
	px, py := ebiten.CursorPosition()
	col, row := cellFromPixel(px, py, cellSize)
	if col < 0 || col >= g.grid.Cols || row < 0 || row >= g.grid.Rows {
		return nil
	}
	cell := &g.grid.Cells[col][row]
	if cell.Health <= 0 {
		return nil
	}

	oldHealth := cell.Health
	cell.Health -= healthDecrement
	if cell.Health <= healthMin {
		cell.Health = 0
	}
	if cell.Health < healthEpsilon {
		cell.Health = 0.0
	}

	g.actionCol = col
	g.actionRow = row

	if cell.Health <= 0 {
		g.handleTargetDeath(col, row, cell)
	} else {
		if !g.handleTargetSurvival(col, row, cell, oldHealth) {
			return nil
		}
	}
	return nil
}

// victimPathParams computes the path parameters for a single victim moving from
// (fromX,fromY) to (toX,toY) and returns a partially-filled VictimAnim with
// pathType, ctrlX, ctrlY, and spiralTurns set.
func victimPathParams(fromX, fromY, toX, toY float32) (pathType int, ctrlX, ctrlY, spiralTurns float32) {
	pathType = rand.Intn(3)
	dx := toX - fromX
	dy := toY - fromY
	dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
	if pathType == 1 { // Bezier arc: control point perpendicular to the line
		midX := (fromX + toX) * 0.5
		midY := (fromY + toY) * 0.5
		var pX, pY float32
		if dist > 0 {
			pX = -dy / dist
			pY = dx / dist
		}
		sign := float32(1)
		if rand.Intn(2) == 0 {
			sign = -1
		}
		offset := sign * dist * (0.5 + rand.Float32()*1.0)
		ctrlX = midX + pX*offset
		ctrlY = midY + pY*offset
	}
	if pathType == 2 { // logarithmic spiral
		turns := float32(0.5 + rand.Float32()*1.0) // 0.5–1.5 turns
		if rand.Intn(2) == 0 {
			turns = -turns
		}
		spiralTurns = turns
	}
	return
}

// handleTargetDeath handles the death path after a cell's health has been
// drained to zero: redraws the dead cell, selects all tied-closest victims to
// animate into it (transitioning to stateVictimMoving), or falls straight to
// stateDropping when no victim exists.
func (g *Game) handleTargetDeath(col, row int, cell *Cell) {
	// Redraw immediately so cell disappears.
	g.drawCell(col, row, cell, cellSize, 1.0)
	g.cellCount-- // decrement for the clicked cell
	victimCoords := closestColorNeighbors(g.grid.Cells, col, row, g.grid.Cols, g.grid.Rows)
	if len(victimCoords) > 0 {
		toX, toY := cellCenter(col, row)
		g.victims = g.victims[:0]
		for _, vc := range victimCoords {
			ncol, nrow := vc[0], vc[1]
			fromX, fromY := cellCenter(ncol, nrow)
			pt, cx, cy, st := victimPathParams(fromX, fromY, toX, toY)
			v := VictimAnim{
				col:         ncol,
				row:         nrow,
				cell:        g.grid.Cells[ncol][nrow], // snapshot
				fromX:       fromX,
				fromY:       fromY,
				toX:         toX,
				toY:         toY,
				pathType:    pt,
				ctrlX:       cx,
				ctrlY:       cy,
				spiralTurns: st,
			}
			g.victims = append(g.victims, v)
			// Clear victim's canvas position (it is now an overlay animation).
			x0, y0 := ncol*cellSize, nrow*cellSize
			g.canvas.SubImage(image.Rect(x0, y0, x0+cellSize, y0+cellSize)).(*ebiten.Image).Clear()
		}
		g.animStart = ebiten.Tick()
		g.currentState = stateVictimMoving
	} else {
		// No victim — go straight to drop.
		spawned := g.buildDropAnims([]int{col})
		if respawnCells {
			g.cellCount += spawned
		}
		if len(g.moveAnims) > 0 {
			g.animStart = ebiten.Tick()
			g.currentState = stateDropping
		}
	}
}

// handleTargetSurvival handles the survival path after a cell has been damaged
// but not killed: shuffles alive neighbours and sets up the bounce animation,
// transitioning to stateShuffling. Returns false if there are no alive
// neighbours and the caller should return nil without a state change.
func (g *Game) handleTargetSurvival(col, row int, cell *Cell, oldHealth float32) bool {
	// SURVIVAL PATH — shuffle alive neighbours, then set up bounce animation.
	neighbors := eightNeighbors(col, row, g.grid.Cols, g.grid.Rows)
	// Only shuffle alive cells — empty slots stay put.
	var aliveNeighbors [][2]int
	for _, n := range neighbors {
		if g.grid.Cells[n[0]][n[1]].Health > 0 {
			aliveNeighbors = append(aliveNeighbors, n)
		}
	}
	if len(aliveNeighbors) == 0 {
		return false
	}

	// Set up bounce animation now that we know a shuffle will proceed.
	maxR := float32(cellSize/2 - 2)
	x0, y0 := col*cellSize, row*cellSize
	g.canvas.SubImage(image.Rect(x0, y0, x0+cellSize, y0+cellSize)).(*ebiten.Image).Clear()
	g.targetAnimating = true
	g.targetAnimStart = ebiten.Tick()
	g.targetAnimStartR = maxR * oldHealth
	g.targetAnimPeakR = maxR * oldHealth * 1.1
	g.targetAnimEndR = maxR * cell.Health
	g.targetAnimColor = g.grid.Cells[col][row].Color
	perm := rand.Perm(len(aliveNeighbors))

	// Collect neighbour cells before rearranging.
	type neighborCell struct {
		col, row int
		cell     Cell
	}
	orig := make([]neighborCell, len(aliveNeighbors))
	for i, n := range aliveNeighbors {
		orig[i] = neighborCell{n[0], n[1], g.grid.Cells[n[0]][n[1]]}
	}

	// Build MoveAnims: aliveNeighbor i moves to position perm[i].
	anims := make([]MoveAnim, len(aliveNeighbors))
	for i := range aliveNeighbors {
		src := orig[i]
		dst := aliveNeighbors[perm[i]]
		fx, fy := cellCenter(src.col, src.row)
		tx, ty := cellCenter(dst[0], dst[1])
		dx := tx - fx
		dy := ty - fy
		dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
		var ctrlX, ctrlY float32
		if dist > 0 {
			pX := -dy / dist
			pY := dx / dist
			sign := float32(1)
			if rand.Intn(2) == 0 {
				sign = -1
			}
			offset := sign * dist * (0.3 + rand.Float32()*0.7)
			ctrlX = (fx+tx)*0.5 + pX*offset
			ctrlY = (fy+ty)*0.5 + pY*offset
		} else {
			ctrlX = fx
			ctrlY = fy
		}
		anims[i] = MoveAnim{fx, fy, tx, ty, ctrlX, ctrlY, src.cell}
	}

	// Clear source positions on canvas.
	for _, n := range aliveNeighbors {
		g.canvas.SubImage(image.Rect(n[0]*cellSize, n[1]*cellSize, (n[0]+1)*cellSize, (n[1]+1)*cellSize)).(*ebiten.Image).Clear()
	}

	// Write cells to their permuted destinations in the grid.
	for i := range aliveNeighbors {
		dst := aliveNeighbors[perm[i]]
		g.grid.Cells[dst[0]][dst[1]] = orig[i].cell
	}

	g.moveAnims = anims
	g.currentState = stateShuffling
	g.animStart = ebiten.Tick()
	return true
}

// updateVictimMoving advances the victim movement animation and transitions to
// stateDropping (or stateWaiting) when the animation completes.
func (g *Game) updateVictimMoving() error {
	t := elapsedFraction(ebiten.Tick(), g.animStart, victimTicks)
	if t >= 1.0 {
		// All victims arrived — kill them and trigger drop.
		colSet := map[int]bool{g.actionCol: true}
		for _, v := range g.victims {
			g.grid.Cells[v.col][v.row].Health = 0
			colSet[v.col] = true
		}
		g.cellCount -= len(g.victims)
		affectedCols := make([]int, 0, len(colSet))
		for c := range colSet {
			affectedCols = append(affectedCols, c)
		}
		g.victims = g.victims[:0]
		spawned := g.buildDropAnims(affectedCols)
		if respawnCells {
			g.cellCount += spawned
		}
		if len(g.moveAnims) > 0 {
			g.animStart = ebiten.Tick()
			g.currentState = stateDropping
		} else {
			g.currentState = stateWaiting
		}
	}
	return nil
}

// updateShuffling waits for the shuffle animation to complete, then commits
// final cell positions and returns to stateWaiting.
func (g *Game) updateShuffling() error {
	elapsed := elapsedFraction(ebiten.Tick(), g.animStart, shuffleTicks)
	if elapsed >= 1.0 {
		for _, anim := range g.moveAnims {
			col := int(anim.ToX) / cellSize
			row := int(anim.ToY) / cellSize
			if col >= 0 && col < g.grid.Cols && row >= 0 && row < g.grid.Rows {
				g.drawCell(col, row, &g.grid.Cells[col][row], cellSize, 1.0)
			}
		}
		g.moveAnims = nil
		g.currentState = stateWaiting
	}
	return nil
}

// updateDropping waits for the drop animation to complete, then commits final
// cell positions and returns to stateWaiting.
func (g *Game) updateDropping() error {
	elapsed := elapsedFraction(ebiten.Tick(), g.animStart, dropTicks)
	if elapsed >= 1.0 {
		for _, anim := range g.moveAnims {
			col := int(anim.ToX) / cellSize
			row := int(anim.ToY) / cellSize
			if col >= 0 && col < g.grid.Cols && row >= 0 && row < g.grid.Rows {
				g.drawCell(col, row, &g.grid.Cells[col][row], cellSize, 1.0)
			}
		}
		g.moveAnims = nil
		g.currentState = stateWaiting
	}
	return nil
}

// Update handles input each tick.
func (g *Game) Update() error {
	ebiten.SetWindowTitle(fmt.Sprintf("go2d — cells: %d", g.cellCount))

	// Reset hover every tick; recompute below if conditions are met.
	g.hoverCol, g.hoverRow = -1, -1
	g.victimHints = g.victimHints[:0]

	if g.targetAnimating {
		if elapsedFraction(ebiten.Tick(), g.targetAnimStart, targetAnimTicks) >= 1.0 {
			g.targetAnimating = false
			g.drawCell(g.actionCol, g.actionRow, &g.grid.Cells[g.actionCol][g.actionRow], cellSize, 1.0)
		}
	}

	switch g.currentState {
	case stateWaiting:
		return g.updateWaiting()
	case stateVictimMoving:
		return g.updateVictimMoving()
	case stateShuffling:
		return g.updateShuffling()
	case stateDropping:
		return g.updateDropping()
	}
	return nil
}

// Draw blits the offscreen canvas to the screen, then composites any
// in-flight animation circles on top.
func (g *Game) Draw(screen *ebiten.Image) {
	if g.glowShader == nil {
		s, err := ebiten.NewShader([]byte(glowShaderSrc))
		if err != nil {
			log.Fatal("glow shader compile error:", err)
		}
		g.glowShader = s
	}
	screen.Fill(color.Black)

	g.drawVictimLine(screen)
	g.drawCanvas(screen)
	g.drawTargetBounce(screen)
	g.drawMoveAnims(screen)
}

// drawVictimLine draws animated dotted hint lines behind the circles, one per
// tied victim, each from that victim's cell toward the hovered cell.
func (g *Game) drawVictimLine(screen *ebiten.Image) {
	if g.hoverCol == -1 || len(g.victimHints) == 0 {
		return
	}
	toX, toY := cellCenter(g.hoverCol, g.hoverRow)
	const dotSpacing = float32(8) // px between dot centres
	const dotRadius = float32(4.0)
	// Offset advances at 20 px/sec = 20/60 px per tick
	rawOff := float64(ebiten.Tick()) * (20.0 / 60.0)
	offset := float32(math.Mod(rawOff, float64(dotSpacing)))
	for _, hint := range g.victimHints {
		hc, hr := hint[0], hint[1]
		fromX, fromY := cellCenter(hc, hr)
		dx := toX - fromX
		dy := toY - fromY
		length := float32(math.Sqrt(float64(dx*dx + dy*dy)))
		if length <= 0 {
			continue
		}
		ux := dx / length
		uy := dy / length
		vc := g.grid.Cells[hc][hr].Color
		dotCol := color.RGBA{vc.R, vc.G, vc.B, 255}
		for pos := offset; pos < length; pos += dotSpacing {
			vector.DrawFilledCircle(screen, fromX+ux*pos, fromY+uy*pos, dotRadius, dotCol, true)
		}
	}
}

// drawCanvas blits the persistent offscreen canvas to the screen.
func (g *Game) drawCanvas(screen *ebiten.Image) {
	screen.DrawImage(g.canvas, nil)
}

// drawTargetBounce renders the target cell bounce animation.
func (g *Game) drawTargetBounce(screen *ebiten.Image) {
	if !g.targetAnimating {
		return
	}
	t := elapsedFraction(ebiten.Tick(), g.targetAnimStart, targetAnimTicks)
	if t > 1.0 {
		t = 1.0
	}
	var r float32
	if t < 0.5 {
		lt := t * 2                // 0→1 in phase 1
		eased := 1 - (1-lt)*(1-lt) // ease-out: fast start, slow at peak
		r = g.targetAnimStartR + (g.targetAnimPeakR-g.targetAnimStartR)*eased
	} else {
		lt := (t - 0.5) * 2 // 0→1 in phase 2
		eased := lt * lt    // ease-in: slow at peak, fast at end
		r = g.targetAnimPeakR + (g.targetAnimEndR-g.targetAnimPeakR)*eased
	}
	cx, cy := cellCenter(g.actionCol, g.actionRow)
	if r > 0 {
		vector.DrawFilledCircle(screen, cx, cy, r, g.targetAnimColor, true)
	}
}

// drawMoveAnims renders in-motion cells for shuffle, drop, and victim-move animations.
func (g *Game) drawMoveAnims(screen *ebiten.Image) {
	if len(g.moveAnims) > 0 {
		var t float32
		ticks := int64(shuffleTicks)
		if g.currentState == stateDropping {
			ticks = dropTicks
		}
		t = elapsedFraction(ebiten.Tick(), g.animStart, ticks)
		if t > 1.0 {
			t = 1.0
		}
		maxR := float32(cellSize/2 - 2)
		for _, anim := range g.moveAnims {
			var cx, cy float32
			if g.currentState == stateShuffling {
				// Quadratic ease-in-out
				var et float32
				if t < 0.5 {
					et = 2 * t * t
				} else {
					et = 1 - 2*(1-t)*(1-t)
				}
				// Quadratic Bézier with eased t
				u := 1 - et
				cx = u*u*anim.FromX + 2*u*et*anim.CtrlX + et*et*anim.ToX
				cy = u*u*anim.FromY + 2*u*et*anim.CtrlY + et*et*anim.ToY
			} else {
				// Drop: quadratic ease-in (gravity), straight line
				posT := t * t
				cx = anim.FromX + posT*(anim.ToX-anim.FromX)
				cy = anim.FromY + posT*(anim.ToY-anim.FromY)
			}
			r := maxR * anim.Cell.Health
			if r > 0 {
				vector.DrawFilledCircle(screen, cx, cy, r, anim.Cell.Color, true)
			}
		}
	}

	if g.currentState == stateVictimMoving {
		t := elapsedFraction(ebiten.Tick(), g.animStart, victimTicks)
		if t > 1.0 {
			t = 1.0
		}
		for _, v := range g.victims {
			var cx, cy float32
			switch v.pathType {
			case 0: // straight line
				cx = v.fromX + t*(v.toX-v.fromX)
				cy = v.fromY + t*(v.toY-v.fromY)
			case 1: // quadratic Bezier arc
				u := float32(1) - t
				cx = u*u*v.fromX + 2*u*t*v.ctrlX + t*t*v.toX
				cy = u*u*v.fromY + 2*u*t*v.ctrlY + t*t*v.toY
			case 2: // inward spiral (radius shrinks, angle rotates)
				ddx := v.fromX - v.toX
				ddy := v.fromY - v.toY
				r0 := float32(math.Sqrt(float64(ddx*ddx + ddy*ddy)))
				theta0 := float32(math.Atan2(float64(ddy), float64(ddx)))
				r := r0 * (1 - t)
				theta := theta0 + v.spiralTurns*2*float32(math.Pi)*t
				cx = v.toX + r*float32(math.Cos(float64(theta)))
				cy = v.toY + r*float32(math.Sin(float64(theta)))
			}
			radius := float32(cellSize/2-2) * v.cell.Health * (1 - t)
			if radius > 0 {
				vector.DrawFilledCircle(screen, cx, cy, radius, v.cell.Color, true)
			}
		}
	}
}

// Layout returns the logical screen dimensions.
func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return windowWidth, windowHeight
}

func main() {
	ebiten.SetWindowSize(windowWidth, windowHeight)
	ebiten.SetWindowTitle("go2d grid")
	ebiten.SetWindowClosingHandled(false)

	grid := NewGrid(gridCols, gridRows)
	canvas := ebiten.NewImage(windowWidth, windowHeight)

	game := newGameState(grid)
	game.canvas = canvas
	game.animStart = ebiten.Tick()

	for c := 0; c < grid.Cols; c++ {
		for r := 0; r < grid.Rows; r++ {
			game.drawCell(c, r, &grid.Cells[c][r], cellSize, 1.0)
		}
	}

	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}

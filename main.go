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

	shuffleTicks = 18        // 0.3s at 60tps
	dropTicks    = 18        // 0.3s at 60tps
	victimTicks  = int64(24) // 0.4s at 60tps

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

// colorDistanceMax returns the max absolute difference across the R, G, B
// channels of two colours. Used for colour matching.
func colorDistanceMax(a, b color.RGBA) int {
	dr := int(a.R) - int(b.R)
	if dr < 0 {
		dr = -dr
	}
	dg := int(a.G) - int(b.G)
	if dg < 0 {
		dg = -dg
	}
	db := int(a.B) - int(b.B)
	if db < 0 {
		db = -db
	}
	if dr >= dg && dr >= db {
		return dr
	}
	if dg >= db {
		return dg
	}
	return db
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
// with Health > 0 that has minimum colorDistanceMax distance to
// cells[col][row].Color. Returns found=false if no healthy neighbour exists.
func closestColorNeighbor(cells [][]Cell, col, row, cols, rows int) (int, int, bool) {
	neighbors := eightNeighbors(col, row, cols, rows)
	target := cells[col][row].Color
	bestDist := -1
	bestCol, bestRow := 0, 0
	found := false
	for _, n := range neighbors {
		nc, nr := n[0], n[1]
		if cells[nc][nr].Health <= 0 {
			continue
		}
		dist := colorDistanceMax(target, cells[nc][nr].Color)
		if !found || dist < bestDist {
			bestDist = dist
			bestCol = nc
			bestRow = nr
			found = true
		}
	}
	return bestCol, bestRow, found
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
	hoverCol,      hoverRow      int // cell under cursor (-1 = none)
	victimHintCol, victimHintRow int // closest-colour neighbour of hovered cell (-1 = none)

	// victim movement animation fields
	victimFromX, victimFromY float32
	victimToX, victimToY     float32
	victimCell               Cell
	victimPathType           int     // 0=straight, 1=arc, 2=spiral
	victimCtrlX, victimCtrlY float32 // Bezier control point for arc
	victimSpiralTurns        float32 // signed: positive=CCW, negative=CW
	victimCol, victimRow     int     // victim's original grid position
}

// newGameState returns a Game with data fields initialised but no ebiten.Image
// allocated.  Callers that need a canvas (i.e. main) must set g.canvas after
// calling this.
func newGameState(grid *Grid) *Game {
	return &Game{
		grid:          grid,
		currentState:  stateWaiting,
		animStart:     0,
		actionCol:     -1,
		actionRow:     -1,
		cellCount:     grid.Cols * grid.Rows,
		hoverCol:      -1,
		hoverRow:      -1,
		victimHintCol: -1,
		victimHintRow: -1,
		victimCol:     -1,
		victimRow:     -1,
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

// Update handles input each tick.
func (g *Game) Update() error {
	ebiten.SetWindowTitle(fmt.Sprintf("go2d — cells: %d", g.cellCount))

	// Reset hover every tick; recompute below if conditions are met.
	g.hoverCol, g.hoverRow = -1, -1
	g.victimHintCol, g.victimHintRow = -1, -1

	if g.currentState == stateWaiting {
		mx, my := ebiten.CursorPosition()
		hc := mx / cellSize
		hr := my / cellSize
		if hc >= 0 && hc < g.grid.Cols && hr >= 0 && hr < g.grid.Rows {
			if g.grid.Cells[hc][hr].Health > 0 {
				g.hoverCol, g.hoverRow = hc, hr
				nc, nr, found := closestColorNeighbor(g.grid.Cells, hc, hr, g.grid.Cols, g.grid.Rows)
				if found {
					g.victimHintCol, g.victimHintRow = nc, nr
				}
			}
		}
	}

	if g.currentState == stateWaiting {
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			px, py := ebiten.CursorPosition()
			col, row := cellFromPixel(px, py, cellSize)
			if col < 0 || col >= g.grid.Cols || row < 0 || row >= g.grid.Rows {
				return nil
			}
			cell := &g.grid.Cells[col][row]
			if cell.Health <= 0 {
				return nil
			}

			cell.Health -= healthDecrement
			if cell.Health <= healthMin {
				cell.Health = 0
			}
			if cell.Health < healthEpsilon {
				cell.Health = 0.0
			}

			// Redraw the clicked cell immediately on the canvas.
			g.drawCell(col, row, cell, cellSize, 1.0)
			g.actionCol = col
			g.actionRow = row

			if cell.Health <= 0 {
				// DEATH PATH
				g.cellCount-- // decrement for the clicked cell
				ncol, nrow, found := closestColorNeighbor(g.grid.Cells, col, row, g.grid.Cols, g.grid.Rows)
				if found {
					// Set up victim animation — do NOT kill victim or call buildDropAnims yet.
					g.victimCol, g.victimRow = ncol, nrow
					g.victimCell = g.grid.Cells[ncol][nrow] // snapshot before any change
					g.victimFromX, g.victimFromY = cellCenter(ncol, nrow)
					g.victimToX, g.victimToY = cellCenter(col, row)

					// Choose random path type.
					g.victimPathType = rand.Intn(3)
					dx := g.victimToX - g.victimFromX
					dy := g.victimToY - g.victimFromY
					dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))

					if g.victimPathType == 1 { // Bezier arc: control point perpendicular to the line
						midX := (g.victimFromX + g.victimToX) * 0.5
						midY := (g.victimFromY + g.victimToY) * 0.5
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
						g.victimCtrlX = midX + pX*offset
						g.victimCtrlY = midY + pY*offset
					}
					if g.victimPathType == 2 { // logarithmic spiral
						turns := float32(0.5 + rand.Float32()*1.0) // 0.5–1.5 turns
						if rand.Intn(2) == 0 {
							turns = -turns
						}
						g.victimSpiralTurns = turns
					}

					// Clear victim's canvas position (it is now an overlay animation).
					x0, y0 := ncol*cellSize, nrow*cellSize
					g.canvas.SubImage(image.Rect(x0, y0, x0+cellSize, y0+cellSize)).(*ebiten.Image).Clear()

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
			} else {
				// SURVIVAL PATH — shuffle alive neighbours only
				neighbors := eightNeighbors(col, row, g.grid.Cols, g.grid.Rows)
				// Only shuffle alive cells — empty slots stay put.
				var aliveNeighbors [][2]int
				for _, n := range neighbors {
					if g.grid.Cells[n[0]][n[1]].Health > 0 {
						aliveNeighbors = append(aliveNeighbors, n)
					}
				}
				if len(aliveNeighbors) == 0 {
					return nil
				}
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
			}
		}
		return nil
	}

	if g.currentState == stateVictimMoving {
		t := elapsedFraction(ebiten.Tick(), g.animStart, victimTicks)
		if t >= 1.0 {
			// Victim arrived — kill it and trigger drop.
			g.grid.Cells[g.victimCol][g.victimRow].Health = 0
			g.cellCount--
			affectedCols := []int{g.actionCol}
			if g.victimCol != g.actionCol {
				affectedCols = append(affectedCols, g.victimCol)
			}
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

	if g.currentState == stateShuffling {
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

	if g.currentState == stateDropping {
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

	// Victim hint line — drawn behind circles
	if g.hoverCol != -1 && g.victimHintCol != -1 {
		fromX, fromY := cellCenter(g.victimHintCol, g.victimHintRow)
		toX, toY := cellCenter(g.hoverCol, g.hoverRow)
		dx := toX - fromX
		dy := toY - fromY
		length := float32(math.Sqrt(float64(dx*dx + dy*dy)))
		if length > 0 {
			ux := dx / length
			uy := dy / length
			const dotSpacing = float32(12) // px between dot centres
			const dotRadius = float32(1.5)
			// Offset advances at 10 px/sec = 10/60 px per tick
			rawOff := float64(ebiten.Tick()) * (10.0 / 60.0)
			offset := float32(math.Mod(rawOff, float64(dotSpacing)))
			dotCol := color.RGBA{255, 255, 255, 160}
			for pos := offset; pos < length; pos += dotSpacing {
				vector.DrawFilledCircle(screen, fromX+ux*pos, fromY+uy*pos, dotRadius, dotCol, true)
			}
		}
	}

	screen.DrawImage(g.canvas, nil)

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

		var cx, cy float32
		switch g.victimPathType {
		case 0: // straight line
			cx = g.victimFromX + t*(g.victimToX-g.victimFromX)
			cy = g.victimFromY + t*(g.victimToY-g.victimFromY)
		case 1: // quadratic Bezier arc
			u := float32(1) - t
			cx = u*u*g.victimFromX + 2*u*t*g.victimCtrlX + t*t*g.victimToX
			cy = u*u*g.victimFromY + 2*u*t*g.victimCtrlY + t*t*g.victimToY
		case 2: // inward spiral (radius shrinks, angle rotates)
			ddx := g.victimFromX - g.victimToX
			ddy := g.victimFromY - g.victimToY
			r0 := float32(math.Sqrt(float64(ddx*ddx + ddy*ddy)))
			theta0 := float32(math.Atan2(float64(ddy), float64(ddx)))
			r := r0 * (1 - t)
			theta := theta0 + g.victimSpiralTurns*2*float32(math.Pi)*t
			cx = g.victimToX + r*float32(math.Cos(float64(theta)))
			cy = g.victimToY + r*float32(math.Sin(float64(theta)))
		}

		radius := float32(cellSize/2-2) * g.victimCell.Health * (1 - t)
		if radius > 0 {
			vector.DrawFilledCircle(screen, cx, cy, radius, g.victimCell.Color, true)
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

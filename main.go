package main

import (
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

	stateWaiting = 1
	stateCollapsing = 2
	stateSpawning = 3

	collapseTicks = int64(5)
	spawnTicks = int64(10)

	healthDecrement = float32(0.2)
	healthMin       = float32(0.2)
)


// randomColor returns a random opaque colour (A=255).
// will be a subset of web-safe colours
func randomColor() color.RGBA {
    for {
        col := color.RGBA{
            R: uint8(51*(rand.Intn(4)+2)),
            G: uint8(51*(rand.Intn(4)+2)),
            B: uint8(51*(rand.Intn(4)+2)),
            A: 255,
        }
        if col.R >= 102 || col.G >= 102 || col.B >= 102 {
            return col
        }
        // go again if color is too dark (yeah i know this is not optimal)
    }
}

// recolor returns a random opaque colour guaranteed to differ from current.
func recolor(cell *Cell) {
	for {
		c := randomColor()
		if cell.Color != c {
			cell.Color = c
			break
		}
	}
}

// cellFromPixel converts pixel coordinates to (col, row) grid indices.
// No bounds clamping is applied.
func cellFromPixel(px, py, cellSize int) (int, int) {
	return px / cellSize, py / cellSize
}

// nearestHealthyCell returns the (col, row) of the nearest cell with Health > 0
// to the given cursor pixel position.  Priority is given to the cell the cursor
// is currently inside; if that cell is healthy the function returns it
// immediately.  Otherwise it scans all cells and returns the one whose centre
// is closest to the cursor using Euclidean distance.  If no healthy cells
// exist, found is false.
func nearestHealthyCell(cells [][]Cell, cursorX, cursorY, cellSize int) (col, row int, found bool) {
	cols := len(cells)
	if cols == 0 {
		return 0, 0, false
	}
	rows := len(cells[0])
	if rows == 0 {
		return 0, 0, false
	}

	// Priority: cell under cursor, if healthy.
	underCol := cursorX / cellSize
	underRow := cursorY / cellSize
	if underCol >= 0 && underCol < cols && underRow >= 0 && underRow < rows {
		if cells[underCol][underRow].Health > 0 {
			return underCol, underRow, true
		}
	}

	// Euclidean scan over all cells.
	bestDist := math.MaxFloat64
	bestCol, bestRow := -1, -1
	half := float64(cellSize) / 2.0
	cx := float64(cursorX)
	cy := float64(cursorY)
	for c := 0; c < cols; c++ {
		for r := 0; r < rows; r++ {
			if cells[c][r].Health <= 0 {
				continue
			}
			cellCX := float64(c)*float64(cellSize) + half
			cellCY := float64(r)*float64(cellSize) + half
			dx := cx - cellCX
			dy := cy - cellCY
			dist := dx*dx + dy*dy
			if dist < bestDist {
				bestDist = dist
				bestCol = c
				bestRow = r
			}
		}
	}
	if bestCol == -1 {
		return 0, 0, false
	}
	return bestCol, bestRow, true
}

type Cell struct {
    Color  color.RGBA
    Health float32
}

// Grid holds the grid dimensions and per-cell colours.
type Grid struct {
	Cols   int
	Rows   int
	Cells [][]Cell
}

// NewGrid initialises a Grid with random opaque colours in every cell.
func NewGrid(cols, rows int) *Grid {
	cells := make([][]Cell, cols)
	for c := 0; c < cols; c++ {
		cells[c] = make([]Cell, rows)
		for r := 0; r < rows; r++ {
			cells[c][r] = Cell {
			    Color: randomColor(),
			    Health: 1.0,
            }
		}
	}
	return &Grid{
		Cols:   cols,
		Rows:   rows,
		Cells: cells,
	}
}

// Game is the Ebitengine game state.
type Game struct {
	grid   *Grid
	canvas *ebiten.Image
    currentState int     // which state are we in?
    startTick int64 // when did the current state start?
    actionRow int // the row any given action is happening
    actionCol int // the column an action is happening
	highlightCol int // col of currently highlighted cell; -1 = none
	highlightRow int // row of currently highlighted cell; -1 = none
}

// newGameState returns a Game with data fields initialised but no ebiten.Image
// allocated.  Callers that need a canvas (i.e. main) must set g.canvas after
// calling this.
func newGameState(grid *Grid) *Game {
	return &Game{
		grid:         grid,
		currentState: stateWaiting,
		startTick:    0,
		actionCol:    -1,
		actionRow:    -1,
		highlightCol: -1,
		highlightRow: -1,
	}
}

// elapsedFraction returns how far through a phase we are as a 0→1 float32.
// currentTick is the game tick at the time of evaluation, startTick is when
// the phase began, and durationTicks is the total length of the phase.
func elapsedFraction(currentTick, startTick, durationTicks int64) float32 {
	return float32(currentTick-startTick) / float32(durationTicks)
}

// drawCell erases the cell's bounding rectangle on canvas with black and
// redraws the antialiased circle for (col, row) using clr.  When highlighted
// is true, a white 1.5px antialiased border is drawn at the natural (full)
// circle edge regardless of current cellFullness or Health.
func drawCell(canvas *ebiten.Image, col, row int, cell *Cell, cs int, cellFullness float32, highlighted bool) {
	x := float32(col * cs)
	y := float32(row * cs)
	csFloat := float32(cs)
	vector.DrawFilledRect(canvas, x, y, csFloat, csFloat, color.Black, false)
	halfCs := csFloat / 2
	cx := x + halfCs
	cy := y + halfCs
	maxRadius := (halfCs - 2)
	radius := maxRadius * cellFullness * cell.Health
	clr := cell.Color
	vector.DrawFilledCircle(canvas, cx, cy, radius, clr, true)
	if highlighted {
		vector.StrokeCircle(canvas, cx, cy, maxRadius, 1.5, color.White, true)
	}
}

// advanceState advances the state machine for a given tick value.  It does not
// call ebiten.Tick() — the caller supplies the current tick so this function
// is pure and unit-testable.  drawCell calls are skipped when g.canvas is nil.
func (g *Game) advanceState(tick int64) {
	if g.currentState == stateCollapsing {
		elapsed := elapsedFraction(tick, g.startTick, collapseTicks)
		actionCell := &g.grid.Cells[g.actionCol][g.actionRow]
		if g.canvas != nil {
			drawCell(g.canvas, g.actionCol, g.actionRow, actionCell, cellSize, 1.0-elapsed, false)
		}
		if elapsed >= 1.0 {
			if actionCell.Health > healthMin {
				actionCell.Health -= healthDecrement
			} else {
				actionCell.Health = 0.0
			}
			g.currentState = stateSpawning
			g.startTick = tick
		}
	} else if g.currentState == stateSpawning {
		elapsed := elapsedFraction(tick, g.startTick, spawnTicks)
		if g.canvas != nil {
			drawCell(g.canvas, g.actionCol, g.actionRow, &g.grid.Cells[g.actionCol][g.actionRow], cellSize, elapsed, false)
		}
		if elapsed >= 1.0 {
			g.currentState = stateWaiting
			g.startTick = tick
		}
	}
}

// clearHighlight removes the white highlight border from the currently
// highlighted cell (if any) and resets highlightCol/Row to -1.
func clearHighlight(g *Game) {
	if g.highlightCol == -1 {
		return
	}
	if g.canvas != nil {
		c := g.highlightCol
		r := g.highlightRow
		drawCell(g.canvas, c, r, &g.grid.Cells[c][r], cellSize, 1.0, false)
	}
	g.highlightCol = -1
	g.highlightRow = -1
}

// updateHighlight evaluates the nearest healthy cell to the cursor and redraws
// only the cells that have changed highlight state.  It is called every tick
// while in stateWaiting.
func updateHighlight(g *Game) {
	px, py := ebiten.CursorPosition()
	if px < 0 || px >= windowWidth || py < 0 || py >= windowHeight {
		clearHighlight(g)
		return
	}
	newCol, newRow, found := nearestHealthyCell(g.grid.Cells, px, py, cellSize)
	if found && newCol == g.highlightCol && newRow == g.highlightRow {
		// No change — nothing to redraw.
		return
	}
	// Erase old highlight if present.
	if g.highlightCol != -1 {
		c := g.highlightCol
		r := g.highlightRow
		if g.canvas != nil {
			drawCell(g.canvas, c, r, &g.grid.Cells[c][r], cellSize, 1.0, false)
		}
	}
	if found {
		if g.canvas != nil {
			drawCell(g.canvas, newCol, newRow, &g.grid.Cells[newCol][newRow], cellSize, 1.0, true)
		}
		g.highlightCol = newCol
		g.highlightRow = newRow
	} else {
		g.highlightCol = -1
		g.highlightRow = -1
	}
}

// Update handles input each tick.
func (g *Game) Update() error {
    if g.currentState == stateWaiting {
        updateHighlight(g)
        if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
            px, py := ebiten.CursorPosition()
            col, row := cellFromPixel(px, py, cellSize)
            if col >= 0 && col < g.grid.Cols && row >= 0 && row < g.grid.Rows {
                recolor(&g.grid.Cells[col][row])
                drawCell(g.canvas, col, row, &g.grid.Cells[col][row], cellSize, 1.0, false)
            }
            clearHighlight(g)
            g.currentState = stateCollapsing
            g.actionCol = col
            g.actionRow = row
            g.startTick = ebiten.Tick()
        }
    } else {
        g.advanceState(ebiten.Tick())
        // After stateSpawning completes, the previously highlighted cell may
        // now be dead; re-evaluate the highlight on the next waiting tick.
        // (highlightCol/Row are already -1 since clearHighlight ran on click.)
    }
	return nil
}

// Draw blits the offscreen canvas to the screen in a single call.
func (g *Game) Draw(screen *ebiten.Image) {
	screen.DrawImage(g.canvas, nil)
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
	canvas.Fill(color.Black)
	for c := 0; c < grid.Cols; c++ {
		for r := 0; r < grid.Rows; r++ {
			drawCell(canvas, c, r, &grid.Cells[c][r], cellSize, 1.0, false)
		}
	}

	game := newGameState(grid)
	game.canvas = canvas
	game.startTick = ebiten.Tick()
	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}

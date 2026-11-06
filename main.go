package main

import (
	"image/color"
	"log"
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
	}
}

// elapsedFraction returns how far through a phase we are as a 0→1 float32.
// currentTick is the game tick at the time of evaluation, startTick is when
// the phase began, and durationTicks is the total length of the phase.
func elapsedFraction(currentTick, startTick, durationTicks int64) float32 {
	return float32(currentTick-startTick) / float32(durationTicks)
}

// drawCell erases the cell's bounding rectangle on canvas with black and
// redraws the antialiased circle for (col, row) using clr.
func drawCell(canvas *ebiten.Image, col, row int, cell *Cell, cs int, cellFullness float32) {
	x := float32(col * cs)
	y := float32(row * cs)
	csFloat := float32(cs)
	vector.DrawFilledRect(canvas, x, y, csFloat, csFloat, color.Black, false)
	halfCs := csFloat /2
	cx := x + halfCs
	cy := y + halfCs
	maxRadius := (halfCs -2)
	radius := maxRadius * cellFullness * cell.Health
	clr := cell.Color
	vector.DrawFilledCircle(canvas, cx, cy, radius, clr, true)
}

// advanceState advances the state machine for a given tick value.  It does not
// call ebiten.Tick() — the caller supplies the current tick so this function
// is pure and unit-testable.  drawCell calls are skipped when g.canvas is nil.
func (g *Game) advanceState(tick int64) {
	if g.currentState == stateCollapsing {
		elapsed := elapsedFraction(tick, g.startTick, collapseTicks)
		actionCell := &g.grid.Cells[g.actionCol][g.actionRow]
		if g.canvas != nil {
			drawCell(g.canvas, g.actionCol, g.actionRow, actionCell, cellSize, 1.0-elapsed)
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
			drawCell(g.canvas, g.actionCol, g.actionRow, &g.grid.Cells[g.actionCol][g.actionRow], cellSize, elapsed)
		}
		if elapsed >= 1.0 {
			g.currentState = stateWaiting
			g.startTick = tick
		}
	}
}

// Update handles input each tick.
func (g *Game) Update() error {
    if g.currentState == stateWaiting {
        if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
            px, py := ebiten.CursorPosition()
            col, row := cellFromPixel(px, py, cellSize)
            if col >= 0 && col < g.grid.Cols && row >= 0 && row < g.grid.Rows {
                recolor(&g.grid.Cells[col][row])
                drawCell(g.canvas, col, row, &g.grid.Cells[col][row], cellSize, 1.0)
            }
            g.currentState = stateCollapsing
            g.actionCol = col
            g.actionRow = row
            g.startTick = ebiten.Tick()
        }
    } else {
        g.advanceState(ebiten.Tick())
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
			drawCell(canvas, c, r, &grid.Cells[c][r], cellSize, 1.0)
		}
	}

	game := newGameState(grid)
	game.canvas = canvas
	game.startTick = ebiten.Tick()
	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}

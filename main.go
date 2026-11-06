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

	stateWaiting    = 1
	stateCollapsing = 2
	stateSpawning   = 3

	collapseTicks = int64(5)
	spawnTicks    = int64(10)

	healthDecrement = float32(0.2)
	healthMin       = float32(0.2)
	healthEpsilon   = float32(0.001)

	glowWidth = float32(6)
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
    if alpha <= 0 {
        return vec4(0, 0, 0, 0)
    }
    return vec4(alpha, alpha, alpha, alpha)
}
`

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
	currentState int   // which state are we in?
	startTick    int64 // when did the current state start?
	actionRow    int   // the row any given action is happening
	actionCol    int   // the column an action is happening
	highlightCol int   // col of currently highlighted cell; -1 = none
	highlightRow int   // row of currently highlighted cell; -1 = none
	glowShader   *ebiten.Shader
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
// redraws the antialiased circle for (col, row).  No glow is drawn here;
// glow is composited onto the screen each frame in Draw().
func (g *Game) drawCell(col, row int, cell *Cell, cs int, cellFullness float32) {
	csFloat := float32(cs)

	// Clear exactly the cell rect — no expansion.
	vector.DrawFilledRect(g.canvas, float32(col*cs), float32(row*cs), csFloat, csFloat, color.Black, false)

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

// advanceState advances the state machine for a given tick value.  It does not
// call ebiten.Tick() — the caller supplies the current tick so this function
// is pure and unit-testable.  drawCell calls are skipped when g.canvas is nil.
func (g *Game) advanceState(tick int64) {
	if g.currentState == stateCollapsing {
		elapsed := elapsedFraction(tick, g.startTick, collapseTicks)
		actionCell := &g.grid.Cells[g.actionCol][g.actionRow]
		if g.canvas != nil {
			g.drawCell(g.actionCol, g.actionRow, actionCell, cellSize, 1.0-elapsed)
		}
		if elapsed >= 1.0 {
			if actionCell.Health > healthMin {
				actionCell.Health -= healthDecrement
			} else {
				actionCell.Health = 0.0
			}
            if actionCell.Health < healthEpsilon {
                actionCell.Health = 0.0
			    g.currentState = stateWaiting
            } else {
			    g.currentState = stateSpawning
            }
			g.startTick = tick
		}
	} else if g.currentState == stateSpawning {
		elapsed := elapsedFraction(tick, g.startTick, spawnTicks)
		if g.canvas != nil {
			g.drawCell(g.actionCol, g.actionRow, &g.grid.Cells[g.actionCol][g.actionRow], cellSize, elapsed)
		}
		if elapsed >= 1.0 {
			g.currentState = stateWaiting
			g.startTick = tick
		}
	}
}

// clearHighlight resets highlightCol/Row to -1 so no glow is drawn this frame.
func clearHighlight(g *Game) {
	if g.highlightCol == -1 {
		return
	}
	g.highlightCol = -1
	g.highlightRow = -1
}

// updateHighlight evaluates the nearest healthy cell to the cursor and updates
// highlightCol/Row accordingly.  Glow rendering happens in Draw(), not here.
// It is called every tick while in stateWaiting.
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
	// Update tracked highlight cell; glow is drawn each frame in Draw().
	if found {
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
				g.drawCell(col, row, &g.grid.Cells[col][row], cellSize, 1.0)
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

// Draw blits the offscreen canvas to the screen, then composites the glow
// ring for the highlighted cell directly onto the screen each frame.
// On the first call it also compiles the glow shader (ebiten must be
// initialised before NewShader can be called).
func (g *Game) Draw(screen *ebiten.Image) {
	if g.glowShader == nil {
		s, err := ebiten.NewShader([]byte(glowShaderSrc))
		if err != nil {
			log.Fatal("glow shader compile error:", err)
		}
		g.glowShader = s
	}
	screen.DrawImage(g.canvas, nil)

	if g.highlightCol != -1 && g.glowShader != nil {
		col := g.highlightCol
		row := g.highlightRow
		cell := &g.grid.Cells[col][row]
		if cell.Health > 0 {
			csFloat := float32(cellSize)
			halfCs := csFloat / 2
			cx := float32(col*cellSize) + halfCs
			cy := float32(row*cellSize) + halfCs
			maxRadius := halfCs - 2
			radius := maxRadius * cell.Health
			outerR := radius + glowWidth
			x0 := cx - outerR
			y0 := cy - outerR
			w := int(outerR * 2)
			h := int(outerR * 2)
			opts := &ebiten.DrawRectShaderOptions{}
			opts.GeoM.Translate(float64(x0), float64(y0))
			opts.Uniforms = map[string]any{
				"Center":      []float32{cx, cy},
				"InnerRadius": float32(radius),
				"OuterRadius": float32(outerR),
			}
			opts.Blend = ebiten.BlendSourceOver
			screen.DrawRectShader(w, h, g.glowShader, opts)
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
	canvas.Fill(color.Black)

	game := newGameState(grid)
	game.canvas = canvas
	game.startTick = ebiten.Tick()

	for c := 0; c < grid.Cols; c++ {
		for r := 0; r < grid.Rows; r++ {
			game.drawCell(c, r, &grid.Cells[c][r], cellSize, 1.0)
		}
	}

	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}

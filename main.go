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
	cellSize     = 32
	gridCols     = windowWidth / cellSize  // 25
	gridRows     = windowHeight / cellSize // 18
)

// randomColor returns a random opaque colour (A=255).
func randomColor() color.RGBA {
	return color.RGBA{
		R: uint8(rand.Intn(256)),
		G: uint8(rand.Intn(256)),
		B: uint8(rand.Intn(256)),
		A: 255,
	}
}

// recolor returns a random opaque colour guaranteed to differ from current.
func recolor(current color.RGBA) color.RGBA {
	for {
		c := randomColor()
		if c != current {
			return c
		}
	}
}

// cellFromPixel converts pixel coordinates to (col, row) grid indices.
// No bounds clamping is applied.
func cellFromPixel(px, py, cellSize int) (int, int) {
	return px / cellSize, py / cellSize
}

// Grid holds the grid dimensions and per-cell colours.
type Grid struct {
	Cols   int
	Rows   int
	Colors [][]color.RGBA
}

// NewGrid initialises a Grid with random opaque colours in every cell.
func NewGrid(cols, rows int) *Grid {
	colors := make([][]color.RGBA, cols)
	for c := 0; c < cols; c++ {
		colors[c] = make([]color.RGBA, rows)
		for r := 0; r < rows; r++ {
			colors[c][r] = randomColor()
		}
	}
	return &Grid{
		Cols:   cols,
		Rows:   rows,
		Colors: colors,
	}
}

// Game is the Ebitengine game state.
type Game struct {
	grid   *Grid
	canvas *ebiten.Image
}

// drawCell erases the cell's bounding rectangle on canvas with black and
// redraws the antialiased circle for (col, row) using clr.
func drawCell(canvas *ebiten.Image, col, row int, clr color.RGBA, cs int) {
	x := float32(col * cs)
	y := float32(row * cs)
	vector.DrawFilledRect(canvas, x, y, float32(cs), float32(cs), color.Black, false)
	cx := x + float32(cs)/2
	cy := y + float32(cs)/2
	radius := float32(cs)/2 - 2
	vector.DrawFilledCircle(canvas, cx, cy, radius, clr, true)
}

// Update handles input each tick.
func (g *Game) Update() error {
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		px, py := ebiten.CursorPosition()
		col, row := cellFromPixel(px, py, cellSize)
		if col >= 0 && col < g.grid.Cols && row >= 0 && row < g.grid.Rows {
			g.grid.Colors[col][row] = recolor(g.grid.Colors[col][row])
			drawCell(g.canvas, col, row, g.grid.Colors[col][row], cellSize)
		}
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
			drawCell(canvas, c, r, grid.Colors[c][r], cellSize)
		}
	}

	game := &Game{
		grid:   grid,
		canvas: canvas,
	}
	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}

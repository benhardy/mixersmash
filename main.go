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
	grid *Grid
}

// Update handles input each tick.
func (g *Game) Update() error {
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		px, py := ebiten.CursorPosition()
		col, row := cellFromPixel(px, py, cellSize)
		if col >= 0 && col < g.grid.Cols && row >= 0 && row < g.grid.Rows {
			g.grid.Colors[col][row] = recolor(g.grid.Colors[col][row])
		}
	}
	return nil
}

// Draw renders the black background and all filled circles.
func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.Black)
	for c := 0; c < g.grid.Cols; c++ {
		for r := 0; r < g.grid.Rows; r++ {
			cx := float32(c*cellSize) + float32(cellSize)/2
			cy := float32(r*cellSize) + float32(cellSize)/2
			radius := float32(cellSize) / 2
			vector.DrawFilledCircle(screen, cx, cy, radius, g.grid.Colors[c][r], true)
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

	game := &Game{
		grid: NewGrid(gridCols, gridRows),
	}
	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}

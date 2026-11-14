package main

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// deviceScale returns the current device scale factor using the non-deprecated
// Monitor API (ebiten v2.6+).  Falls back to 1.0 when no monitor is available.
func deviceScale() float64 {
	m := ebiten.Monitor()
	if m == nil {
		return 1.0
	}
	s := m.DeviceScaleFactor()
	if s <= 0 {
		return 1.0
	}
	return s
}

// deviceCanvasDims returns the device-pixel canvas width and height for the
// given logical window size and scale factor, rounded to the nearest integer.
func deviceCanvasDims(s float64) (int, int) {
	w := int(math.Round(float64(windowWidth) * s))
	h := int(math.Round(float64(windowHeight) * s))
	return w, h
}

// deviceCellSizeFor derives the device-pixel cell size from the rounded canvas
// dimensions, dividing by the grid dimensions so tiles cover exactly.
func deviceCellSizeFor(canvasW int) int {
	dcs := canvasW / gridCols
	if dcs < 1 {
		dcs = 1
	}
	return dcs
}

// spriteRefSizeFor returns the device-pixel bake diameter for creature sprites.
// Capped at 512 to avoid excessive GPU memory on very high-DPI displays.
func spriteRefSizeFor(dcs int) int {
	sz := int(math.Ceil(float64(dcs) * 2))
	if sz > 512 {
		sz = 512
	}
	if sz < 2 {
		sz = 2
	}
	return sz
}

// LayoutF implements ebiten.LayoutFer, causing ebiten to call this instead of
// Layout.  It returns the device-pixel screen size so that Draw receives a
// device-sized *ebiten.Image.  When the scale factor changes (e.g. window moved
// to a different monitor, or browser zoom), it reallocates the canvas and clears
// the sprite cache so all cached sprites are rebaked at the new size.
func (g *Game) LayoutF(outsideWidth, outsideHeight float64) (float64, float64) {
	s := deviceScale()
	dw, dh := deviceCanvasDims(s)
	if s != g.renderScale {
		g.renderScale = s
		g.deviceCellSize = deviceCellSizeFor(dw)
		g.spriteRefSize = spriteRefSizeFor(g.deviceCellSize)
		g.canvas = ebiten.NewImage(dw, dh)
		// Clear sprite cache — sprites were baked at the old size.
		g.glyphCache = nil
		// Repaint all live cells into the new device-sized canvas.
		for c := 0; c < g.grid.Cols; c++ {
			for r := 0; r < g.grid.Rows; r++ {
				g.drawCell(c, r, 1.0)
			}
		}
	}
	return float64(dw), float64(dh)
}

// Layout satisfies the ebiten.Game interface.  When LayoutF is implemented,
// ebiten never calls Layout, but the method is required by the interface.
func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return windowWidth, windowHeight
}

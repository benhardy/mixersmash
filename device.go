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
	dcs = max(dcs, 1)
	return dcs
}

// spriteRefSizeFor returns the device-pixel bake diameter for creature sprites.
// Capped at 512 to avoid excessive GPU memory on very high-DPI displays.
func spriteRefSizeFor(dcs int) int {
	sz := int(math.Ceil(float64(dcs) * 2))
	sz = min(sz, 512)
	sz = max(sz, 2)
	return sz
}

// LayoutF implements ebiten.LayoutFer, causing ebiten to call this instead of
// Layout.  It returns the actual device-pixel screen dimensions so that Draw
// receives a *ebiten.Image matching the real framebuffer (in whatever orientation).
//
// The landscape design frame (frameW × frameH = round(800*s) × round(600*s)) is
// allocated here and kept at the landscape size regardless of screen orientation.
// deviceCellSize is derived from the frame width, not the screen width, so board
// coordinates are stable across orientation changes.
//
// When the scale factor changes (e.g. window moved to a different monitor, or
// browser zoom), the frame and canvas are reallocated and the sprite cache is
// cleared so all cached sprites are rebaked at the new size.
func (g *Game) LayoutF(outsideWidth, outsideHeight float64) (float64, float64) {
	s := deviceScale()
	// screenW/screenH = actual device-pixel screen in current orientation.
	sw := int(math.Round(outsideWidth * s))
	sh := int(math.Round(outsideHeight * s))
	if sw < 1 {
		sw = 1
	}
	if sh < 1 {
		sh = 1
	}
	g.screenW = sw
	g.screenH = sh
	g.portrait = sh > sw

	// Fixed-design landscape frame: always windowWidth × windowHeight at scale s.
	fw, fh := deviceCanvasDims(s) // round(800*s), round(600*s)

	if s != g.renderScale {
		g.renderScale = s
		// deviceCellSize is derived from the frame width, not the screen width.
		g.deviceCellSize = deviceCellSizeFor(fw)
		g.spriteRefSize = spriteRefSizeFor(g.deviceCellSize)
		g.frame = ebiten.NewImage(fw, fh)
		g.canvas = ebiten.NewImage(fw, fh)
		// Invalidate per-creature sprite cache — sprites were baked at the old size.
		for i := range creaturePalette {
			creaturePalette[i].Sprite = nil
		}
		// Repaint all live cells into the new device-sized canvas.
		for c := 0; c < g.grid.Cols; c++ {
			for r := 0; r < g.grid.Rows; r++ {
				g.drawCell(c, r, 1.0)
			}
		}
	}
	return float64(sw), float64(sh)
}

// Layout satisfies the ebiten.Game interface.  When LayoutF is implemented,
// ebiten never calls Layout, but the method is required by the interface.
func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return windowWidth, windowHeight
}

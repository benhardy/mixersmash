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

// frameScaleFor computes the presented fit-scale k (device pixels per logical
// unit) for the landscape design placed into a screen of size sw×sh.
//
// The design is designW×designH logical pixels (hudWidth+windowWidth by
// windowHeight).  In landscape orientation the design is fit directly into the
// screen; in portrait the design is rotated 90° CCW first, so its effective
// footprint becomes designH wide × designW tall before fitting.
//
// k mirrors the scale factor that presentGeoM computes, expressed in design
// logical units, so allocating a frame at round(designW*k)×round(designH*k)
// makes the presentGeoM fit-scale ≈1 (a near-identity rotation+centre).
func frameScaleFor(sw, sh int, portrait bool) float64 {
	designW := float64(hudWidth + windowWidth) // 1080 logical px
	designH := float64(windowHeight)           // 600 logical px
	fsw := float64(sw)
	fsh := float64(sh)
	var k float64
	if !portrait {
		// Landscape: fit designW×designH into sw×sh.
		k = math.Min(fsw/designW, fsh/designH)
	} else {
		// Portrait: design is rotated 90° CCW; effective footprint is designH×designW.
		k = math.Min(fsw/designH, fsh/designW)
	}
	if k <= 0 {
		k = 1
	}
	return k
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
// The frame is sized to the PRESENTED resolution: k = frameScaleFor(sw, sh,
// portrait) is the device-pixels-per-logical-unit that presentGeoM would apply,
// so allocating at round(designW*k)×round(designH*k) makes presentGeoM's
// fit-scale ≈1 (near-identity rotation+centre).  Every per-frame composite op
// therefore touches only as many pixels as the screen actually shows.
//
// Reallocation is triggered whenever the computed frame pixel dimensions differ
// from the current g.frame.Bounds() — this catches screen resize and orientation
// changes in addition to DeviceScaleFactor changes, while ignoring sub-pixel
// jitter (e.g. mobile address-bar animation) that produces identical dims.
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

	// Compute the presented fit-scale k: device pixels per logical design unit.
	k := frameScaleFor(sw, sh, g.portrait)

	// Frame at the presented size: designW×designH logical → round(·*k) device px.
	hudW := int(math.Round(float64(hudWidth) * k))
	boardW := int(math.Round(float64(windowWidth) * k))
	fh := int(math.Round(float64(windowHeight) * k))
	fw := hudW + boardW

	// Realloc only when the target pixel dimensions differ from the current frame.
	// Comparing Bounds() avoids thrashing on tiny mobile viewport jitter that
	// produces the same integer dimensions.
	needRealloc := g.frame == nil ||
		g.frame.Bounds().Dx() != fw ||
		g.frame.Bounds().Dy() != fh

	if needRealloc {
		g.renderScale = s
		g.frameScale = k
		g.hudDevW = hudW
		// deviceCellSize is derived from boardW (not the full frame width).
		g.deviceCellSize = deviceCellSizeFor(boardW)
		g.spriteRefSize = spriteRefSizeFor(g.deviceCellSize)
		g.frame = ebiten.NewImage(fw, fh)
		g.canvas = ebiten.NewImage(boardW, fh)
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

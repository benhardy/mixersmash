package main

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// containsTouchID reports whether ids contains id.
func containsTouchID(ids []ebiten.TouchID, id ebiten.TouchID) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

// findReleasedID reports whether target is present in ids.
// Pure function used to test whether a specific touch ID has just been released.
func findReleasedID(ids []ebiten.TouchID, target ebiten.TouchID) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

// frameDims returns the design-frame dimensions and HUD device-pixel width.
// When g.frame is nil (pre-LayoutF) it falls back to the design constants with
// no HUD, matching the inline logic previously in updateWaiting.
func (g *Game) frameDims() (frameW, frameH float64, hudDevW int) {
	if g.view.frame != nil {
		frameW = float64(g.view.frame.Bounds().Dx())
		frameH = float64(g.view.frame.Bounds().Dy())
		hudDevW = g.view.hudDevW
		return
	}
	frameW = float64(windowWidth)
	frameH = float64(windowHeight)
	return
}

// updateHover sets g.input.hoverCol/g.input.hoverRow when (hc, hr) is in-bounds and alive.
// For animal cells, victimHints is populated with the flood-match cluster coords
// (the cells that would clear on click) so the hover preview highlights the match.
// For non-animal or empty cells, victimHints is cleared.
// If the cell is out of bounds or dead the hover state is left unchanged.
func (g *Game) updateHover(hc, hr int) {
	if hc < 0 || hc >= g.board.grid.Cols || hr < 0 || hr >= g.board.grid.Rows {
		return
	}
	cell := g.board.grid.At(hc, hr)
	if cell.Health() <= 0 {
		return
	}
	g.input.hoverCol, g.input.hoverRow = hc, hr
	if cell.Organism() == nil || !cell.Organism().IsAnimal() {
		g.input.victimHints = g.input.victimHints[:0]
		return
	}
	cluster := g.board.grid.FloodMatchCluster(hc, hr)
	if g.board.grid.ClusterHasFood(cluster) {
		g.input.victimHints = cluster
	} else {
		g.input.victimHints = g.input.victimHints[:0]
	}
}

// updateWaiting handles hover recompute and click/touch processing while in stateWaiting.
func (g *Game) updateWaiting() error {
	dcs := g.view.deviceCellSize
	if dcs <= 0 {
		dcs = cellSize
	}

	frameW, frameH, hudDevW := g.frameDims()
	sw := float64(g.view.screenW)
	sh := float64(g.view.screenH)
	if sw <= 0 {
		sw = frameW
		sh = frameH
	}

	// Determine whether any touch activity is happening this frame so the mouse
	// path can be suppressed (dedupe: synthesised mouse clicks on touch devices).
	g.input.touchIDsBuf = ebiten.AppendTouchIDs(g.input.touchIDsBuf[:0])
	g.input.justPressedBuf = inpututil.AppendJustPressedTouchIDs(g.input.justPressedBuf[:0])
	g.input.justReleasedBuf = inpututil.AppendJustReleasedTouchIDs(g.input.justReleasedBuf[:0])
	currentTouches := g.input.touchIDsBuf
	justPressed := g.input.justPressedBuf
	justReleased := g.input.justReleasedBuf
	touchFrameActive := g.input.touchActive || len(currentTouches) > 0 || len(justPressed) > 0 || len(justReleased) > 0

	// Touch input block.
	// New press: begin tracking the first finger.
	if !g.input.touchActive && len(justPressed) > 0 {
		g.input.touchActive = true
		g.input.activeTouchID = justPressed[0]
		// Store the raw screen press position so a single-frame tap has a valid stored position.
		g.input.lastTouchX, g.input.lastTouchY = ebiten.TouchPosition(g.input.activeTouchID)
	}

	// Hold-as-hover: mirror the mouse hover recompute using the active touch position.
	// lastTouchX/Y store RAW screen coords; mapped to board via screenToBoard.
	// Guard requires the tracked ID to still be present in currentTouches: on the
	// release frame the ID is absent, so lastTouchX/Y retain the last valid position
	// and the release-commit loop below reads the correct cell instead of (0,0).
	if g.input.touchActive && containsTouchID(currentTouches, g.input.activeTouchID) {
		tx, ty := ebiten.TouchPosition(g.input.activeTouchID)
		g.input.lastTouchX, g.input.lastTouchY = tx, ty
		hc, hr := screenToBoard(float64(tx), float64(ty), sw, sh, frameW, frameH, g.view.portrait, hudDevW, dcs)
		g.updateHover(hc, hr)
	}

	// Release-to-commit: when the tracked finger lifts, commit using the stored last
	// raw screen position, mapped to a board cell via screenToBoard.
	// Do NOT re-read ebiten.TouchPosition for the released ID — on this Ebitengine
	// version it returns (0,0) for an already-released touch.
	if findReleasedID(justReleased, g.input.activeTouchID) {
		col, row := screenToBoard(float64(g.input.lastTouchX), float64(g.input.lastTouchY), sw, sh, frameW, frameH, g.view.portrait, hudDevW, dcs)
		// Clear touch state before commit so a reused ID cannot match stale state.
		g.input.touchActive = false
		if err := g.commitCellAt(col, row); err != nil {
			return err
		}
	}

	// Mouse input block — skipped entirely when touch is active/just-active this frame.
	if !touchFrameActive {
		// Hover recompute.
		// CursorPosition returns device-pixel screen coords; map to board via screenToBoard.
		mx, my := ebiten.CursorPosition()
		hc, hr := screenToBoard(float64(mx), float64(my), sw, sh, frameW, frameH, g.view.portrait, hudDevW, dcs)
		g.updateHover(hc, hr)

		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			px, py := ebiten.CursorPosition()
			col, row := screenToBoard(float64(px), float64(py), sw, sh, frameW, frameH, g.view.portrait, hudDevW, dcs)
			if err := g.commitCellAt(col, row); err != nil {
				return err
			}
		}
	}

	return nil
}

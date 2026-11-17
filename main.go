package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	text "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/basicfont"
)

const (
	windowWidth  = 800
	windowHeight = 600
	cellSize     = 64
	gridCols     = windowWidth / cellSize  // 12
	gridRows     = windowHeight / cellSize // 9

	// hudWidth is the logical-pixel width of the left HUD strip. The board
	// occupies the remaining windowWidth device-pixels of the frame.
	// Widened to 280 so that 3x-scaled debug text fits comfortably.
	hudWidth = 280

	stateWaiting          = 1
	stateShuffling        = 2
	stateDropping         = 3
	stateVictimMoving     = 4
	stateColumnCollapsing = 5

	// Animation durations expressed as wall-clock time.
	// Values are equivalent to the original tick counts at 60 TPS:
	//   shuffleDuration  = 18 ticks / 60 TPS = 300ms
	//   dropDuration     = 18 ticks / 60 TPS = 300ms
	//   victimDuration   = 24 ticks / 60 TPS = 400ms
	//   targetAnimDuration = 15 ticks / 60 TPS = 250ms
	shuffleDuration    = 18 * time.Second / 60 // 300ms
	dropDuration       = 18 * time.Second / 60 // 300ms
	victimDuration     = 24 * time.Second / 60 // 400ms
	targetAnimDuration = 15 * time.Second / 60 // 250ms

	healthDecrementKeep = float32(0.75)
	healthMin           = float32(0.562)
	healthEpsilon       = float32(0.001)

	respawnCells = false // set false to leave gaps at top instead of spawning new cells

	// showOrientationDebug controls the small orientation/dimension readout drawn
	// in the bottom-left corner of the frame each tick.  Set false to hide it.
	showOrientationDebug = false

	// showPerfDebug controls the per-frame performance readout drawn in the
	// bottom-right corner of the frame.  Set false to compile it out entirely.
	showPerfDebug = true
)

// compositeSpriteRefSize is the logical fallback side length used only before
// the first LayoutF call sets g.spriteRefSize.  At runtime, spriteRefSize is
// derived from deviceCellSize*2 (capped at 512) and updated on scale changes.
const compositeSpriteRefSize = 120

// hudFace is the shared GoXFace for HUD and orientation-debug text.
// Created once at package init to avoid a per-frame allocation.
var hudFace = text.NewGoXFace(basicfont.Face7x13)

// Game is the Ebitengine game state.
type Game struct {
	grid         *Grid
	canvas       *ebiten.Image
	frame        *ebiten.Image // landscape design-size offscreen frame; board renders here
	currentState int           // which state are we in?
	animStart    time.Time     // when did the current animation phase start?
	actionCol    int           // the column an action is happening
	actionRow    int           // the row any given action is happening
	moveAnims    []MoveAnim    // used for both shuffle and drop phases
	glowShader   *ebiten.Shader
	gelShader    *ebiten.Shader
	cellCount    int        // number of live cells currently in the grid
	score        int        // cumulative score
	targetColor  color.RGBA // colour of the most recently killed target cell

	// hover hint fields
	hoverCol, hoverRow int      // cell under cursor (-1 = none)
	victimHints        [][2]int // closest-colour neighbours of hovered cell (empty = none)

	// touch tracking fields
	touchActive   bool
	activeTouchID ebiten.TouchID
	lastTouchX    int // last device-pixel X while the tracked finger was down (raw screen coords)
	lastTouchY    int // last device-pixel Y while the tracked finger was down (raw screen coords)

	// victim movement animation fields (one entry per simultaneous victim)
	victims []MoveAnim

	// target cell bounce animation fields
	targetAnimating    bool
	targetAnimStart    time.Time
	targetAnimStartR   float32
	targetAnimPeakR    float32
	targetAnimEndR     float32
	targetAnimCreature *Creature

	// glyph rendering
	glyphFont *text.GoTextFaceSource

	// HiDPI render scale — set once per LayoutF call; derived from
	// ebiten.Monitor().DeviceScaleFactor().
	renderScale    float64 // current device scale factor (1.0 on standard displays)
	frameScale     float64 // presented fit-scale k (device px per logical unit); see frameScaleFor
	hudDevW        int     // device-pixel HUD strip width at the current frameScale
	deviceCellSize int     // device-pixel cell size, derived from frame width
	spriteRefSize  int     // device-pixel sprite bake diameter (≤512)

	// screenW, screenH are the actual device-pixel screen dimensions, updated
	// each LayoutF call.  portrait is true when screenH > screenW.
	screenW, screenH int
	portrait         bool

	// initialPainted is false until the first in-loop frame, at which point
	// repaintGrid is called to bake all sprites with a live graphics context.
	initialPainted bool

	// Performance counters for showPerfDebug readout.
	// spritesThisFrame counts drawCompositeSprite blits in the current Draw call;
	// reset to 0 at the top of each Draw.
	// bakeCount is cumulative: incremented each time compositeCachedSprite builds
	// (bakes) a new sprite on a cache miss; never reset.
	spritesThisFrame int
	bakeCount        int

	// Worst-frame-time tracking for showPerfDebug.
	// lastDrawTime records when the previous Draw call started (zero = unset).
	// worstFrameMs is the worst Draw-to-Draw delta seen in the current 2s window.
	// worstWindowStart marks when the current window began.
	// prevWindowWorstMs is the worst value from the completed previous window,
	// displayed so the readout stays stable while the current window accumulates.
	lastDrawTime      time.Time
	worstFrameMs      float64
	worstWindowStart  time.Time
	prevWindowWorstMs float64

	// Min-FPS and max-sprite-count tracking over the same 2s window.
	// minFpsWindow is the lowest ActualFPS sample seen in the current window
	// (-1 means no sample yet).  maxSprWindow is the highest spritesThisFrame
	// seen in the current window.  The prev* fields hold the completed window's
	// values for display (same latch pattern as prevWindowWorstMs).
	minFpsWindow     float64
	maxSprWindow     int
	prevWindowMinFps float64
	prevWindowMaxSpr int

	// HUD cache: g.hudImage is a persistent image of the HUD strip redrawn only
	// when its content or size changes.  The last-rendered state is tracked in the
	// fields below; a mismatch triggers a re-render.
	hudImage         *ebiten.Image
	lastHUDScore     int
	lastHUDHoverCol  int
	lastHUDHoverRow  int
	lastHUDThemeName string
}

// newGameState returns a Game with data fields initialised but no ebiten.Image
// allocated.  Callers that need a canvas (i.e. main) must set g.canvas after
// calling this.
func newGameState(grid *Grid) *Game {
	return &Game{
		grid:            grid,
		currentState:    stateWaiting,
		actionCol:       -1,
		actionRow:       -1,
		cellCount:       grid.Cols * grid.Rows,
		hoverCol:        -1,
		hoverRow:        -1,
		victimHints:     nil,
		victims:         nil,
		targetAnimating: false,
	}
}

// compositeCachedSprite returns a pre-rendered composite *ebiten.Image for cr,
// building and caching it on cr.Sprite on first call.  The image carries a
// circa-2003 Apple Aqua jelly-button look rendered via the gel fragment shader:
//
//  1. Base image — an sz×sz opaque image filled with cr.Color, then the creature
//     glyph cr.Glyph (tinted darkerShade(cr.Color)) drawn directly onto it.  The
//     base image is fully opaque so its rgb values equal straight (non-premultiplied) rgb.
//  2. Gel shader pass — gelShaderSrc runs via DrawRectShader, sampling the base
//     image (Images[0]) at each pixel to obtain the local colour, then applies
//     the full Aqua gel lighting (vertical gradient, bottom caustic, gloss
//     highlight, rim) over that sampled colour.  The shader owns the circular
//     AA edge and outputs premultiplied vec4(col*alpha, alpha).
//
// The glyph is embedded under the gel lighting (gradient, gloss, rim all apply
// over it), giving an inset "carved into jelly" appearance.
//
// Returns nil if cr is nil, the font is not loaded, or the gel shader fails to compile.
func (g *Game) compositeCachedSprite(cr *Creature) *ebiten.Image {
	if cr == nil || g.glyphFont == nil {
		return nil
	}
	if cr.Sprite != nil {
		return cr.Sprite
	}

	// Compile the gel shader once per game instance (lazy, mirrors glowShader).
	if g.gelShader == nil {
		s, err := ebiten.NewShader([]byte(gelShaderSrc))
		if err != nil {
			log.Printf("gel shader compile error: %v", err)
			return nil
		}
		g.gelShader = s
	}

	// Use the device-pixel bake size when available; fall back to the logical
	// constant before the first LayoutF call initialises spriteRefSize.
	sz := g.spriteRefSize
	if sz <= 0 {
		sz = compositeSpriteRefSize
	}
	half := float32(sz) / 2
	clr := cr.Color

	// ------------------------------------------------------------------ //
	// Step 1: build the base image — opaque fill colour + creature glyph.
	// This is the source image the gel shader will sample.
	// The image is fully opaque (A=255 everywhere after the fill), so the
	// shader can use sampled .rgb directly as straight colour values.
	// ------------------------------------------------------------------ //
	base := ebiten.NewImage(sz, sz)
	base.Fill(clr)

	glyphSize := float64(sz) * 0.75
	face := &text.GoTextFace{
		Source: g.glyphFont,
		Size:   glyphSize,
	}
	str := string(cr.Glyph)
	w, h := text.Measure(str, face, 0)
	tint := darkerShade(clr)
	var drawOpts text.DrawOptions
	drawOpts.ColorScale.ScaleWithColor(tint)
	nudgeY := float64(sz) * 0.03
	drawOpts.GeoM.Translate(float64(half)-w/2, float64(half)-h/2+nudgeY)
	text.Draw(base, str, face, &drawOpts)

	// ------------------------------------------------------------------ //
	// Step 2: apply the gel shader from base → composite.
	// The shader samples Images[0] (base) for the per-pixel base colour and
	// applies all gel lighting math over it, outputting premultiplied RGBA
	// with the circular AA alpha. composite is a DIFFERENT image from base,
	// so DrawRectShader reads and writes distinct images as required.
	// ------------------------------------------------------------------ //
	composite := ebiten.NewImage(sz, sz)
	var gelOpts ebiten.DrawRectShaderOptions
	gelOpts.Blend = ebiten.BlendSourceOver
	gelOpts.Uniforms = map[string]any{
		"Size":  []float32{float32(sz), float32(sz)},
		"Hover": float32(0),
	}
	gelOpts.Images[0] = base
	composite.DrawRectShader(sz, sz, g.gelShader, &gelOpts)

	g.bakeCount++
	cr.Sprite = composite
	return composite
}

// drawCompositeSprite blits the pre-rendered composite creature sprite for cr
// centred at (cx, cy) scaled so its inscribed circle matches radius.  Does
// nothing if radius < 1, cr is nil, or the sprite could not be baked.
func (g *Game) drawCompositeSprite(dst *ebiten.Image, cx, cy, radius float32, cr *Creature) {
	if cr == nil || radius < 1 {
		return
	}
	img := g.compositeCachedSprite(cr)
	if img == nil {
		return
	}
	g.spritesThisFrame++
	// Scale so the sprite's inscribed circle (diameter = sprite bake size)
	// matches the requested radius.
	refSz := g.spriteRefSize
	if refSz <= 0 {
		refSz = compositeSpriteRefSize
	}
	scale := float64(radius*2) / float64(refSz)
	var opts ebiten.DrawImageOptions
	opts.GeoM.Scale(scale, scale)
	// Top-left of the scaled image: (cx - radius, cy - radius).
	opts.GeoM.Translate(float64(cx)-float64(radius), float64(cy)-float64(radius))
	// FilterLinear interpolates smoothly when downscaling; the default
	// FilterNearest would not antialias a fractional-scale blit.
	opts.Filter = ebiten.FilterLinear
	dst.DrawImage(img, &opts)
}

// drawCell erases the cell's bounding rectangle on canvas with black and
// redraws the antialiased circle for (col, row).  No glow is drawn here;
// glow is composited onto the screen each frame in Draw().
func (g *Game) drawCell(col, row int, cellFullness float32) {
	cell := &g.grid.Cells[col][row]
	cs := g.deviceCellSize
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
	g.drawCompositeSprite(g.canvas, cx, cy, radius, cell.Creature)
}

// emptyColumns returns the sorted indices of columns in which every cell has
// Health <= 0.
func (g *Game) emptyColumns() []int {
	var result []int
	for c := 0; c < g.grid.Cols; c++ {
		empty := true
		for r := 0; r < g.grid.Rows; r++ {
			if g.grid.Cells[c][r].Health > 0 {
				empty = false
				break
			}
		}
		if empty {
			result = append(result, c)
		}
	}
	return result
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
				g.grid.Cells[col][i] = Cell{Creature: paletteCreature(), Health: 1.0}
				spawned++
			}
		}

		// Create MoveAnims for moved alive cells — clear only their old (vacated) positions.
		dcs := g.deviceCellSize
		cw := g.canvas.Bounds().Max.X // guard against canvas size mismatch
		ch := g.canvas.Bounds().Max.Y
		for _, m := range moves {
			cell := g.grid.Cells[col][m[1]]
			fx, fy := deviceCellCenter(col, m[0], dcs)
			tx, ty := deviceCellCenter(col, m[1], dcs)
			g.moveAnims = append(g.moveAnims, MoveAnim{FromX: fx, FromY: fy, ToX: tx, ToY: ty, Cell: cell})
			// Clear the vacated canvas position (clipped to canvas bounds).
			x0, y0 := col*dcs, m[0]*dcs
			x1, y1 := x0+dcs, y0+dcs
			x1 = min(x1, cw)
			y1 = min(y1, ch)
			if x0 < x1 && y0 < y1 {
				g.canvas.SubImage(image.Rect(x0, y0, x1, y1)).(*ebiten.Image).Clear()
			}
		}

		if respawnCells {
			// Create MoveAnims for new cells animating in from above screen.
			for i := 0; i < len(holeRows); i++ {
				cell := g.grid.Cells[col][i]
				fx := float32(col*dcs + dcs/2)
				fy := float32(-dcs / 2)
				tx, ty := deviceCellCenter(col, i, dcs)
				g.moveAnims = append(g.moveAnims, MoveAnim{FromX: fx, FromY: fy, ToX: tx, ToY: ty, Cell: cell})
				// The canvas position for row i was vacated by the moved cell above — already cleared.
			}
		}
	}
	return spawned
}

// buildCollapseAnims computes MoveAnim entries for the horizontal column-collapse
// phase. Empty columns are detected, non-empty columns are packed toward the
// centre, the grid is mutated in-place (via snapshot), cleared canvas strips are
// emitted for affected columns, and horizontal MoveAnims are built.
//
// Returns true if stateColumnCollapsing was entered; false when no moves are
// needed (caller falls through to stateWaiting).
func (g *Game) buildCollapseAnims() bool {
	emptyCols := g.emptyColumns()
	moves := horizontalCollapseMoves(g.grid.Cols, emptyCols)
	if len(moves) == 0 {
		return false
	}

	// Snapshot entire Cells slice to avoid aliasing during column moves.
	snapshot := make([][]Cell, g.grid.Cols)
	for c := 0; c < g.grid.Cols; c++ {
		snapshot[c] = make([]Cell, g.grid.Rows)
		copy(snapshot[c], g.grid.Cells[c])
	}

	// Apply the pure column reassignment.
	newCells := applyColumnCollapse(snapshot, moves)
	for c := 0; c < g.grid.Cols; c++ {
		copy(g.grid.Cells[c], newCells[c])
	}

	// Clear canvas strips for every column that is a move source OR destination.
	// During the slide, stale painted cells at both source and destination positions
	// would show through under the animating sprites, causing visible doubling.
	// Columns not involved in any move are untouched.
	dcs := g.deviceCellSize
	cw := g.canvas.Bounds().Max.X
	ch := g.canvas.Bounds().Max.Y
	clearColSet := make(map[int]bool, len(moves)*2)
	for _, m := range moves {
		clearColSet[m[0]] = true
		clearColSet[m[1]] = true
	}
	for col := range clearColSet {
		x0 := col * dcs
		x1 := x0 + dcs
		x1 = min(x1, cw)
		if x0 < x1 {
			g.canvas.SubImage(image.Rect(x0, 0, x1, ch)).(*ebiten.Image).Clear()
		}
	}

	// Build horizontal MoveAnims.
	if cap(g.moveAnims) > 0 {
		g.moveAnims = g.moveAnims[:0]
	}
	for _, m := range moves {
		oldCol, newCol := m[0], m[1]
		for r := 0; r < g.grid.Rows; r++ {
			cell := g.grid.Cells[newCol][r]
			if cell.Health <= 0 {
				continue
			}
			fx, fy := deviceCellCenter(oldCol, r, dcs)
			tx, ty := deviceCellCenter(newCol, r, dcs)
			g.moveAnims = append(g.moveAnims, MoveAnim{FromX: fx, FromY: fy, ToX: tx, ToY: ty, Cell: cell})
		}
	}

	g.animStart = time.Now()
	g.currentState = stateColumnCollapsing
	return true
}

// repaintGrid redraws every cell from the model onto the offscreen canvas,
// fully re-syncing canvas state with the grid. Dead cells are cleared to blank.
// This is called only at settle points (no animation is in flight).
func (g *Game) repaintGrid() {
	for c := 0; c < g.grid.Cols; c++ {
		for r := 0; r < g.grid.Rows; r++ {
			g.drawCell(c, r, 1.0)
		}
	}
}

// enterWaiting repaints the full grid canvas from the model and transitions to
// stateWaiting. All settle-to-waiting paths must go through this helper so
// canvas drift is corrected on every settle.
func (g *Game) enterWaiting() {
	g.repaintGrid()
	g.currentState = stateWaiting
}

// settleAfterRemoval is called at every settle point that follows a cell
// removal. It runs the horizontal collapse check: if empty columns exist,
// buildCollapseAnims enters stateColumnCollapsing and returns true, so this
// function does nothing further. Otherwise it transitions to stateWaiting via
// enterWaiting, which repaints the full canvas from the model.
//
// All removal→waiting paths funnel through this helper so the collapse check
// is never skipped. The shuffle/survival path (no removal) must NOT call this.
func (g *Game) settleAfterRemoval() {
	if g.buildCollapseAnims() {
		return
	}
	g.enterWaiting()
}

// updateColumnCollapsing waits for the horizontal column-collapse animation to
// complete, commits cell positions to canvas, and returns to stateWaiting.
func (g *Game) updateColumnCollapsing() error {
	elapsed := elapsedFraction(g.animStart, time.Now(), dropDuration)
	if elapsed >= 1.0 {
		dcs := g.deviceCellSize
		for _, anim := range g.moveAnims {
			col := int(anim.ToX) / dcs
			row := int(anim.ToY) / dcs
			if col >= 0 && col < g.grid.Cols && row >= 0 && row < g.grid.Rows {
				g.drawCell(col, row, 1.0)
			}
		}
		g.moveAnims = nil
		g.enterWaiting()
	}
	return nil
}

// commitCellAt applies one hit to the cell at (col, row): decrements health,
// then delegates to handleTargetDeath or handleTargetSurvival as appropriate.
// It is a no-op (returns nil) when the cell is out-of-bounds or already dead.
func (g *Game) commitCellAt(col, row int) error {
	if col < 0 || col >= g.grid.Cols || row < 0 || row >= g.grid.Rows {
		return nil
	}
	cell := &g.grid.Cells[col][row]
	if cell.Health <= 0 {
		return nil
	}

	oldHealth := cell.Health
	cell.Health *= healthDecrementKeep
	if cell.Health <= healthMin {
		cell.Health = 0
	}
	if cell.Health < healthEpsilon {
		cell.Health = 0.0
	}

	g.actionCol = col
	g.actionRow = row

	if cell.Health <= 0 {
		g.handleTargetDeath(col, row)
	} else if !g.handleTargetSurvival(col, row, cell, oldHealth) {
		return nil
	}
	return nil
}

// updateWaiting handles hover recompute and click/touch processing while in stateWaiting.
// containsTouchID reports whether ids contains id.
func containsTouchID(ids []ebiten.TouchID, id ebiten.TouchID) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

func (g *Game) updateWaiting() error {
	dcs := g.deviceCellSize
	if dcs <= 0 {
		dcs = cellSize
	}

	// frameW/frameH: dimensions of the full (HUD+board) landscape design frame.
	// When g.frame is nil (pre-LayoutF) fall back to design constants with no HUD.
	var frameW, frameH float64
	hudDevW := 0
	if g.frame != nil {
		frameW = float64(g.frame.Bounds().Dx())
		frameH = float64(g.frame.Bounds().Dy())
		hudDevW = g.hudDevW
	} else {
		frameW = float64(windowWidth)
		frameH = float64(windowHeight)
	}
	sw := float64(g.screenW)
	sh := float64(g.screenH)
	if sw <= 0 {
		sw = frameW
		sh = frameH
	}

	// Determine whether any touch activity is happening this frame so the mouse
	// path can be suppressed (dedupe: synthesised mouse clicks on touch devices).
	currentTouches := ebiten.AppendTouchIDs(nil)
	justPressed := inpututil.AppendJustPressedTouchIDs(nil)
	justReleased := inpututil.AppendJustReleasedTouchIDs(nil)
	touchFrameActive := g.touchActive || len(currentTouches) > 0 || len(justPressed) > 0 || len(justReleased) > 0

	// Touch input block.
	// New press: begin tracking the first finger.
	if !g.touchActive && len(justPressed) > 0 {
		g.touchActive = true
		g.activeTouchID = justPressed[0]
		// Store the raw screen press position so a single-frame tap has a valid stored position.
		g.lastTouchX, g.lastTouchY = ebiten.TouchPosition(g.activeTouchID)
	}

	// Hold-as-hover: mirror the mouse hover recompute using the active touch position.
	// lastTouchX/Y store RAW screen coords; mapped to board via screenToBoard.
	// Guard requires the tracked ID to still be present in currentTouches: on the
	// release frame the ID is absent, so lastTouchX/Y retain the last valid position
	// and the release-commit loop below reads the correct cell instead of (0,0).
	if g.touchActive && containsTouchID(currentTouches, g.activeTouchID) {
		tx, ty := ebiten.TouchPosition(g.activeTouchID)
		g.lastTouchX, g.lastTouchY = tx, ty
		hc, hr := screenToBoard(float64(tx), float64(ty), sw, sh, frameW, frameH, g.portrait, hudDevW, dcs)
		if hc >= 0 && hc < g.grid.Cols && hr >= 0 && hr < g.grid.Rows {
			if g.grid.Cells[hc][hr].Health > 0 {
				g.hoverCol, g.hoverRow = hc, hr
				g.victimHints = connectionVictims(g.grid.Cells, hc, hr, g.grid.Cols, g.grid.Rows)
			}
		}
	}

	// Release-to-commit: when the tracked finger lifts, commit using the stored last
	// raw screen position, mapped to a board cell via screenToBoard.
	// Do NOT re-read ebiten.TouchPosition for the released ID — on this Ebitengine
	// version it returns (0,0) for an already-released touch.
	for _, id := range justReleased {
		if id == g.activeTouchID {
			col, row := screenToBoard(float64(g.lastTouchX), float64(g.lastTouchY), sw, sh, frameW, frameH, g.portrait, hudDevW, dcs)
			// Clear touch state before commit so a reused ID cannot match stale state.
			g.touchActive = false
			if err := g.commitCellAt(col, row); err != nil {
				return err
			}
			break
		}
	}

	// Mouse input block — skipped entirely when touch is active/just-active this frame.
	if !touchFrameActive {
		// Hover recompute.
		// CursorPosition returns device-pixel screen coords; map to board via screenToBoard.
		mx, my := ebiten.CursorPosition()
		hc, hr := screenToBoard(float64(mx), float64(my), sw, sh, frameW, frameH, g.portrait, hudDevW, dcs)
		if hc >= 0 && hc < g.grid.Cols && hr >= 0 && hr < g.grid.Rows {
			if g.grid.Cells[hc][hr].Health > 0 {
				g.hoverCol, g.hoverRow = hc, hr
				g.victimHints = connectionVictims(g.grid.Cells, hc, hr, g.grid.Cols, g.grid.Rows)
			}
		}

		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			px, py := ebiten.CursorPosition()
			col, row := screenToBoard(float64(px), float64(py), sw, sh, frameW, frameH, g.portrait, hudDevW, dcs)
			if err := g.commitCellAt(col, row); err != nil {
				return err
			}
		}
	}

	return nil
}

// victimPathParams computes the path parameters for a single victim moving from
// (fromX,fromY) to (toX,toY) and returns PathType, CtrlX, CtrlY, and
// SpiralTurns for use in a MoveAnim victim entry.
func victimPathParams(fromX, fromY, toX, toY float32) (pathType int, ctrlX, ctrlY, spiralTurns float32) {
	pathType = rand.Intn(3)
	dx := toX - fromX
	dy := toY - fromY
	dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
	if pathType == 1 { // Bezier arc: control point perpendicular to the line
		midX := (fromX + toX) * 0.5
		midY := (fromY + toY) * 0.5
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
		ctrlX = midX + pX*offset
		ctrlY = midY + pY*offset
	}
	if pathType == 2 { // logarithmic spiral
		turns := float32(0.5 + rand.Float32()*1.0) // 0.5–1.5 turns
		if rand.Intn(2) == 0 {
			turns = -turns
		}
		spiralTurns = turns
	}
	return
}

// handleTargetDeath handles the death path after a cell's health has been
// drained to zero: redraws the dead cell, selects all tied-closest victims to
// animate into it (transitioning to stateVictimMoving), or falls straight to
// stateDropping when no victim exists.
func (g *Game) handleTargetDeath(col, row int) {
	// Capture target colour before the cell is cleared.
	g.targetColor = g.grid.Cells[col][row].Color()
	// Redraw immediately so cell disappears.
	g.drawCell(col, row, 1.0)
	g.cellCount-- // decrement for the clicked cell
	victimCoords := connectionVictims(g.grid.Cells, col, row, g.grid.Cols, g.grid.Rows)
	if len(victimCoords) > 0 {
		dcs := g.deviceCellSize
		toX, toY := deviceCellCenter(col, row, dcs)
		g.victims = g.victims[:0]
		for _, vc := range victimCoords {
			ncol, nrow := vc[0], vc[1]
			fromX, fromY := deviceCellCenter(ncol, nrow, dcs)
			pt, cx, cy, st := victimPathParams(fromX, fromY, toX, toY)
			v := MoveAnim{
				Col:         ncol,
				Row:         nrow,
				Cell:        g.grid.Cells[ncol][nrow], // snapshot
				FromX:       fromX,
				FromY:       fromY,
				ToX:         toX,
				ToY:         toY,
				PathType:    pt,
				CtrlX:       cx,
				CtrlY:       cy,
				SpiralTurns: st,
			}
			g.victims = append(g.victims, v)
			// Clear victim's canvas position (it is now an overlay animation).
			x0, y0 := ncol*dcs, nrow*dcs
			g.canvas.SubImage(image.Rect(x0, y0, x0+dcs, y0+dcs)).(*ebiten.Image).Clear()
		}
		g.animStart = time.Now()
		g.currentState = stateVictimMoving
	} else {
		// No victim — score the kill and go straight to drop.
		g.score += scoreForKill(nil, g.targetColor)
		spawned := g.buildDropAnims([]int{col})
		if respawnCells {
			g.cellCount += spawned
		}
		if len(g.moveAnims) > 0 {
			g.animStart = time.Now()
			g.currentState = stateDropping
		} else {
			g.settleAfterRemoval()
		}
	}
}

// handleTargetSurvival handles the survival path after a cell has been damaged
// but not killed: shuffles alive neighbours and sets up the bounce animation,
// transitioning to stateShuffling. Returns false if there are no alive
// neighbours and the caller should return nil without a state change.
func (g *Game) handleTargetSurvival(col, row int, cell *Cell, oldHealth float32) bool {
	// SURVIVAL PATH — shuffle alive neighbours, then set up bounce animation.
	neighbors := eightNeighbors(col, row, g.grid.Cols, g.grid.Rows)
	// Only shuffle alive cells — empty slots stay put.
	var aliveNeighbors [][2]int
	for _, n := range neighbors {
		if g.grid.Cells[n[0]][n[1]].Health > 0 {
			aliveNeighbors = append(aliveNeighbors, n)
		}
	}
	if len(aliveNeighbors) == 0 {
		return false
	}

	// Set up bounce animation now that we know a shuffle will proceed.
	dcs := g.deviceCellSize
	maxR := float32(dcs/2 - 2)
	x0, y0 := col*dcs, row*dcs
	g.canvas.SubImage(image.Rect(x0, y0, x0+dcs, y0+dcs)).(*ebiten.Image).Clear()
	g.targetAnimating = true
	g.targetAnimStart = time.Now()
	g.targetAnimStartR = maxR * oldHealth
	g.targetAnimPeakR = maxR * oldHealth * 1.1
	g.targetAnimEndR = maxR * cell.Health
	g.targetAnimCreature = g.grid.Cells[col][row].Creature
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
		fx, fy := deviceCellCenter(src.col, src.row, dcs)
		tx, ty := deviceCellCenter(dst[0], dst[1], dcs)
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
		anims[i] = MoveAnim{FromX: fx, FromY: fy, ToX: tx, ToY: ty, CtrlX: ctrlX, CtrlY: ctrlY, Cell: src.cell}
	}

	// Clear source positions on canvas.
	for _, n := range aliveNeighbors {
		g.canvas.SubImage(image.Rect(n[0]*dcs, n[1]*dcs, (n[0]+1)*dcs, (n[1]+1)*dcs)).(*ebiten.Image).Clear()
	}

	// Write cells to their permuted destinations in the grid.
	for i := range aliveNeighbors {
		dst := aliveNeighbors[perm[i]]
		g.grid.Cells[dst[0]][dst[1]] = orig[i].cell
	}

	g.moveAnims = anims
	g.currentState = stateShuffling
	g.animStart = time.Now()
	return true
}

// updateVictimMoving advances the victim movement animation and transitions to
// stateDropping (or stateWaiting) when the animation completes.
func (g *Game) updateVictimMoving() error {
	t := elapsedFraction(g.animStart, time.Now(), victimDuration)
	if t >= 1.0 {
		// All victims arrived — score the kill, kill them, and trigger drop.
		victimColors := make([]color.RGBA, len(g.victims))
		for i, v := range g.victims {
			victimColors[i] = v.Cell.Color()
		}
		g.score += scoreForKill(victimColors, g.targetColor)
		colSet := map[int]bool{g.actionCol: true}
		for _, v := range g.victims {
			g.grid.Cells[v.Col][v.Row].Health = 0
			colSet[v.Col] = true
		}
		g.cellCount -= len(g.victims)
		affectedCols := make([]int, 0, len(colSet))
		for c := range colSet {
			affectedCols = append(affectedCols, c)
		}
		g.victims = g.victims[:0]
		spawned := g.buildDropAnims(affectedCols)
		if respawnCells {
			g.cellCount += spawned
		}
		if len(g.moveAnims) > 0 {
			g.animStart = time.Now()
			g.currentState = stateDropping
		} else {
			g.settleAfterRemoval()
		}
	}
	return nil
}

// updateShuffling waits for the shuffle animation to complete, then commits
// final cell positions and returns to stateWaiting.
func (g *Game) updateShuffling() error {
	elapsed := elapsedFraction(g.animStart, time.Now(), shuffleDuration)
	if elapsed >= 1.0 {
		dcs := g.deviceCellSize
		for _, anim := range g.moveAnims {
			col := int(anim.ToX) / dcs
			row := int(anim.ToY) / dcs
			if col >= 0 && col < g.grid.Cols && row >= 0 && row < g.grid.Rows {
				g.drawCell(col, row, 1.0)
			}
		}
		g.moveAnims = nil
		g.enterWaiting()
	}
	return nil
}

// updateDropping waits for the drop animation to complete, then commits final
// cell positions and returns to stateWaiting (or stateColumnCollapsing when
// empty columns remain after the drop settles).
func (g *Game) updateDropping() error {
	elapsed := elapsedFraction(g.animStart, time.Now(), dropDuration)
	if elapsed >= 1.0 {
		dcs := g.deviceCellSize
		for _, anim := range g.moveAnims {
			col := int(anim.ToX) / dcs
			row := int(anim.ToY) / dcs
			if col >= 0 && col < g.grid.Cols && row >= 0 && row < g.grid.Rows {
				g.drawCell(col, row, 1.0)
			}
		}
		g.moveAnims = nil
		g.settleAfterRemoval()
	}
	return nil
}

// Update handles input each tick.
func (g *Game) Update() error {
	// First in-loop frame: bake all sprites with a live graphics context so
	// that ebiten.NewImage / DrawRectShader / text.Draw have a valid backend.
	// This must run before any state logic so the canvas is fully painted
	// before the first Draw call.
	if !g.initialPainted {
		g.repaintGrid()
		g.initialPainted = true
	}

	ebiten.SetWindowTitle(fmt.Sprintf("MixerSmash — cells: %d  score: %d", g.cellCount, g.score))

	// Reset hover every tick; recompute below if conditions are met.
	g.hoverCol, g.hoverRow = -1, -1
	g.victimHints = g.victimHints[:0]

	if g.targetAnimating {
		if elapsedFraction(g.targetAnimStart, time.Now(), targetAnimDuration) >= 1.0 {
			g.targetAnimating = false
			g.drawCell(g.actionCol, g.actionRow, 1.0)
		}
	}

	switch g.currentState {
	case stateWaiting:
		return g.updateWaiting()
	case stateVictimMoving:
		return g.updateVictimMoving()
	case stateShuffling:
		return g.updateShuffling()
	case stateDropping:
		return g.updateDropping()
	case stateColumnCollapsing:
		return g.updateColumnCollapsing()
	}
	return nil
}

// Draw blits the offscreen canvas to g.frame, then presents g.frame onto the
// screen with optional 90° CCW rotation for portrait orientation.
func (g *Game) Draw(screen *ebiten.Image) {
	g.spritesThisFrame = 0

	if g.glowShader == nil {
		s, err := ebiten.NewShader([]byte(glowShaderSrc))
		if err != nil {
			log.Fatal("glow shader compile error:", err)
		}
		g.glowShader = s
	}

	// Render the board into the landscape frame image.
	// Guard: frame may be nil on the very first frame before LayoutF runs.
	if g.frame == nil {
		screen.Fill(color.Black)
		return
	}
	g.frame.Fill(color.Black)

	// Board destination: the right portion of the frame, offset by hudDeviceW.
	// We draw directly into g.frame using an explicit x-offset rather than a
	// SubImage, because Ebiten SubImage-as-destination clips but does NOT
	// translate the origin — board-local coords at x=0 would land at x=0 in
	// the frame and be hidden under the HUD strip.
	frameH := g.frame.Bounds().Dy()
	frameW := g.frame.Bounds().Dx()
	hudDevW := g.hudDevW

	g.drawVictimLine(g.frame, hudDevW)
	g.drawCanvas(g.frame, hudDevW)
	g.drawTargetBounce(g.frame, hudDevW)
	g.drawMoveAnims(g.frame, hudDevW)

	// Draw the HUD strip in the left portion of the frame.
	g.drawHUD(g.frame, hudDevW)

	// Draw orientation debug readout (gated by showOrientationDebug const).
	if showOrientationDebug {
		g.drawOrientationDebug(g.frame)
	}

	// Draw performance readout (gated by showPerfDebug const).
	if showPerfDebug {
		g.drawPerfDebug(g.frame)
	}

	// Present the frame onto the actual screen.
	screen.Fill(color.Black)
	fw := float64(frameW)
	fh := float64(frameH)
	sw := float64(g.screenW)
	sh := float64(g.screenH)
	if sw <= 0 {
		// LayoutF not yet called; fall back to screen bounds.
		sw = float64(screen.Bounds().Dx())
		sh = float64(screen.Bounds().Dy())
	}
	var opts ebiten.DrawImageOptions
	opts.GeoM = presentGeoM(g.portrait, sw, sh, fw, fh)
	opts.Filter = ebiten.FilterLinear
	screen.DrawImage(g.frame, &opts)
}

// presentGeoM returns the GeoM that maps g.frame onto the screen.
//
// Landscape: uniform fit-scale centred with letterbox bars.
// Portrait: rotate the landscape frame 90° CCW so it stands upright on a
// portrait screen, then fit-scale and centre.
//
// The portrait forward mapping satisfies: frame point (fx, fy) maps to screen
// point (ox + (fh-fy)*scale, oy + fx*scale).
func presentGeoM(portrait bool, sw, sh, fw, fh float64) ebiten.GeoM {
	var m ebiten.GeoM
	if !portrait {
		scale := math.Min(sw/fw, sh/fh)
		ox := (sw - fw*scale) / 2
		oy := (sh - fh*scale) / 2
		m.Scale(scale, scale)
		m.Translate(ox, oy)
		return m
	}
	// Portrait: rotate 90° CCW about the frame centre, then fit-scale, then centre.
	// 1. Translate frame centre to origin.
	m.Translate(-fw/2, -fh/2)
	// 2. Rotate 90° CCW.  Ebitengine's Rotate is CCW for positive angle.
	m.Rotate(-math.Pi / 2)
	// After this rotation the frame footprint spans [-fh/2, fh/2] in X
	// and [-fw/2, fw/2] in Y (width=fh, height=fw).
	// 3. Scale to fit the portrait screen.
	scale := math.Min(sw/fh, sh/fw)
	m.Scale(scale, scale)
	// 4. Translate to screen centre.
	m.Translate(sw/2, sh/2)
	return m
}

// drawVictimLine draws animated dotted hint lines behind the circles, one per
// tied victim, each from that victim's cell toward the hovered cell.
// xOffset is added to all board-local X coordinates before drawing into dst.
func (g *Game) drawVictimLine(dst *ebiten.Image, xOffset int) {
	if g.hoverCol == -1 || len(g.victimHints) == 0 {
		return
	}
	dcs := g.deviceCellSize
	if dcs <= 0 {
		dcs = cellSize
	}
	s := g.renderScale
	if s <= 0 {
		s = 1.0
	}
	xOff := float32(xOffset)
	toX, toY := deviceCellCenter(g.hoverCol, g.hoverRow, dcs)
	// Dot spacing and radius scale with device scale so they appear the same
	// physical size on all displays.
	dotSpacing := float32(8) * float32(s)
	dotRadius := float32(4.0) * float32(s)
	// Offset advances at 20 logical px/sec = 20*s device px/sec = 20*s/60 device px/tick.
	rawOff := float64(ebiten.Tick()) * (20.0 * s / 60.0)
	offset := float32(math.Mod(rawOff, float64(dotSpacing)))
	for _, hint := range g.victimHints {
		hc, hr := hint[0], hint[1]
		fromX, fromY := deviceCellCenter(hc, hr, dcs)
		dx := toX - fromX
		dy := toY - fromY
		length := float32(math.Sqrt(float64(dx*dx + dy*dy)))
		if length <= 0 {
			continue
		}
		ux := dx / length
		uy := dy / length
		vc := g.grid.Cells[hc][hr].Color()
		dotCol := color.RGBA{vc.R, vc.G, vc.B, 255}
		for pos := offset; pos < length; pos += dotSpacing {
			vector.FillCircle(dst, xOff+fromX+ux*pos, fromY+uy*pos, dotRadius, dotCol, true)
		}
	}
}

// drawCanvas blits the persistent offscreen canvas into dst at an explicit
// x-offset so board-local pixel coords land at the right position in the frame.
func (g *Game) drawCanvas(dst *ebiten.Image, xOffset int) {
	if dst == nil || g.canvas == nil {
		return
	}
	var opts ebiten.DrawImageOptions
	opts.GeoM.Translate(float64(xOffset), 0)
	dst.DrawImage(g.canvas, &opts)
}

// drawTargetBounce renders the target cell bounce animation.
// xOffset is added to the board-local X coordinate before drawing into dst.
func (g *Game) drawTargetBounce(dst *ebiten.Image, xOffset int) {
	if !g.targetAnimating {
		return
	}
	t := elapsedFraction(g.targetAnimStart, time.Now(), targetAnimDuration)
	t = min(t, 1.0)
	var r float32
	if t < 0.5 {
		lt := t * 2              // 0→1 in phase 1
		eased := easeOutQuad(lt) // ease-out: fast start, slow at peak
		r = lerp(g.targetAnimStartR, g.targetAnimPeakR, eased)
	} else {
		lt := (t - 0.5) * 2     // 0→1 in phase 2
		eased := easeInQuad(lt) // ease-in: slow at peak, fast at end
		r = lerp(g.targetAnimPeakR, g.targetAnimEndR, eased)
	}
	dcs := g.deviceCellSize
	if dcs <= 0 {
		dcs = cellSize
	}
	cx, cy := deviceCellCenter(g.actionCol, g.actionRow, dcs)
	if r > 0 {
		g.drawCompositeSprite(dst, cx+float32(xOffset), cy, r, g.targetAnimCreature)
	}
}

// drawMoveAnims renders in-motion cells for shuffle, drop, and victim-move animations.
// xOffset is added to all board-local X coordinates before drawing into dst.
func (g *Game) drawMoveAnims(dst *ebiten.Image, xOffset int) {
	xOff := float32(xOffset)
	if len(g.moveAnims) > 0 {
		var t float32
		dur := shuffleDuration
		if g.currentState == stateDropping || g.currentState == stateColumnCollapsing {
			dur = dropDuration
		}
		t = elapsedFraction(g.animStart, time.Now(), dur)
		t = min(t, 1.0)
		dcs := g.deviceCellSize
		if dcs <= 0 {
			dcs = cellSize
		}
		maxR := float32(dcs/2 - 2)
		for _, anim := range g.moveAnims {
			var cx, cy float32
			if g.currentState == stateShuffling {
				// Quadratic ease-in-out, then quadratic Bézier with eased t
				et := easeInOutQuad(t)
				cx, cy = quadBezier2(anim.FromX, anim.FromY, anim.CtrlX, anim.CtrlY, anim.ToX, anim.ToY, et)
			} else {
				// Drop: quadratic ease-in (gravity), straight line
				posT := easeInQuad(t)
				cx, cy = lerp2(anim.FromX, anim.FromY, anim.ToX, anim.ToY, posT)
			}
			r := maxR * anim.Cell.Health
			if r > 0 {
				g.drawCompositeSprite(dst, xOff+cx, cy, r, anim.Cell.Creature)
			}
		}
	}

	if g.currentState == stateVictimMoving {
		t := elapsedFraction(g.animStart, time.Now(), victimDuration)
		t = min(t, 1.0)
		for _, v := range g.victims {
			var cx, cy float32
			switch v.PathType {
			case 0: // straight line
				cx, cy = lerp2(v.FromX, v.FromY, v.ToX, v.ToY, t)
			case 1: // quadratic Bezier arc
				cx, cy = quadBezier2(v.FromX, v.FromY, v.CtrlX, v.CtrlY, v.ToX, v.ToY, t)
			case 2: // inward spiral (radius shrinks, angle rotates)
				ddx := v.FromX - v.ToX
				ddy := v.FromY - v.ToY
				r0 := float32(math.Sqrt(float64(ddx*ddx + ddy*ddy)))
				theta0 := float32(math.Atan2(float64(ddy), float64(ddx)))
				r := r0 * (1 - t)
				theta := theta0 + v.SpiralTurns*2*float32(math.Pi)*t
				cx = v.ToX + r*float32(math.Cos(float64(theta)))
				cy = v.ToY + r*float32(math.Sin(float64(theta)))
			}
			dcs2 := g.deviceCellSize
			if dcs2 <= 0 {
				dcs2 = cellSize
			}
			radius := float32(dcs2/2-2) * v.Cell.Health * (1 - t)
			if radius > 0 {
				g.drawCompositeSprite(dst, xOff+cx, cy, radius, v.Cell.Creature)
			}
		}
	}
}

// hudTextScale is the integer magnification factor applied to HUD text.
// Text is first rendered into a small temporary image at 1× (basicfont
// Face7x13: 7×13 px glyphs) and then blit into the frame scaled up by this
// factor, giving blocky crisp 3× text with no drop shadow.
const hudTextScale = 3

// hudWrapText splits text into lines each no longer than maxChars characters,
// breaking only at space boundaries.  Lines that have no space and exceed
// maxChars are left unbroken.
func hudWrapText(text string, maxChars int) []string {
	if maxChars <= 0 || len(text) <= maxChars {
		return []string{text}
	}
	var lines []string
	for len(text) > maxChars {
		cut := maxChars
		// Walk back from maxChars to find the last space.
		for cut > 0 && text[cut-1] != ' ' {
			cut--
		}
		if cut == 0 {
			// No space found — emit the whole remaining string as one line.
			break
		}
		lines = append(lines, strings.TrimRight(text[:cut], " "))
		text = text[cut:]
	}
	if len(text) > 0 {
		lines = append(lines, text)
	}
	return lines
}

// drawHUD renders the left HUD strip of g.frame.
// hudDevW is the device-pixel width of the strip.
//
// The HUD is rendered into g.hudImage (a persistent cache) and re-rendered only
// when the content or size changes.  Every frame we blit the cached image with
// one DrawImage — no per-frame text render, no per-frame allocation.
func (g *Game) drawHUD(frame *ebiten.Image, hudDevW int) {
	frameH := frame.Bounds().Dy()
	themeName := activeTheme.name()

	// Dirty check: re-render the cache if anything changed.
	needRender := g.hudImage == nil ||
		g.hudImage.Bounds().Dx() != hudDevW ||
		g.hudImage.Bounds().Dy() != frameH ||
		g.score != g.lastHUDScore ||
		g.hoverCol != g.lastHUDHoverCol ||
		g.hoverRow != g.lastHUDHoverRow ||
		themeName != g.lastHUDThemeName

	if needRender {
		// Allocate or reallocate the cache image when size changed.
		if g.hudImage == nil || g.hudImage.Bounds().Dx() != hudDevW || g.hudImage.Bounds().Dy() != frameH {
			g.hudImage = ebiten.NewImage(hudDevW, frameH)
		} else {
			g.hudImage.Clear()
		}

		// Fill the HUD strip with a light background so the dark title image
		// and black text are visible.
		vector.FillRect(g.hudImage, 0, 0, float32(hudDevW), float32(frameH),
			color.RGBA{0x99, 0x99, 0xAA, 0xFF}, false)

		// Title image at the top of the HUD strip.
		titleH := 0
		if titleImage != nil {
			tw := titleImage.Bounds().Dx()
			th := titleImage.Bounds().Dy()
			if tw > 0 && th > 0 {
				// Scale to fit the HUD width; preserve aspect ratio.
				scaleX := float64(hudDevW) / float64(tw)
				const topMargin = 4
				var tOpts ebiten.DrawImageOptions
				tOpts.GeoM.Scale(scaleX, scaleX)
				tOpts.GeoM.Translate(0, topMargin)
				tOpts.Filter = ebiten.FilterLinear
				g.hudImage.DrawImage(titleImage, &tOpts)
				titleH = int(math.Round(float64(th)*scaleX)) + topMargin
			}
		}

		// Render text at 1× into a small temporary image using basicfont.Face7x13
		// (7×13 px glyphs, no built-in drop shadow), then blit it scaled up by
		// hudTextScale.  tmpW = hudDevW / hudTextScale so that after Scale(3,3)
		// the text fills the HUD strip width exactly.
		tmpW := hudDevW / hudTextScale
		if tmpW < 1 {
			tmpW = 1
		}

		// basicFontW / basicFontH are the glyph cell dimensions of Face7x13.
		const basicFontW = 7
		const basicFontH = 13
		// maxChars is the maximum characters per line that fit in tmpW.
		maxChars := (tmpW - 2) / basicFontW // 2 px left-margin reserve
		if maxChars < 1 {
			maxChars = 1
		}

		// Build the list of text lines (wrapping long instruction text).
		type textLine struct{ s string }
		lines := []textLine{
			{"Level 1"},
		}
		for _, wrapped := range hudWrapText(activeTheme.instructions(), maxChars) {
			lines = append(lines, textLine{wrapped})
		}
		lines = append(lines, textLine{"Score: " + strconv.Itoa(g.score)})

		// Hovered piece info line (optional).
		if g.hoverCol >= 0 && g.hoverRow >= 0 &&
			g.hoverCol < g.grid.Cols && g.hoverRow < g.grid.Rows {
			cell := &g.grid.Cells[g.hoverCol][g.hoverRow]
			if cell.Health > 0 && cell.Creature != nil {
				name := cell.Creature.Name
				if len(name) > 0 {
					name = strings.ToUpper(name[:1]) + name[1:]
				}
				lines = append(lines, textLine{""}) // blank spacer
				lines = append(lines, textLine{name + " - " + strconv.Itoa(cell.Creature.Power)})
			}
		}

		// lineSpacing doubles the font height to leave a blank line between each
		// rendered text line.
		lineSpacing := basicFontH * 2
		tmpH := lineSpacing*len(lines) + basicFontH
		if tmpH < 1 {
			tmpH = 1
		}

		tmp := ebiten.NewImage(tmpW, tmpH)
		const marginX = 2
		for i, l := range lines {
			var tOpts text.DrawOptions
			tOpts.ColorScale.ScaleWithColor(color.Black)
			tOpts.GeoM.Translate(float64(marginX), float64(i*lineSpacing))
			text.Draw(tmp, l.s, hudFace, &tOpts)
		}

		// Blit the temporary text image into the cache, scaled up by hudTextScale.
		// FilterNearest preserves the crisp pixel edges of the bitmap font.
		// Text is already drawn in black so no ColorScale hack is needed.
		// startY is pushed down by several line-heights to clear the title image.
		startY := titleH + basicFontH*hudTextScale // ~3 line-heights gap below title
		var opts ebiten.DrawImageOptions
		opts.GeoM.Scale(float64(hudTextScale), float64(hudTextScale))
		opts.GeoM.Translate(0, float64(startY))
		opts.Filter = ebiten.FilterNearest
		g.hudImage.DrawImage(tmp, &opts)

		// Update last-rendered state.
		g.lastHUDScore = g.score
		g.lastHUDHoverCol = g.hoverCol
		g.lastHUDHoverRow = g.hoverRow
		g.lastHUDThemeName = themeName
	}

	// Every frame: blit the cached HUD image into the frame at the HUD strip origin.
	frame.DrawImage(g.hudImage, nil)
}

// drawOrientationDebug renders a one-line orientation/dimension readout in the
// bottom-left corner of the frame, gated by the showOrientationDebug const.
// It is intentionally small (1× basicfont, no scale) so it does not obscure gameplay.
func (g *Game) drawOrientationDebug(frame *ebiten.Image) {
	if !showOrientationDebug {
		return
	}
	orient := "L"
	if g.portrait {
		orient = "P"
	}
	angle := deviceOrientationAngle()
	angleStr := strconv.Itoa(angle)
	if angle < 0 {
		angleStr = "n/a"
	}
	line := fmt.Sprintf("%s %dx%d ang=%s sc=%.2f", orient, g.screenW, g.screenH, angleStr, g.renderScale)
	var opts text.DrawOptions
	opts.ColorScale.ScaleWithColor(color.RGBA{0xFF, 0xFF, 0x00, 0xFF})
	const marginBottom = 16
	frameH := frame.Bounds().Dy()
	opts.GeoM.Translate(2, float64(frameH-marginBottom))
	text.Draw(frame, line, hudFace, &opts)
}

// drawPerfDebug renders a 3x-scaled multi-line performance readout in the
// top-left corner of the frame, gated by the showPerfDebug const.  It reuses
// the package-level hudFace (basicfont.Face7x13) so there is no per-frame
// font allocation.  When showPerfDebug is false this method is never called.
//
// Worst-frame-time is tracked here because Draw is the measurement site:
// wall-clock time between consecutive Draw calls captures scheduler jank that
// ActualFPS (a 1-second average) smooths over.  A rolling 2-second window is
// maintained; the previous completed window's maximum is displayed so the
// readout is stable while the current window accumulates.
func (g *Game) drawPerfDebug(frame *ebiten.Image) {
	// --- worst-frame-time measurement ---
	now := time.Now()
	if !g.lastDrawTime.IsZero() {
		deltaMs := float64(now.Sub(g.lastDrawTime).Microseconds()) / 1000.0
		if deltaMs > g.worstFrameMs {
			g.worstFrameMs = deltaMs
		}

		// Sample min-FPS and max-sprite-count for the current window.
		fps := ebiten.ActualFPS()
		if g.minFpsWindow < 0 || fps < g.minFpsWindow {
			g.minFpsWindow = fps
		}
		if g.spritesThisFrame > g.maxSprWindow {
			g.maxSprWindow = g.spritesThisFrame
		}

		// Advance the 2-second window when it has elapsed.
		if g.worstWindowStart.IsZero() {
			g.worstWindowStart = now
		} else if now.Sub(g.worstWindowStart) >= 2*time.Second {
			g.prevWindowWorstMs = g.worstFrameMs
			g.prevWindowMinFps = g.minFpsWindow
			g.prevWindowMaxSpr = g.maxSprWindow
			g.worstFrameMs = deltaMs
			g.minFpsWindow = fps
			g.maxSprWindow = g.spritesThisFrame
			g.worstWindowStart = now
		}
	} else {
		g.worstWindowStart = now
		g.minFpsWindow = -1
	}
	g.lastDrawTime = now

	// --- build display lines ---
	line1 := fmt.Sprintf("fps %.0f  tps %.0f", ebiten.ActualFPS(), ebiten.ActualTPS())
	line2 := fmt.Sprintf("max %.1fms", g.prevWindowWorstMs)
	line3 := fmt.Sprintf("spr %d  bk %d", g.spritesThisFrame, g.bakeCount)
	line4 := fmt.Sprintf("lo %.0f  spr^ %d", g.prevWindowMinFps, g.prevWindowMaxSpr)

	const scale = 3.0
	const lineSpacing = 13 // basicfont.Face7x13 glyph height
	const padX = 4.0
	const padY = 4.0

	// Measure the widest line to size the backing rectangle.
	w1, _ := text.Measure(line1, hudFace, 0)
	w2, _ := text.Measure(line2, hudFace, 0)
	w3, _ := text.Measure(line3, hudFace, 0)
	w4, _ := text.Measure(line4, hudFace, 0)
	maxW := w1
	if w2 > maxW {
		maxW = w2
	}
	if w3 > maxW {
		maxW = w3
	}
	if w4 > maxW {
		maxW = w4
	}
	rectW := float32(maxW*scale + padX*2)
	rectH := float32((lineSpacing*4)*scale + padY*2)

	// Dark semi-transparent backing rect for legibility.
	vector.FillRect(frame, 0, 0, rectW, rectH, color.RGBA{0, 0, 0, 180}, false)

	// Draw each line scaled 3x, white, top-left.
	var opts text.DrawOptions
	opts.ColorScale.ScaleWithColor(color.RGBA{0xFF, 0xFF, 0xFF, 0xFF})
	for i, l := range []string{line1, line2, line3, line4} {
		opts.GeoM.Reset()
		opts.GeoM.Scale(scale, scale)
		y := padY + float64(i)*lineSpacing*scale + lineSpacing*scale
		opts.GeoM.Translate(padX, y)
		text.Draw(frame, l, hudFace, &opts)
	}
}

func main() {
	ebiten.SetWindowSize(windowWidth, windowHeight)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowTitle("go2d grid")
	ebiten.SetWindowClosingHandled(false)

	grid := NewGrid(gridCols, gridRows)

	game := newGameState(grid)
	game.animStart = time.Now()

	glyphFont, err := loadGlyphFont()
	if err != nil {
		log.Fatal("load glyph font:", err)
	}
	game.glyphFont = glyphFont

	// Allocate the frame and canvas before RunGame so that dimensions and
	// renderScale are set for the first LayoutF call.  Drawing/baking is deferred
	// to the first Update tick (initialPainted gate) so that ebiten.NewImage /
	// DrawRectShader / text.Draw run with a live graphics context rather than a
	// cold one that can produce blank sprites.
	//
	// The initial frame is sized to the presented resolution using the design
	// window dimensions as a proxy for the initial screen size.  LayoutF will
	// realloc when the real screen dims are known (first call), since the frame
	// dims will differ from the initial allocation whenever the screen is not
	// exactly windowWidth×windowHeight device pixels.
	//
	// The frame is [ HUD strip (hudDevW) | board (boardW) ] wide.
	// deviceCellSize is derived from boardW (not the full frame width).
	s := deviceScale()
	game.renderScale = s
	initSW := int(math.Round(float64(windowWidth) * s))
	initSH := int(math.Round(float64(windowHeight) * s))
	k := frameScaleFor(initSW, initSH, false) // initial: landscape assumed
	game.frameScale = k
	hudW := int(math.Round(float64(hudWidth) * k))
	boardW := int(math.Round(float64(windowWidth) * k))
	dh := int(math.Round(float64(windowHeight) * k))
	game.hudDevW = hudW
	game.deviceCellSize = deviceCellSizeFor(boardW)
	game.spriteRefSize = spriteRefSizeFor(game.deviceCellSize)
	game.frame = ebiten.NewImage(hudW+boardW, dh)
	game.canvas = ebiten.NewImage(boardW, dh)

	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}

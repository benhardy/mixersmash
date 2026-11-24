package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"math/rand"
	"runtime"
	"strconv"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	text "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/basicfont"
)

// gameState identifies which phase of the update/animation state machine the
// game is in. It is a distinct type so the dispatch switch and the state-aware
// animation helpers cannot be passed an arbitrary int.
type gameState int

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

	stateWaiting          gameState = 1
	stateShuffling        gameState = 2
	stateDropping         gameState = 3
	stateVictimMoving     gameState = 4
	stateColumnCollapsing gameState = 5

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
	showPerfDebug = false

	mixerHealthChangeOnClick = float32(-0.334) // each click reduces health
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
	// render resources (shaders and glyph font) — baked once, not game state.
	glowShader *ebiten.Shader
	gelShader  *ebiten.Shader
	glyphFont  *text.GoTextFaceSource

	// spriteCache holds the lazily-baked composite sprite per organism, keyed by
	// organism pointer. Cleared on a device-scale change (LayoutF) so sprites
	// rebake at the new size. This is a render cache, not game state.
	spriteCache map[*Organism]*ebiten.Image

	// board holds the logical game-board model: cell grid, live-cell count, score.
	board boardState

	// anim holds the animation/state-machine state: current phase, phase timing,
	// the in-flight action cell, and the active move/victim/target animations.
	anim animState

	// input holds pointer/touch tracking and hover-hint state derived from device input.
	input inputState

	// view holds device/presentation geometry and the offscreen render targets.
	view viewState

	// perf holds the per-frame performance counters for the showPerfDebug readout.
	perf perfCounters

	// hud holds the persistent HUD strip cache and its dirty-check state.
	hud hud
}

// boardState holds the logical game-board model: the cell grid, the live-cell
// count, and the cumulative score. It is the model layer that the update*
// methods mutate and the HUD and renderer read.
type boardState struct {
	grid      *Grid
	cellCount int // number of live cells currently in the grid
	score     int // cumulative score
}

// animState holds the update/animation state machine: the current phase, the
// wall-clock start of the active phase, the in-flight action cell, and the
// move/victim/target animation buffers. It is driven by the update* methods and
// consumed by the draw* methods.
type animState struct {
	currentState gameState  // which state are we in?
	animStart    time.Time  // when did the current animation phase start?
	actionCol    int        // the column an action is happening
	actionRow    int        // the row any given action is happening
	moveAnims    []MoveAnim // used for both shuffle and drop phases

	// victim movement animation fields (one entry per simultaneous victim)
	victimCount int
	victimAnims []MoveAnim

	// target cell bounce animation fields
	targetAnimating    bool
	targetAnimStart    time.Time
	targetAnimStartR   float32
	targetAnimPeakR    float32
	targetAnimEndR     float32
	targetAnimOrganism *Organism
}

// viewState holds the device/presentation geometry and offscreen render targets.
// Every field is derived from the device (screen size, DPI, orientation) in
// LayoutF and consumed by the Draw and coordinate-mapping paths.
type viewState struct {
	canvas *ebiten.Image
	frame  *ebiten.Image // landscape design-size offscreen frame; board renders here

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
}

// perfCounters holds the performance instrumentation for the showPerfDebug
// readout. Every field is written only on the Draw path and read only by
// drawPerfDebug; none carries game-logic meaning.
type perfCounters struct {
	// spritesThisFrame counts drawCompositeSprite blits in the current Draw
	// call; reset to 0 at the top of each Draw. bakeCount is cumulative:
	// incremented each time compositeCachedSprite bakes a new sprite on a cache
	// miss; never reset.
	spritesThisFrame int
	bakeCount        int

	// Worst-frame-time tracking. lastDrawTime records when the previous Draw
	// call started (zero = unset). worstFrameMs is the worst Draw-to-Draw delta
	// in the current 2s window. worstWindowStart marks when the window began.
	// prevWindowWorstMs is the worst value from the completed previous window,
	// displayed so the readout stays stable while the current window accumulates.
	lastDrawTime      time.Time
	worstFrameMs      float64
	worstWindowStart  time.Time
	prevWindowWorstMs float64

	// Min-FPS and max-sprite-count tracking over the same 2s window. minFpsWindow
	// is the lowest ActualFPS sample in the current window (-1 = no sample yet).
	// maxSprWindow is the highest spritesThisFrame. The prev* fields latch the
	// completed window's values for display.
	minFpsWindow     float64
	maxSprWindow     int
	prevWindowMinFps float64
	prevWindowMaxSpr int
}

// inputState holds the pointer/touch tracking and hover-hint state derived from
// device input each tick. It is written by the input-handling path (updateWaiting,
// updateHover) and read by the HUD and victim-line rendering.
type inputState struct {
	// hover hint fields
	hoverCol, hoverRow int      // cell under cursor (-1 = none)
	victimHints        [][2]int // closest-colour neighbours of hovered cell (empty = none)

	// touch tracking fields
	touchActive   bool
	activeTouchID ebiten.TouchID
	lastTouchX    int // last device-pixel X while the tracked finger was down (raw screen coords)
	lastTouchY    int // last device-pixel Y while the tracked finger was down (raw screen coords)

	// per-frame reuse buffers for touch ID slices (avoids per-frame allocation)
	touchIDsBuf     []ebiten.TouchID // reused for ebiten.AppendTouchIDs
	justPressedBuf  []ebiten.TouchID // reused for inpututil.AppendJustPressedTouchIDs
	justReleasedBuf []ebiten.TouchID // reused for inpututil.AppendJustReleasedTouchIDs
}

// newGameState returns a Game with data fields initialised but no ebiten.Image
// allocated.  Callers that need a canvas (i.e. main) must set g.canvas after
// calling this.
func newGameState(grid *Grid) *Game {
	return &Game{
		spriteCache: make(map[*Organism]*ebiten.Image),
		board: boardState{
			grid:      grid,
			cellCount: grid.Cols * grid.Rows,
		},
		anim: animState{
			currentState:    stateWaiting,
			actionCol:       -1,
			actionRow:       -1,
			victimAnims:     make([]MoveAnim, gridCols*gridRows), // flood can clear up to Cols*Rows cells
			victimCount:     0,
			targetAnimating: false,
		},
		input: inputState{
			hoverCol:        -1,
			hoverRow:        -1,
			touchIDsBuf:     make([]ebiten.TouchID, 0, 10),
			justPressedBuf:  make([]ebiten.TouchID, 0, 10),
			justReleasedBuf: make([]ebiten.TouchID, 0, 10),
		},
	}
}

// compositeCachedSprite returns a pre-rendered composite *ebiten.Image for o,
// building and caching it in g.spriteCache on first call.  The image carries a
// circa-2003 Apple Aqua jelly-button look rendered via the gel fragment shader:
//
//  1. Base image — an sz×sz opaque image filled with o.Color, then the organism
//     glyph o.Glyph (tinted glyphTint(o.Color)) drawn directly onto it.  The
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
// Returns nil if o is nil, the font is not loaded, or the gel shader fails to compile.
func (g *Game) compositeCachedSprite(o *Organism) *ebiten.Image {
	if o == nil || g.glyphFont == nil {
		return nil
	}
	if s := g.spriteCache[o]; s != nil {
		return s
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
	sz := g.view.spriteRefSize
	if sz <= 0 {
		sz = compositeSpriteRefSize
	}
	half := float32(sz) / 2
	clr := o.Color

	// ------------------------------------------------------------------ //
	// Step 1: build the base image — opaque fill colour + organism glyph.
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
	str := string(o.Glyph)
	w, h := text.Measure(str, face, 0)
	tint := GlyphTint(clr)
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

	g.perf.bakeCount++
	g.spriteCache[o] = composite
	return composite
}

// drawCompositeSprite blits the pre-rendered composite organism sprite for o
// centred at (cx, cy) scaled so its inscribed circle matches radius.  Does
// nothing if radius < 1, o is nil, or the sprite could not be baked.
func (g *Game) drawCompositeSprite(dst *ebiten.Image, cx, cy, radius float32, o *Organism) {
	if o == nil || radius < 1 {
		return
	}
	img := g.compositeCachedSprite(o)
	if img == nil {
		return
	}
	g.perf.spritesThisFrame++
	// Scale so the sprite's inscribed circle (diameter = sprite bake size)
	// matches the requested radius.
	refSz := g.view.spriteRefSize
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
	cell := &g.board.grid.Cells[col][row]
	cs := g.view.deviceCellSize
	csFloat := float32(cs)

	// Clear exactly the cell rect — full pixel replacement, not a blend.
	x0 := col * cs
	y0 := row * cs
	g.view.canvas.SubImage(image.Rect(x0, y0, x0+cs, y0+cs)).(*ebiten.Image).Clear()

	if cell.IsDead() {
		return
	}
	halfCs := csFloat / 2
	cx := float32(col*cs) + halfCs
	cy := float32(row*cs) + halfCs
	maxRadius := halfCs - 2
	radius := maxRadius * cellFullness * cell.Health
	g.drawCompositeSprite(g.view.canvas, cx, cy, radius, cell.Organism)
}

// emptyColumns returns the sorted indices of columns in which every cell has
// Health <= 0.
func (g *Game) emptyColumns() []int {
	var result []int
	for c := 0; c < g.board.grid.Cols; c++ {
		empty := true
		for r := 0; r < g.board.grid.Rows; r++ {
			if g.board.grid.Cells[c][r].Health > 0 {
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

// applyDropAndClear writes the gravity-compacted cells back into the grid and
// clears the vacated canvas rects. It must be called before buildDropMoveAnims
// so that the grid is in its final state when anims are built.
//
// col is the column being processed. holeRows is the slice of dead-cell row
// indices in that column. moves is the [][2]int output of gravityMoves.
// snapshot is a per-row copy of the column taken before any writes (aliasing
// safety). spawned is incremented for each new cell written (respawnCells path).
func (g *Game) applyDropAndClear(col int, holeRows []int, moves [][2]int, snapshot []Cell) (spawned int) {
	// Write moved alive cells to their new positions.
	for _, m := range moves {
		g.board.grid.Cells[col][m[1]] = snapshot[m[0]]
	}

	if respawnCells {
		// Write new random cells into the top slots (0..len(holeRows)-1).
		for i := 0; i < len(holeRows); i++ {
			g.board.grid.Cells[col][i] = Cell{Organism: PaletteOrganism(), Health: 1.0}
			spawned++
		}
	} else {
		// Zero the top slots that were vacated and are not being filled.
		for i := 0; i < len(holeRows); i++ {
			g.board.grid.Cells[col][i] = Cell{} // Health=0, invisible; canvas already cleared by vacating moves
		}
	}

	// Clear only the vacated canvas positions (clipped to canvas bounds).
	dcs := g.view.deviceCellSize
	cw := g.view.canvas.Bounds().Max.X // guard against canvas size mismatch
	ch := g.view.canvas.Bounds().Max.Y
	for _, m := range moves {
		x0, y0 := col*dcs, m[0]*dcs
		x1, y1 := x0+dcs, y0+dcs
		x1 = min(x1, cw)
		y1 = min(y1, ch)
		if x0 < x1 && y0 < y1 {
			g.view.canvas.SubImage(image.Rect(x0, y0, x1, y1)).(*ebiten.Image).Clear()
		}
	}
	return spawned
}

// buildDropMoveAnims appends MoveAnim entries for a single column after the
// grid has already been updated by applyDropAndClear. It does not touch the
// canvas.
//
// col is the column. holeRows is the slice of dead-cell row indices. moves is
// the [][2]int output of gravityMoves. dcs is g.deviceCellSize (passed in to
// avoid re-reading from g).
func (g *Game) buildDropMoveAnims(col int, holeRows []int, moves [][2]int, dcs int) {
	// Anims for moved alive cells (from their old row to their new row).
	for _, m := range moves {
		cell := g.board.grid.Cells[col][m[1]]
		fx, fy := DeviceCellCenter(col, m[0], dcs)
		tx, ty := DeviceCellCenter(col, m[1], dcs)
		g.anim.moveAnims = append(g.anim.moveAnims, MoveAnim{FromX: fx, FromY: fy, ToX: tx, ToY: ty, Cell: cell})
	}

	if respawnCells {
		// Anims for new cells animating in from above screen.
		for i := 0; i < len(holeRows); i++ {
			cell := g.board.grid.Cells[col][i]
			fx := float32(col*dcs + dcs/2)
			fy := float32(-dcs / 2)
			tx, ty := DeviceCellCenter(col, i, dcs)
			g.anim.moveAnims = append(g.anim.moveAnims, MoveAnim{FromX: fx, FromY: fy, ToX: tx, ToY: ty, Cell: cell})
			// The canvas position for row i was vacated by the moved cell above — already cleared.
		}
	}
}

// buildDropAnims computes MoveAnim entries for each affected column: alive cells
// compact downward, and new random cells fall from above the screen top.
// Only canvas positions that cells are vacating are cleared — not the whole column.
// Returns the number of new cells spawned (non-zero only when respawnCells is true).
func (g *Game) buildDropAnims(removedCols []int) int {
	if cap(g.anim.moveAnims) > 0 {
		g.anim.moveAnims = g.anim.moveAnims[:0]
	}
	spawned := 0
	var seen [gridCols]bool
	for _, col := range removedCols {
		if seen[col] {
			continue
		}
		seen[col] = true

		// Find which rows in this column are dead (holes just created this round).
		var holeRowsArr [gridRows]int
		holeRowsCount := 0
		for row := 0; row < g.board.grid.Rows; row++ {
			if g.board.grid.Cells[col][row].Health <= 0 {
				holeRowsArr[holeRowsCount] = row
				holeRowsCount++
			}
		}
		holeRows := holeRowsArr[:holeRowsCount]
		if len(holeRows) == 0 {
			continue
		}

		moves := GravityMoves(g.board.grid.Rows, holeRows)

		// Take a snapshot of the column before any writes (aliasing safety).
		snapshot := make([]Cell, g.board.grid.Rows)
		for row := 0; row < g.board.grid.Rows; row++ {
			snapshot[row] = g.board.grid.Cells[col][row]
		}

		spawned += g.applyDropAndClear(col, holeRows, moves, snapshot)
		g.buildDropMoveAnims(col, holeRows, moves, g.view.deviceCellSize)
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
	moves := HorizontalCollapseMoves(g.board.grid.Cols, emptyCols)
	if len(moves) == 0 {
		return false
	}

	// Snapshot entire Cells slice to avoid aliasing during column moves.
	snapshot := make([][]Cell, g.board.grid.Cols)
	for c := 0; c < g.board.grid.Cols; c++ {
		snapshot[c] = make([]Cell, g.board.grid.Rows)
		copy(snapshot[c], g.board.grid.Cells[c])
	}

	// Apply the pure column reassignment.
	newCells := ApplyColumnCollapse(snapshot, moves)
	for c := 0; c < g.board.grid.Cols; c++ {
		copy(g.board.grid.Cells[c], newCells[c])
	}

	// Clear canvas strips for every column that is a move source OR destination.
	// During the slide, stale painted cells at both source and destination positions
	// would show through under the animating sprites, causing visible doubling.
	// Columns not involved in any move are untouched.
	dcs := g.view.deviceCellSize
	cw := g.view.canvas.Bounds().Max.X
	ch := g.view.canvas.Bounds().Max.Y
	var clearColSet [gridCols]bool
	for _, m := range moves {
		clearColSet[m[0]] = true
		clearColSet[m[1]] = true
	}
	for col, marked := range clearColSet {
		if !marked {
			continue
		}
		x0 := col * dcs
		x1 := x0 + dcs
		x1 = min(x1, cw)
		if x0 < x1 {
			g.view.canvas.SubImage(image.Rect(x0, 0, x1, ch)).(*ebiten.Image).Clear()
		}
	}

	// Build horizontal MoveAnims.
	if cap(g.anim.moveAnims) > 0 {
		g.anim.moveAnims = g.anim.moveAnims[:0]
	}
	for _, m := range moves {
		oldCol, newCol := m[0], m[1]
		for r := 0; r < g.board.grid.Rows; r++ {
			cell := g.board.grid.Cells[newCol][r]
			if cell.Health <= 0 {
				continue
			}
			fx, fy := DeviceCellCenter(oldCol, r, dcs)
			tx, ty := DeviceCellCenter(newCol, r, dcs)
			g.anim.moveAnims = append(g.anim.moveAnims, MoveAnim{FromX: fx, FromY: fy, ToX: tx, ToY: ty, Cell: cell})
		}
	}

	g.anim.animStart = time.Now()
	g.anim.currentState = stateColumnCollapsing
	return true
}

// repaintGrid redraws every cell from the model onto the offscreen canvas,
// fully re-syncing canvas state with the grid. Dead cells are cleared to blank.
// This is called only at settle points (no animation is in flight).
func (g *Game) repaintGrid() {
	for c := 0; c < g.board.grid.Cols; c++ {
		for r := 0; r < g.board.grid.Rows; r++ {
			g.drawCell(c, r, 1.0)
		}
	}
}

// enterWaiting repaints the full grid canvas from the model and transitions to
// stateWaiting. All settle-to-waiting paths must go through this helper so
// canvas drift is corrected on every settle.
func (g *Game) enterWaiting() {
	g.repaintGrid()
	g.anim.currentState = stateWaiting
	// Force a GC collection while the screen is static between moves, so the
	// collector does not trigger mid-animation and cause visible frame spikes.
	runtime.GC()
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
	elapsed := ElapsedFraction(g.anim.animStart, time.Now(), dropDuration)
	if elapsed >= 1.0 {
		dcs := g.view.deviceCellSize
		for _, anim := range g.anim.moveAnims {
			col := int(anim.ToX) / dcs
			row := int(anim.ToY) / dcs
			if col >= 0 && col < g.board.grid.Cols && row >= 0 && row < g.board.grid.Rows {
				g.drawCell(col, row, 1.0)
			}
		}
		g.anim.moveAnims = nil
		g.enterWaiting()
	}
	return nil
}

// buildShuffleAnims gathers all alive 8-neighbours of the Mixer at (col, row),
// Fisher-Yates shuffles their positions, writes the permuted cells back into
// the grid, clears their canvas rects, and sets up bezier MoveAnim entries.
// Transitions to stateShuffling.
// Also makes an animation for the mixer itself, which is not moved but is drawn
// with a size interpolation and bounce to indicate it was clicked.
// oldMixerHealth is the Mixer's health before this click; newMixerHealth is after.
func (g *Game) buildShuffleAnims(col, row int, oldMixerHealth float32, newMixerHealth float32) {
	dcs := g.view.deviceCellSize
	if dcs <= 0 {
		dcs = cellSize
	}

	var nbuf [8][2]int
	neighbors := eightNeighbors(nbuf[:0], col, row, g.board.grid.Cols, g.board.grid.Rows)

	// Collect alive neighbours.
	var aliveNeighbors [][2]int
	for _, n := range neighbors {
		if !g.board.grid.Cells[n[0]][n[1]].IsDead() {
			aliveNeighbors = append(aliveNeighbors, n)
		}
	}
	mixerAnim := MoveAnim{
		FromX:     float32(col*dcs + dcs/2),
		FromY:     float32(row*dcs + dcs/2),
		ToX:       float32(col*dcs + dcs/2),
		ToY:       float32(row*dcs + dcs/2),
		CtrlX:     float32(col*dcs + dcs/2),
		CtrlY:     float32(row*dcs + dcs/2),
		Cell:      Cell{Organism: g.board.grid.Cells[col][row].Organism, Health: oldMixerHealth},
		NewHealth: newMixerHealth,
		PathType:  0, // stationary path
		Rotations: 1, // reserved for future spin; currently unused
	}
	anims := make([]MoveAnim, len(aliveNeighbors)+1) // +1 for the mixer itself
	anims[0] = mixerAnim
	if len(aliveNeighbors) > 0 {
		// Fisher-Yates shuffle the positions (perm[i] = destination index for neighbour i).
		perm := rand.Perm(len(aliveNeighbors))

		// Snapshot cells before modifying grid.
		type neighborCell struct {
			col, row int
			cell     Cell
		}
		orig := make([]neighborCell, len(aliveNeighbors))
		for i, n := range aliveNeighbors {
			orig[i] = neighborCell{n[0], n[1], g.board.grid.Cells[n[0]][n[1]]}
		}

		// Build MoveAnims: neighbour i animates from its current centre to perm[i]'s centre.
		for i := range aliveNeighbors {
			src := orig[i]
			dst := aliveNeighbors[perm[i]]
			fx, fy := DeviceCellCenter(src.col, src.row, dcs)
			tx, ty := DeviceCellCenter(dst[0], dst[1], dcs)
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
			anims[i+1] = MoveAnim{FromX: fx, FromY: fy, ToX: tx, ToY: ty, CtrlX: ctrlX, CtrlY: ctrlY, Cell: src.cell}
		}

		// Clear source positions on canvas (they will be drawn as moving overlays).
		for _, n := range aliveNeighbors {
			x0, y0 := n[0]*dcs, n[1]*dcs
			g.view.canvas.SubImage(image.Rect(x0, y0, x0+dcs, y0+dcs)).(*ebiten.Image).Clear()
		}

		// Write cells to their permuted destinations in the grid now (canvas repaint on settle).
		for i := range aliveNeighbors {
			dst := aliveNeighbors[perm[i]]
			g.board.grid.Cells[dst[0]][dst[1]] = orig[i].cell
		}
	}
	g.anim.moveAnims = anims
	g.anim.animStart = time.Now()
	g.anim.currentState = stateShuffling
}

// updateShuffling waits for the shuffle animation to complete. For a non-killing
// click it repaints the full grid canvas and returns to stateWaiting. For a
// killing click (the action cell IsDead() after BumpHealth reached 0) it routes
// into the drop pipeline so the dead Mixer's column collapses.
func (g *Game) updateShuffling() error {
	elapsed := ElapsedFraction(g.anim.animStart, time.Now(), shuffleDuration)
	if elapsed < 1.0 {
		return nil
	}
	g.anim.moveAnims = nil
	// Check whether the mixer at the action position was killed this click.
	killed := g.anim.actionCol >= 0 &&
		g.anim.actionCol < g.board.grid.Cols &&
		g.anim.actionRow >= 0 &&
		g.anim.actionRow < g.board.grid.Rows &&
		g.board.grid.Cells[g.anim.actionCol][g.anim.actionRow].IsDead()
	if !killed {
		g.enterWaiting()
		return nil
	}
	// Route the dead column through the drop/collapse pipeline.
	// No flood score is added for a Mixer removal.
	//
	// Repaint all columns that were part of the shuffle before entering the drop.
	// buildShuffleAnims cleared the canvas rects for all shuffled neighbours
	// (across potentially multiple columns), and those blank rects persist on the
	// canvas.  buildDropAnims will immediately re-clear the source rows of the
	// falling cells in actionCol, so painting them first is harmless.  Without
	// this repaint the non-actionCol neighbours stay blank throughout the drop.
	g.repaintGrid()
	spawned := g.buildDropAnims([]int{g.anim.actionCol})
	if respawnCells {
		g.board.cellCount += spawned
	}
	if len(g.anim.moveAnims) > 0 {
		g.anim.animStart = time.Now()
		g.anim.currentState = stateDropping
	} else {
		g.settleAfterRemoval()
	}
	return nil
}

func (g *Game) commitMixer(cell *Cell, col int, row int) error {
	oldHealth := cell.BumpHealth(mixerHealthChangeOnClick)
	if cell.IsDead() {
		g.board.cellCount--
	}
	g.drawCell(col, row, 1.0)
	// Record the action position so updateShuffling can detect a killing click.
	g.anim.actionCol = col
	g.anim.actionRow = row
	// Show new size, shuffle neighbours.  On the killing click the mixer
	// shrinks to 0 during this animation; updateShuffling will route to the
	// drop pipeline when it detects IsDead() at the action position.
	g.buildShuffleAnims(col, row, oldHealth, cell.Health)
	return nil
}

// commitCellAt clears the flood-match cluster rooted at an animal cell through
// the victim-shrink→drop pipeline. Clicking a non-animal, empty, or dead cell
// is a no-op. Out-of-bounds clicks are ignored.
func (g *Game) commitCellAt(col, row int) error {
	if col < 0 || col >= g.board.grid.Cols || row < 0 || row >= g.board.grid.Rows {
		return nil
	}
	cell := &g.board.grid.Cells[col][row]
	if cell.IsDead() {
		return nil
	}

	// Mixer click path: increment click count and either shuffle or remove.
	if cell.Organism != nil && cell.Organism.IsMixer {
		return g.commitMixer(cell, col, row)
	}

	if cell.Organism == nil || !cell.Organism.IsAnimal {
		return nil
	}

	coords := FloodMatchCluster(g.board.grid.Cells, col, row, g.board.grid.Cols, g.board.grid.Rows)

	// No-op if the cluster contains no food: a lone animal (or animals-only
	// cluster with no paired food) must not clear.
	if !ClusterHasFood(g.board.grid.Cells, coords) {
		return nil
	}

	g.anim.actionCol = col
	g.anim.actionRow = row
	g.anim.victimCount = 0

	dcs := g.view.deviceCellSize
	if dcs <= 0 {
		dcs = cellSize
	}

	seedCenterX, seedCenterY := DeviceCellCenter(col, row, dcs)

	for _, coord := range coords {
		c, r := coord[0], coord[1]
		organism := g.board.grid.Cells[c][r].Organism
		oldHealth := g.board.grid.Cells[c][r].Health
		g.board.grid.Cells[c][r].Health = 0
		g.drawCell(c, r, 1.0)
		g.board.cellCount--

		fromX, fromY := DeviceCellCenter(c, r, dcs)

		var v MoveAnim
		if c == col && r == row {
			// Seed cell: shrinks in place (stationary).
			v = MoveAnim{
				Col:       c,
				Row:       r,
				Cell:      Cell{Organism: organism, Health: oldHealth},
				FromX:     fromX,
				FromY:     fromY,
				ToX:       fromX,
				ToY:       fromY,
				PathType:  0,
				Rotations: 0,
			}
		} else {
			// Non-seed: animate toward the seed using spiral/curved path.
			pt, cx, cy, st := victimPathParams(fromX, fromY, seedCenterX, seedCenterY)
			// Clear victim's canvas position (it is now an overlay animation).
			x0, y0 := c*dcs, r*dcs
			g.view.canvas.SubImage(image.Rect(x0, y0, x0+dcs, y0+dcs)).(*ebiten.Image).Clear()
			v = MoveAnim{
				Col:         c,
				Row:         r,
				Cell:        Cell{Organism: organism, Health: oldHealth},
				FromX:       fromX,
				FromY:       fromY,
				ToX:         seedCenterX,
				ToY:         seedCenterY,
				PathType:    pt,
				CtrlX:       cx,
				CtrlY:       cy,
				SpiralTurns: st,
				Rotations:   0,
			}
		}
		g.anim.victimAnims[g.anim.victimCount] = v
		g.anim.victimCount++
	}

	g.anim.animStart = time.Now()
	g.anim.currentState = stateVictimMoving
	return nil
}

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
	cell := g.board.grid.Cells[hc][hr]
	if cell.IsDead() {
		return
	}
	g.input.hoverCol, g.input.hoverRow = hc, hr
	if cell.Organism == nil || !cell.Organism.IsAnimal {
		g.input.victimHints = g.input.victimHints[:0]
		return
	}
	cluster := FloodMatchCluster(g.board.grid.Cells, hc, hr, g.board.grid.Cols, g.board.grid.Rows)
	if ClusterHasFood(g.board.grid.Cells, cluster) {
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

// updateVictimMoving advances the victim movement animation and transitions to
// stateDropping (or stateWaiting) when the animation completes.
func (g *Game) updateVictimMoving() error {
	t := ElapsedFraction(g.anim.animStart, time.Now(), victimDuration)
	if t < 1.0 {
		return nil
	}
	// Animation complete — score the clear and trigger drop.
	// All victim cells already have Health=0 (set in commitCellAt).
	g.board.score += ScoreForCluster(g.anim.victimCount)
	var colSeen [gridCols]bool
	colSeen[g.anim.actionCol] = true
	for i := 0; i < g.anim.victimCount; i++ {
		v := g.anim.victimAnims[i]
		colSeen[v.Col] = true
	}
	var affectedColsArr [gridCols]int
	affectedColsCount := 0
	for c := 0; c < gridCols; c++ {
		if colSeen[c] {
			affectedColsArr[affectedColsCount] = c
			affectedColsCount++
		}
	}
	g.anim.victimCount = 0
	spawned := g.buildDropAnims(affectedColsArr[:affectedColsCount])
	if respawnCells {
		g.board.cellCount += spawned
	}
	if len(g.anim.moveAnims) > 0 {
		g.anim.animStart = time.Now()
		g.anim.currentState = stateDropping
	} else {
		g.settleAfterRemoval()
	}
	return nil
}

// updateDropping waits for the drop animation to complete, then commits final
// cell positions and returns to stateWaiting (or stateColumnCollapsing when
// empty columns remain after the drop settles).
func (g *Game) updateDropping() error {
	elapsed := ElapsedFraction(g.anim.animStart, time.Now(), dropDuration)
	if elapsed < 1.0 {
		return nil
	}
	dcs := g.view.deviceCellSize
	for _, anim := range g.anim.moveAnims {
		col := int(anim.ToX) / dcs
		row := int(anim.ToY) / dcs
		if col >= 0 && col < g.board.grid.Cols && row >= 0 && row < g.board.grid.Rows {
			g.drawCell(col, row, 1.0)
		}
	}
	g.anim.moveAnims = nil
	g.settleAfterRemoval()
	return nil
}

// Update handles input each tick.
func (g *Game) Update() error {
	// First in-loop frame: bake all sprites with a live graphics context so
	// that ebiten.NewImage / DrawRectShader / text.Draw have a valid backend.
	// This must run before any state logic so the canvas is fully painted
	// before the first Draw call.
	if !g.view.initialPainted {
		g.repaintGrid()
		g.view.initialPainted = true
	}

	ebiten.SetWindowTitle(fmt.Sprintf("MixerSmash — cells: %d  score: %d", g.board.cellCount, g.board.score))

	// Reset hover every tick; recompute below if conditions are met.
	g.input.hoverCol, g.input.hoverRow = -1, -1
	g.input.victimHints = g.input.victimHints[:0]

	if g.anim.targetAnimating {
		if ElapsedFraction(g.anim.targetAnimStart, time.Now(), targetAnimDuration) >= 1.0 {
			g.anim.targetAnimating = false
			g.drawCell(g.anim.actionCol, g.anim.actionRow, 1.0)
		}
	}

	switch g.anim.currentState {
	case stateWaiting:
		return g.updateWaiting()
	case stateShuffling:
		return g.updateShuffling()
	case stateVictimMoving:
		return g.updateVictimMoving()
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
	g.perf.spritesThisFrame = 0

	if g.glowShader == nil {
		s, err := ebiten.NewShader([]byte(glowShaderSrc))
		if err != nil {
			log.Fatal("glow shader compile error:", err)
		}
		g.glowShader = s
	}

	// Render the board into the landscape frame image.
	// Guard: frame may be nil on the very first frame before LayoutF runs.
	if g.view.frame == nil {
		screen.Fill(color.Black)
		return
	}
	g.view.frame.Fill(color.Black)

	// Board destination: the right portion of the frame, offset by hudDeviceW.
	// We draw directly into g.frame using an explicit x-offset rather than a
	// SubImage, because Ebiten SubImage-as-destination clips but does NOT
	// translate the origin — board-local coords at x=0 would land at x=0 in
	// the frame and be hidden under the HUD strip.
	frameH := g.view.frame.Bounds().Dy()
	frameW := g.view.frame.Bounds().Dx()
	hudDevW := g.view.hudDevW

	g.drawCanvas(g.view.frame, hudDevW)
	g.drawVictimLine(g.view.frame, hudDevW)
	g.drawTargetBounce(g.view.frame, hudDevW)
	g.drawMoveAnims(g.view.frame, hudDevW)

	// Draw the HUD strip in the left portion of the frame.
	g.hud.draw(g.view.frame, hudDevW, g.board.score, g.input.hoverCol, g.input.hoverRow, g.board.grid)

	// Draw orientation debug readout (gated by showOrientationDebug const).
	if showOrientationDebug {
		g.drawOrientationDebug(g.view.frame)
	}

	// Draw performance readout (gated by showPerfDebug const).
	if showPerfDebug {
		g.drawPerfDebug(g.view.frame)
	}

	// Present the frame onto the actual screen.
	screen.Fill(color.Black)
	fw := float64(frameW)
	fh := float64(frameH)
	sw := float64(g.view.screenW)
	sh := float64(g.view.screenH)
	if sw <= 0 {
		// LayoutF not yet called; fall back to screen bounds.
		sw = float64(screen.Bounds().Dx())
		sh = float64(screen.Bounds().Dy())
	}
	var opts ebiten.DrawImageOptions
	opts.GeoM = presentGeoM(g.view.portrait, sw, sh, fw, fh)
	opts.Filter = ebiten.FilterLinear
	screen.DrawImage(g.view.frame, &opts)
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
	if g.input.hoverCol == -1 || len(g.input.victimHints) == 0 {
		return
	}
	dcs := g.view.deviceCellSize
	if dcs <= 0 {
		dcs = cellSize
	}
	s := g.view.renderScale
	if s <= 0 {
		s = 1.0
	}
	xOff := float32(xOffset)
	toX, toY := DeviceCellCenter(g.input.hoverCol, g.input.hoverRow, dcs)
	// Dot spacing and radius scale with device scale so they appear the same
	// physical size on all displays.
	dotSpacing := float32(8) * float32(s)
	dotRadius := float32(4.0) * float32(s)
	// Offset advances at 20 logical px/sec = 20*s device px/sec = 20*s/60 device px/tick.
	rawOff := float64(ebiten.Tick()) * (20.0 * s / 60.0)
	offset := float32(math.Mod(rawOff, float64(dotSpacing)))
	for _, hint := range g.input.victimHints {
		hc, hr := hint[0], hint[1]
		fromX, fromY := DeviceCellCenter(hc, hr, dcs)
		dx := toX - fromX
		dy := toY - fromY
		length := float32(math.Sqrt(float64(dx*dx + dy*dy)))
		if length <= 0 {
			continue
		}
		ux := dx / length
		uy := dy / length
		vc := g.board.grid.Cells[hc][hr].Color()
		dotCol := color.RGBA{vc.R, vc.G, vc.B, 255}
		for pos := offset; pos < length; pos += dotSpacing {
			vector.FillCircle(dst, xOff+fromX+ux*pos, fromY+uy*pos, dotRadius, dotCol, true)
		}
	}
}

// drawCanvas blits the persistent offscreen canvas into dst at an explicit
// x-offset so board-local pixel coords land at the right position in the frame.
func (g *Game) drawCanvas(dst *ebiten.Image, xOffset int) {
	if dst == nil || g.view.canvas == nil {
		return
	}
	var opts ebiten.DrawImageOptions
	opts.GeoM.Translate(float64(xOffset), 0)
	dst.DrawImage(g.view.canvas, &opts)
}

// drawTargetBounce renders the target cell bounce animation.
// xOffset is added to the board-local X coordinate before drawing into dst.
func (g *Game) drawTargetBounce(dst *ebiten.Image, xOffset int) {
	if !g.anim.targetAnimating {
		return
	}
	t := ElapsedFraction(g.anim.targetAnimStart, time.Now(), targetAnimDuration)
	t = min(t, 1.0)
	var r float32
	if t < 0.5 {
		lt := t * 2              // 0→1 in phase 1
		eased := easeOutQuad(lt) // ease-out: fast start, slow at peak
		r = lerp(g.anim.targetAnimStartR, g.anim.targetAnimPeakR, eased)
	} else {
		lt := (t - 0.5) * 2     // 0→1 in phase 2
		eased := easeInQuad(lt) // ease-in: slow at peak, fast at end
		r = lerp(g.anim.targetAnimPeakR, g.anim.targetAnimEndR, eased)
	}
	dcs := g.view.deviceCellSize
	if dcs <= 0 {
		dcs = cellSize
	}
	cx, cy := DeviceCellCenter(g.anim.actionCol, g.anim.actionRow, dcs)
	if r > 0 {
		g.drawCompositeSprite(dst, cx+float32(xOffset), cy, r, g.anim.targetAnimOrganism)
	}
}

// drawMoveAnims renders in-motion cells for shuffle, drop, and victim-move animations.
// xOffset is added to all board-local X coordinates before drawing into dst.
func (g *Game) drawMoveAnims(dst *ebiten.Image, xOffset int) {
	xOff := float32(xOffset)
	if len(g.anim.moveAnims) > 0 {
		dur := moveAnimDuration(g.anim.currentState)
		t := ElapsedFraction(g.anim.animStart, time.Now(), dur)
		t = min(t, 1.0)
		dcs := g.view.deviceCellSize
		if dcs <= 0 {
			dcs = cellSize
		}
		maxR := float32(dcs/2 - 2)
		for _, anim := range g.anim.moveAnims {
			cx, cy := moveAnimPos(anim, g.anim.currentState, t)
			var r float32
			if g.anim.currentState == stateShuffling && anim.Rotations != 0 {
				// Clicked-mixer entry only: interpolate radius from old health to new health
				// with a gentle bounce envelope so the shrink is visually sprung.
				// Identified by Rotations!=0 (set to 1 only on the prepended mixer entry in
				// buildShuffleAnims); neighbour Mixers have Rotations==0 and fall through
				// to the standard radius path below. Works for NewHealth==0 (final click).
				// anim.Cell.Health = pre-click health; anim.NewHealth = post-click health.
				et := easeInOutQuad(t)
				base := lerp(anim.Cell.Health, anim.NewHealth, et)
				// Bounce: gentle spring envelope peaks at t=0.5, amplitude 15%.
				bounce := base * (1 + 0.15*float32(math.Sin(float64(t)*math.Pi)))
				r = maxR * bounce
			} else {
				r = maxR * anim.Cell.Health
			}
			if r > 0 {
				g.drawCompositeSprite(dst, xOff+cx, cy, r, anim.Cell.Organism)
			}
		}
	}

	if g.anim.currentState == stateVictimMoving {
		t := ElapsedFraction(g.anim.animStart, time.Now(), victimDuration)
		t = min(t, 1.0)
		for i := 0; i < g.anim.victimCount; i++ {
			v := g.anim.victimAnims[i]
			cx, cy := victimAnimPos(v, t)
			dcs2 := g.view.deviceCellSize
			if dcs2 <= 0 {
				dcs2 = cellSize
			}
			radius := float32(dcs2/2-2) * v.Cell.Health * (1 - t)
			if radius > 0 {
				g.drawCompositeSprite(dst, xOff+cx, cy, radius, v.Cell.Organism)
			}
		}
	}
}

// drawOrientationDebug renders a one-line orientation/dimension readout in the
// bottom-left corner of the frame, gated by the showOrientationDebug const.
// It is intentionally small (1× basicfont, no scale) so it does not obscure gameplay.
func (g *Game) drawOrientationDebug(frame *ebiten.Image) {
	if !showOrientationDebug {
		return
	}
	orient := "L"
	if g.view.portrait {
		orient = "P"
	}
	angle := deviceOrientationAngle()
	angleStr := strconv.Itoa(angle)
	if angle < 0 {
		angleStr = "n/a"
	}
	line := fmt.Sprintf("%s %dx%d ang=%s sc=%.2f", orient, g.view.screenW, g.view.screenH, angleStr, g.view.renderScale)
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
	if !g.perf.lastDrawTime.IsZero() {
		deltaMs := float64(now.Sub(g.perf.lastDrawTime).Microseconds()) / 1000.0
		if deltaMs > g.perf.worstFrameMs {
			g.perf.worstFrameMs = deltaMs
		}

		// Sample min-FPS and max-sprite-count for the current window.
		fps := ebiten.ActualFPS()
		if g.perf.minFpsWindow < 0 || fps < g.perf.minFpsWindow {
			g.perf.minFpsWindow = fps
		}
		if g.perf.spritesThisFrame > g.perf.maxSprWindow {
			g.perf.maxSprWindow = g.perf.spritesThisFrame
		}

		// Advance the 2-second window when it has elapsed.
		if g.perf.worstWindowStart.IsZero() {
			g.perf.worstWindowStart = now
		} else if now.Sub(g.perf.worstWindowStart) >= 2*time.Second {
			g.perf.prevWindowWorstMs = g.perf.worstFrameMs
			g.perf.prevWindowMinFps = g.perf.minFpsWindow
			g.perf.prevWindowMaxSpr = g.perf.maxSprWindow
			g.perf.worstFrameMs = deltaMs
			g.perf.minFpsWindow = fps
			g.perf.maxSprWindow = g.perf.spritesThisFrame
			g.perf.worstWindowStart = now
		}
	} else {
		g.perf.worstWindowStart = now
		g.perf.minFpsWindow = -1
	}
	g.perf.lastDrawTime = now

	// --- build display lines ---
	line1 := fmt.Sprintf("fps %.0f  tps %.0f", ebiten.ActualFPS(), ebiten.ActualTPS())
	line2 := fmt.Sprintf("max %.1fms", g.perf.prevWindowWorstMs)
	line3 := fmt.Sprintf("spr %d  bk %d", g.perf.spritesThisFrame, g.perf.bakeCount)
	line4 := fmt.Sprintf("lo %.0f  spr^ %d", g.perf.prevWindowMinFps, g.perf.prevWindowMaxSpr)

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
	ebiten.SetWindowSize(hudWidth+windowWidth, windowHeight)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowTitle("go2d grid")
	ebiten.SetWindowClosingHandled(false)

	grid := NewGrid(gridCols, gridRows)

	game := newGameState(grid)
	game.anim.animStart = time.Now()

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
	game.view.renderScale = s
	initSW := int(math.Round(float64(windowWidth) * s))
	initSH := int(math.Round(float64(windowHeight) * s))
	k := frameScaleFor(initSW, initSH, false) // initial: landscape assumed
	game.view.frameScale = k
	hudW := int(math.Round(float64(hudWidth) * k))
	boardW := int(math.Round(float64(windowWidth) * k))
	dh := int(math.Round(float64(windowHeight) * k))
	game.view.hudDevW = hudW
	game.view.deviceCellSize = deviceCellSizeFor(boardW)
	game.view.spriteRefSize = spriteRefSizeFor(game.view.deviceCellSize)
	game.view.frame = ebiten.NewImage(hudW+boardW, dh)
	game.view.canvas = ebiten.NewImage(boardW, dh)

	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}

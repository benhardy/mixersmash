package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"math/rand"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	text "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	windowWidth  = 800
	windowHeight = 600
	cellSize     = 64
	gridCols     = windowWidth / cellSize  // 12
	gridRows     = windowHeight / cellSize // 9

	stateWaiting          = 1
	stateShuffling        = 2
	stateDropping         = 3
	stateVictimMoving     = 4
	stateColumnCollapsing = 5

	shuffleTicks    = 18        // 0.3s at 60tps
	dropTicks       = 18        // 0.3s at 60tps
	victimTicks     = int64(24) // 0.4s at 60tps
	targetAnimTicks = int64(15) // 0.25s at 60 TPS

	healthDecrement = float32(1.0 / 3.0)
	healthMin       = float32(0.2)
	healthEpsilon   = float32(0.001)

	respawnCells = false // set false to leave gaps at top instead of spawning new cells
)

// compositeSpriteRefSize is the logical fallback side length used only before
// the first LayoutF call sets g.spriteRefSize.  At runtime, spriteRefSize is
// derived from deviceCellSize*2 (capped at 512) and updated on scale changes.
const compositeSpriteRefSize = 120

// Game is the Ebitengine game state.
type Game struct {
	grid         *Grid
	canvas       *ebiten.Image
	currentState int        // which state are we in?
	animStart    int64      // when did the current animation phase start?
	actionCol    int        // the column an action is happening
	actionRow    int        // the row any given action is happening
	moveAnims    []MoveAnim // used for both shuffle and drop phases
	glowShader   *ebiten.Shader
	gelShader    *ebiten.Shader
	cellCount    int        // number of live cells currently in the grid
	score        int        // cumulative score
	targetColor  color.RGBA // colour of the most recently killed target cell

	// hover hint fields
	hoverCol, hoverRow int      // cell under cursor (-1 = none)
	victimHints        [][2]int // closest-colour neighbours of hovered cell (empty = none)

	// victim movement animation fields (one entry per simultaneous victim)
	victims []VictimAnim

	// target cell bounce animation fields
	targetAnimating  bool
	targetAnimStart  int64
	targetAnimStartR float32
	targetAnimPeakR  float32
	targetAnimEndR   float32
	targetAnimColor  color.RGBA

	// glyph rendering
	glyphFont  *text.GoTextFaceSource
	glyphCache map[color.RGBA]*ebiten.Image

	// HiDPI render scale — set once per LayoutF call; derived from
	// ebiten.Monitor().DeviceScaleFactor().
	renderScale    float64 // current device scale factor (1.0 on standard displays)
	deviceCellSize int     // device-pixel cell size, derived from rounded canvas dims
	spriteRefSize  int     // device-pixel sprite bake diameter (≤512)

	// initialPainted is false until the first in-loop frame, at which point
	// repaintGrid is called to bake all sprites with a live graphics context.
	initialPainted bool
}

// newGameState returns a Game with data fields initialised but no ebiten.Image
// allocated.  Callers that need a canvas (i.e. main) must set g.canvas after
// calling this.
func newGameState(grid *Grid) *Game {
	return &Game{
		grid:            grid,
		currentState:    stateWaiting,
		animStart:       0,
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

// compositeCachedSprite returns a pre-rendered composite *ebiten.Image for clr,
// building and caching it on first call.  The image carries a circa-2003 Apple
// Aqua jelly-button look rendered via the gel fragment shader:
//
//  1. Base image — an sz×sz opaque image filled with clr, then the creature
//     glyph (tinted darkerShade(clr)) drawn directly onto it.  The base image
//     is fully opaque so its rgb values equal straight (non-premultiplied) rgb.
//  2. Gel shader pass — gelShaderSrc runs via DrawRectShader, sampling the base
//     image (Images[0]) at each pixel to obtain the local colour, then applies
//     the full Aqua gel lighting (vertical gradient, bottom caustic, gloss
//     highlight, rim) over that sampled colour.  The shader owns the circular
//     AA edge and outputs premultiplied vec4(col*alpha, alpha).
//
// The glyph is embedded under the gel lighting (gradient, gloss, rim all apply
// over it), giving an inset "carved into jelly" appearance.
//
// Returns nil if the gel shader could not be compiled, the font is not loaded,
// or no glyph is mapped to clr.
func (g *Game) compositeCachedSprite(clr color.RGBA) *ebiten.Image {
	if g.glyphFont == nil {
		return nil
	}
	glyph := glyphForColor(clr)
	if glyph == 0 {
		return nil
	}
	if g.glyphCache == nil {
		g.glyphCache = make(map[color.RGBA]*ebiten.Image, len(creaturePalette))
	}
	if img, ok := g.glyphCache[clr]; ok {
		return img
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
	str := string(glyph)
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

	g.glyphCache[clr] = composite
	return composite
}

// drawCompositeSprite blits the pre-rendered composite creature sprite for clr
// centred at (cx, cy) scaled so its inscribed circle matches radius.  Does
// nothing if radius < 1, the font is not loaded, or no glyph is mapped to clr.
func (g *Game) drawCompositeSprite(dst *ebiten.Image, cx, cy, radius float32, clr color.RGBA) {
	if radius < 1 {
		return
	}
	img := g.compositeCachedSprite(clr)
	if img == nil {
		return
	}
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
	clr := cell.Color()
	g.drawCompositeSprite(g.canvas, cx, cy, radius, clr)
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

// rowCell is a helper pairing a row index with its cell, used in buildDropAnims.
type rowCell struct {
	row  int
	cell Cell
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
			g.moveAnims = append(g.moveAnims, MoveAnim{fx, fy, tx, ty, 0, 0, cell})
			// Clear the vacated canvas position (clipped to canvas bounds).
			x0, y0 := col*dcs, m[0]*dcs
			x1, y1 := x0+dcs, y0+dcs
			if x1 > cw {
				x1 = cw
			}
			if y1 > ch {
				y1 = ch
			}
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
				g.moveAnims = append(g.moveAnims, MoveAnim{fx, fy, tx, ty, 0, 0, cell})
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
		if x1 > cw {
			x1 = cw
		}
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
			g.moveAnims = append(g.moveAnims, MoveAnim{fx, fy, tx, ty, 0, 0, cell})
		}
	}

	g.animStart = ebiten.Tick()
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
	elapsed := elapsedFraction(ebiten.Tick(), g.animStart, dropTicks)
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

// updateWaiting handles hover recompute and click processing while in stateWaiting.
func (g *Game) updateWaiting() error {
	// Hover recompute.
	// CursorPosition returns device-pixel coords when LayoutF is in effect.
	dcs := g.deviceCellSize
	if dcs <= 0 {
		dcs = cellSize
	}
	mx, my := ebiten.CursorPosition()
	hc := mx / dcs
	hr := my / dcs
	if hc >= 0 && hc < g.grid.Cols && hr >= 0 && hr < g.grid.Rows {
		if g.grid.Cells[hc][hr].Health > 0 {
			g.hoverCol, g.hoverRow = hc, hr
			g.victimHints = closestColorNeighbors(g.grid.Cells, hc, hr, g.grid.Cols, g.grid.Rows)
		}
	}

	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		// nothing to do
		return nil
	}
	// Click handling.
	px, py := ebiten.CursorPosition()
	col, row := cellFromPixel(px, py, dcs)
	if col < 0 || col >= g.grid.Cols || row < 0 || row >= g.grid.Rows {
		return nil
	}
	cell := &g.grid.Cells[col][row]
	if cell.Health <= 0 {
		return nil
	}

	oldHealth := cell.Health
	cell.Health -= healthDecrement
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
	} else {
		if !g.handleTargetSurvival(col, row, cell, oldHealth) {
			return nil
		}
	}
	return nil
}

// victimPathParams computes the path parameters for a single victim moving from
// (fromX,fromY) to (toX,toY) and returns a partially-filled VictimAnim with
// pathType, ctrlX, ctrlY, and spiralTurns set.
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
	victimCoords := closestColorNeighbors(g.grid.Cells, col, row, g.grid.Cols, g.grid.Rows)
	if len(victimCoords) > 0 {
		dcs := g.deviceCellSize
		toX, toY := deviceCellCenter(col, row, dcs)
		g.victims = g.victims[:0]
		for _, vc := range victimCoords {
			ncol, nrow := vc[0], vc[1]
			fromX, fromY := deviceCellCenter(ncol, nrow, dcs)
			pt, cx, cy, st := victimPathParams(fromX, fromY, toX, toY)
			v := VictimAnim{
				col:         ncol,
				row:         nrow,
				cell:        g.grid.Cells[ncol][nrow], // snapshot
				fromX:       fromX,
				fromY:       fromY,
				toX:         toX,
				toY:         toY,
				pathType:    pt,
				ctrlX:       cx,
				ctrlY:       cy,
				spiralTurns: st,
			}
			g.victims = append(g.victims, v)
			// Clear victim's canvas position (it is now an overlay animation).
			x0, y0 := ncol*dcs, nrow*dcs
			g.canvas.SubImage(image.Rect(x0, y0, x0+dcs, y0+dcs)).(*ebiten.Image).Clear()
		}
		g.animStart = ebiten.Tick()
		g.currentState = stateVictimMoving
	} else {
		// No victim — score the kill and go straight to drop.
		g.score += scoreForKill(nil, g.targetColor)
		spawned := g.buildDropAnims([]int{col})
		if respawnCells {
			g.cellCount += spawned
		}
		if len(g.moveAnims) > 0 {
			g.animStart = ebiten.Tick()
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
	g.targetAnimStart = ebiten.Tick()
	g.targetAnimStartR = maxR * oldHealth
	g.targetAnimPeakR = maxR * oldHealth * 1.1
	g.targetAnimEndR = maxR * cell.Health
	g.targetAnimColor = g.grid.Cells[col][row].Color()
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
		anims[i] = MoveAnim{fx, fy, tx, ty, ctrlX, ctrlY, src.cell}
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
	g.animStart = ebiten.Tick()
	return true
}

// updateVictimMoving advances the victim movement animation and transitions to
// stateDropping (or stateWaiting) when the animation completes.
func (g *Game) updateVictimMoving() error {
	t := elapsedFraction(ebiten.Tick(), g.animStart, victimTicks)
	if t >= 1.0 {
		// All victims arrived — score the kill, kill them, and trigger drop.
		victimColors := make([]color.RGBA, len(g.victims))
		for i, v := range g.victims {
			victimColors[i] = v.cell.Color()
		}
		g.score += scoreForKill(victimColors, g.targetColor)
		colSet := map[int]bool{g.actionCol: true}
		for _, v := range g.victims {
			g.grid.Cells[v.col][v.row].Health = 0
			colSet[v.col] = true
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
			g.animStart = ebiten.Tick()
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
	elapsed := elapsedFraction(ebiten.Tick(), g.animStart, shuffleTicks)
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
	elapsed := elapsedFraction(ebiten.Tick(), g.animStart, dropTicks)
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
		if elapsedFraction(ebiten.Tick(), g.targetAnimStart, targetAnimTicks) >= 1.0 {
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

// Draw blits the offscreen canvas to the screen, then composites any
// in-flight animation circles on top.
func (g *Game) Draw(screen *ebiten.Image) {
	if g.glowShader == nil {
		s, err := ebiten.NewShader([]byte(glowShaderSrc))
		if err != nil {
			log.Fatal("glow shader compile error:", err)
		}
		g.glowShader = s
	}
	screen.Fill(color.Black)

	g.drawVictimLine(screen)
	g.drawCanvas(screen)
	g.drawTargetBounce(screen)
	g.drawMoveAnims(screen)
}

// drawVictimLine draws animated dotted hint lines behind the circles, one per
// tied victim, each from that victim's cell toward the hovered cell.
func (g *Game) drawVictimLine(screen *ebiten.Image) {
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
			vector.DrawFilledCircle(screen, fromX+ux*pos, fromY+uy*pos, dotRadius, dotCol, true)
		}
	}
}

// drawCanvas blits the persistent offscreen canvas to the screen.
func (g *Game) drawCanvas(screen *ebiten.Image) {
	screen.DrawImage(g.canvas, nil)
}

// drawTargetBounce renders the target cell bounce animation.
func (g *Game) drawTargetBounce(screen *ebiten.Image) {
	if !g.targetAnimating {
		return
	}
	t := elapsedFraction(ebiten.Tick(), g.targetAnimStart, targetAnimTicks)
	if t > 1.0 {
		t = 1.0
	}
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
		g.drawCompositeSprite(screen, cx, cy, r, g.targetAnimColor)
	}
}

// drawMoveAnims renders in-motion cells for shuffle, drop, and victim-move animations.
func (g *Game) drawMoveAnims(screen *ebiten.Image) {
	if len(g.moveAnims) > 0 {
		var t float32
		ticks := int64(shuffleTicks)
		if g.currentState == stateDropping || g.currentState == stateColumnCollapsing {
			ticks = dropTicks
		}
		t = elapsedFraction(ebiten.Tick(), g.animStart, ticks)
		if t > 1.0 {
			t = 1.0
		}
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
				g.drawCompositeSprite(screen, cx, cy, r, anim.Cell.Color())
			}
		}
	}

	if g.currentState == stateVictimMoving {
		t := elapsedFraction(ebiten.Tick(), g.animStart, victimTicks)
		if t > 1.0 {
			t = 1.0
		}
		for _, v := range g.victims {
			var cx, cy float32
			switch v.pathType {
			case 0: // straight line
				cx, cy = lerp2(v.fromX, v.fromY, v.toX, v.toY, t)
			case 1: // quadratic Bezier arc
				cx, cy = quadBezier2(v.fromX, v.fromY, v.ctrlX, v.ctrlY, v.toX, v.toY, t)
			case 2: // inward spiral (radius shrinks, angle rotates)
				ddx := v.fromX - v.toX
				ddy := v.fromY - v.toY
				r0 := float32(math.Sqrt(float64(ddx*ddx + ddy*ddy)))
				theta0 := float32(math.Atan2(float64(ddy), float64(ddx)))
				r := r0 * (1 - t)
				theta := theta0 + v.spiralTurns*2*float32(math.Pi)*t
				cx = v.toX + r*float32(math.Cos(float64(theta)))
				cy = v.toY + r*float32(math.Sin(float64(theta)))
			}
			dcs2 := g.deviceCellSize
			if dcs2 <= 0 {
				dcs2 = cellSize
			}
			radius := float32(dcs2/2-2) * v.cell.Health * (1 - t)
			if radius > 0 {
				g.drawCompositeSprite(screen, cx, cy, radius, v.cell.Color())
			}
		}
	}
}

func main() {
	ebiten.SetWindowSize(windowWidth, windowHeight)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowTitle("go2d grid")
	ebiten.SetWindowClosingHandled(false)

	grid := NewGrid(gridCols, gridRows)

	game := newGameState(grid)
	game.animStart = ebiten.Tick()

	glyphFont, err := loadGlyphFont()
	if err != nil {
		log.Fatal("load glyph font:", err)
	}
	game.glyphFont = glyphFont

	// Allocate the device-resolution canvas before RunGame so that dimensions
	// and renderScale are set for the first LayoutF call.  Drawing/baking is
	// deferred to the first Update tick (initialPainted gate) so that
	// ebiten.NewImage / DrawRectShader / text.Draw run with a live graphics
	// context rather than a cold one that can produce blank sprites.
	s := deviceScale()
	game.renderScale = s
	dw, dh := deviceCanvasDims(s)
	game.deviceCellSize = deviceCellSizeFor(dw)
	game.spriteRefSize = spriteRefSizeFor(game.deviceCellSize)
	game.canvas = ebiten.NewImage(dw, dh)

	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}

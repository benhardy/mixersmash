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
	gridCols     = windowWidth / cellSize  // 25
	gridRows     = windowHeight / cellSize // 18

	stateWaiting      = 1
	stateShuffling    = 2
	stateDropping     = 3
	stateVictimMoving = 4

	shuffleTicks    = 18        // 0.3s at 60tps
	dropTicks       = 18        // 0.3s at 60tps
	victimTicks     = int64(24) // 0.4s at 60tps
	targetAnimTicks = int64(15) // 0.25s at 60 TPS

	healthDecrement = float32(0.2)
	healthMin       = float32(0.2)
	healthEpsilon   = float32(0.001)

	respawnCells = false // set false to leave gaps at top instead of spawning new cells
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
    return vec4(alpha, alpha, alpha, alpha) // RGB values premultiplied by alpha
}
`

// gelShaderSrc is a Kage fragment shader that renders an Aqua-style gel body.
// srcPos is in pixel coordinates (0,0)→(Size.x,Size.y) because of //kage:unit pixels.
// Output is premultiplied RGBA.
const gelShaderSrc = `//kage:unit pixels
package main

var Size vec2
var BaseColor vec3
var Hover float

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
    center := Size * 0.5
    radius := min(center.x, center.y)
    p := (srcPos - center) / radius
    dist := length(p)
    pixelWidth := 1.5 / radius
    alpha := 1.0 - smoothstep(1.0-pixelWidth, 1.0, dist)
    if alpha <= 0.0 { return vec4(0.0) }
    t := clamp((p.y+1.0)*0.5, 0.0, 1.0)
    topColor := BaseColor * 0.35
    bottomColor := mix(BaseColor, vec3(1.0), 0.35)
    col := mix(topColor, bottomColor, pow(t, 1.3))
    bottomGlow := smoothstep(0.1, 0.95, p.y) * (1.0 - smoothstep(0.65, 1.0, dist))
    col += mix(BaseColor, vec3(1.0), 0.65) * bottomGlow * (0.55 + Hover*0.2)
    glossP := vec2(p.x*1.18, (p.y+0.44)*1.85)
    glossDist := length(glossP)
    glossMask := 1.0 - smoothstep(0.72, 0.78, glossDist)
    glossFade := clamp(1.0-(p.y+0.92)*1.15, 0.0, 1.0)
    col = mix(col, vec3(1.0), glossMask*glossFade*0.85)
    rim := smoothstep(0.90, 0.98, dist) * (1.0 - smoothstep(0.98, 1.0, dist))
    col += mix(BaseColor, vec3(1.0), 0.5) * rim * 0.45
    return vec4(col*alpha, alpha)
}
`

// colorDistanceRedmean returns a perceptually-weighted colour distance using the
// "redmean" approximation, which adapts RGB channel weights based on average redness.
func colorDistanceRedmean(a, b color.RGBA) float32 {
	rBar := (float32(a.R) + float32(b.R)) / 2
	dR := float32(a.R) - float32(b.R)
	dG := float32(a.G) - float32(b.G)
	dB := float32(a.B) - float32(b.B)
	return float32(math.Sqrt(float64(
		(2+rBar/256)*dR*dR + 4*dG*dG + (2+(255-rBar)/256)*dB*dB,
	)))
}

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
		if col.R == 255 && col.G == 255 && col.B == 255 {
			continue // don't use white
		}
		if col.R < 102 || col.G < 102 || col.B < 102 {
			continue // don't use too-dark colours
		}
		return col
	}
}

// cellFromPixel converts pixel coordinates to (col, row) grid indices.
// No bounds clamping is applied.
func cellFromPixel(px, py, cellSize int) (int, int) {
	return px / cellSize, py / cellSize
}

// eightNeighbors returns all in-bounds (col+dx, row+dy) for dx,dy in
// {-1,0,1}×{-1,0,1} excluding (0,0).
func eightNeighbors(col, row, cols, rows int) [][2]int {
	var result [][2]int
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			nc := col + dx
			nr := row + dy
			if nc >= 0 && nc < cols && nr >= 0 && nr < rows {
				result = append(result, [2]int{nc, nr})
			}
		}
	}
	return result
}

// closestColorNeighbor returns the (col, row) of the 8-neighbour of (col,row)
// with Health > 0 that has minimum colorDistanceRedmean distance to
// cells[col][row].Color. Returns found=false if no healthy neighbour exists.
func closestColorNeighbor(cells [][]Cell, col, row, cols, rows int) (int, int, bool) {
	neighbors := eightNeighbors(col, row, cols, rows)
	target := cells[col][row].Color
	bestDist := float32(-1)
	bestCol, bestRow := 0, 0
	found := false
	for _, n := range neighbors {
		nc, nr := n[0], n[1]
		if cells[nc][nr].Health <= 0 {
			continue
		}
		dist := colorDistanceRedmean(target, cells[nc][nr].Color)
		if !found || dist < bestDist {
			bestDist = dist
			bestCol = nc
			bestRow = nr
			found = true
		}
	}
	return bestCol, bestRow, found
}

// closestColorNeighbors returns all 8-neighbours of (col,row) with Health > 0
// that share the exact minimum colorDistanceRedmean distance to
// cells[col][row].Color. Tie is determined by exact float32 equality.
// Returns an empty slice if no healthy neighbour exists.
func closestColorNeighbors(cells [][]Cell, col, row, cols, rows int) [][2]int {
	neighbors := eightNeighbors(col, row, cols, rows)
	target := cells[col][row].Color
	bestDist := float32(-1)
	found := false
	var result [][2]int
	for _, n := range neighbors {
		nc, nr := n[0], n[1]
		if cells[nc][nr].Health <= 0 {
			continue
		}
		dist := colorDistanceRedmean(target, cells[nc][nr].Color)
		if !found || dist < bestDist {
			bestDist = dist
			result = result[:0]
			result = append(result, [2]int{nc, nr})
			found = true
		} else if dist == bestDist {
			result = append(result, [2]int{nc, nr})
		}
	}
	return result
}

type Cell struct {
	Color  color.RGBA
	Health float32
}

// MoveAnim describes the animated movement of a cell from one pixel position to
// another during shuffle or drop phases. FromX/FromY and ToX/ToY are pixel centres.
// CtrlX/CtrlY is the quadratic Bézier control point used during shuffle; zero
// value falls back to straight-line interpolation (used for drops).
type MoveAnim struct {
	FromX, FromY float32
	ToX, ToY     float32
	CtrlX, CtrlY float32 // quadratic Bézier control point; zero = straight line
	Cell         Cell
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
				Color:  paletteColor(),
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

// VictimAnim holds per-victim animation state for the stateVictimMoving phase.
// All victims converge on the same target cell centre (toX, toY).
type VictimAnim struct {
	col, row     int     // victim's original grid position
	cell         Cell    // snapshot of the victim cell before animation
	fromX, fromY float32 // pixel start (victim cell centre)
	toX, toY     float32 // pixel end (target cell centre, same for all victims)
	pathType     int     // 0=straight, 1=arc, 2=spiral
	ctrlX, ctrlY float32 // Bezier control point for arc path
	spiralTurns  float32 // signed: positive=CCW, negative=CW
}

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
}

// scoreForKill returns the points earned for a single kill.
// victimColors holds the colours of all victims absorbed; targetColor is the
// colour of the target cell that died.
//
// Formula:
//
//	N            = len(victimColors)
//	base         = 5
//	additional   = max(0, N-1) * 20
//	sameColour   = 30*N  if N>=2 and all victim colours are the exact same color.RGBA, else 0
//	targetColour = 40 * M  where M = count of victims whose colour == targetColor
//	total        = base + additional + sameColour + targetColour
func scoreForKill(victimColors []color.RGBA, targetColor color.RGBA) int {
	n := len(victimColors)
	base := 5
	additional := 0
	if n > 1 {
		additional = (n - 1) * 20
	}
	sameColour := 0
	if n >= 2 {
		allSame := true
		for i := 1; i < n; i++ {
			if victimColors[i] != victimColors[0] {
				allSame = false
				break
			}
		}
		if allSame {
			sameColour = 30 * n
		}
	}
	m := 0
	for _, c := range victimColors {
		if c == targetColor {
			m++
		}
	}
	targetColour := 40 * m
	return base + additional + sameColour + targetColour
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

// elapsedFraction returns how far through a phase we are as a 0→1 float32.
// currentTick is the game tick at the time of evaluation, startTick is when
// the phase began, and durationTicks is the total length of the phase.
func elapsedFraction(currentTick, startTick, durationTicks int64) float32 {
	return float32(currentTick-startTick) / float32(durationTicks)
}

// cellCenter returns the logical pixel centre of a grid cell using the const
// cellSize.  Kept pure so existing unit tests pass unchanged.
func cellCenter(col, row int) (float32, float32) {
	return float32(col*cellSize + cellSize/2), float32(row*cellSize + cellSize/2)
}

// deviceCellCenter returns the device-pixel centre of a grid cell for the given
// device cell size dcs.  Used wherever pixel coordinates must map to the
// device-resolution canvas or screen.
func deviceCellCenter(col, row, dcs int) (float32, float32) {
	return float32(col*dcs + dcs/2), float32(row*dcs + dcs/2)
}

// compositeSpriteRefSize is the logical fallback side length used only before
// the first LayoutF call sets g.spriteRefSize.  At runtime, spriteRefSize is
// derived from deviceCellSize*2 (capped at 512) and updated on scale changes.
const compositeSpriteRefSize = 120

// compositeCachedSprite returns a pre-rendered composite *ebiten.Image for clr,
// building and caching it on first call.  The image carries a circa-2003 Apple
// Aqua jelly-button look rendered via the gel fragment shader:
//
//  1. Gel body — rendered by gelShaderSrc via DrawRectShader (BlendSourceOver).
//     The shader owns the circular edge (soft antialiased rim); no separate
//     mask is applied to the body.
//  2. Creature glyph, tinted darkerShade(clr), clipped to the circle.
//     The glyph is drawn into a temp image, masked with a filled-circle
//     BlendDestinationIn pass, then composited over the body with
//     BlendSourceOver.  This keeps the shader's smooth rim intact.
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

	composite := ebiten.NewImage(sz, sz)

	// ------------------------------------------------------------------ //
	// Layer 1: gel body via fragment shader.
	// DrawRectShader with //kage:unit pixels: srcPos runs (0,0)→(sz,sz).
	// Size uniform = {sz, sz} so the shader computes center = Size*0.5 = {half,half}.
	// BaseColor is normalised to [0,1]. Hover = 0 for the baked sprite.
	// The shader already returns premultiplied vec4(col*alpha, alpha) and
	// produces a smooth antialiased circular rim — no additional mask needed
	// on the body.
	// ------------------------------------------------------------------ //
	var gelOpts ebiten.DrawRectShaderOptions
	gelOpts.Blend = ebiten.BlendSourceOver
	gelOpts.Uniforms = map[string]any{
		"Size":      []float32{float32(sz), float32(sz)},
		"BaseColor": []float32{float32(clr.R) / 255.0, float32(clr.G) / 255.0, float32(clr.B) / 255.0},
		"Hover":     float32(0),
	}
	composite.DrawRectShader(sz, sz, g.gelShader, &gelOpts)

	// ------------------------------------------------------------------ //
	// Layer 2: creature glyph, tinted darkerShade(clr), clipped to circle.
	// Draw the glyph into a temp image, apply a filled-circle
	// BlendDestinationIn mask so wide glyphs (e.g. octopus) are clipped,
	// then composite the result over the shader body with BlendSourceOver.
	// The body's smooth shader rim is not touched by this pass.
	// ------------------------------------------------------------------ //
	glyphImg := ebiten.NewImage(sz, sz)
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
	text.Draw(glyphImg, str, face, &drawOpts)

	// Clip the glyph image to the circle.
	maskImg := ebiten.NewImage(sz, sz)
	vector.DrawFilledCircle(maskImg, half, half, half-1, color.White, true)
	var maskOpts ebiten.DrawImageOptions
	maskOpts.Blend = ebiten.BlendDestinationIn
	glyphImg.DrawImage(maskImg, &maskOpts)

	// Composite clipped glyph over the gel body.
	var glyphOpts ebiten.DrawImageOptions
	glyphOpts.Blend = ebiten.BlendSourceOver
	composite.DrawImage(glyphImg, &glyphOpts)

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
func (g *Game) drawCell(col, row int, cell *Cell, cs int, cellFullness float32) {
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
	clr := cell.Color
	g.drawCompositeSprite(g.canvas, cx, cy, radius, clr)
}

// gravityMoves returns the [oldRow, newRow] mapping for cells in a column of numRows
// when holes appear at holeRows. Only returns entries for cells that actually change row.
// Cells not in holeRows are "alive" and compact downward; holeRows are removed.
func gravityMoves(numRows int, holeRows []int) [][2]int {
	// Build a set of hole rows for fast lookup
	holeSet := make(map[int]bool, len(holeRows))
	for _, r := range holeRows {
		holeSet[r] = true
	}
	// Collect alive rows top-to-bottom
	var alive []int
	for r := 0; r < numRows; r++ {
		if !holeSet[r] {
			alive = append(alive, r)
		}
	}
	newCount := numRows - len(alive) // number of new cells needed at top
	var moves [][2]int
	for i, oldRow := range alive {
		newRow := newCount + i
		if oldRow != newRow {
			moves = append(moves, [2]int{oldRow, newRow})
		}
	}
	return moves
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
				g.grid.Cells[col][i] = Cell{Color: paletteColor(), Health: 1.0}
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
		g.handleTargetDeath(col, row, cell)
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
func (g *Game) handleTargetDeath(col, row int, cell *Cell) {
	// Capture target colour before the cell is cleared.
	g.targetColor = g.grid.Cells[col][row].Color
	// Redraw immediately so cell disappears.
	g.drawCell(col, row, cell, g.deviceCellSize, 1.0)
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
	g.targetAnimColor = g.grid.Cells[col][row].Color
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
			victimColors[i] = v.cell.Color
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
			g.currentState = stateWaiting
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
				g.drawCell(col, row, &g.grid.Cells[col][row], dcs, 1.0)
			}
		}
		g.moveAnims = nil
		g.currentState = stateWaiting
	}
	return nil
}

// updateDropping waits for the drop animation to complete, then commits final
// cell positions and returns to stateWaiting.
func (g *Game) updateDropping() error {
	elapsed := elapsedFraction(ebiten.Tick(), g.animStart, dropTicks)
	if elapsed >= 1.0 {
		dcs := g.deviceCellSize
		for _, anim := range g.moveAnims {
			col := int(anim.ToX) / dcs
			row := int(anim.ToY) / dcs
			if col >= 0 && col < g.grid.Cols && row >= 0 && row < g.grid.Rows {
				g.drawCell(col, row, &g.grid.Cells[col][row], dcs, 1.0)
			}
		}
		g.moveAnims = nil
		g.currentState = stateWaiting
	}
	return nil
}

// Update handles input each tick.
func (g *Game) Update() error {
	ebiten.SetWindowTitle(fmt.Sprintf("MixerSmash — cells: %d  score: %d", g.cellCount, g.score))

	// Reset hover every tick; recompute below if conditions are met.
	g.hoverCol, g.hoverRow = -1, -1
	g.victimHints = g.victimHints[:0]

	if g.targetAnimating {
		if elapsedFraction(ebiten.Tick(), g.targetAnimStart, targetAnimTicks) >= 1.0 {
			g.targetAnimating = false
			g.drawCell(g.actionCol, g.actionRow, &g.grid.Cells[g.actionCol][g.actionRow], g.deviceCellSize, 1.0)
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
		vc := g.grid.Cells[hc][hr].Color
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
		lt := t * 2                // 0→1 in phase 1
		eased := 1 - (1-lt)*(1-lt) // ease-out: fast start, slow at peak
		r = g.targetAnimStartR + (g.targetAnimPeakR-g.targetAnimStartR)*eased
	} else {
		lt := (t - 0.5) * 2 // 0→1 in phase 2
		eased := lt * lt    // ease-in: slow at peak, fast at end
		r = g.targetAnimPeakR + (g.targetAnimEndR-g.targetAnimPeakR)*eased
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
		if g.currentState == stateDropping {
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
				// Quadratic ease-in-out
				var et float32
				if t < 0.5 {
					et = 2 * t * t
				} else {
					et = 1 - 2*(1-t)*(1-t)
				}
				// Quadratic Bézier with eased t
				u := 1 - et
				cx = u*u*anim.FromX + 2*u*et*anim.CtrlX + et*et*anim.ToX
				cy = u*u*anim.FromY + 2*u*et*anim.CtrlY + et*et*anim.ToY
			} else {
				// Drop: quadratic ease-in (gravity), straight line
				posT := t * t
				cx = anim.FromX + posT*(anim.ToX-anim.FromX)
				cy = anim.FromY + posT*(anim.ToY-anim.FromY)
			}
			r := maxR * anim.Cell.Health
			if r > 0 {
				g.drawCompositeSprite(screen, cx, cy, r, anim.Cell.Color)
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
				cx = v.fromX + t*(v.toX-v.fromX)
				cy = v.fromY + t*(v.toY-v.fromY)
			case 1: // quadratic Bezier arc
				u := float32(1) - t
				cx = u*u*v.fromX + 2*u*t*v.ctrlX + t*t*v.toX
				cy = u*u*v.fromY + 2*u*t*v.ctrlY + t*t*v.toY
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
				g.drawCompositeSprite(screen, cx, cy, radius, v.cell.Color)
			}
		}
	}
}

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
				g.drawCell(c, r, &g.grid.Cells[c][r], g.deviceCellSize, 1.0)
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

	// Initialise device-resolution canvas before RunGame so the first Draw is
	// not blank.  LayoutF will repaint when the scale factor changes later.
	s := deviceScale()
	game.renderScale = s
	dw, dh := deviceCanvasDims(s)
	game.deviceCellSize = deviceCellSizeFor(dw)
	game.spriteRefSize = spriteRefSizeFor(game.deviceCellSize)
	game.canvas = ebiten.NewImage(dw, dh)
	for c := 0; c < grid.Cols; c++ {
		for r := 0; r < grid.Rows; r++ {
			game.drawCell(c, r, &grid.Cells[c][r], game.deviceCellSize, 1.0)
		}
	}

	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}

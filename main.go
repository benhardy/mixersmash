package main

import (
	"image"
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
	cellSize     = 64
	gridCols     = windowWidth / cellSize  // 25
	gridRows     = windowHeight / cellSize // 18

	stateWaiting    = 1
	stateCollapsing = 2
	stateSpawning   = 3

	collapseTicks = int64(5)
	spawnTicks    = int64(10)

	healthDecrement = float32(0.2)
	healthMin       = float32(0.2)
	healthEpsilon   = float32(0.001)

	glowWidth = float32(6)

	flowRate           = float32(0.12) // colour blend per tick per unit health
	sourceRecoveryRate = float32(0.01) // health recovered per tick for source corners
	winTolerance       = 15            // max channel delta for win condition
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

// lerpColor linearly interpolates between two colours.
// t=0 returns from, t=1 returns to. Alpha is always 255.
func lerpColor(from, to color.RGBA, t float32) color.RGBA {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return color.RGBA{
		R: uint8(float32(from.R) + t*(float32(to.R)-float32(from.R))),
		G: uint8(float32(from.G) + t*(float32(to.G)-float32(from.G))),
		B: uint8(float32(from.B) + t*(float32(to.B)-float32(from.B))),
		A: 255,
	}
}

// colorDistanceMax returns the max absolute difference across the R, G, B
// channels of two colours. Used for the win condition.
func colorDistanceMax(a, b color.RGBA) int {
	dr := int(a.R) - int(b.R)
	if dr < 0 {
		dr = -dr
	}
	dg := int(a.G) - int(b.G)
	if dg < 0 {
		dg = -dg
	}
	db := int(a.B) - int(b.B)
	if db < 0 {
		db = -db
	}
	if dr >= dg && dr >= db {
		return dr
	}
	if dg >= db {
		return dg
	}
	return db
}

// blendColorsWeighted returns the weighted average of a slice of colours.
// Weights need not sum to 1 — they are normalised internally.
// Alpha is always 255. If weights sum to 0, returns colors[0].
func blendColorsWeighted(colors []color.RGBA, weights []float32) color.RGBA {
	var sum float32
	for _, w := range weights {
		sum += w
	}
	if sum == 0 {
		return colors[0]
	}
	var r, g, b float32
	for i, c := range colors {
		w := weights[i] / sum
		r += float32(c.R) * w
		g += float32(c.G) * w
		b += float32(c.B) * w
	}
	return color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}
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

// recolor returns a random opaque colour guaranteed to differ from current.
func recolor(cell *Cell) {
	for {
		c := randomColor()
		if cell.Color != c {
			cell.Color = c
			break
		}
	}
}

// cellFromPixel converts pixel coordinates to (col, row) grid indices.
// No bounds clamping is applied.
func cellFromPixel(px, py, cellSize int) (int, int) {
	return px / cellSize, py / cellSize
}

// neighbourOffsets lists the cardinal (N/S/E/W) direction vectors used by
// cellNeighbours.  Defined as a package-level var so it can be swapped out for
// an 8-directional set without changing any call sites.
var neighbourOffsets = [][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}}

// cellNeighbours returns the valid cardinal neighbours of (col, row) within a
// grid of dimensions cols×rows.  The returned slice contains [col, row] pairs.
func cellNeighbours(col, row, cols, rows int) [][2]int {
	var result [][2]int
	for _, off := range neighbourOffsets {
		nc := col + off[0]
		nr := row + off[1]
		if nc >= 0 && nc < cols && nr >= 0 && nr < rows {
			result = append(result, [2]int{nc, nr})
		}
	}
	return result
}

// normaliseLink returns a canonical [srcCol, srcRow, dstCol, dstRow] where the
// "src" end is always the lexicographically smaller (col, row) pair, so that
// A→B and B→A produce the same key.
func normaliseLink(aCol, aRow, bCol, bRow int) [4]int {
	if aCol < bCol || (aCol == bCol && aRow < bRow) {
		return [4]int{aCol, aRow, bCol, bRow}
	}
	return [4]int{bCol, bRow, aCol, aRow}
}

// addLink adds a normalised link to links if it does not already exist.
// Returns the (possibly extended) slice.
func addLink(links [][4]int, aCol, aRow, bCol, bRow int) [][4]int {
	norm := normaliseLink(aCol, aRow, bCol, bRow)
	for _, l := range links {
		if l == norm {
			return links
		}
	}
	return append(links, norm)
}

// hasLink reports whether a link already exists between (aCol, aRow) and
// (bCol, bRow) in links.  The pair is normalised before comparison so that
// A→B and B→A are treated as the same link.
func hasLink(links [][4]int, aCol, aRow, bCol, bRow int) bool {
	norm := normaliseLink(aCol, aRow, bCol, bRow)
	for _, l := range links {
		if l == norm {
			return true
		}
	}
	return false
}

// healthDrainPerCell returns the health drain applied to each individual cell
// when a click drains totalDrain spread evenly across the clicked cell plus all
// cells it is linked to.  linkCount is the number of linked neighbours.
func healthDrainPerCell(totalDrain float32, linkCount int) float32 {
	return totalDrain / float32(1+linkCount)
}

// linkedCells returns a slice of [col, row] pairs for every cell that shares a
// confirmed link with (col, row), scanning both endpoints of each link entry.
func linkedCells(links [][4]int, col, row int) [][2]int {
	var result [][2]int
	for _, l := range links {
		sc, sr, dc, dr := l[0], l[1], l[2], l[3]
		if sc == col && sr == row {
			result = append(result, [2]int{dc, dr})
		} else if dc == col && dr == row {
			result = append(result, [2]int{sc, sr})
		}
	}
	return result
}

// removeLinksForCell removes all links where either endpoint matches (col, row).
// Returns the filtered slice.
func removeLinksForCell(links [][4]int, col, row int) [][4]int {
	result := links[:0:0]
	for _, l := range links {
		if (l[0] == col && l[1] == row) || (l[2] == col && l[3] == row) {
			continue
		}
		result = append(result, l)
	}
	return result
}

type Cell struct {
	Color  color.RGBA
	Health float32
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
				Color:  randomColor(),
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

// Game is the Ebitengine game state.
type Game struct {
	grid           *Grid
	canvas         *ebiten.Image
	currentState   int   // which state are we in?
	startTick      int64 // when did the current state start?
	actionRow      int   // the row any given action is happening
	actionCol      int   // the column an action is happening
	glowShader     *ebiten.Shader
	confirmedLinks [][4]int // persistent links between cells
	linkDragging   bool     // true while a link drag is in progress
	linkSrcCol     int      // source cell col during drag; -1 = none
	linkSrcRow     int      // source cell row during drag; -1 = none
	linkHoverCol   int      // neighbour cell under cursor during drag; -1 = none
	linkHoverRow   int      // neighbour cell row under cursor during drag; -1 = none

	// colour-flow fields
	sourceCorners [3][2]int    // col,row of the 3 source corners
	targetCorner  [2]int       // col,row of the target corner
	sourceColors  [3]color.RGBA
	targetColor   color.RGBA
	gameWon       bool
}

// newGameState returns a Game with data fields initialised but no ebiten.Image
// allocated.  Callers that need a canvas (i.e. main) must set g.canvas after
// calling this.
func newGameState(grid *Grid) *Game {
	return &Game{
		grid:         grid,
		currentState: stateWaiting,
		startTick:    0,
		actionCol:    -1,
		actionRow:    -1,
		linkSrcCol:   -1,
		linkSrcRow:   -1,
		linkHoverCol: -1,
		linkHoverRow: -1,
	}
}

// initColourFlow sets up the colour-flow game mode on g after the grid is built.
// It assigns source and target corners, generates source colours, computes the
// target blend colour, and seeds each source corner cell with its source colour.
func (g *Game) initColourFlow() {
	cols := g.grid.Cols
	rows := g.grid.Rows
	// All four corners as (col, row) pairs.
	corners := [4][2]int{
		{0, 0},
		{cols - 1, 0},
		{0, rows - 1},
		{cols - 1, rows - 1},
	}
	// Shuffle corner indices.
	perm := rand.Perm(4)
	for i := 0; i < 3; i++ {
		g.sourceCorners[i] = corners[perm[i]]
	}
	g.targetCorner = corners[perm[3]]

	// Generate 3 distinct vibrant source colours.
	g.sourceColors[0] = randomColor()
	for {
		g.sourceColors[1] = randomColor()
		if g.sourceColors[1] != g.sourceColors[0] {
			break
		}
	}
	for {
		g.sourceColors[2] = randomColor()
		if g.sourceColors[2] != g.sourceColors[0] && g.sourceColors[2] != g.sourceColors[1] {
			break
		}
	}

	// Seed source corner cells with their source colours.
	for i, sc := range g.sourceCorners {
		g.grid.Cells[sc[0]][sc[1]].Color = g.sourceColors[i]
	}

	// Compute target colour from random positive-weight blend of source colours.
	var ratios [3]float32
	for i := range ratios {
		ratios[i] = rand.Float32()
		if ratios[i] < 0.1 {
			ratios[i] = 0.1
		}
	}
	g.targetColor = blendColorsWeighted(g.sourceColors[:], ratios[:])
}

// isSourceCorner reports whether (col, row) is one of the three source corners.
func (g *Game) isSourceCorner(col, row int) bool {
	for _, sc := range g.sourceCorners {
		if sc[0] == col && sc[1] == row {
			return true
		}
	}
	return false
}

// elapsedFraction returns how far through a phase we are as a 0→1 float32.
// currentTick is the game tick at the time of evaluation, startTick is when
// the phase began, and durationTicks is the total length of the phase.
func elapsedFraction(currentTick, startTick, durationTicks int64) float32 {
	return float32(currentTick-startTick) / float32(durationTicks)
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
	vector.DrawFilledCircle(g.canvas, cx, cy, radius, clr, true)
}

// advanceState advances the state machine for a given tick value.  It does not
// call ebiten.Tick() — the caller supplies the current tick so this function
// is pure and unit-testable.  drawCell calls are skipped when g.canvas is nil.
func (g *Game) advanceState(tick int64) {
	if g.currentState == stateCollapsing {
		elapsed := elapsedFraction(tick, g.startTick, collapseTicks)
		actionCell := &g.grid.Cells[g.actionCol][g.actionRow]
		if g.canvas != nil {
			g.drawCell(g.actionCol, g.actionRow, actionCell, cellSize, 1.0-elapsed)
		}
		if elapsed >= 1.0 {
			linked := linkedCells(g.confirmedLinks, g.actionCol, g.actionRow)
			drain := healthDrainPerCell(healthDecrement, len(linked))
			if actionCell.Health > healthMin {
				actionCell.Health -= drain
			} else {
				actionCell.Health = 0.0
				g.confirmedLinks = removeLinksForCell(g.confirmedLinks, g.actionCol, g.actionRow)
			}
			if actionCell.Health < healthEpsilon {
				actionCell.Health = 0.0
				g.confirmedLinks = removeLinksForCell(g.confirmedLinks, g.actionCol, g.actionRow)
				// Source corners never stay dead — resurrect them immediately.
				if g.isSourceCorner(g.actionCol, g.actionRow) {
					for i, sc := range g.sourceCorners {
						if sc[0] == g.actionCol && sc[1] == g.actionRow {
							actionCell.Health = 1.0
							actionCell.Color = g.sourceColors[i]
							break
						}
					}
					g.currentState = stateSpawning
				} else {
					g.currentState = stateWaiting
				}
			} else {
				g.currentState = stateSpawning
			}
			for _, n := range linked {
				nc := &g.grid.Cells[n[0]][n[1]]
				if nc.Health > drain {
					nc.Health -= drain
				} else {
					nc.Health = 0.0
				}
				if nc.Health < healthEpsilon {
					nc.Health = 0.0
					g.confirmedLinks = removeLinksForCell(g.confirmedLinks, n[0], n[1])
				}
				if g.canvas != nil {
					g.drawCell(n[0], n[1], nc, cellSize, 1.0)
				}
			}
			g.startTick = tick
		}
	} else if g.currentState == stateSpawning {
		elapsed := elapsedFraction(tick, g.startTick, spawnTicks)
		if g.canvas != nil {
			g.drawCell(g.actionCol, g.actionRow, &g.grid.Cells[g.actionCol][g.actionRow], cellSize, elapsed)
		}
		if elapsed >= 1.0 {
			g.currentState = stateWaiting
			g.startTick = tick
		}
	}
}

// Update handles input each tick.
func (g *Game) Update() error {
	// --- Colour-flow: recover source corner health ---
	for _, sc := range g.sourceCorners {
		cell := &g.grid.Cells[sc[0]][sc[1]]
		if cell.Health < 1.0 {
			cell.Health = min(float32(1.0), cell.Health+sourceRecoveryRate)
		}
	}

	// --- Colour-flow: propagate colours along confirmed links ---
	// Pin sources BEFORE propagation so they always emit pure source colour.
	for i, sc := range g.sourceCorners {
		g.grid.Cells[sc[0]][sc[1]].Color = g.sourceColors[i]
	}

	// Snapshot all cell colours (column-major: Cells[col][row]).
	cols := g.grid.Cols
	rows := g.grid.Rows
	snap := make([][]color.RGBA, cols)
	for c := range snap {
		snap[c] = make([]color.RGBA, rows)
		for r := range snap[c] {
			snap[c][r] = g.grid.Cells[c][r].Color
		}
	}

	// Apply all lerps reading from snapshot, writing to grid.
	for _, link := range g.confirmedLinks {
		sc, sr, dc, dr := link[0], link[1], link[2], link[3]
		srcSnap := snap[sc][sr]
		dstSnap := snap[dc][dr]
		srcHealth := g.grid.Cells[sc][sr].Health
		dstHealth := g.grid.Cells[dc][dr].Health
		g.grid.Cells[dc][dr].Color = lerpColor(dstSnap, srcSnap, flowRate*srcHealth)
		g.grid.Cells[sc][sr].Color = lerpColor(srcSnap, dstSnap, flowRate*dstHealth)
	}

	// Re-pin sources after propagation (bidirectional lerp may have pulled them slightly).
	for i, sc := range g.sourceCorners {
		g.grid.Cells[sc[0]][sc[1]].Color = g.sourceColors[i]
	}

	// Redraw all cells to reflect propagated colours.
	isAnimating := g.currentState == stateCollapsing || g.currentState == stateSpawning
	if g.canvas != nil {
		for col := 0; col < g.grid.Cols; col++ {
			for row := 0; row < g.grid.Rows; row++ {
				if isAnimating && col == g.actionCol && row == g.actionRow {
					continue // advanceState handles the action cell during animation
				}
				g.drawCell(col, row, &g.grid.Cells[col][row], cellSize, 1.0)
			}
		}
	}

	// --- Colour-flow: check win condition ---
	if !g.gameWon {
		tc := g.targetCorner
		cell := g.grid.Cells[tc[0]][tc[1]]
		if colorDistanceMax(cell.Color, g.targetColor) <= winTolerance {
			g.gameWon = true
		}
	}

	if g.currentState == stateWaiting {
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			px, py := ebiten.CursorPosition()
			col, row := cellFromPixel(px, py, cellSize)
			if col < 0 || col >= g.grid.Cols || row < 0 || row >= g.grid.Rows {
				return nil
			}
			if g.grid.Cells[col][row].Health <= 0 {
				return nil
			}
			g.drawCell(col, row, &g.grid.Cells[col][row], cellSize, 1.0)
			g.currentState = stateCollapsing
			g.actionCol = col
			g.actionRow = row
			g.startTick = ebiten.Tick()
			// Begin link drag from the clicked cell.
			g.linkDragging = true
			g.linkSrcCol = col
			g.linkSrcRow = row
			g.linkHoverCol = -1
			g.linkHoverRow = -1
		}
	} else {
		g.advanceState(ebiten.Tick())
	}
	// Link drag hover and release are handled outside the state branch because
	// the click that starts the drag also transitions to stateCollapsing on the
	// same tick, so subsequent ticks arrive in the non-waiting branch.
	if g.linkDragging {
		px, py := ebiten.CursorPosition()
		hc, hr := cellFromPixel(px, py, cellSize)
		g.linkHoverCol = -1
		g.linkHoverRow = -1
		if hc >= 0 && hc < g.grid.Cols && hr >= 0 && hr < g.grid.Rows {
			neighbours := cellNeighbours(g.linkSrcCol, g.linkSrcRow, g.grid.Cols, g.grid.Rows)
			for _, n := range neighbours {
				if n[0] == hc && n[1] == hr && g.grid.Cells[hc][hr].Health > 0 &&
					!hasLink(g.confirmedLinks, g.linkSrcCol, g.linkSrcRow, hc, hr) {
					g.linkHoverCol = hc
					g.linkHoverRow = hr
					break
				}
			}
		}
	}
	if g.linkDragging && inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
		if g.linkHoverCol != -1 {
			g.confirmedLinks = addLink(g.confirmedLinks, g.linkSrcCol, g.linkSrcRow, g.linkHoverCol, g.linkHoverRow)
		}
		g.linkDragging = false
		g.linkSrcCol = -1
		g.linkSrcRow = -1
		g.linkHoverCol = -1
		g.linkHoverRow = -1
	}
	return nil
}

// cellCentre returns the pixel centre of a grid cell.
func cellCentre(col, row int) (float32, float32) {
	return float32(col*cellSize + cellSize/2), float32(row*cellSize + cellSize/2)
}

// Draw blits the offscreen canvas to the screen, then composites glow rings
// and potential-link lines onto the screen each frame while a link drag is
// active.  On the first call it also compiles the glow shader (ebiten must be
// initialised before NewShader can be called).
func (g *Game) Draw(screen *ebiten.Image) {
	if g.glowShader == nil {
		s, err := ebiten.NewShader([]byte(glowShaderSrc))
		if err != nil {
			log.Fatal("glow shader compile error:", err)
		}
		g.glowShader = s
	}
	screen.Fill(color.Black)

	// Draw confirmed links as metallic pipes (three overlapping strokes) before
	// blitting the canvas so circles appear on top of links.
	for _, l := range g.confirmedLinks {
		x0, y0 := cellCentre(l[0], l[1])
		x1, y1 := cellCentre(l[2], l[3])
		vector.StrokeLine(screen, x0, y0, x1, y1, 5, color.RGBA{60, 60, 60, 255}, true)
		vector.StrokeLine(screen, x0, y0, x1, y1, 3, color.RGBA{140, 140, 140, 255}, true)
		vector.StrokeLine(screen, x0, y0, x1, y1, 1, color.RGBA{220, 220, 220, 255}, true)
	}

	screen.DrawImage(g.canvas, nil)

	if g.linkDragging {
		// Build the set of glowing cells: always the source cell; also the hovered
		// destination cell when the cursor is over a valid neighbour.
		glowCells := [][2]int{{g.linkSrcCol, g.linkSrcRow}}
		if g.linkHoverCol != -1 {
			glowCells = append(glowCells, [2]int{g.linkHoverCol, g.linkHoverRow})
		}

		// Glow pass: fire the shader for every cell in glowCells.
		for _, gc := range glowCells {
			col := gc[0]
			row := gc[1]
			cell := &g.grid.Cells[col][row]
			csFloat := float32(cellSize)
			halfCs := csFloat / 2
			cx := float32(col*cellSize) + halfCs
			cy := float32(row*cellSize) + halfCs
			radius := (halfCs - 2) * cell.Health
			outerR := radius + glowWidth
			x0 := cx - outerR
			y0 := cy - outerR
			w := int(outerR * 2)
			h := int(outerR * 2)
			opts := &ebiten.DrawRectShaderOptions{}
			opts.GeoM.Translate(float64(x0), float64(y0))
			opts.Uniforms = map[string]any{
				"Center":      []float32{cx, cy},
				"InnerRadius": float32(radius),
				"OuterRadius": float32(outerR),
			}
			opts.Blend = ebiten.BlendSourceOver
			screen.DrawRectShader(w, h, g.glowShader, opts)
		}

		// Potential link lines: iterate all valid neighbours so lines are drawn to
		// every valid unlinked healthy neighbour regardless of glow state.
		sx, sy := cellCentre(g.linkSrcCol, g.linkSrcRow)
		neighbours := cellNeighbours(g.linkSrcCol, g.linkSrcRow, g.grid.Cols, g.grid.Rows)
		for _, n := range neighbours {
			if g.grid.Cells[n[0]][n[1]].Health > 0 &&
				!hasLink(g.confirmedLinks, g.linkSrcCol, g.linkSrcRow, n[0], n[1]) {
				nx, ny := cellCentre(n[0], n[1])
				if n[0] == g.linkHoverCol && n[1] == g.linkHoverRow {
					vector.StrokeLine(screen, sx, sy, nx, ny, 2, color.RGBA{255, 255, 255, 200}, true)
				} else {
					vector.StrokeLine(screen, sx, sy, nx, ny, 1.5, color.RGBA{255, 255, 255, 80}, true)
				}
			}
		}
	}

	// --- Colour-flow: source corner rings ---
	// Draw a white ring on each source corner cell.
	for _, sc := range g.sourceCorners {
		cx, cy := cellCentre(sc[0], sc[1])
		radius := float32(cellSize/2) - 2.0
		vector.StrokeCircle(screen, cx, cy, radius, 2.5, color.RGBA{220, 220, 220, 220}, true)
	}

	// --- Colour-flow: target corner rings ---
	// Draw two concentric rings: white outer + goal-colour inner.
	{
		tc := g.targetCorner
		cx, cy := cellCentre(tc[0], tc[1])
		outerRadius := float32(cellSize/2) - 2.0
		innerRadius := float32(cellSize/2) - 6.0
		vector.StrokeCircle(screen, cx, cy, outerRadius, 2.5, color.RGBA{220, 220, 220, 220}, true)
		targetClr := g.targetColor
		targetClr.A = 255
		vector.StrokeCircle(screen, cx, cy, innerRadius, 2.5, targetClr, true)
	}

	// --- Colour-flow: HUD panel ---
	// Semi-transparent dark background at top-centre.
	vector.DrawFilledRect(screen, 300, 4, 200, 28, color.RGBA{0, 0, 0, 180}, false)
	// Target colour swatch.
	vector.DrawFilledRect(screen, 310, 8, 20, 20, g.targetColor, false)
	// Current target corner cell colour swatch (for comparison).
	{
		tc := g.targetCorner
		currentColor := g.grid.Cells[tc[0]][tc[1]].Color
		vector.DrawFilledRect(screen, 370, 8, 20, 20, currentColor, false)
	}

	// --- Colour-flow: win overlay ---
	if g.gameWon {
		vector.DrawFilledRect(screen, 0, 0, windowWidth, windowHeight, color.RGBA{0, 200, 0, 80}, false)
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

	grid := NewGrid(gridCols, gridRows)
	canvas := ebiten.NewImage(windowWidth, windowHeight)

	game := newGameState(grid)
	game.canvas = canvas
	game.startTick = ebiten.Tick()
	game.initColourFlow()

	for c := 0; c < grid.Cols; c++ {
		for r := 0; r < grid.Rows; r++ {
			game.drawCell(c, r, &grid.Cells[c][r], cellSize, 1.0)
		}
	}

	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}

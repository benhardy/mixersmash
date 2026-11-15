package main

import (
	"image/color"
	"math"
	"math/rand"
)

// Cell holds a grid cell's creature reference and health (1.0 = full, 0.0 = dead).
// A nil Creature means the cell is empty/dead.
type Cell struct {
	Creature *Creature
	Health   float32
}

// Color returns the palette colour for this cell's creature, or a zero color.RGBA
// (A=0) if the cell is empty (Creature == nil). All palette colours have A=255,
// so A=0 is a safe sentinel for an empty cell.
func (c Cell) Color() color.RGBA {
	if c.Creature == nil {
		return color.RGBA{}
	}
	return c.Creature.Color
}

// MoveAnim describes the animated movement of a cell from one pixel position to
// another during shuffle, drop, or victim-move phases. FromX/FromY and ToX/ToY
// are pixel centres. CtrlX/CtrlY is the quadratic Bézier control point used
// during shuffle and arc-path victim moves; zero value falls back to straight-line
// interpolation (used for drops).
//
// Col, Row, PathType, and SpiralTurns are used only by the victim-move phase.
// PathType: 0 = straight line, 1 = quadratic Bézier arc, 2 = inward spiral.
// SpiralTurns: signed — positive = CCW, negative = CW.
type MoveAnim struct {
	FromX, FromY float32
	ToX, ToY     float32
	CtrlX, CtrlY float32 // quadratic Bézier control point; zero = straight line
	Cell         Cell
	Col          int     // victim-move phase: original grid column
	Row          int     // victim-move phase: original grid row
	PathType     int     // victim-move phase: 0=straight, 1=arc, 2=spiral
	SpiralTurns  float32 // victim-move phase: signed turn count
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
				Creature: paletteCreature(),
				Health:   1.0,
			}
		}
	}
	return &Grid{
		Cols:  cols,
		Rows:  rows,
		Cells: cells,
	}
}

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

// colorDistanceHSV returns a hue-weighted HSV colour distance between a and b.
//
// Algorithm:
//  1. Convert each RGB to HSV (H in [0,360), S and V in [0,1]).
//  2. Compute a circular hue difference: dH = min(|h1-h2|, 360-|h1-h2|) / 180,
//     normalised to [0,1].
//  3. Scale the hue term by min(s1,s2) so that hue barely contributes when
//     either colour is near-achromatic (grey guard).
//  4. dist = sqrt( (hueW * 2*dH)^2 + (1*dS)^2 + (0.5*dV)^2 )
//
// Tunable weight constants (documented as such):
//
//	hueScale  = 2.0  — multiplies the hue component (hue dominates)
//	satScale  = 1.0  — multiplies the saturation difference
//	valScale  = 0.5  — multiplies the value difference (lightness matters less)
func colorDistanceHSV(a, b color.RGBA) float32 {
	// Tunable weights — adjust here to change metric behaviour.
	const hueScale = 2.0
	const satScale = 1.0
	const valScale = 0.5

	h1, s1, v1 := rgbToHSV(a)
	h2, s2, v2 := rgbToHSV(b)

	diff := h1 - h2
	if diff < 0 {
		diff = -diff
	}
	if diff > 180 {
		diff = 360 - diff
	}
	dH := diff / 180 // normalised to [0,1]

	hueW := min(s1, s2) // hue only matters when both are colourful

	dS := s1 - s2
	if dS < 0 {
		dS = -dS
	}
	dV := v1 - v2
	if dV < 0 {
		dV = -dV
	}

	hTerm := hueScale * dH * hueW
	sTerm := satScale * dS
	vTerm := valScale * dV

	return float32(math.Sqrt(float64(hTerm*hTerm + sTerm*sTerm + vTerm*vTerm)))
}

// rgbToHSV converts a color.RGBA to hue (degrees, [0,360)), saturation ([0,1]),
// and value ([0,1]). Alpha is ignored. For achromatic colours (s==0) hue is 0.
func rgbToHSV(c color.RGBA) (h, s, v float32) {
	r := float32(c.R) / 255.0
	g := float32(c.G) / 255.0
	b := float32(c.B) / 255.0

	cMax := max(r, max(g, b))
	cMin := min(r, min(g, b))
	delta := cMax - cMin

	v = cMax

	if cMax == 0 {
		s = 0
	} else {
		s = delta / cMax
	}

	if delta == 0 {
		h = 0
		return
	}

	switch cMax {
	case r:
		h = 60 * (((g - b) / delta) - float32(math.Floor(float64((g-b)/delta/6)))*6)
	case g:
		h = 60 * (((b - r) / delta) + 2)
	default: // b
		h = 60 * (((r - g) / delta) + 4)
	}

	if h < 0 {
		h += 360
	}
	return
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

// closestColorNeighbors returns all 8-neighbours of (col,row) with Health > 0
// that share the exact minimum colorDistanceRedmean distance to
// cells[col][row].Color. Tie is determined by exact float32 equality.
// Returns an empty slice if no healthy neighbour exists.
func closestColorNeighbors(cells [][]Cell, col, row, cols, rows int) [][2]int {
	neighbors := eightNeighbors(col, row, cols, rows)
	target := cells[col][row].Color()
	bestDist := float32(-1)
	found := false
	var result [][2]int
	for _, n := range neighbors {
		nc, nr := n[0], n[1]
		if cells[nc][nr].Health <= 0 {
			continue
		}
		dist := colorDistanceRedmean(target, cells[nc][nr].Color())
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

// horizontalCollapseMoves returns the [oldCol, newCol] mapping for non-empty
// columns when empty columns are removed and the remaining columns are packed
// toward the centre.
//
// nonEmpty = columns in [0,numCols) not in emptyCols, ascending.
// leftEmpties = count of emptyCols entries with index < numCols/2.
// nonEmpty[i] maps to (leftEmpties + i).
// Only pairs where oldCol != newCol are returned.
func horizontalCollapseMoves(numCols int, emptyCols []int) [][2]int {
	emptySet := make(map[int]bool, len(emptyCols))
	for _, c := range emptyCols {
		emptySet[c] = true
	}
	var nonEmpty []int
	for c := 0; c < numCols; c++ {
		if !emptySet[c] {
			nonEmpty = append(nonEmpty, c)
		}
	}
	half := numCols / 2
	leftEmpties := 0
	for _, c := range emptyCols {
		if c < half {
			leftEmpties++
		}
	}
	var moves [][2]int
	for i, oldCol := range nonEmpty {
		newCol := leftEmpties + i
		if oldCol != newCol {
			moves = append(moves, [2]int{oldCol, newCol})
		}
	}
	return moves
}

// applyColumnCollapse returns a new column layout derived from snapshot by
// applying the given moves. It is a pure function with no ebiten dependency.
//
// Algorithm:
//  1. Start with every column copied from snapshot (preserves unchanged columns).
//  2. For each move {oldCol, newCol}, set result[newCol] = snapshot[oldCol].
//  3. Zero only the vacated columns: {oldCol for each move} MINUS {newCol for
//     each move}. These are the edge columns that lost their content and were
//     not themselves targeted as a destination.
//
// Columns that neither moved nor were landed on are preserved unchanged.
func applyColumnCollapse(snapshot [][]Cell, moves [][2]int) [][]Cell {
	numCols := len(snapshot)
	result := make([][]Cell, numCols)
	for c := 0; c < numCols; c++ {
		col := make([]Cell, len(snapshot[c]))
		copy(col, snapshot[c])
		result[c] = col
	}

	// Collect newCol values so we can compute the vacated set.
	newColSet := make(map[int]bool, len(moves))
	for _, m := range moves {
		newColSet[m[1]] = true
	}

	// Apply moves: destination gets source column's data.
	for _, m := range moves {
		copy(result[m[1]], snapshot[m[0]])
	}

	// Zero vacated columns: sources that are not also destinations.
	for _, m := range moves {
		if !newColSet[m[0]] {
			for r := range result[m[0]] {
				result[m[0]][r] = Cell{}
			}
		}
	}

	return result
}

// elapsedFraction returns how far through a phase we are as a 0→1 float32.
// currentTick is the game tick at the time of evaluation, startTick is when
// the phase began, and durationTicks is the total length of the phase.
func elapsedFraction(currentTick, startTick, durationTicks int64) float32 {
	return float32(currentTick-startTick) / float32(durationTicks)
}

// deviceCellCenter returns the device-pixel centre of a grid cell for the given
// device cell size dcs.  Used wherever pixel coordinates must map to the
// device-resolution canvas or screen.
func deviceCellCenter(col, row, dcs int) (float32, float32) {
	return float32(col*dcs + dcs/2), float32(row*dcs + dcs/2)
}

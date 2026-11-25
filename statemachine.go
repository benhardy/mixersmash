package main

import (
	"fmt"
	"image"
	"math"
	"math/rand"
	"runtime"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"mixersmash/internal/anim"
	"mixersmash/internal/board"
)

// gameState identifies which phase of the update/animation state machine the
// game is in. It is a distinct type so the dispatch switch and the state-aware
// animation helpers cannot be passed an arbitrary int.
type gameState int

const (
	stateWaiting          gameState = 1
	stateShuffling        gameState = 2
	stateDropping         gameState = 3
	stateVictimMoving     gameState = 4
	stateColumnCollapsing gameState = 5

	// Animation durations expressed as wall-clock time.
	// Values are equivalent to the original tick counts at 60 TPS:
	//   anim.ShuffleDuration = 18 ticks / 60 TPS = 300ms
	//   anim.DropDuration    = 18 ticks / 60 TPS = 300ms
	//   victimDuration       = 24 ticks / 60 TPS = 400ms
	//   targetAnimDuration   = 15 ticks / 60 TPS = 250ms
	victimDuration     = 24 * time.Second / 60 // 400ms
	targetAnimDuration = 15 * time.Second / 60 // 250ms

	healthDecrementKeep = float32(0.75)
	healthMin           = float32(0.562)
	healthEpsilon       = float32(0.001)

	mixerHealthChangeOnClick = float32(-0.334) // each click reduces health
)

// emptyColumns returns the sorted indices of columns in which every cell has
// Health <= 0.
func (g *Game) emptyColumns() []int {
	var result []int
	for c := 0; c < g.board.grid.Cols; c++ {
		empty := true
		for r := 0; r < g.board.grid.Rows; r++ {
			if g.board.grid.At(c, r).Health() > 0 {
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
// safety).
func (g *Game) applyDropAndClear(col int, holeRows []int, moves [][2]int, snapshot []board.Cell) {
	// Write moved alive cells to their new positions.
	for _, m := range moves {
		g.board.grid.Place(col, m[1], snapshot[m[0]])
	}

	// Zero the top slots that were vacated and are not being filled.
	for i := 0; i < len(holeRows); i++ {
		g.board.grid.Place(col, i, board.Cell{}) // Health=0, invisible; canvas already cleared by vacating moves
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
		cell := g.board.grid.At(col, m[1])
		fx, fy := board.DeviceCellCenter(col, m[0], dcs)
		tx, ty := board.DeviceCellCenter(col, m[1], dcs)
		g.anim.moveAnims = append(g.anim.moveAnims, board.MoveAnim{FromX: fx, FromY: fy, ToX: tx, ToY: ty, Cell: cell})
	}
}

// buildDropAnims computes MoveAnim entries for each affected column: alive cells
// compact downward. Only canvas positions that cells are vacating are cleared —
// not the whole column.
func (g *Game) buildDropAnims(removedCols []int) {
	if cap(g.anim.moveAnims) > 0 {
		g.anim.moveAnims = g.anim.moveAnims[:0]
	}
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
			if g.board.grid.At(col, row).Health() <= 0 {
				holeRowsArr[holeRowsCount] = row
				holeRowsCount++
			}
		}
		holeRows := holeRowsArr[:holeRowsCount]
		if len(holeRows) == 0 {
			continue
		}

		moves := board.GravityMoves(g.board.grid.Rows, holeRows)

		// Take a snapshot of the column before any writes (aliasing safety).
		snapshot := g.board.grid.Column(col)

		g.applyDropAndClear(col, holeRows, moves, snapshot)
		g.buildDropMoveAnims(col, holeRows, moves, g.view.deviceCellSize)
	}
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
	moves := board.HorizontalCollapseMoves(g.board.grid.Cols, emptyCols)
	if len(moves) == 0 {
		return false
	}

	// Apply the pure column reassignment (Snapshot copy acceptable: rare click-time path).
	newCells := board.ApplyColumnCollapse(g.board.grid.Snapshot(), moves)
	for c := 0; c < g.board.grid.Cols; c++ {
		g.board.grid.ReplaceColumn(c, newCells[c])
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
			cell := g.board.grid.At(newCol, r)
			if cell.Health() <= 0 {
				continue
			}
			fx, fy := board.DeviceCellCenter(oldCol, r, dcs)
			tx, ty := board.DeviceCellCenter(newCol, r, dcs)
			g.anim.moveAnims = append(g.anim.moveAnims, board.MoveAnim{FromX: fx, FromY: fy, ToX: tx, ToY: ty, Cell: cell})
		}
	}

	g.anim.animStart = time.Now()
	g.anim.currentState = stateColumnCollapsing
	return true
}

// enterWaiting repaints the full grid canvas from the model and transitions to
// stateWaiting. All settle-to-waiting paths must go through this helper so
// canvas drift is corrected on every settle.
func (g *Game) enterWaiting() {
	// If a device realloc fired during animation, the sprite cache was not cleared
	// mid-flight. Consume the pending flag now so repaintGrid bakes at the new scale.
	if g.anim.pendingSpriteCacheReset {
		clear(g.spriteCache)
		g.anim.pendingSpriteCacheReset = false
	}
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
	elapsed := board.ElapsedFraction(g.anim.animStart, time.Now(), anim.DropDuration)
	if elapsed >= 1.0 {
		dcs := g.view.deviceCellSize
		for _, mv := range g.anim.moveAnims {
			col := int(mv.ToX) / dcs
			row := int(mv.ToY) / dcs
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
	neighbors := board.EightNeighbors(nbuf[:0], col, row, g.board.grid.Cols, g.board.grid.Rows)

	// Collect alive neighbours.
	var aliveNeighbors [][2]int
	for _, n := range neighbors {
		if g.board.grid.At(n[0], n[1]).Health() > 0 {
			aliveNeighbors = append(aliveNeighbors, n)
		}
	}
	mixerAnim := board.MoveAnim{
		FromX:     float32(col*dcs + dcs/2),
		FromY:     float32(row*dcs + dcs/2),
		ToX:       float32(col*dcs + dcs/2),
		ToY:       float32(row*dcs + dcs/2),
		CtrlX:     float32(col*dcs + dcs/2),
		CtrlY:     float32(row*dcs + dcs/2),
		Cell:      board.NewCell(g.board.grid.At(col, row).Organism(), oldMixerHealth),
		NewHealth: newMixerHealth,
		PathType:  0, // stationary path
		Rotations: 1, // reserved for future spin; currently unused
	}
	anims := make([]board.MoveAnim, len(aliveNeighbors)+1) // +1 for the mixer itself
	anims[0] = mixerAnim
	if len(aliveNeighbors) > 0 {
		// Fisher-Yates shuffle the positions (perm[i] = destination index for neighbour i).
		perm := rand.Perm(len(aliveNeighbors))

		// Snapshot cells before modifying grid.
		type neighborCell struct {
			col, row int
			cell     board.Cell
		}
		orig := make([]neighborCell, len(aliveNeighbors))
		for i, n := range aliveNeighbors {
			orig[i] = neighborCell{n[0], n[1], g.board.grid.At(n[0], n[1])}
		}

		// Build MoveAnims: neighbour i animates from its current centre to perm[i]'s centre.
		for i := range aliveNeighbors {
			src := orig[i]
			dst := aliveNeighbors[perm[i]]
			fx, fy := board.DeviceCellCenter(src.col, src.row, dcs)
			tx, ty := board.DeviceCellCenter(dst[0], dst[1], dcs)
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
			anims[i+1] = board.MoveAnim{FromX: fx, FromY: fy, ToX: tx, ToY: ty, CtrlX: ctrlX, CtrlY: ctrlY, Cell: src.cell}
		}

		// Clear source positions on canvas (they will be drawn as moving overlays).
		for _, n := range aliveNeighbors {
			x0, y0 := n[0]*dcs, n[1]*dcs
			g.view.canvas.SubImage(image.Rect(x0, y0, x0+dcs, y0+dcs)).(*ebiten.Image).Clear()
		}

		// Write cells to their permuted destinations in the grid now (canvas repaint on settle).
		for i := range aliveNeighbors {
			dst := aliveNeighbors[perm[i]]
			g.board.grid.Place(dst[0], dst[1], orig[i].cell)
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
	elapsed := board.ElapsedFraction(g.anim.animStart, time.Now(), anim.ShuffleDuration)
	if elapsed < 1.0 {
		return nil
	}
	g.anim.moveAnims = nil
	// Check whether the mixer at the action position was killed this click.
	killed := g.anim.actionCol >= 0 &&
		g.anim.actionCol < g.board.grid.Cols &&
		g.anim.actionRow >= 0 &&
		g.anim.actionRow < g.board.grid.Rows &&
		g.board.grid.At(g.anim.actionCol, g.anim.actionRow).Health() <= 0
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
	g.buildDropAnims([]int{g.anim.actionCol})
	if len(g.anim.moveAnims) > 0 {
		g.anim.animStart = time.Now()
		g.anim.currentState = stateDropping
	} else {
		g.settleAfterRemoval()
	}
	return nil
}

func (g *Game) commitMixer(col int, row int) error {
	oldHealth := g.board.grid.BumpHealthAt(col, row, mixerHealthChangeOnClick)
	newHealth := g.board.grid.At(col, row).Health()
	g.drawCell(col, row, 1.0)
	// Record the action position so updateShuffling can detect a killing click.
	g.anim.actionCol = col
	g.anim.actionRow = row
	// Show new size, shuffle neighbours.  On the killing click the mixer
	// shrinks to 0 during this animation; updateShuffling will route to the
	// drop pipeline when it detects IsDead() at the action position.
	g.buildShuffleAnims(col, row, oldHealth, newHealth)
	return nil
}

// commitCellAt clears the flood-match cluster rooted at an animal cell through
// the victim-shrink→drop pipeline. Clicking a non-animal, empty, or dead cell
// is a no-op. Out-of-bounds clicks are ignored.
func (g *Game) commitCellAt(col, row int) error {
	if col < 0 || col >= g.board.grid.Cols || row < 0 || row >= g.board.grid.Rows {
		return nil
	}
	cell := g.board.grid.At(col, row)
	if cell.Health() <= 0 {
		return nil
	}

	// Mixer click path: increment click count and either shuffle or remove.
	if cell.Organism() != nil && cell.Organism().IsMixer {
		return g.commitMixer(col, row)
	}

	if cell.Organism() == nil || !cell.Organism().IsAnimal {
		return nil
	}

	coords := g.board.grid.FloodMatchCluster(col, row)

	// No-op if the cluster contains no food: a lone animal (or animals-only
	// cluster with no paired food) must not clear.
	if !g.board.grid.ClusterHasFood(coords) {
		return nil
	}

	g.anim.actionCol = col
	g.anim.actionRow = row
	g.anim.victimCount = 0

	dcs := g.view.deviceCellSize
	if dcs <= 0 {
		dcs = cellSize
	}

	seedCenterX, seedCenterY := board.DeviceCellCenter(col, row, dcs)

	for _, coord := range coords {
		c, r := coord[0], coord[1]
		snap := g.board.grid.At(c, r)
		organism := snap.Organism()
		oldHealth := snap.Health()
		g.board.grid.SetHealthZero(c, r)
		g.drawCell(c, r, 1.0)

		fromX, fromY := board.DeviceCellCenter(c, r, dcs)

		var v board.MoveAnim
		if c == col && r == row {
			// Seed cell: shrinks in place (stationary).
			v = board.MoveAnim{
				Col:       c,
				Row:       r,
				Cell:      board.NewCell(organism, oldHealth),
				FromX:     fromX,
				FromY:     fromY,
				ToX:       fromX,
				ToY:       fromY,
				PathType:  0,
				Rotations: 0,
			}
		} else {
			// Non-seed: animate toward the seed using spiral/curved path.
			pt, cx, cy, st := anim.VictimPathParams(fromX, fromY, seedCenterX, seedCenterY)
			// Clear victim's canvas position (it is now an overlay animation).
			x0, y0 := c*dcs, r*dcs
			g.view.canvas.SubImage(image.Rect(x0, y0, x0+dcs, y0+dcs)).(*ebiten.Image).Clear()
			v = board.MoveAnim{
				Col:         c,
				Row:         r,
				Cell:        board.NewCell(organism, oldHealth),
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

// updateVictimMoving advances the victim movement animation and transitions to
// stateDropping (or stateWaiting) when the animation completes.
func (g *Game) updateVictimMoving() error {
	t := board.ElapsedFraction(g.anim.animStart, time.Now(), victimDuration)
	if t < 1.0 {
		return nil
	}
	// Animation complete — score the clear and trigger drop.
	// All victim cells already have Health=0 (set in commitCellAt).
	g.board.score += board.ScoreForCluster(g.anim.victimCount)
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
	g.buildDropAnims(affectedColsArr[:affectedColsCount])
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
	elapsed := board.ElapsedFraction(g.anim.animStart, time.Now(), anim.DropDuration)
	if elapsed < 1.0 {
		return nil
	}
	dcs := g.view.deviceCellSize
	for _, mv := range g.anim.moveAnims {
		col := int(mv.ToX) / dcs
		row := int(mv.ToY) / dcs
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

	ebiten.SetWindowTitle(fmt.Sprintf("MixerSmash — cells: %d  score: %d", g.board.grid.Live(), g.board.score))

	// Reset hover every tick; recompute below if conditions are met.
	g.input.hoverCol, g.input.hoverRow = -1, -1
	g.input.victimHints = g.input.victimHints[:0]

	if g.anim.targetAnimating {
		if board.ElapsedFraction(g.anim.targetAnimStart, time.Now(), targetAnimDuration) >= 1.0 {
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

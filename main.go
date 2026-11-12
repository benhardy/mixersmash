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

    stateWaiting   = 1
    stateShuffling = 2
    stateDropping  = 3

    shuffleTicks = 18 // 0.3s at 60tps
    dropTicks    = 18 // 0.3s at 60tps

    healthDecrement = float32(0.2)
    healthMin       = float32(0.2)
    healthEpsilon   = float32(0.001)
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

// colorDistanceMax returns the max absolute difference across the R, G, B
// channels of two colours. Used for colour matching.
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
// with Health > 0 that has minimum colorDistanceMax distance to
// cells[col][row].Color. Returns found=false if no healthy neighbour exists.
func closestColorNeighbor(cells [][]Cell, col, row, cols, rows int) (int, int, bool) {
    neighbors := eightNeighbors(col, row, cols, rows)
    target := cells[col][row].Color
    bestDist := -1
    bestCol, bestRow := 0, 0
    found := false
    for _, n := range neighbors {
        nc, nr := n[0], n[1]
        if cells[nc][nr].Health <= 0 {
            continue
        }
        dist := colorDistanceMax(target, cells[nc][nr].Color)
        if !found || dist < bestDist {
            bestDist = dist
            bestCol = nc
            bestRow = nr
            found = true
        }
    }
    return bestCol, bestRow, found
}

type Cell struct {
    Color  color.RGBA
    Health float32
}

// MoveAnim describes the animated movement of a cell from one pixel position to
// another during shuffle or drop phases. FromX/FromY and ToX/ToY are pixel centres.
type MoveAnim struct {
    FromX, FromY float32
    ToX, ToY     float32
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
    grid         *Grid
    canvas       *ebiten.Image
    currentState int        // which state are we in?
    animStart    int64      // when did the current animation phase start?
    actionCol    int        // the column an action is happening
    actionRow    int        // the row any given action is happening
    moveAnims    []MoveAnim // used for both shuffle and drop phases
    glowShader   *ebiten.Shader
}

// newGameState returns a Game with data fields initialised but no ebiten.Image
// allocated.  Callers that need a canvas (i.e. main) must set g.canvas after
// calling this.
func newGameState(grid *Grid) *Game {
    return &Game{
        grid:         grid,
        currentState: stateWaiting,
        animStart:    0,
        actionCol:    -1,
        actionRow:    -1,
    }
}

// elapsedFraction returns how far through a phase we are as a 0→1 float32.
// currentTick is the game tick at the time of evaluation, startTick is when
// the phase began, and durationTicks is the total length of the phase.
func elapsedFraction(currentTick, startTick, durationTicks int64) float32 {
    return float32(currentTick-startTick) / float32(durationTicks)
}

// cellCenter returns the pixel centre of a grid cell.
func cellCenter(col, row int) (float32, float32) {
    return float32(col*cellSize + cellSize/2), float32(row*cellSize + cellSize/2)
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

// rowCell is a helper pairing a row index with its cell, used in buildDropAnims.
type rowCell struct {
    row  int
    cell Cell
}

// buildDropAnims computes MoveAnim entries for each affected column: alive cells
// compact downward, and new random cells fall from above the screen top.
func (g *Game) buildDropAnims(removedCols []int) {
    g.moveAnims = g.moveAnims[:0]
    seen := make(map[int]bool)
    for _, col := range removedCols {
        if seen[col] {
            continue
        }
        seen[col] = true

        // Collect alive cells in top-to-bottom order, tracking their current row.
        var alive []rowCell
        for row := 0; row < g.grid.Rows; row++ {
            if g.grid.Cells[col][row].Health > 0 {
                alive = append(alive, rowCell{row, g.grid.Cells[col][row]})
            }
        }

        newCount := g.grid.Rows - len(alive)

        // Build MoveAnims for alive cells moving from their old row to their new row.
        for i, rc := range alive {
            newRow := newCount + i
            if rc.row != newRow {
                fx, fy := cellCenter(col, rc.row)
                tx, ty := cellCenter(col, newRow)
                g.moveAnims = append(g.moveAnims, MoveAnim{fx, fy, tx, ty, rc.cell})
            }
            g.grid.Cells[col][newRow] = rc.cell
        }

        // Add new random cells at the top, animating in from above the screen.
        for i := 0; i < newCount; i++ {
            newCell := Cell{Color: randomColor(), Health: 1.0}
            g.grid.Cells[col][i] = newCell
            fx := float32(col*cellSize + cellSize/2)
            fy := float32(-cellSize / 2)
            tx, ty := cellCenter(col, i)
            g.moveAnims = append(g.moveAnims, MoveAnim{fx, fy, tx, ty, newCell})
        }

        // Clear entire column on canvas so animations render over blank cells.
        for row := 0; row < g.grid.Rows; row++ {
            g.canvas.SubImage(image.Rect(col*cellSize, row*cellSize, (col+1)*cellSize, (row+1)*cellSize)).(*ebiten.Image).Clear()
        }
    }
}

// Update handles input each tick.
func (g *Game) Update() error {
    if g.currentState == stateWaiting {
        if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
            px, py := ebiten.CursorPosition()
            col, row := cellFromPixel(px, py, cellSize)
            if col < 0 || col >= g.grid.Cols || row < 0 || row >= g.grid.Rows {
                return nil
            }
            cell := &g.grid.Cells[col][row]
            if cell.Health <= 0 {
                return nil
            }

            cell.Health -= healthDecrement
            if cell.Health <= healthMin {
                cell.Health = 0
            }
            if cell.Health < healthEpsilon {
                cell.Health = 0.0
            }

            // Redraw the clicked cell immediately on the canvas.
            g.drawCell(col, row, cell, cellSize, 1.0)
            g.actionCol = col
            g.actionRow = row

            if cell.Health <= 0 {
                // DEATH PATH
                affectedCols := []int{col}
                ncol, nrow, found := closestColorNeighbor(g.grid.Cells, col, row, g.grid.Cols, g.grid.Rows)
                if found {
                    g.grid.Cells[ncol][nrow].Health = 0
                    g.drawCell(ncol, nrow, &g.grid.Cells[ncol][nrow], cellSize, 1.0)
                    // Track the neighbor's column too if different.
                    if ncol != col {
                        affectedCols = append(affectedCols, ncol)
                    }
                }
                g.buildDropAnims(affectedCols)
                g.currentState = stateDropping
                g.animStart = ebiten.Tick()
            } else {
                // SURVIVAL PATH — shuffle neighbours
                neighbors := eightNeighbors(col, row, g.grid.Cols, g.grid.Rows)
                if len(neighbors) == 0 {
                    return nil
                }
                perm := rand.Perm(len(neighbors))

                // Collect neighbour cells before rearranging.
                type neighborCell struct {
                    col, row int
                    cell     Cell
                }
                orig := make([]neighborCell, len(neighbors))
                for i, n := range neighbors {
                    orig[i] = neighborCell{n[0], n[1], g.grid.Cells[n[0]][n[1]]}
                }

                // Build MoveAnims: neighbor i moves to position perm[i].
                anims := make([]MoveAnim, len(neighbors))
                for i := range neighbors {
                    src := orig[i]
                    dst := neighbors[perm[i]]
                    fx, fy := cellCenter(src.col, src.row)
                    tx, ty := cellCenter(dst[0], dst[1])
                    anims[i] = MoveAnim{fx, fy, tx, ty, src.cell}
                }

                // Clear source positions on canvas.
                for _, n := range neighbors {
                    g.canvas.SubImage(image.Rect(n[0]*cellSize, n[1]*cellSize, (n[0]+1)*cellSize, (n[1]+1)*cellSize)).(*ebiten.Image).Clear()
                }

                // Write cells to their permuted destinations in the grid.
                for i := range neighbors {
                    dst := neighbors[perm[i]]
                    g.grid.Cells[dst[0]][dst[1]] = orig[i].cell
                }

                g.moveAnims = anims
                g.currentState = stateShuffling
                g.animStart = ebiten.Tick()
            }
        }
        return nil
    }

    if g.currentState == stateShuffling {
        elapsed := elapsedFraction(ebiten.Tick(), g.animStart, shuffleTicks)
        if elapsed >= 1.0 {
            for _, anim := range g.moveAnims {
                col := int(anim.ToX) / cellSize
                row := int(anim.ToY) / cellSize
                if col >= 0 && col < g.grid.Cols && row >= 0 && row < g.grid.Rows {
                    g.drawCell(col, row, &g.grid.Cells[col][row], cellSize, 1.0)
                }
            }
            g.moveAnims = nil
            g.currentState = stateWaiting
        }
        return nil
    }

    if g.currentState == stateDropping {
        elapsed := elapsedFraction(ebiten.Tick(), g.animStart, dropTicks)
        if elapsed >= 1.0 {
            for _, anim := range g.moveAnims {
                col := int(anim.ToX) / cellSize
                row := int(anim.ToY) / cellSize
                if col >= 0 && col < g.grid.Cols && row >= 0 && row < g.grid.Rows {
                    g.drawCell(col, row, &g.grid.Cells[col][row], cellSize, 1.0)
                }
            }
            g.moveAnims = nil
            g.currentState = stateWaiting
        }
        return nil
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
    screen.DrawImage(g.canvas, nil)

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
        maxR := float32(cellSize/2 - 2)
        for _, anim := range g.moveAnims {
            cx := anim.FromX + t*(anim.ToX-anim.FromX)
            cy := anim.FromY + t*(anim.ToY-anim.FromY)
            r := maxR * anim.Cell.Health
            if r > 0 {
                vector.DrawFilledCircle(screen, cx, cy, r, anim.Cell.Color, true)
            }
        }
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
    game.animStart = ebiten.Tick()

    for c := 0; c < grid.Cols; c++ {
        for r := 0; r < grid.Rows; r++ {
            game.drawCell(c, r, &grid.Cells[c][r], cellSize, 1.0)
        }
    }

    if err := ebiten.RunGame(game); err != nil {
        log.Fatal(err)
    }
}

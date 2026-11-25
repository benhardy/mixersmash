package main

import (
	"log"
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	text "github.com/hajimehoshi/ebiten/v2/text/v2"
	"mixersmash/internal/board"
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

	// showOrientationDebug controls the small orientation/dimension readout drawn
	// in the bottom-left corner of the frame each tick.  Set false to hide it.
	showOrientationDebug = false

	// showPerfDebug controls the per-frame performance readout drawn in the
	// bottom-right corner of the frame.  Set false to compile it out entirely.
	showPerfDebug = false
)

// Game is the Ebitengine game state.
type Game struct {
	// render resources (shaders and glyph font) — baked once, not game state.
	glowShader *ebiten.Shader
	gelShader  *ebiten.Shader
	glyphFont  *text.GoTextFaceSource

	// spriteCache holds the lazily-baked composite sprite per organism, keyed by
	// organism pointer. Cleared on a device-scale change (LayoutF) so sprites
	// rebake at the new size. This is a render cache, not game state.
	spriteCache map[*board.Organism]*ebiten.Image

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
	grid      *board.Grid
	cellCount int // number of live cells currently in the grid
	score     int // cumulative score
}

// animState holds the update/animation state machine: the current phase, the
// wall-clock start of the active phase, the in-flight action cell, and the
// move/victim/target animation buffers. It is driven by the update* methods and
// consumed by the draw* methods.
type animState struct {
	currentState gameState        // which state are we in?
	animStart    time.Time        // when did the current animation phase start?
	actionCol    int              // the column an action is happening
	actionRow    int              // the row any given action is happening
	moveAnims    []board.MoveAnim // used for both shuffle and drop phases

	// victim movement animation fields (one entry per simultaneous victim)
	victimCount int
	victimAnims []board.MoveAnim

	// target cell bounce animation fields
	targetAnimating    bool
	targetAnimStart    time.Time
	targetAnimStartR   float32
	targetAnimPeakR    float32
	targetAnimEndR     float32
	targetAnimOrganism *board.Organism

	// pendingSpriteCacheReset is set when a device realloc (LayoutF) fires during
	// an animation. The sprite cache is not cleared mid-animation to avoid a
	// re-bake storm; instead the clear is deferred to the next enterWaiting call.
	pendingSpriteCacheReset bool
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
func newGameState(grid *board.Grid) *Game {
	return &Game{
		spriteCache: make(map[*board.Organism]*ebiten.Image),
		board: boardState{
			grid:      grid,
			cellCount: grid.Cols * grid.Rows,
		},
		anim: animState{
			currentState:    stateWaiting,
			actionCol:       -1,
			actionRow:       -1,
			victimAnims:     make([]board.MoveAnim, gridCols*gridRows), // flood can clear up to Cols*Rows cells
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

func main() {
	ebiten.SetWindowSize(hudWidth+windowWidth, windowHeight)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowTitle("go2d grid")
	ebiten.SetWindowClosingHandled(false)

	grid := board.NewGrid(gridCols, gridRows)

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

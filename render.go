package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"strconv"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	text "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/basicfont"
	"mixersmash/internal/anim"
	"mixersmash/internal/board"
)

// compositeSpriteRefSize is the logical fallback side length used only before
// the first LayoutF call sets g.spriteRefSize.  At runtime, spriteRefSize is
// derived from deviceCellSize*2 (capped at 512) and updated on scale changes.
const compositeSpriteRefSize = 120

// hudFace is the shared GoXFace for HUD and orientation-debug text.
// Created once at package init to avoid a per-frame allocation.
var hudFace = text.NewGoXFace(basicfont.Face7x13)

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
func (g *Game) compositeCachedSprite(o *board.Organism) *ebiten.Image {
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
	tint := board.GlyphTint(clr)
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
func (g *Game) drawCompositeSprite(dst *ebiten.Image, cx, cy, radius float32, o *board.Organism) {
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
	c := g.board.grid.At(col, row)
	cell := &c
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
	toX, toY := board.DeviceCellCenter(g.input.hoverCol, g.input.hoverRow, dcs)
	// Dot spacing and radius scale with device scale so they appear the same
	// physical size on all displays.
	dotSpacing := float32(8) * float32(s)
	dotRadius := float32(4.0) * float32(s)
	// Offset advances at 20 logical px/sec = 20*s device px/sec = 20*s/60 device px/tick.
	rawOff := float64(ebiten.Tick()) * (20.0 * s / 60.0)
	offset := float32(math.Mod(rawOff, float64(dotSpacing)))
	for _, hint := range g.input.victimHints {
		hc, hr := hint[0], hint[1]
		fromX, fromY := board.DeviceCellCenter(hc, hr, dcs)
		dx := toX - fromX
		dy := toY - fromY
		length := float32(math.Sqrt(float64(dx*dx + dy*dy)))
		if length <= 0 {
			continue
		}
		ux := dx / length
		uy := dy / length
		vc := g.board.grid.At(hc, hr).Color()
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
	t := board.ElapsedFraction(g.anim.targetAnimStart, time.Now(), targetAnimDuration)
	t = min(t, 1.0)
	var r float32
	if t < 0.5 {
		lt := t * 2                   // 0→1 in phase 1
		eased := anim.EaseOutQuad(lt) // ease-out: fast start, slow at peak
		r = anim.Lerp(g.anim.targetAnimStartR, g.anim.targetAnimPeakR, eased)
	} else {
		lt := (t - 0.5) * 2          // 0→1 in phase 2
		eased := anim.EaseInQuad(lt) // ease-in: slow at peak, fast at end
		r = anim.Lerp(g.anim.targetAnimPeakR, g.anim.targetAnimEndR, eased)
	}
	dcs := g.view.deviceCellSize
	if dcs <= 0 {
		dcs = cellSize
	}
	cx, cy := board.DeviceCellCenter(g.anim.actionCol, g.anim.actionRow, dcs)
	if r > 0 {
		g.drawCompositeSprite(dst, cx+float32(xOffset), cy, r, g.anim.targetAnimOrganism)
	}
}

// kindOf converts a main-package gameState to the anim.StateKind used by the
// anim package functions.  This is the only coupling point between the two
// state enumerations; no shim types or aliases are created.
func kindOf(state gameState) anim.StateKind {
	switch state {
	case stateShuffling:
		return anim.KindShuffle
	case stateDropping, stateColumnCollapsing:
		return anim.KindDrop
	default:
		return anim.KindOther
	}
}

// drawMoveAnims renders in-motion cells for shuffle, drop, and victim-move animations.
// xOffset is added to all board-local X coordinates before drawing into dst.
func (g *Game) drawMoveAnims(dst *ebiten.Image, xOffset int) {
	xOff := float32(xOffset)
	if len(g.anim.moveAnims) > 0 {
		dur := anim.MoveAnimDuration(kindOf(g.anim.currentState))
		t := board.ElapsedFraction(g.anim.animStart, time.Now(), dur)
		t = min(t, 1.0)
		dcs := g.view.deviceCellSize
		if dcs <= 0 {
			dcs = cellSize
		}
		maxR := float32(dcs/2 - 2)
		for _, mv := range g.anim.moveAnims {
			cx, cy := anim.MoveAnimPos(mv, kindOf(g.anim.currentState), t)
			var r float32
			if g.anim.currentState == stateShuffling && mv.Rotations != 0 {
				// Clicked-mixer entry only: interpolate radius from old health to new health
				// with a gentle bounce envelope so the shrink is visually sprung.
				// Identified by Rotations!=0 (set to 1 only on the prepended mixer entry in
				// buildShuffleAnims); neighbour Mixers have Rotations==0 and fall through
				// to the standard radius path below. Works for NewHealth==0 (final click).
				// mv.Cell.Health = pre-click health; mv.NewHealth = post-click health.
				et := anim.EaseInOutQuad(t)
				base := anim.Lerp(mv.Cell.Health, mv.NewHealth, et)
				// Bounce: gentle spring envelope peaks at t=0.5, amplitude 15%.
				bounce := base * (1 + 0.15*float32(math.Sin(float64(t)*math.Pi)))
				r = maxR * bounce
			} else {
				r = maxR * mv.Cell.Health
			}
			if r > 0 {
				g.drawCompositeSprite(dst, xOff+cx, cy, r, mv.Cell.Organism)
			}
		}
	}

	if g.anim.currentState == stateVictimMoving {
		t := board.ElapsedFraction(g.anim.animStart, time.Now(), victimDuration)
		t = min(t, 1.0)
		for i := 0; i < g.anim.victimCount; i++ {
			v := g.anim.victimAnims[i]
			cx, cy := anim.VictimAnimPos(v, t)
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

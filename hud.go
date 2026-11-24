package main

import (
	"image/color"
	"math"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// hud holds the persistent cache image for the HUD strip and the last-rendered
// state used by the dirty-check.  Zero value is safe; image starts nil.
type hud struct {
	image        *ebiten.Image // persistent cache image, nil until first render
	lastScore    int           // was Game.lastHUDScore
	lastHoverCol int           // was Game.lastHUDHoverCol
	lastHoverRow int           // was Game.lastHUDHoverRow
	lastTheme    string        // was Game.lastHUDThemeName
}

// hudTextScale is the integer magnification factor applied to HUD text.
// Text is first rendered into a small temporary image at 1× (basicfont
// Face7x13: 7×13 px glyphs) and then blit into the frame scaled up by this
// factor, giving blocky crisp 3× text with no drop shadow.
const hudTextScale = 3

// hudWrapText splits text into lines each no longer than maxChars characters,
// breaking only at space boundaries.  Lines that have no space and exceed
// maxChars are left unbroken.
func hudWrapText(txt string, maxChars int) []string {
	if maxChars <= 0 || len(txt) <= maxChars {
		return []string{txt}
	}
	var lines []string
	for len(txt) > maxChars {
		cut := maxChars
		// Walk back from maxChars to find the last space.
		for cut > 0 && txt[cut-1] != ' ' {
			cut--
		}
		if cut == 0 {
			// No space found — emit the whole remaining string as one line.
			break
		}
		lines = append(lines, strings.TrimRight(txt[:cut], " "))
		txt = txt[cut:]
	}
	if len(txt) > 0 {
		lines = append(lines, txt)
	}
	return lines
}

// draw renders the left HUD strip into frame.
// hudDevW is the device-pixel width of the strip.
//
// The HUD is rendered into h.image (a persistent cache) and re-rendered only
// when the content or size changes.  Every frame we blit the cached image with
// one DrawImage — no per-frame text render, no per-frame allocation.
func (h *hud) draw(frame *ebiten.Image, hudDevW int, score int, hoverCol, hoverRow int, grid *Grid) {
	frameH := frame.Bounds().Dy()
	const themeName = "MixerSmash"

	// Dirty check: re-render the cache if anything changed.
	needRender := h.needsRedraw(hudDevW, frameH, score, hoverCol, hoverRow, themeName)

	if needRender {
		// Allocate or reallocate the cache image when size changed.
		if h.image == nil || h.image.Bounds().Dx() != hudDevW || h.image.Bounds().Dy() != frameH {
			h.image = ebiten.NewImage(hudDevW, frameH)
		} else {
			h.image.Clear()
		}

		// Fill the HUD strip with a light background so the dark title image
		// and black text are visible.
		vector.FillRect(h.image, 0, 0, float32(hudDevW), float32(frameH),
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
				h.image.DrawImage(titleImage, &tOpts)
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
		rawLines := h.buildTextLines(score, hoverCol, hoverRow, maxChars, grid)
		type textLine struct{ s string }
		lines := make([]textLine, len(rawLines))
		for i, s := range rawLines {
			lines[i] = textLine{s}
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
		h.image.DrawImage(tmp, &opts)

		// Update last-rendered state.
		h.lastScore = score
		h.lastHoverCol = hoverCol
		h.lastHoverRow = hoverRow
		h.lastTheme = themeName
	}

	// Every frame: blit the cached HUD image into the frame at the HUD strip origin.
	frame.DrawImage(h.image, nil)
}

// needsRedraw reports whether the HUD cache needs to be re-rendered.
// It is true when the cache image is absent, the size has changed, or any
// piece of displayed content has changed since the last render.
func (h *hud) needsRedraw(hudDevW, frameH, score, hoverCol, hoverRow int, themeName string) bool {
	return h.image == nil ||
		h.image.Bounds().Dx() != hudDevW ||
		h.image.Bounds().Dy() != frameH ||
		score != h.lastScore ||
		hoverCol != h.lastHoverCol ||
		hoverRow != h.lastHoverRow ||
		themeName != h.lastTheme
}

// buildTextLines returns the ordered list of HUD text strings: the fixed
// "Level 1" header, the game instructions (wrapped to maxChars), score line, and
// an optional blank spacer + "Name - Power" line when the hovered cell is
// alive.  Pure; no ebiten calls.
func (h *hud) buildTextLines(score, hoverCol, hoverRow, maxChars int, grid *Grid) []string {
	var lines []string
	lines = append(lines, "Level 1")
	lines = append(lines, hudWrapText("Match animals with their food", maxChars)...)
	lines = append(lines, "Score: "+strconv.Itoa(score))

	if hoverCol >= 0 && hoverRow >= 0 &&
		hoverCol < grid.Cols && hoverRow < grid.Rows {
		cell := &grid.Cells[hoverCol][hoverRow]
		if cell.Health > 0 && cell.Organism != nil {
			name := cell.Organism.Name
			if len(name) > 0 {
				name = strings.ToUpper(name[:1]) + name[1:]
			}
			lines = append(lines, "") // blank spacer
			lines = append(lines, name)
		}
	}
	return lines
}

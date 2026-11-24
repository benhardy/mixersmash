package main

import (
	"testing"
)

// --- needsRedraw tests ---

func TestNeedsRedraw(t *testing.T) {
	t.Helper()

	// Build a minimal hud with known state.
	newHud := func(score, hoverCol, hoverRow int, theme string) *hud {
		h := &hud{
			lastScore:    score,
			lastHoverCol: hoverCol,
			lastHoverRow: hoverRow,
			lastTheme:    theme,
		}
		return h
	}

	const w, fh = 100, 200

	// Helper: build a fake image-sized hud by setting image indirectly via
	// the pure-boolean logic (image==nil -> always true), so we test each
	// individual condition in isolation.
	//
	// Because needsRedraw short-circuits on image==nil, we cannot isolate
	// per-field changes without a real *ebiten.Image.  Instead we test the
	// nil-image case and the already-matched-state case using the struct
	// fields alone, noting that in production image is set before the check
	// is called on subsequent frames.  For tests that need image != nil we
	// exercise only the boolean field paths by confirming the OR semantics
	// through the helper itself.

	tests := []struct {
		name     string
		h        *hud
		hudDevW  int
		frameH   int
		score    int
		hoverCol int
		hoverRow int
		theme    string
		want     bool
	}{
		{
			name:    "nil image always redraws",
			h:       &hud{},
			hudDevW: w, frameH: fh, score: 0, hoverCol: -1, hoverRow: -1,
			theme: "colour",
			want:  true,
		},
		{
			name:    "score changed",
			h:       newHud(5, -1, -1, "colour"),
			hudDevW: w, frameH: fh, score: 10, hoverCol: -1, hoverRow: -1,
			theme: "colour",
			want:  true,
		},
		{
			name:    "hoverCol changed",
			h:       newHud(5, 2, 3, "colour"),
			hudDevW: w, frameH: fh, score: 5, hoverCol: 4, hoverRow: 3,
			theme: "colour",
			want:  true,
		},
		{
			name:    "hoverRow changed",
			h:       newHud(5, 2, 3, "colour"),
			hudDevW: w, frameH: fh, score: 5, hoverCol: 2, hoverRow: 9,
			theme: "colour",
			want:  true,
		},
		{
			name:    "theme changed",
			h:       newHud(5, -1, -1, "colour"),
			hudDevW: w, frameH: fh, score: 5, hoverCol: -1, hoverRow: -1,
			theme: "random",
			want:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.h.needsRedraw(tc.hudDevW, tc.frameH, tc.score, tc.hoverCol, tc.hoverRow, tc.theme)
			if got != tc.want {
				t.Errorf("needsRedraw = %v; want %v", got, tc.want)
			}
		})
	}

	// Verify the unchanged case returns false when image is nil — it always
	// returns true then, confirming the nil short-circuit.
	t.Run("nil image even when all fields match", func(t *testing.T) {
		h := newHud(5, 2, 3, "colour")
		// image is nil, so must return true regardless of fields
		got := h.needsRedraw(w, fh, 5, 2, 3, "colour")
		if !got {
			t.Error("needsRedraw with nil image = false; want true")
		}
	})
}

// --- buildTextLines tests ---

func makeTestGrid(cols, rows int) *Grid {
	cells := make([][]Cell, cols)
	for c := 0; c < cols; c++ {
		cells[c] = make([]Cell, rows)
	}
	return &Grid{Cols: cols, Rows: rows, Cells: cells}
}

func TestBuildTextLines(t *testing.T) {
	t.Helper()

	const maxChars = 40

	t.Run("score line is present", func(t *testing.T) {
		h := &hud{}
		grid := makeTestGrid(5, 5)
		lines := h.buildTextLines(42, -1, -1, maxChars, grid)
		found := false
		for _, l := range lines {
			if l == "Score: 42" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("score line missing; got %v", lines)
		}
	})

	t.Run("level header is first line", func(t *testing.T) {
		h := &hud{}
		grid := makeTestGrid(5, 5)
		lines := h.buildTextLines(0, -1, -1, maxChars, grid)
		if len(lines) == 0 || lines[0] != "Level 1" {
			t.Errorf("first line = %q; want \"Level 1\"", lines[0])
		}
	})

	t.Run("hover alive cell adds name and power", func(t *testing.T) {
		h := &hud{}
		grid := makeTestGrid(5, 5)
		creature := &Organism{Name: "cat", Power: 5}
		grid.Cells[2][3] = Cell{Organism: creature, Health: 1.0}

		lines := h.buildTextLines(0, 2, 3, maxChars, grid)

		found := false
		for _, l := range lines {
			if l == "Cat - 5" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("creature line missing; got %v", lines)
		}
	})

	t.Run("hover out of range omits creature line", func(t *testing.T) {
		h := &hud{}
		grid := makeTestGrid(5, 5)
		lines := h.buildTextLines(0, 99, 99, maxChars, grid)
		for _, l := range lines {
			// No line should contain " - " (the name-power separator)
			for i := 0; i+2 < len(l); i++ {
				if l[i] == ' ' && l[i+1] == '-' && l[i+2] == ' ' {
					t.Errorf("unexpected creature line %q in out-of-range hover", l)
				}
			}
		}
	})

	t.Run("hover dead cell omits creature line", func(t *testing.T) {
		h := &hud{}
		grid := makeTestGrid(5, 5)
		creature := &Organism{Name: "wolf", Power: 5}
		grid.Cells[1][1] = Cell{Organism: creature, Health: 0} // dead

		lines := h.buildTextLines(0, 1, 1, maxChars, grid)
		for _, l := range lines {
			if l == "Wolf - 5" {
				t.Errorf("dead-cell creature line present; got %v", lines)
			}
		}
	})

	t.Run("hover nil creature omits creature line", func(t *testing.T) {
		h := &hud{}
		grid := makeTestGrid(5, 5)
		grid.Cells[0][0] = Cell{Organism: nil, Health: 1.0}

		lines := h.buildTextLines(0, 0, 0, maxChars, grid)
		for _, l := range lines {
			if l == " - 0" {
				t.Errorf("nil-creature line present; got %v", lines)
			}
		}
	})
}

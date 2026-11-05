package main

import (
	"image/color"
	"testing"
)

// TestRandomColorAlpha verifies that randomColor returns a colour with A == 255.
func TestRandomColorAlpha(t *testing.T) {
	for i := 0; i < 100; i++ {
		c := randomColor()
		if c.A != 255 {
			t.Fatalf("randomColor() returned A=%d, want 255", c.A)
		}
	}
}

// TestRandomColorReturnsRGBA verifies that randomColor returns a color.RGBA value.
func TestRandomColorReturnsRGBA(t *testing.T) {
	var _ color.RGBA = randomColor()
}

// TestRecolorReturnsDifferentColor verifies that recolor returns a colour
// different from the one passed in. We try several times because there is a
// tiny probability that two random colours coincide.
func TestRecolorReturnsDifferentColor(t *testing.T) {
	current := color.RGBA{R: 128, G: 64, B: 32, A: 255}
	same := 0
	const trials = 50
	for i := 0; i < trials; i++ {
		next := recolor(current)
		if next.A != 255 {
			t.Fatalf("recolor() returned A=%d, want 255", next.A)
		}
		if next == current {
			same++
		}
	}
	// Collisions are acceptable occasionally but never on every trial.
	if same == trials {
		t.Fatalf("recolor() returned the same colour on all %d trials", trials)
	}
}

// TestCellFromPixel verifies that cellFromPixel maps pixel coordinates to the
// correct (col, row) grid cell.
func TestCellFromPixel(t *testing.T) {
	cases := []struct {
		px, py, cellSize int
		wantCol, wantRow int
	}{
		{0, 0, 32, 0, 0},
		{31, 31, 32, 0, 0},
		{32, 0, 32, 1, 0},
		{0, 32, 32, 0, 1},
		{64, 96, 32, 2, 3},
		{10, 20, 10, 1, 2},
	}
	for _, tc := range cases {
		col, row := cellFromPixel(tc.px, tc.py, tc.cellSize)
		if col != tc.wantCol || row != tc.wantRow {
			t.Errorf("cellFromPixel(%d, %d, %d) = (%d, %d), want (%d, %d)",
				tc.px, tc.py, tc.cellSize, col, row, tc.wantCol, tc.wantRow)
		}
	}
}

// TestGridInitialisation verifies that NewGrid returns a Grid whose Colors
// slice has dimensions [Cols][Rows] with every cell having A == 255.
func TestGridInitialisation(t *testing.T) {
	const cols, rows = 5, 4
	g := NewGrid(cols, rows)

	if g.Cols != cols {
		t.Errorf("Grid.Cols = %d, want %d", g.Cols, cols)
	}
	if g.Rows != rows {
		t.Errorf("Grid.Rows = %d, want %d", g.Rows, rows)
	}
	if len(g.Colors) != cols {
		t.Fatalf("len(Grid.Colors) = %d, want %d", len(g.Colors), cols)
	}
	for c := 0; c < cols; c++ {
		if len(g.Colors[c]) != rows {
			t.Fatalf("len(Grid.Colors[%d]) = %d, want %d", c, len(g.Colors[c]), rows)
		}
		for r := 0; r < rows; r++ {
			cell := g.Colors[c][r]
			if cell.A != 255 {
				t.Errorf("Grid.Colors[%d][%d].A = %d, want 255", c, r, cell.A)
			}
		}
	}
}

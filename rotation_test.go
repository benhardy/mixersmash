package main

import "testing"

// TestShuffleRotation verifies that shuffleRotation maps uniform input r in
// [0,1) to a signed spin count according to the half-open band specification:
//
//	[0.00, 0.05) -> -2
//	[0.05, 0.25) -> -1
//	[0.25, 0.75) ->  0
//	[0.75, 0.95) -> +1
//	[0.95, 1.00) -> +2
//
// Boundaries belong to the band they open: 0.05 -> -1, 0.25 -> 0, 0.75 -> +1,
// 0.95 -> +2.
func TestShuffleRotation(t *testing.T) {
	t.Helper()

	cases := []struct {
		r    float32
		want float32
	}{
		// Band [0.00, 0.05) -> -2
		{r: 0.000, want: -2},
		{r: 0.025, want: -2},
		{r: 0.049, want: -2},
		// Band [0.05, 0.25) -> -1  (0.05 is the band boundary opening this range)
		{r: 0.050, want: -1},
		{r: 0.100, want: -1},
		{r: 0.249, want: -1},
		// Band [0.25, 0.75) -> 0  (0.25 is the band boundary opening this range)
		{r: 0.250, want: 0},
		{r: 0.500, want: 0},
		{r: 0.749, want: 0},
		// Band [0.75, 0.95) -> +1  (0.75 is the band boundary opening this range)
		{r: 0.750, want: +1},
		{r: 0.850, want: +1},
		{r: 0.949, want: +1},
		// Band [0.95, 1.00) -> +2  (0.95 is the band boundary opening this range)
		{r: 0.950, want: +2},
		{r: 0.999, want: +2},
	}

	for _, tc := range cases {
		got := shuffleRotation(tc.r)
		if got != tc.want {
			t.Errorf("shuffleRotation(%v) = %v; want %v", tc.r, got, tc.want)
		}
	}
}

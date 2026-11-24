package main

import (
	"math/rand"
	"sort"
	"testing"
)

// TestMixerFullness verifies the three shrink levels.
func TestMixerFullness(t *testing.T) {
	cases := []struct {
		clicks int
		want   float32
	}{
		{0, 1.0},
		{1, 2.0 / 3.0},
		{2, 1.0 / 3.0},
	}
	for _, tc := range cases {
		t.Run("", func(t *testing.T) {
			got := mixerFullness(tc.clicks)
			if got != tc.want {
				t.Errorf("mixerFullness(%d) = %v; want %v", tc.clicks, got, tc.want)
			}
		})
	}
}

// TestMixerClickOutcome verifies increment and clear behaviour.
func TestMixerClickOutcome(t *testing.T) {
	cases := []struct {
		clicks    int
		wantNew   int
		wantClear bool
	}{
		{0, 1, false},
		{1, 2, false},
		{2, 3, true},
	}
	for _, tc := range cases {
		t.Run("", func(t *testing.T) {
			newClicks, clears := mixerClickOutcome(tc.clicks)
			if newClicks != tc.wantNew {
				t.Errorf("mixerClickOutcome(%d) newClicks = %d; want %d", tc.clicks, newClicks, tc.wantNew)
			}
			if clears != tc.wantClear {
				t.Errorf("mixerClickOutcome(%d) clears = %v; want %v", tc.clicks, clears, tc.wantClear)
			}
		})
	}
}

// TestMixerClickOutcome_HealthEffect checks that clicks 1 and 2 do not set
// clears=true (Health must not be zeroed) and click 3 does set clears=true.
func TestMixerClickOutcome_HealthEffect(t *testing.T) {
	// Simulate three sequential clicks on a fresh Mixer.
	health := float32(1.0)
	clicks := 0
	for i := 1; i <= 3; i++ {
		newClicks, clears := mixerClickOutcome(clicks)
		clicks = newClicks
		if clears {
			health = 0
		}
		if i < 3 && health != 1.0 {
			t.Errorf("click %d: health should remain 1.0 but got %v", i, health)
		}
		if i == 3 && health != 0 {
			t.Errorf("click 3: health should be 0 but got %v", health)
		}
	}
}

// shufflePermutation applies a Fisher-Yates shuffle to n positions using the
// provided rand.Rand and returns the permutation (perm[i] = destination of i).
// This matches the rand.Perm call inside buildShuffleAnims.
func shufflePermutation(n int, r *rand.Rand) []int {
	return r.Perm(n)
}

// TestShufflePermutation verifies that shufflePermutation returns a valid
// bijection: every index in [0,n) appears exactly once.
func TestShufflePermutation(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	for _, n := range []int{1, 2, 4, 8} {
		perm := shufflePermutation(n, r)
		if len(perm) != n {
			t.Fatalf("n=%d: perm length = %d", n, len(perm))
		}
		got := make([]int, n)
		copy(got, perm)
		sort.Ints(got)
		for i, v := range got {
			if v != i {
				t.Errorf("n=%d: sorted perm[%d] = %d; want %d (not a valid permutation)", n, i, v, i)
			}
		}
	}
}

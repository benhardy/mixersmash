package main

import (
	"math/rand"
	"sort"
	"testing"
)

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

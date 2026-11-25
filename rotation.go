package main

// shuffleRotation maps a uniform random value r in [0,1) to a signed spin
// count for the shuffle-neighbour cosmetic rotation animation.
//
// Bands (half-open, boundary opens the band):
//
//	[0.00, 0.05) -> -2
//	[0.05, 0.25) -> -1
//	[0.25, 0.75) ->  0
//	[0.75, 0.95) -> +1
//	[0.95, 1.00) -> +2
func shuffleRotation(r float32) float32 {
	switch {
	case r < 0.05:
		return -2
	case r < 0.25:
		return -1
	case r < 0.75:
		return 0
	case r < 0.95:
		return 1
	default:
		return 2
	}
}

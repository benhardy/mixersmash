package main

import (
	"math"
	"time"
)

// moveAnimDuration returns the wall-clock animation duration for the given
// game state.  stateDropping and stateColumnCollapsing use dropDuration;
// all other states (including stateShuffling) use shuffleDuration.
func moveAnimDuration(state int) time.Duration {
	if state == stateDropping || state == stateColumnCollapsing {
		return dropDuration
	}
	return shuffleDuration
}

// moveAnimPos returns the pixel centre (cx, cy) for a MoveAnim at normalised
// time t.  For stateShuffling the path is a quadratic ease-in-out Bézier; for
// all other states it is a quadratic ease-in straight-line lerp.
// Math copied verbatim from drawMoveAnims.
func moveAnimPos(anim MoveAnim, state int, t float32) (cx, cy float32) {
	if state == stateShuffling {
		et := easeInOutQuad(t)
		cx, cy = quadBezier2(anim.FromX, anim.FromY, anim.CtrlX, anim.CtrlY, anim.ToX, anim.ToY, et)
	} else {
		posT := easeInQuad(t)
		cx, cy = lerp2(anim.FromX, anim.FromY, anim.ToX, anim.ToY, posT)
	}
	return cx, cy
}

// victimAnimPos returns the pixel centre (cx, cy) for a victim MoveAnim at
// normalised time t.  PathType 0 = lerp, 1 = Bézier arc, 2 = inward spiral.
// Math copied verbatim from drawMoveAnims.
func victimAnimPos(v MoveAnim, t float32) (cx, cy float32) {
	switch v.PathType {
	case 0: // straight line
		cx, cy = lerp2(v.FromX, v.FromY, v.ToX, v.ToY, t)
	case 1: // quadratic Bézier arc
		cx, cy = quadBezier2(v.FromX, v.FromY, v.CtrlX, v.CtrlY, v.ToX, v.ToY, t)
	case 2: // inward spiral (radius shrinks, angle rotates)
		ddx := v.FromX - v.ToX
		ddy := v.FromY - v.ToY
		r0 := float32(math.Sqrt(float64(ddx*ddx + ddy*ddy)))
		theta0 := float32(math.Atan2(float64(ddy), float64(ddx)))
		r := r0 * (1 - t)
		theta := theta0 + v.SpiralTurns*2*float32(math.Pi)*t
		cx = v.ToX + r*float32(math.Cos(float64(theta)))
		cy = v.ToY + r*float32(math.Sin(float64(theta)))
	}
	return cx, cy
}

package main

import (
	"math"
	"math/rand"
	"time"

	"mixersmash/internal/board"
)

// MoveAnimDuration returns the wall-clock animation duration for the given
// game state.  stateDropping and stateColumnCollapsing use dropDuration;
// all other states (including stateShuffling) use shuffleDuration.
func MoveAnimDuration(state gameState) time.Duration {
	if state == stateDropping || state == stateColumnCollapsing {
		return dropDuration
	}
	return shuffleDuration
}

// MoveAnimPos returns the pixel centre (cx, cy) for a MoveAnim at normalised
// time t.  For stateShuffling the path is a quadratic ease-in-out Bézier; for
// all other states it is a quadratic ease-in straight-line lerp.
// Math copied verbatim from drawMoveAnims.
func MoveAnimPos(anim board.MoveAnim, state gameState, t float32) (cx, cy float32) {
	if state == stateShuffling {
		et := EaseInOutQuad(t)
		cx, cy = QuadBezier2(anim.FromX, anim.FromY, anim.CtrlX, anim.CtrlY, anim.ToX, anim.ToY, et)
	} else {
		posT := EaseInQuad(t)
		cx, cy = Lerp2(anim.FromX, anim.FromY, anim.ToX, anim.ToY, posT)
	}
	return cx, cy
}

// VictimPathParams selects a random path type for a flood victim animating
// (fromX,fromY) to (toX,toY) and returns PathType, CtrlX, CtrlY, and
// SpiralTurns for use in a MoveAnim victim entry.
func VictimPathParams(fromX, fromY, toX, toY float32) (pathType int, ctrlX, ctrlY, spiralTurns float32) {
	pathType = rand.Intn(3)
	dx := toX - fromX
	dy := toY - fromY
	dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
	if pathType == 1 { // Bezier arc: control point perpendicular to the line
		midX := (fromX + toX) * 0.5
		midY := (fromY + toY) * 0.5
		var pX, pY float32
		if dist > 0 {
			pX = -dy / dist
			pY = dx / dist
		}
		sign := float32(1)
		if rand.Intn(2) == 0 {
			sign = -1
		}
		offset := sign * dist * (0.5 + rand.Float32()*1.0)
		ctrlX = midX + pX*offset
		ctrlY = midY + pY*offset
	}
	if pathType == 2 { // logarithmic spiral
		turns := float32(0.5 + rand.Float32()*1.0) // 0.5–1.5 turns
		if rand.Intn(2) == 0 {
			turns = -turns
		}
		spiralTurns = turns
	}
	return
}

// VictimAnimPos returns the pixel centre (cx, cy) for a victim MoveAnim at
// normalised time t.  PathType 0 = lerp, 1 = Bézier arc, 2 = inward spiral.
// Math copied verbatim from drawMoveAnims.
func VictimAnimPos(v board.MoveAnim, t float32) (cx, cy float32) {
	switch v.PathType {
	case 0: // straight line
		cx, cy = Lerp2(v.FromX, v.FromY, v.ToX, v.ToY, t)
	case 1: // quadratic Bézier arc
		cx, cy = QuadBezier2(v.FromX, v.FromY, v.CtrlX, v.CtrlY, v.ToX, v.ToY, t)
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

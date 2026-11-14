package main

// lerp returns the linear interpolation between a and b at parameter t.
// t=0 returns a, t=1 returns b; values outside [0,1] extrapolate linearly.
func lerp(a, b, t float32) float32 {
	return a + (b-a)*t
}

// lerp2 returns the 2D linear interpolation between (ax,ay) and (bx,by) at t.
func lerp2(ax, ay, bx, by, t float32) (float32, float32) {
	return lerp(ax, bx, t), lerp(ay, by, t)
}

// quadBezier returns the 1D quadratic Bézier value at parameter t, with
// start p0, control point c, and end p1.
// t=0 returns p0, t=1 returns p1.
func quadBezier(p0, c, p1, t float32) float32 {
	u := 1 - t
	return u*u*p0 + 2*u*t*c + t*t*p1
}

// quadBezier2 returns the 2D quadratic Bézier position at parameter t.
// (p0x,p0y) is the start, (cx,cy) is the control point, (p1x,p1y) is the end.
func quadBezier2(p0x, p0y, cx, cy, p1x, p1y, t float32) (float32, float32) {
	return quadBezier(p0x, cx, p1x, t), quadBezier(p0y, cy, p1y, t)
}

// easeInQuad returns the quadratic ease-in value for t: t².
// t=0 returns 0, t=1 returns 1.
func easeInQuad(t float32) float32 {
	return t * t
}

// easeOutQuad returns the quadratic ease-out value for t: 1-(1-t)².
// t=0 returns 0, t=1 returns 1.
func easeOutQuad(t float32) float32 {
	u := 1 - t
	return 1 - u*u
}

// easeInOutQuad returns the quadratic ease-in-out value for t.
// t=0 returns 0, t=0.5 returns 0.5, t=1 returns 1.
func easeInOutQuad(t float32) float32 {
	if t < 0.5 {
		return 2 * t * t
	}
	return 1 - 2*(1-t)*(1-t)
}

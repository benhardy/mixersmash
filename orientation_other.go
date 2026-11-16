//go:build !js

package main

// deviceOrientationAngle returns the device orientation angle in degrees as
// reported by the browser's screen.orientation.angle API.  On non-JS platforms
// this always returns -1 (unavailable).
func deviceOrientationAngle() int {
	return -1
}

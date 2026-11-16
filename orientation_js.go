//go:build js

package main

import "syscall/js"

// deviceOrientationAngle returns the device orientation angle in degrees as
// reported by the browser's screen.orientation.angle API.  Returns -1 when
// the API is unavailable (older browsers or non-screen environments) or when
// the value is NaN/undefined.
func deviceOrientationAngle() int {
	screen := js.Global().Get("screen")
	if screen.IsUndefined() || screen.IsNull() {
		return -1
	}
	orientation := screen.Get("orientation")
	if orientation.IsUndefined() || orientation.IsNull() {
		return -1
	}
	angleVal := orientation.Get("angle")
	if angleVal.IsUndefined() || angleVal.IsNull() {
		return -1
	}
	if angleVal.Type() != js.TypeNumber {
		return -1
	}
	f := angleVal.Float()
	if f != f { // NaN check
		return -1
	}
	return int(f)
}

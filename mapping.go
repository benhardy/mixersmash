package main

// screenToBoard converts raw screen-pixel coordinates (sx, sy) to a board cell
// (col, row), accounting for both the presentGeoM letterbox and the HUD strip
// that occupies the left hudDeviceW device-pixels of the frame.
//
// The function first converts to frame coordinates via screenToFrame, then
// subtracts the HUD offset.  A tap that lands in the HUD strip (boardX < 0)
// or outside the frame returns (-1, -1).  dcs is the device-pixel cell size.
func screenToBoard(sx, sy, sw, sh, fw, fh float64, portrait bool, hudDeviceW, dcs int) (col, row int) {
	fx, fy := screenToFrame(sx, sy, sw, sh, fw, fh, portrait)
	boardX := fx - float64(hudDeviceW)
	if boardX < 0 || fy < 0 || fx > fw || fy > fh {
		return -1, -1
	}
	if dcs <= 0 {
		return -1, -1
	}
	return int(boardX) / dcs, int(fy) / dcs
}

// screenToFrame converts a raw screen-pixel coordinate (sx, sy) to a frame
// coordinate (fx, fy), inverting the presentGeoM mapping applied in Draw.
//
// The inverse is computed by building the same GeoM as presentGeoM and calling
// Invert(), ensuring perfect consistency regardless of orientation.  A tap that
// maps outside [0, fw] × [0, fh] lands in a letterbox bar; callers should treat
// out-of-range results as misses (the existing cell bounds check will reject them).
func screenToFrame(sx, sy, sw, sh, fw, fh float64, portrait bool) (float64, float64) {
	m := presentGeoM(portrait, sw, sh, fw, fh)
	if !m.IsInvertible() {
		return sx, sy
	}
	m.Invert()
	return m.Apply(sx, sy)
}

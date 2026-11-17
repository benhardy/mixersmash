#!/usr/bin/env bash
# headless_capture.sh — build and screenshot the game via Xvfb + software GL.
#
# Usage: scripts/headless_capture.sh [output.png] [sleep_seconds]
#   output.png     destination path (default: /tmp/go2d_capture.png)
#   sleep_seconds  how long to let the game warm up before capture (default: 4)
#
# Requires: Xvfb, ImageMagick (import), LIBGL_ALWAYS_SOFTWARE-capable Mesa.
# Run from the repo root.

set -euo pipefail

OUTFILE="${1:-/tmp/go2d_capture.png}"
SLEEP_SECS="${2:-4}"
BINARY=/tmp/go2d_headless_cap
DISPLAY_NUM=:98

# Clean up on exit
cleanup() {
    kill "$GAME_PID" 2>/dev/null || true
    kill "$XVFB_PID" 2>/dev/null || true
}
trap cleanup EXIT

# 1. Build
LIBGL_ALWAYS_SOFTWARE=1 go build -o "$BINARY" . 1>&2

# 2. Start virtual display
Xvfb "$DISPLAY_NUM" -screen 0 1280x800x24 &
XVFB_PID=$!
sleep 1

# 3. Run game
DISPLAY="$DISPLAY_NUM" LIBGL_ALWAYS_SOFTWARE=1 "$BINARY" &
GAME_PID=$!

# 4. Wait for game to render
sleep "$SLEEP_SECS"

# 5. Capture
import -window root -display "$DISPLAY_NUM" "$OUTFILE" 1>&2

echo "$OUTFILE"

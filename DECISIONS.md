# Design Decisions

A chronological log of significant decisions made during development.

---

## Project setup

**Go + Ebitengine**
Chose `github.com/hajimehoshi/ebiten/v2` as the game engine. Ebitengine's `purego` darwin backend means no CGo is required on macOS, so cross-compilation from Linux to macOS arm64 works out of the box without osxcross.

**Window: 800×600, cell size: 32×32**
Gives a 25×18 grid (450 cells). User selected 800×600 from options of 640×480, 800×600, 1024×768.

**Default build target: macOS arm64**
Development environment is a Linux VM; target machine is Apple Silicon Mac. `make` cross-compiles with `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0`. Ebitengine's purego darwin backend makes this feasible.

---

## Rendering

**Offscreen canvas**
Circles are drawn to a persistent `*ebiten.Image` (offscreen canvas). `Draw()` blits it to screen in one call. Per-cell redraws happen only on state changes (click, animation tick), not every frame. This keeps per-frame work at O(1) for the circle layer.

**Transparent canvas**
The canvas background is transparent (not black). `Draw()` fills the screen black first, then composites layers in order: background → links → canvas (circles) → overlays. This allows elements to be rendered under circles without the canvas's opaque black covering them.

`SubImage.Clear()` is used to erase cell regions on the canvas. Drawing `color.RGBA{0,0,0,0}` with normal blend mode has no effect (blending transparent over existing pixels changes nothing); `SubImage.Clear()` performs a pixel-replace (BlendCopy equivalent) that actually zeros the region.

**Alpha-coverage antialiasing**
`vector.DrawFilledCircle(..., antialias: true)` uses Ebitengine's built-in GPU-side alpha-coverage antialiasing — a single draw call with no extra render passes. Same for `StrokeCircle`.

**Circle margin: 2px**
Circles are drawn at radius 14px within a 32px cell (2px margin each side, 4px gap between adjacent circles).

---

## Health mechanic

`healthDecrement = 0.2` — five clicks kill a circle. Health scales the circle radius: `radius = maxRadius × health`. Dead cells (health ≤ 0) are invisible and not clickable.

`healthMin = 0.2` and `healthEpsilon = 0.001` — a cell at or below `healthMin` clamps to zero on the next drain rather than subtracting. `healthEpsilon` zeroes out float noise near zero.

**Skip spawning on death**
When health hits zero, the collapse animation completes but no spawn animation plays. The circle stays invisible. This was added after the initial collapse/spawn animation was introduced.

---

## Glow / highlight

**Kage shader for radial gradient glow**
A custom Kage (Ebitengine shading language) shader computes a radial gradient ring: white at `InnerRadius`, transparent at `OuterRadius`. Ebitengine uses premultiplied alpha internally — the shader returns `vec4(alpha, alpha, alpha, alpha)` not `vec4(1,1,1,alpha)`. Shader is compiled once on first `Draw()` call and reused.

The inner edge of the glow is softened over 1px (`clamp(dist - (InnerRadius - 1.0), 0, 1)`) to match the antialiased circle edge and eliminate the visible gap between circle and glow ring.

The glow is drawn directly to the screen (not to the offscreen canvas) so it does not interfere with canvas cell management.

**Proximity highlight → removed**
An initial "nearest healthy cell" highlight was replaced by a link-drag glow (glow fires only while mouse is held down during link creation). Later removed entirely when the linking mechanic was abandoned.

---

## Linking mechanic (implemented, then abandoned)

**4-directional neighbours**
Cardinal directions only (N/S/E/W) chosen over 8-directional. `neighbourOffsets` var makes it easy to extend to 8 directions later by changing one line.

**Links as normalised pairs**
Links stored as `[4]int{srcCol, srcRow, dstCol, dstRow}` with lexicographic normalisation so A→B and B→A are the same entry. No directed links.

**Links disappear on cell death**
When a cell's health reaches zero, all its confirmed links are removed. Source corner cells in the colour-flow mode are exempt from this (links preserved on resurrection).

**Distributed health drain**
Clicking a cell with N confirmed links drains all N+1 cells (clicked + linked) by `healthDecrement / (N+1)` each. Total drain is always `healthDecrement` regardless of link count.

**Render order: links under circles**
Confirmed links (metallic pipe strokes) are drawn to the screen before the canvas blit so circles appear on top of pipes.

---

## Colour-flow prototype (implemented, then abandoned)

A game mode where three source corners emit fixed colours along link paths toward a target corner. Automatic per-tick colour propagation along confirmed links.

**Key technical findings:**
- Bidirectional in-place lerp in a single loop pass causes exponential signal attenuation — each hop sees an already-diluted colour. Fixed with a snapshot-before-apply pattern: all lerps read from a pre-tick snapshot and write to the live grid.
- Target corner must be a pure sink (never emits colour back upstream) and source corners must be pure emitters (never absorb). Without these constraints, the target's initial random colour bleeds backward and contaminates the network.

**Abandoned** — the core gameplay loop was passive (build a network, then wait). Insufficient moment-to-moment decision-making.

---

## Shuffle + drop game (current direction)

**On click (survival):** health reduced; if health > 0, the clicked circle's 8 immediate neighbours (all cardinal + diagonal) are randomly shuffled to new positions among themselves, animated over 0.3 seconds (18 ticks at 60 TPS).

**On click (death):** health reaches zero; no shuffle. The dead circle and the neighbour with the most closely matching colour (by max channel distance) both disappear. Both affected columns collapse downward with Tetris-style gravity — alive cells pack toward the bottom, new random circles spawn at the top — animated over 0.3 seconds.

**8-directional neighbours for shuffle**
Unlike the linking mechanic (4-directional), the shuffle uses all 8 neighbours (cardinal + diagonal) for a richer visual effect.

**MoveAnim stores pixel centres**
Animation interpolates between `(FromX, FromY)` and `(ToX, ToY)` pixel centre coordinates rather than grid indices. New cells spawning from above screen use `FromY = -cellSize/2` (above the top edge). Animating circles are drawn as screen overlays; their canvas cells are cleared for the duration of the animation and redrawn on canvas when animation completes.

---

## Testing approach

Pure functions are extracted from game logic so they can be unit-tested without an Ebitengine context (which requires a display). Functions that touch `*ebiten.Image`, `ebiten.GameTick()`, or input are not directly tested; their callers are tested where possible via injectable tick parameters or struct initialisation helpers (`newGameState` constructs game data without calling `ebiten.NewImage`).

---

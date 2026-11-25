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

## Victim selection

**Perceptual colour matching via redmean**
Replaced the original max-channel distance metric with the redmean approximation: green is weighted 4× relative to red/blue, and the red/blue weights shift with average redness. Chosen because human colour perception is non-uniform; max-channel distance incorrectly treats large blue differences as equivalent to large green ones. The formula is a single pass over R/G/B — no external dependency.

---

## Click interaction

**Target-cell bounce on click**
When the player clicks a cell, the target cell animates +10% radius then shrinks back to its natural size before the outcome (shuffle or death) plays. This gives immediate tactile feedback before the slower outcome animation.

**Multiple victims on a closest-colour tie**
When the closest-colour neighbour search produces a tie, all tied neighbours become victims — not just one. All tied victims converge on the target cell's position, then die together. Ties are resolved symmetrically; the player sees all matching cells converge before they disappear.

---

## Scoring

**Base kill + stacking bonuses**
Each target-removing kill awards 5 points base. Bonuses stack on top:
- Each additional victim beyond the first: +20
- Each victim that shares the target cell's colour: +40 per victim
- Each victim matching the colour of the other victims (all-same-colour kill): +30 per victim

Same-colour-as-target (+40) and all-same-colour (+30) bonuses are mutually exclusive per victim; same-colour-as-target takes precedence.

---

## Creature silhouettes

**Monochrome glyph font for per-cell decoration**
Each cell displays a decorative creature silhouette rendered from Noto Emoji (OFL licence, embedded in the binary via `embed.FS`). A fixed palette maps each of the 64 palette colours to a specific creature; colour is derived from the creature, not stored independently on the cell. The glyph is drawn in a darker shade of the cell colour so it reads as a shadow/silhouette rather than a separate element.

---

## Rendering evolution

**Sprite baking pipeline**
The cell sprite went through three generations:
1. Per-frame `DrawFilledCircle` + glyph drawn at render time — no caching.
2. Pre-rendered circle-clipped composite sprite (fill + glyph masked to circle boundary), cached per palette colour. 2× supersampling + downscale at blit provided the antialiased edge.
3. Single composite sprite baked through a Kage fragment shader (`gel shader`): the shader computes a vertical gradient, bottom caustic, upper gloss crescent, and rim analytically, producing an aqua "jelly" appearance. The glyph is drawn onto the fill image *before* the gel shader runs, so it appears embedded under the gel lighting.

Sprites are baked once per palette entry and cached on `Creature.Sprite`; they are re-baked on device scale change.

**Critical Kage/atlas finding**
When a Kage shader samples a bound source image, geometry must be computed in local coordinates (`srcPos - imageSrc0Origin()`, range `0..Size`), not in raw `srcPos` (atlas coordinates). Using raw `srcPos` causes sprites whose backing image lands at a non-zero atlas offset to bake fully transparent — a silent failure with no shader error. Sampling continues to use `srcPos`; only geometry (distance-from-centre, etc.) uses localised coords.

**Premultiplied alpha**
Ebitengine stores images in premultiplied alpha. A shader returning `vec4(1, 1, 1, alpha)` produces blown-out pixels at partial transparency. Correct form: `vec4(alpha, alpha, alpha, alpha)` — RGB components must not exceed alpha.

**Baking inside the game loop**
Initial grid paint and sprite baking must happen on the first in-loop frame (after `RunGame` starts), not before. Calling `ebiten.NewImage` or drawing before the loop starts can produce blank images on some backends.

---

## HiDPI and display

**Device-resolution rendering**
The scene is rendered at `LayoutF × DeviceScaleFactor` so sprites are crisp at native resolution in both a browser (`devicePixelRatio > 1`) and on Retina displays. Sprites are re-baked whenever the device scale changes. The native window is made resizable.

---

## Naming and build targets

**Game name: MixerSmash**
The display name is MixerSmash; the Go module and build artifacts remain `go2d`. Rename was limited to the module declaration and artifact names — the directory and import path were not changed.

**WASM build target and GitHub Pages hosting**
Added `GOOS=js GOARCH=wasm` as a build target for a browser-playable version. A local Go web server (`tools/web_server/main.go`, `//go:build ignore`) serves the WASM build during development. The browser version is hosted on GitHub Pages (static files, deploy action).

---

## Gameplay mechanics

**Horizontal column collapse toward centre**
After vertical gravity (dead cells fall, new cells spawn at top), empty columns collapse horizontally toward the grid centre. This prevents the board fragmenting into sparse islands after many deaths.

**Three clicks to kill**
`healthDecrement` changed from `0.2` (five clicks) to `1/3` (three clicks). Increases pace; cells clear faster, raising the tempo of collapse and scoring opportunities.

---

## Code organisation

**Model: Cell holds a Creature**
A `Cell`'s identity is the creature it holds. `Cell` holds a `*Creature` reference; colour is derived via `Cell.Color()` rather than stored directly. The baked sprite is cached on `Creature.Sprite`, keyed by creature pointer. This keeps colour and sprite always consistent with the creature.

**File and type splits**
- `lerp.go`: interpolation and easing functions extracted from `main.go`.
- `main.go` split into `grid.go`, `shaders.go`, `device.go`.
- `VictimAnim` merged into `MoveAnim` — the same type handles both ordinary moves and victim convergence.
- Builtin `min`/`max` adopted in place of local clamp helpers.

---

## Mobile browser support

**Touch input alongside mouse**
Touch events are handled in parallel with mouse events. Touch-and-hold mirrors hover; release commits the tap. Mouse and touch are deduped to avoid double-fire on synthesised click events that mobile browsers emit after a touch sequence. When a touch ends, the commit uses the last-known touch position rather than the released touch coordinates, because a released touch ID reports `(0, 0)`.

**Landscape-only web shell**
The HTML shell targets landscape orientation only:
- A `tap-to-play` overlay requests best-effort fullscreen and orientation lock (via the Screen Orientation API) on tap. This works on Android; it no-ops on iOS, which does not permit JS-triggered orientation lock.
- A CSS portrait-mode overlay ("rotate to landscape") provides the iOS guarantee: if the device is held in portrait, the overlay covers the canvas and prompts the user to rotate.
- Viewport meta, `touch-action: none`, and safe-area CSS env vars are set to prevent browser chrome interference and rubber-band scroll.
- A loading indicator appears only if WASM load exceeds approximately 1 second, avoiding a flash on fast connections.

Verified on desktop browsers and mobile (Chrome/Android, Safari/iOS).

---

## Tooling and process

**golangci-lint as part of the done check**
`golangci-lint` is run with staticcheck, errcheck, ineffassign, unused, and gocritic enabled. Lint passes are required before a task is considered done, alongside `go build ./...` and `go test ./...`.

**Checked-in pre-commit hook**
`.githooks/pre-commit` runs gofmt, `go vet`, `go build`, `go test`, and golangci-lint in sequence. Activated via `git config core.hooksPath .githooks` (or `make hooks`). This ensures the full gate cannot be skipped: `go build ./...` alone does not compile test files, so the hook explicitly runs both build and test passes.

---

## Major mechanic pivot: creature connection graph

**Colour similarity as matching basis — explored then abandoned**
Two perceptual colour metrics were prototyped: redmean (weighted RGB Euclidean) and a hue-weighted HSV distance. A palette comparison was also explored. All were discarded: colour similarity produced perceptually surprising matches (players could not reliably predict which neighbours would be affected) and gave no strategic depth. Colour is now purely visual identity and plays no role in matching.

**Reduced palette and OKLCh derivation**
Creature count reduced from ~57 to 23. The new palette is generated in OKLCh (two lightness bands: L=0.68 and L=0.52, moderate chroma 0.13, 12 hues evenly spaced) for perceptually even, pleasant, non-neon colours. Values are baked as sRGB constants with a documented derivation in `tools/gen_palette`. Colour-distance code (redmean, hue-weighted HSV, closest-colour neighbour selection) was removed as dead.

**Connection graph mechanic**
Each `Creature` has a `Power` (Fibonacci values 2/3/5/8; distribution 10/7/4/2 creatures per tier) and a `Connections` list of length equal to its Power. When a clicked creature dies, its victims are:
- alive adjacent neighbours whose creature type appears in the clicked creature's `Connections` list, plus
- any adjacent neighbour of the same type (always, regardless of `Connections`).

Connections are directed and hardcoded (explicit lists per creature), replacing an earlier fixed-seed RNG approach. Hardcoding keeps the graph stable so players can learn it. Board seeding weights spawn probability by `1/Power`, making high-power connector creatures rare.

**Scoring follow-up deferred**
`scoreForKill`'s colour-matching bonuses can no longer fire (all 23 creatures have distinct colours). The bonuses remain in place; a scoring redesign around the connection/power mechanic is noted as a follow-up task.

---

## Connection themes

**Per-level theme interface**
Connections and Power are no longer fixed on the creature type. A `connectionTheme` interface wraps the two concerns that vary per level: which creatures each creature connects to, and each creature's Power value. `setActiveTheme` recomputes every creature's Power and Connections and rebuilds the `1/Power`-weighted spawn CDF, so power distribution can differ between themes without changing creature definitions.

**Two themes implemented**
- *Colour theme* (default, level 1): each creature connects to its Power nearest neighbours by OKLab perceptual distance. The intent is a learnable entry point — a player can read approximate connections from colour proximity on-screen. This is the active theme.
- *Random theme*: the former hardcoded per-creature connection lists, preserved as a named theme. Provided consistent behaviour while the theme architecture was being introduced.

Same-type adjacency always triggers a kill regardless of which theme is active; this is enforced outside the theme.

**Level progression between themes deferred**
The mechanism for advancing a player from one theme to the next (trigger condition, transition animation, difficulty curve) is an open design question and is not yet implemented.

---

## Always-landscape rendering

**Rotation instead of overlay**
Rather than blocking portrait use with a "rotate your device" overlay, the game renders its scene into a fixed-size landscape frame and presents that frame to the real screen rotated 90° when the device is in portrait orientation. The user sees the game sideways and simply turns the phone; no interaction is blocked.

**Two-seam design**
The rotation is implemented at two narrow seams only:
1. The `Draw` present transform rotates and translates the offscreen frame onto the actual screen.
2. Input coordinates are unmapped through the inverse of that transform before reaching any game logic (`screenToFrame` / `screenToBoard`).

No drawing, animation, shader, or HUD code was changed. `LayoutF` returns the real device-pixel screen dimensions; a fixed design frame holds the board. `screenToFrame` and `screenToBoard` are pure functions covered by unit tests.

---

## HUD

**Left strip layout**
A fixed-width vertical strip to the left of the board displays: an embedded title image, the current Level, theme instructions, Score, and the hovered piece as "Name – Power". The board origin is shifted right by an explicit pixel offset, and that offset is folded into the same `screenToFrame`/`screenToBoard` coordinate mapping that handles the landscape rotation, so the HUD imposes no additional coordinate logic elsewhere.

**Cheap bitmap text**
Text is rendered with `text/v2` and `basicfont` at approximately 3× scale on a `#9999AA` background strip. No shadow layer. Black text on the mid-grey strip provides sufficient contrast without a separate outline or drop-shadow pass.

---

## Mobile web shell

**PWA manifest**
A `manifest.webmanifest` declares `display: standalone` and `orientation: landscape`, enabling an installable app without browser chrome. Icons are referenced from the manifest. The rotate-device and tap-to-start overlays were removed; the in-game rotation (see Always-landscape rendering above) is the sole orientation mechanism.

**Scroll and rubber-band suppression**
`overscroll-behavior: none`, `touch-action: none`, and `touchmove` with `preventDefault` collectively kill pull-to-refresh and rubber-band scroll on both Android and iOS.

**Fullscreen and orientation lock**
On the first user gesture, the shell requests fullscreen and calls `screen.orientation.lock('landscape')` best-effort. On Android this locks to a single landscape orientation and hides the browser bars. On iOS both calls are ignored; the in-game rotation serves as the iOS fallback.

**Lesson: orientation APIs are platform-fragmented**
Locking to one orientation via `screen.orientation.lock` is the most reliable cross-platform escape from handling all four device orientations. Full four-orientation correctness (including the two 180°-wrong orientations on Android before the lock) would require reading `screen.orientation.angle` on every resize and applying a conditional transform — this is deferred. iOS has no orientation-lock API at all; the in-game rotation is permanent there.

---

## Performance and rendering (2026-11-17)

**Frame sized to presented resolution**
The frame and canvas were being allocated at `designSize × DeviceScaleFactor` but presented at a materially smaller size, so the compositor was processing roughly 3× more pixels than the screen ever used. The fix is to size the frame to the actual presented resolution from the start, so every pixel allocated is a pixel displayed. The alternative — keeping the over-resolved frame and downscaling — was rejected because the visual quality difference is imperceptible while the per-frame cost is not.

**Loading overlay held until landscape is confirmed**
Fullscreen and orientation-lock are async browser operations: the board was briefly visible in portrait before the lock resolved, producing a sideways flash. The decision was to keep the loading overlay visible until the browser confirms landscape orientation, rather than unblocking immediately on the user's tap. A timeout fallback covers browsers (iOS) where the lock never fires. The goal was to hide the transition entirely; showing a briefly sideways board and then snapping was judged worse than a slightly longer load.

**Wall-clock animation timing**
The root cause of reported animation jank was that animation progress was computed from the logic tick counter, which advances at a fixed 60 TPS. On high-refresh displays (100–120 fps), `Draw` is called more often than `Update`, so multiple consecutive frames produced identical interpolated positions followed by a visible jump. The fix is to drive animation progress from wall-clock time, making every rendered frame show the geometrically correct position regardless of display refresh rate. This keeps animation durations as natural time constants (milliseconds) and decouples rendering smoothness from the logic rate. This was the primary fix for the jank reports.

**Forced GC at settle**
Measured frame spikes of ~40ms appeared intermittently during play while fps was otherwise 90–100 and sprite bake counts were constant — a profile consistent with Go-WASM garbage collection pauses on the animation path. The chosen approach is to call `runtime.GC()` once at each settle point (when the board is static between moves), front-loading collection work onto a frame where nothing is animating. The alternative — tuning `GOGC` — was considered and explicitly rejected because it would alter heap growth behaviour and potentially change game feel. That option is deferred pending `runtime.NumGC`-based measurement to establish whether it would help.

**Instrument before optimising**
When the jank was first reported, the cause was not obvious: candidates included draw-call volume, sprite rebaking, or render/logic rate mismatch. Rather than optimise on a guess, a `const`-gated on-screen readout was added to measure FPS, TPS, sprite count, cumulative bake count, and worst-frame time in a live 2-second window. The measurement ruled out being draw-bound (fps held high) and ruled out a rebake storm (bake count was constant). The actual causes — cadence judder from fps exceeding tps, and GC spikes — were only identifiable from measurement. The readout remains in the codebase behind `showPerfDebug = false`; it costs nothing disabled.

---

## Creature palette rework (2026-11-17)

**Colour encodes power tier, not individual identity**
The previous palette (12 hues × 2 lightness bands) was visually dull and the hues too similar to tell apart at a glance. The new palette is tier-keyed: one near-black for the single power-8 creature, three vivid unique hues for the three power-5 creatures, three vivid hues each shared by a power-3 pair, and six very-light near-white pastels distributed across the thirteen power-2 creatures. Within a shared-colour tier the emoji glyph is now the sole differentiator between creatures. Alternative rejected: keeping a unique colour per creature — this would require more hues than can be reliably distinguished and obscures tier information.

**Luminance-adaptive glyph tint (OKLab L threshold)**
The previous approach tinted the glyph a fixed darker shade of the base colour. On the near-black power-8 tier a darker shade is effectively invisible; on light pastel cells it is faint. The new approach (`glyphTint`) uses an OKLab lightness threshold: near-white on dark cells, near-black on light cells. This is essential because the glyph is now the sole within-tier differentiator.

**Chroma tuned to post-shader appearance**
The gel/bevel shader visibly desaturates cells. Saturated-hue chroma is set higher than looks correct in raw RGBA, and the pastel tier is kept at high L and low chroma, so that the on-screen result after the shader passes looks right. The source values are intentionally not "balanced" in isolation; the visual check on the running game is the reference.

---

## Tooling (2026-11-22)

**`make analyse` is on-demand, not commit-gated**
A `make analyse` target runs gocyclo, gocognit, and maintidx for occasional complexity review. It is deliberately not wired into the pre-commit hook or `make lint`: the user does not want every commit blocked on complexity thresholds, only the ability to inspect them when it seems useful.

**Tools run via `go run <tool>@latest`, not installed binaries**
All three tools are invoked through `go run` rather than PATH-installed binaries or `go.mod` tool directives. This keeps `go.mod` clean and makes the target self-contained on any host with Go and network access. A PATH-based invocation of golangci-lint was tried first; it broke on the Mac host where golangci-lint is not installed.

**maintidx runs through golangci-lint, not its standalone cmd**
The standalone `maintidx` binary errors on this module with "package 'bytes' without types" because its loader lacks full type information. golangci-lint loads types correctly and runs maintidx cleanly, so golangci-lint is the execution vehicle for that metric.

---

## Process (2026-11-17)

**ROADMAP for parked ideas**
Ideas that are plausible improvements but lack the measurement to justify acting on them now are recorded in ROADMAP.md rather than acted on speculatively or discarded. The current parked items (GOGC tuning, draw-call reduction, deferring sprite-cache clears to settle) each carry an explicit evidence gate — the measurement that would need to show a problem before the idea is worth implementing. This keeps the codebase stable while preserving the rationale for revisiting.

---

## Animal/food matching re-theme (2026-11-24)

**Domain recentred on Australian animals and their food**
The game's subject matter was changed from an abstract creature/power graph to Australian animals matched with the food they eat. The type rename `Creature` → `Organism` reflects the broadened domain (animals and food items are both organisms). No mechanical meaning attaches to the rename; it is a naming-correctness fix.

**Core mechanic: flood-clear a cluster only when a paired food is present**
Clicking an animal flood-clears its 8-way-connected same-animal cluster plus any food tiles adjacent to that cluster, but only if the cluster contains at least one food tile paired to that animal. A lone animal cluster with no adjacent food cannot be cleared.

The food-required gate was chosen to make play about deliberately building animal+food clusters rather than freely clearing any group. Without the gate, a player can clear any lone animal for free — the food requirement adds the combinatorial constraint that gives the matching its strategic weight.

Scoring is super-linear: `clusterSize²`. This rewards patience and cluster-building over rapid small clears.

**Power/health/shuffle system and connection-graph themes dropped**
The Power tier, health, shuffle-on-click, and connection-graph mechanic were all removed. The matching model supersedes them structurally: the cluster+food rule is the new decision space. The removal is a large simplification — roughly the last several months of connection-graph work was reverted or deleted.

Alternative considered: keep Power as a difficulty multiplier (larger-Power animals require a bigger paired cluster to clear). Rejected — it re-introduces a hidden variable and pulls player attention away from the visible animal/food matching, which is the game's identity.

**Colour as a hue link between animal and food**
Each animal/food pair shares one hue. The visual effect is that scanning the board for a matching cluster is a colour-grouping task as well as a spatial one. Glyphs are placeholder emoji pending an SVG sprite overhaul; several are inaccurate stand-ins for animals that lack a standard Unicode glyph.

---

## Architecture

## Grid encapsulation (2026-11-25)

**Grid as single owner of cell population**
`board.Grid` is now the fully-encapsulated owner of the cell slice: the backing slice is unexported, all mutations go through Grid methods, and Grid maintains an exact live-cell count exposed via `Live()`.

The maintained count is only drift-proof if no code outside Grid can mutate cells — an exported slice makes single-writer a convention, not a constraint. Unexporting the slice makes it a compiler error. The same change removed the main package's direct coupling to the grid's internal representation, which was the proximate source of the count drifting: main was adjusting the count ad-hoc rather than letting Grid track it.

Two alternatives were rejected:

- **Recompute-on-read (`CountLiveCells` scan):** drift-proof because it derives the count from truth each time, but it leaves the slice exposed so encapsulation is not improved. Cells can still be mutated behind Grid's back; the count just no longer relies on those mutations being announced.
- **Route writes through Grid but keep the slice exported:** single-writer by convention only. A future stray write bypasses the convention without any compiler feedback. Because the slice is still exported, this is strictly weaker than the scan approach if the convention erodes — the worst of both worlds.

The decision was driven by a user preference that all population changes be channelled through Grid so nothing mutates cell state without Grid being notified. The unexported slice is the mechanical guarantee that this preference holds permanently, not just until the next casual write.

---

## Encapsulation pass: internal/board and internal/anim (2026-11-25)

**Aggressive field-level encapsulation across board and anim packages**
All fields on `board.Cell`, `board.Organism`, and `board.MoveAnim` were unexported; constructors and zero-alloc accessor methods are the only public interface. The `OrganismPalette` var was replaced by `PaletteOrganismAt`/`PaletteSize` read-only functions. The exported free functions `EightNeighbors`, `FloodMatchCluster`, and `ClusterHasFood` were removed; `Grid` methods are now the sole cluster API. In `internal/anim`, `Lerp2`, `QuadBezier`, and `QuadBezier2` were unexported (no callers outside the package).

The goal was to shrink the mutable public surface of these packages: an exported struct field is an implicit contract that any caller can read or write it at any time. Unexported fields turn unintended mutation from a convention violation into a compile error, eliminating a category of drift.

**Pointer-identity cache invariant drove the Organism approach**
`main` uses `*board.Organism` pointer identity as the sprite-cache key (a `map[*board.Organism]...`) and also takes palette entry addresses for board seeding (`&palette[i]`). These two uses require that the slice of `Organism` values not be replaced or moved. Exporting a mutable slice (`var OrganismPalette []Organism`) makes copying or reassigning it easy to do accidentally. The fix was to unexport the slice and expose only index-based accessors, preserving the pointer-identity guarantee at the API level while removing the footgun.

**Stop boundary**
Encapsulation stopped where the remaining exported `board` and `anim` symbols (`GravityMoves`, `HorizontalCollapseMoves`, `ApplyColumnCollapse`, `ScoreForCluster`, `ElapsedFraction`, `Grid.Cols`/`Rows`) each have a legitimate cross-package caller in `main`. Unexporting them would require moving render or state-machine logic into `board`, which is code relocation, not encapsulation. All added accessors are inlinable field returns, so no measurable per-frame cost was incurred; the stop was the API boundary, not a performance constraint. `DeviceCellCenter` and `GridFromCells` appeared in this list at the time but were subsequently removed in a follow-up pass (see below).

---

## Concern-separation pass (2026-11-25)

**Encapsulation difficulty as a signal of misplaced logic**
The encapsulation pass identified two `internal/board` exports that could not be cleanly justified as model concerns: `DeviceCellCenter` (converts a grid coordinate to a device-pixel centre) and `GridFromCells` (constructs a `Grid` from a raw cell slice, used only in tests). The difficulty of keeping these in the board package — `DeviceCellCenter` requires device-pixel knowledge that board should never hold; `GridFromCells` exposed a test-construction seam on the production surface — was treated as a signal that the layering was wrong, not that encapsulation was too expensive.

`DeviceCellCenter` was removed from `internal/board` and reimplemented as unexported `deviceCellCenter` in `main/device.go`. This keeps the board package entirely free of device or render concepts; the function's natural home is beside the other device-geometry helpers. `GridFromCells` was deleted from `internal/board`'s public API; the two `main` test call sites were updated to construct grids via `NewGrid` + `Place`/`NewCell`, which are the same constructors production code uses.

The improvement on both moves is the same: the board package no longer needs to know what it should not know, and the reduction in exported surface is a by-product, not the goal.

**ElapsedFraction left in internal/board**
`ElapsedFraction` (wall-clock animation progress) was identified as a borderline case: its natural home is arguably `internal/anim`, but it has no cross-package caller in `anim` and moving it would be code relocation without an encapsulation gain. The move was judged a cohesion nit and deferred; it is not blocking further layering work.

---

**Game struct separation of concerns (Plan A: sub-structs)**
The `Game` struct had grown to ~60 flat fields in a 1485-line `main.go`, mixing board/model state, animation/state-machine state, device/presentation state, input state, and perf-debug counters. Grouped the fields into named sub-structs on `Game` — `boardState` (grid, cellCount, score), `animState` (phase, timing, action cell, move/victim/target anims), `inputState` (hover + pointer/touch), `viewState` (canvas/frame, scales, screen dims, portrait), and `perfCounters` — plus a distinct typed `gameState` enum for the dispatch switch. Render resources (shaders, glyph font, hud cache) stay on `Game` as they are not state.

Chose Plan A (data regrouping, package `main` unchanged) over two alternatives: Plan B (extract `internal/board` + `internal/anim` packages for compiler-enforced boundaries) and Plan C (replace the int-switch state machine with per-phase state objects + World/Animator/Presenter ownership). A and B/C are not mutually exclusive — A is a prerequisite subset. B and C were deferred: they carry much higher churn and B edges against the project's flat-layout convention; the intent is to reassess them once the seams Plan A exposes prove whether enforced boundaries or a behavioural restructure are worth it. Methods were left on `*Game` and `main.go` was not split into per-concern files (deferred as cosmetic).

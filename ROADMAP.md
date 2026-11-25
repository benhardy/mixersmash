# Roadmap / Parked Ideas

Low-priority ideas deferred from active work. Each notes the trigger/evidence that would justify picking it up.

## Performance

### Reduce animation-time GC spikes (GOGC / draw-call reduction)
Under heavy play, occasional ~33ms frame spikes remain during fast vertical drops (fps otherwise 90-100; `bk` constant, so not sprite baking). `enterWaiting()` already forces `runtime.GC()` at each settle, which cut spikes from ~40ms to ~33ms.

Before spending effort here, CONFIRM the spikes are actually GC: re-enable the on-screen readout (`const showPerfDebug = true` in main.go) and add `runtime.MemStats` `NumGC`/`HeapAlloc` to it. If `NumGC` ticks up exactly when `max` spikes, it is GC and these levers help:
- `debug.SetGCPercent(400)` (or similar) so the automatic collector rarely fires between the settle-GCs — safe because the settle-GC keeps the heap small; costs a modest memory bump.
- Reduce per-frame draw-call count (fewer `DrawImage` calls per frame). In the browser, Ebitengine's per-draw `syscall/js` marshaling is a likely dominant garbage source that scales with draw-call count — so cutting draw calls (e.g. blitting shifted canvas chunks for mass linear moves instead of one sprite blit per cell) could reduce garbage. This is the "chunk-blit" idea; only worth it if the readout shows it is draw-call-driven GC, not if `max` spikes with `NumGC` flat (then it is the per-drop build cost or a browser/vsync hiccup, different fix).

### SVG sprite overhaul — replace generated/emoji organism sprites with SVG artwork
Current organism glyphs are placeholder emoji. Several are inaccurate stand-ins for animals that lack a standard Unicode glyph (echidna rendered as hedgehog, magpie as duck, kookaburra as eagle, blue-tongued lizard as frog). Worm and beetle use Unicode 13 emoji that may not render in all fonts or on all platforms.

Trigger: user plans this next. Pick up when SVG source art is available. Scope is the sprite-baking pipeline: replace the font-based glyph draw step with an SVG rasterisation step, keeping the gel-shader pass and cache-per-organism structure unchanged.

### Defer the sprite-cache clear to the next settle
`LayoutF`'s `needRealloc` branch (device.go ~115-134) clears every `Creature.Sprite` (nil) IMMEDIATELY on a scale change. This contradicts the codified lesson "do not invalidate the per-creature sprite cache mid-animation -> rebake storm." It is latent in fullscreen (dims are stable, so it does not fire during steady play), but a viewport change landing mid-animation would rebake all sprites in-flight and spike. Harden by deferring the clear to the next settle (`stateWaiting`/`enterWaiting`) instead of doing it in `LayoutF`. Low-risk; do it next time this area is touched.

## Architecture

### Game struct refactor — Plan B / Plan C (deferred from Plan A)
Plan A grouped `Game`'s fields into `boardState`/`animState`/`inputState`/`viewState`/`perfCounters` sub-structs (see DECISIONS.md → Architecture). Two further steps were considered:
- **Plan B — enforced boundaries**: extract the model (`grid.go`) to `internal/board` and the animation core (`anim.go`, `lerp.go`, the `gameState` enum) to `internal/anim`, so the compiler forbids them importing ebiten or reaching into `Game`. Payoff: real isolation + unit-testability without a graphics context.
- **Plan C — behavioural restructure**: replace the `int`-switch state machine in `Update` with a `gameState`-interface per phase (waiting/shuffling/dropping/victimMoving/columnCollapsing) and split ownership into World/Animator/Presenter. Payoff: dissolves the `update*` methods that interleave input/model/anim.

Also deferred as cosmetic: splitting `main.go` (still ~1485 lines) into per-concern files (render.go/statemachine.go/input.go), and moving methods off `*Game` onto the sub-structs.

Trigger: pick up B if the model/anim layers need testing in isolation or a second consumer appears; pick up C if a new state-machine phase must be added or the `update*` interleaving causes a bug. Both are higher-churn than A; B edges against the flat-layout convention in CLAUDE.md.

#### Plan B — status and staged execution plan
Prerequisites DONE (commits `105ce26`, `ed1863b`):
- **B1** — removed `Organism.Sprite *ebiten.Image`; composites now live in a render-side `Game.spriteCache` map keyed by organism pointer, cleared on scale change in `LayoutF`. This was NOT free as the old ROADMAP text assumed — `Cell.Organism → Organism.Sprite → ebiten` was a transitive model→render coupling that had to be broken first.
- **B2** — `floodMatchCluster` now sizes its `visited` slice from its `cols,rows` args instead of the `[gridCols][gridRows]` constants, so `grid.go` no longer depends on package-level grid-dimension constants.

Completed:
- **B3 — create `internal/board`** — DONE (commits `56ffb71`, `a4ce4b4`). `internal/board` now holds the pure, ebiten-free model layer: `Cell` (+`Die`/`BumpHealth`/`IsDead`/`Color`), `Grid`, `NewGrid`, `MoveAnim`, `eightNeighbors`, `GravityMoves`, `HorizontalCollapseMoves`, `ApplyColumnCollapse`, `FloodMatchCluster`, `ClusterHasFood`, `ScoreForCluster`, `ElapsedFraction`, `DeviceCellCenter`, `Organism`, `OrganismPalette`, `PaletteOrganism`, `mixerBias`, `SRGBToOKLab`, `GlyphTint`. 139 subtests pass across `main` and `internal/board`. The asset embeds, `titleImage`, `init()`, `loadGlyphFont`, and `Game.spriteCache` remain in `main`.

Remaining, when picked up:
- **B4 — create `internal/anim`.** Move `anim.go` + `lerp.go` and the `gameState` enum + state constants. `anim` depends on `board.MoveAnim` and `board.ElapsedFraction`; decide whether `gameState` lives in `anim` or `board` (it is consumed by the state machine in `main` and by `anim`'s `moveAnimDuration`/`moveAnimPos`). Export `MoveAnimDuration`, `MoveAnimPos`, `VictimPathParams`, `VictimAnimPos`, and the lerp/easing helpers `main` still calls.

Reference-count hotspots to expect (all `.go`): `Organism` 72, `Cell` 69, `MoveAnim` 33, `floodMatchCluster` 26, `gravityMoves` 24, `organismPalette` 21, `Grid` 18, `elapsedFraction` 16, `eightNeighbors` 15, `deviceCellCenter` 14.

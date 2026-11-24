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
Plan A grouped `Game`'s fields into `boardState`/`animState`/`inputState`/`viewState`/`perfCounters` sub-structs (see DECISIONS.md → Architecture). Two further steps were considered and deferred:
- **Plan B — enforced boundaries**: extract the already-ebiten-free model (`grid.go`) to `internal/board` and the animation core (`anim.go`, path math, the `gameState` enum) to `internal/anim`, so the compiler forbids them importing ebiten or reaching into `Game`. Payoff: real isolation + unit-testability without a graphics context.
- **Plan C — behavioural restructure**: replace the `int`-switch state machine in `Update` with a `gameState`-interface per phase (waiting/shuffling/dropping/victimMoving/columnCollapsing) and split ownership into World/Animator/Presenter. Payoff: dissolves the `update*` methods that interleave input/model/anim.

Also deferred as cosmetic: splitting `main.go` (still ~1485 lines) into per-concern files (render.go/statemachine.go/input.go), and moving methods off `*Game` onto the sub-structs.

Trigger: pick up B if the model/anim layers need testing in isolation or a second consumer appears; pick up C if a new state-machine phase must be added or the `update*` interleaving causes a bug. Both are higher-churn than A; B edges against the flat-layout convention in CLAUDE.md.

# HANDOFF — MixerSmash (go2d)

Orientation document for an agent or contributor picking this up cold.

---

## What this is

MixerSmash is a 2D grid/match game in Go using [Ebitengine](https://ebitengine.org/). The Go module and all built binaries are named `go2d`; the display name shown to players is MixerSmash.

**Targets:** desktop (macOS arm64, Linux amd64) and WebAssembly (deployed via GitHub Pages; mobile browsers are the primary target).

**Mechanic:** a connection-graph match game. Clicking or tapping a creature removes it and all connected neighbours — same type, or types listed in its `Connections` field. Cells above drop and collapse to fill gaps. Score accrues per removal. Each level has a connection "theme": `Colour` (nearest-neighbour in OKLab colour space) or `Random` (hardcoded lists in `creatures.go`).

---

## Build / run / deploy

Authoritative targets are in `Makefile`. Summary:

- `make mac` — builds `mixersmash_darwin_arm64` (CGO off)
- `make linux` — builds `mixersmash_linux_amd64`
- `make wasm` — builds `web/mixersmash.wasm`, copies `wasm_exec.js` from GOROOT (Go 1.24+ uses `lib/wasm/`; older uses `misc/wasm/`), writes `web/wasm-meta.js` with the binary size for the progress bar
- `make serve` — runs `tools/web_server/main.go` as a local HTTP server for `web/` (required; browsers reject `.wasm` over `file://`)
- `make test` — `go test ./...`
- `make lint` — `golangci-lint run ./...` (binary at `/home/ubuntu/go/bin/golangci-lint`, v2)
- `make hooks` — sets `core.hooksPath .githooks`; the pre-commit hook runs gofmt, vet, build, test, and golangci-lint

**After every change:** `go build ./... && go test ./...` before declaring done.

**Commits** use `--no-gpg-sign` (signing key absent on this machine).

---

## Architecture (file map)

The repo root holds the flat `main` package, split across several files. `internal/board` holds the pure, ebiten-free model layer. `internal/anim` holds the animation core. Import graph: `main → internal/anim → internal/board`, with `main` also importing `internal/board` directly; no cycles.

**Main package files:**

- `main.go` — `Game` struct and its named sub-structs by concern (`board`, `anim`, `input`, `view`, `perf`), plus render resources (`shaders`, glyph font, `spriteCache`) and the `hud` cache; global consts; `newGameState`; `main()`; `gameState` enum and the `kindOf` helper that maps it to `internal/anim.StateKind`
- `render.go` — `Draw` path; HUD compositing; the `present` step that blits the offscreen frame via `presentGeoM`; baked-sprite cache; perf debug overlay
- `statemachine.go` — `Update`; all per-state handler functions; `repaintGrid` / `enterWaiting` settle logic
- `input.go` — touch / pointer / hover event handling
- `assets.go` — `//go:embed` declarations; glyph-font loader; decoded title image (GPU creation is deferred to the draw path, not `init()`); `init()` for embed setup
- `device.go` — `LayoutF`; `frameScaleFor`; `deviceCellSize` / `spriteRef` sizing; `deviceCellCenter` (pixel centre of a board cell in device space); sprite-cache clear deferred to settle when animating
- `mapping.go` — `screenToFrame` / `screenToBoard`; inverts `presentGeoM` for input hit-testing
- `hud.go` — HUD layout and rendering helpers
- `shaders.go` — Kage gel/glow shader source strings
- `grid.go` — retained in `main`; bridges call sites that qualify model types from `internal/board`
- `orientation_js.go` / `orientation_other.go` — build-tagged orientation lock and fullscreen (JS-only on WASM; no-ops elsewhere)

**Internal packages:**

- `internal/board/` — ebiten-free model layer (`package board`): `Cell`, `Grid`, `NewGrid`, `MoveAnim`; gravity/collapse logic; flood-match cluster + food gating; `ElapsedFraction` (wall-clock animation progress); `Organism`, `GlyphTint`, OKLab colour math. Nothing here imports ebiten. All fields on `Cell`, `Organism`, and `MoveAnim` are unexported; callers use constructors (`NewCell`, `NewOrganism`, `NewMoveAnim`/`NewShuffleAnim`/`NewMixerAnim`/`NewVictimSeedAnim`/`NewVictimAnim`) and accessor methods. The organism palette is unexported (`organismPalette`); read-only access is via `PaletteOrganismAt(idx)` and `PaletteSize()`. `Grid` fully encapsulates its cell storage (unexported `cells` slice); callers never index cells directly — all reads go through `At`/`Column`/`Snapshot`/`Live` and all mutations through `Place`/`BumpHealthAt`/`SetHealthZero`/`ReplaceColumn`, which maintain an exact live-cell count internally. Cross-package helpers `EightNeighbors`, `FloodMatchCluster`, and `ClusterHasFood` have been removed; `Grid` methods are the sole public API for cluster operations. Device-pixel geometry (`deviceCellCenter`) is NOT part of the board API; it lives in `main/device.go` as an unexported function.
- `internal/anim/` — ebiten-free animation core (`package anim`): `anim.go` + `lerp.go`; owns the `StateKind` enum; exports `MoveAnimDuration`, `MoveAnimPos`, `VictimPathParams`, `VictimAnimPos`, and the easing helpers. `Lerp2`, `QuadBezier`, and `QuadBezier2` are unexported (no cross-package callers). Depends on `internal/board`; nothing here imports ebiten.

**Other:**

- `tools/web_server/main.go` — minimal static file server for `make serve`
- `web/` — `index.html` (loading screen, progress bar, Start overlay), `manifest.webmanifest` (PWA), `wasm_exec.js`, `mixersmash.wasm`, `wasm-meta.js`, icons, `title.png`

**Rendering model.** Every frame is drawn into an offscreen `*ebiten.Image` (the "frame"), then presented to the real screen via `presentGeoM`: identity in landscape, 90-degree rotation in portrait. Input coordinates are mapped back through the inverse of that same `GeoM` (`mapping.go`). The board canvas and HUD are model-derived caches; they are re-synced from the model on every settle to `stateWaiting` via `repaintGrid` / `enterWaiting`.

**Animation timing** is wall-clock-based (`time.Time` / `time.Duration`, not ticks) so it stays smooth above 60 fps on high-refresh displays (`elapsedFraction` in `grid.go`).

**`Game` struct layout.** `Game` (in `main.go`) is grouped into named sub-structs by concern — `board` (grid/score/cellCount), `anim` (state machine + animations), `input` (hover/touch), `view` (device geometry + offscreen targets), `perf` (debug counters) — plus render resources (shaders, glyph font, `spriteCache`) and the `hud` cache. The dispatch phase is a typed `gameState`. See DECISIONS.md → Architecture.

---

## Conventions / workflow

See `CLAUDE.md` (checked in at repo root) for:

- Communication style
- Go conventions (code style, layout, error handling, testing patterns)
- Shell hygiene
- Ebitengine rendering notes
- Debugging workflow
- WASM/mobile performance notes

**Agent pipeline.** This project is maintained via an orchestrator + architect/engineer/builder/tester subagent pipeline (defined in the user's global `~/.claude/CLAUDE.md`). The engineer subagent does **not** git commit; the orchestrator commits directly. Always verify with builder (`go build ./...`) and tester (`go test ./...`) before committing.

---

## Known gotchas (engine)

Full details in `CLAUDE.md` under "Ebitengine rendering notes". One line each:

- **Premultiplied alpha:** translucent RGB values must be <= the alpha channel; never `{255,255,255,a}` for translucent white.
- **Kage atlas origin:** shaders sampling a bound source image must compute geometry in local coords (`srcPos - imageSrc0Origin()`), not raw `srcPos`.
- **Build cannot catch Kage failures:** `go build` / `go test` pass even when a shader is broken; rendering changes require a visual check.
- **Bake inside the game loop:** drawing or baking before `ebiten.RunGame()` has no graphics context and produces blank output.
- **Sprite cache:** cell sprites are baked once per `color.RGBA` and cached; the shader runs only at bake time — do not invalidate the cache mid-animation.
- **Sprite cache clear in `LayoutF`:** the clear is deferred to the next settle when an animation is in progress, avoiding a mid-animation re-bake storm. If this deferral logic breaks, expect visual glitches during scale changes.

---

## Current state

Working: desktop and mobile play; mobile touch (hold = hover, release = commit); orientation lock and fullscreen on Start; loading screen with accurate progress bar and Start button; PWA manifest; animal/food cluster-matching mechanic (clear a flood-connected cluster that includes its paired food) plus the Mixer; HiDPI two-seam rotated rendering; perf tuned (wall-clock timing, GC-at-settle; ~90-100 fps, occasional ~33 ms spike). Build, tests, and lint are all green; tree is clean on `main` as of `24dd49e`.

A temporary perf readout exists behind `const showPerfDebug` in `render.go` (currently `false`).

Architecture refactor: Plan A (`Game` sub-structs) complete. Plan B fully complete — `internal/board` extracted (B3), `internal/anim` extracted (B4), `main.go` split into `render.go` / `statemachine.go` / `input.go`, `creatures.go` renamed to `assets.go`, `Grid` population fully encapsulated, `titleImage` GPU creation deferred out of `init()`. `gameState` stays in `main`; `internal/anim` owns a `StateKind` enum bridged via `kindOf`. Encapsulation pass complete: all fields on `Cell`, `Organism`, and `MoveAnim` are unexported; palette access is via read-only functions; free cluster helpers removed in favour of `Grid` methods; `anim` package internals (`Lerp2`/`QuadBezier`/`QuadBezier2`) unexported. Concern-separation follow-up complete: `DeviceCellCenter` removed from `internal/board` and reimplemented as unexported `deviceCellCenter` in `main/device.go` (device-pixel geometry is a render concern, not a model concern); `GridFromCells` removed from `internal/board`'s public API (test-only construction seam eliminated from production surface).

---

## Where to look next

**Untriaged code-quality items** (catalogued, not yet fixed):

- `MoveAnim` in `internal/board/grid.go` is a union struct spanning three path types; typed constructors now exist per variant (`NewMoveAnim`/`NewShuffleAnim`/`NewMixerAnim`/`NewVictimSeedAnim`/`NewVictimAnim`). The `Rotations` field (signed spin count — full 360° rotations over the animation) is live: shuffle neighbours draw from a weighted distribution (−2:5% −1:20% 0:50% +1:20% +2:5%) via `shuffleRotation`; the render path applies `rotations×2π` eased with `EaseInOutQuad` around the sprite's centre (`spriteTurnGeoM`), positive = clockwise. Purely cosmetic — `Rotations()` is read only in the render path, no gameplay or model effect. If the variants diverge further, splitting into distinct types remains an option.
- `GravityMoves` in `internal/board/grid.go` allocates a `map[int]bool` per call on the drop path — could use a fixed-size scratch buffer.
- `containsTouchID` / `findReleasedID` in `input.go` are near-duplicate linear searches — small but worth collapsing.
- `updateColumnCollapsing` / `updateDropping` in `statemachine.go` truncate pixel→grid back-conversion — needs triage; likely safe-by-construction if coords are always cell centres, but not verified.
- Lazy shader init in `render.go` is inconsistent (some paths fatal, some nil) — audit for uniform error handling.
- `SetWindowTitle` is called every tick in `statemachine.go` — should be called only on change.
- `drawPerfDebug` in `render.go` computes a manual four-way max — minor cleanup.

**Parked on `ROADMAP.md`:** receiver migration of `*Game` methods onto sub-structs; SVG sprite overhaul (needs art assets); GC-spike diagnosis/tuning; Plan C state-machine-as-interfaces restructure.

**Decision on record:** variable board sizes are YAGNI — `gridCols`/`gridRows` fixed-array sizing stays; do not decouple it.

**Deferred product work:** scoring redesign (colour-bonus scoring is dead under the connection mechanic); nicer icon and HUD art; predator/prey connection theme; level progression.

- `ROADMAP.md` — full list of parked ideas with pick-up triggers
- `DECISIONS.md` — chronological log of design rationale

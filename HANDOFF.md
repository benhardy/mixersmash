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

The repo root holds the flat `main` package. `internal/board` holds the pure, ebiten-free model layer extracted under Plan B step B3.

- `main.go` — game loop entry point; Ebitengine `Update` state machine with per-state handlers; `Draw` + all rendering; input handling (mouse + touch); HUD; the `present` step that blits the offscreen frame to the real screen via `presentGeoM`
- `internal/board/` — ebiten-free model layer (`package board`): `Cell`, `Grid`, `NewGrid`, `MoveAnim`; gravity/collapse logic; flood-match cluster + food gating; `ElapsedFraction` (wall-clock animation progress); `Organism`, `OrganismPalette`, `GlyphTint`, OKLab colour math. `main` depends on this package; nothing here imports ebiten.
- `grid.go` — retained in `main` for call-site qualification bridge; pure model types now live in `internal/board`
- `creatures.go` — glyph-font loader and embedded title-image asset; baked sprites are NOT stored here — they live in `Game.spriteCache` in the render layer. Pure `Organism` data and palette helpers have moved to `internal/board`
- `device.go` — `LayoutF`; `frameScaleFor`; `deviceCellSize` / `spriteRef` sizing; sprite-cache clear on realloc (currently immediate — see gotchas)
- `mapping.go` — `screenToFrame` / `screenToBoard`; inverts `presentGeoM` for input hit-testing
- `lerp.go` — easing and bezier helpers
- `shaders.go` — Kage gel/glow shader source strings
- `orientation_js.go` / `orientation_other.go` — build-tagged orientation lock and fullscreen (JS-only on WASM; no-ops elsewhere)
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
- **Sprite cache clear is immediate in `LayoutF`:** a scale change during an animation triggers a re-bake storm; deferring this clear to the next settle is a known latent hazard (tracked in `ROADMAP.md`).

---

## Current state

Working: desktop and mobile play; mobile touch (hold = hover, release = commit); orientation lock and fullscreen on Start; loading screen with accurate progress bar and Start button; PWA manifest; animal/food cluster-matching mechanic (clear a flood-connected cluster that includes its paired food) plus the Mixer; HiDPI two-seam rotated rendering; perf tuned (wall-clock timing, GC-at-settle; ~90-100 fps, occasional ~33 ms spike).

A temporary perf readout exists behind `const showPerfDebug` in `main.go` (currently `false`).

Architecture refactor in progress: Plan A (grouping `Game`'s fields into sub-structs) is complete. Plan B step B3 is complete — `internal/board` now holds the ebiten-free model layer and `main` depends on it (commits `56ffb71`, `a4ce4b4`; 139 subtests pass). B4 (extract `internal/anim`) is next; its `gameState`-placement open question is documented in `ROADMAP.md` → Architecture.

---

## Where to look next

- `ROADMAP.md` — parked perf ideas and deferred technical work
- `DECISIONS.md` — chronological log of design rationale

Deferred product work noted by the user: scoring redesign (the old colour-bonus scoring is dead under the connection mechanic); nicer icon and HUD art; a predator/prey connection theme; level progression.

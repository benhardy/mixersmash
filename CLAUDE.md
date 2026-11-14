# Communication style
- Open with the answer, result or decision. No preamble, no restating the question, no closing recap of what you did.
- Separate observed from inferred. Observed = command output, file contents, test results, a doc URL. Inferred = everything else. If nothing was observed, write "unverified".
- Base decisions on observable evidence and name it. If the evidence is missing, run the check that produces it or state what is missing. Don't decide on plausibility.
- No filler or framing phrases ("the honest answer", "the real fix", "let me just", "to be clear"). Delete them instead of substituting synonyms.
- Report failures and unresolved items as plainly as successes.
- No emojis. Prose by default; lists only for parallel items.

# Agent Behaviour

## Step budget (orchestrator and subagents)
- Count tool calls per task. At 10 without the success condition met, stop.
- When stopping, report: calls used, the goal of the current attempt, the result of each of the last attempts, and what is blocking. Do not start a new approach.
- Same tool, same target, varied arguments counts as one failed approach. After 2, stop and report.
- The orchestrator passes these reports to me unedited and asks before re-spawning on the same task.

## Source handling

Read with Read. Modify with Edit/Write. Never use sed -i, tee, redirects, heredocs or scripts on source files. If Edit fails, re-Read and widen old_string; do not switch to the shell.

## Dependency and API lookup
- Order: (1) existing usage of the symbol in this repo, (2) `go doc` or `javap` on the resolved jar, (3) the sources jar or vendored source at a known path, (4) Maven Central, or (5) ask me.
- Never glob or recursively grep the module cache, ~/.m2, ~/.gradle or GOPATH.
- Maximum 3 lookup calls per symbol. Then report what is still unknown.

# Go project conventions

## Commands
- Build: `go build ./...`
- Test: `go test ./...` (single package: `go test ./pkg/foo -run TestName -v`)
- Vet/format: `go vet ./...` and `gofmt -l .` (or `goimports`). Run both before declaring work done.
- Race check for anything concurrent: `go test -race ./...`

## Workflow
- Run `go build ./... && go test ./...` after every change. Don't report done until both pass.
- Write the failing test first, then the minimal code to pass it.
- Keep diffs small. Don't refactor unrelated code or rename things unasked.

## Dependencies and API lookup
- Inspect dependency APIs with `go doc <pkg> [Symbol]`. Do not grep the module cache.
- Don't add dependencies without asking. Stdlib first.
- Don't hand-edit go.mod/go.sum. Use `go get` and `go mod tidy`.

## Shell hygiene (avoids permission prompts)
- No `$(...)` or backticks in commands. Use plain paths or separate commands.
- Search code with the Grep tool, not bash grep/rg.
- One command per call. Avoid long `&&`/pipe chains.

## Code style
- Idiomatic Go: small functions, early returns, errors returned and wrapped with `fmt.Errorf("...: %w", err)`. No panics outside main/init.
- Accept interfaces, return structs. Define interfaces where they're used, only when needed.
- Table-driven tests with `t.Run`. Use `t.Helper()` in helpers.
- Pass `context.Context` as first arg where I/O or cancellation applies.
- No global mutable state. No `init()` side effects.
- Exported identifiers get doc comments starting with the name.

## Layout
- Small project: flat `main` package, or `cmd/<name>/` plus `internal/`. Don't invent `pkg/`, `utils/` or `common/` packages.
- Tests live next to the code (`foo_test.go`).

# Ebitengine rendering notes
- Colours are premultiplied alpha everywhere (vector fills, DrawImage tints, Kage shader output). Translucent RGB must be <= alpha; for white at opacity `a` use `{a,a,a,a}`, not `{255,255,255,a}`. Overshoot renders opaque/harsh.
- Kage shaders sampling a bound source image: `srcPos` is in atlas coordinates (offset varies per allocation). Sample with `imageSrc0At(srcPos)`, but compute all geometry (center, radius, gradients, alpha) in local space via `local := srcPos - imageSrc0Origin()`. Mismatch bakes blank/offset sprites for some atlas placements. Alternative: allocate the source image Unmanaged so its origin is 0.
- `go build` / `go test` do not catch Kage/rendering failures (Kage compiles at runtime; failures are pixels). A green build is not confidence for a rendering change — it needs a visual check.
- Do all drawing/baking inside the game loop, never before `ebiten.RunGame()` (no graphics context yet; deferred/mis-ordered GPU ops bake blank).
- Cell sprites are baked once per colour and cached (map keyed by `color.RGBA`), then blitted; the shader runs only at bake time. The offscreen canvas is a model-derived cache, re-synced from the model on every settle to `stateWaiting` (`repaintGrid`/`enterWaiting`).
- Inspect shader/dependency APIs with `go doc`; never glob or grep the module cache. Some Kage builtins may not appear in `go doc` — ask rather than spelunk the cache.

# Debugging workflow
- Bisect before theorising. When a bug "started happening," git stash / check the last commit to confirm committed-vs-uncommitted, then narrow to the introducing change. Hunt the mechanism only after.
- Commit each change after its own verification (a visual check for rendering changes). Keep the uncommitted suspect set to one thing; do not stack several unverified changes.
- For visual/GPU bugs, instrument (dump a baked sprite to PNG, log atlas origin/bounds) instead of reasoning from code — the running game is the only source of truth for pixels.
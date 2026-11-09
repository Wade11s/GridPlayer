# Repository Guidelines

GridPlayer is the Go + MyGo rewrite of vzhd1701/gridplayer (`gridplayer-go`, Go 1.27.3). macOS/arm64 only. The license stays GPL-3.0-or-later.

## Project Structure & Module Organization

- `cmd/gridplayer` is the app. `cmd/poc-hw` and `cmd/poc-cb` are isolated probes; leave them out of the app.
- Domain code is under `internal/`: `engine`, `grid`, `vlcmac`, `playlist`, `settings`, `keymap`, `snapshots`, `bookmarks`, `medialist`, `resolve`, `nativehost`, and `dummy`.
- Tests sit beside source as `*_test.go`. Fixtures are in `testdata/`, design docs in `docs/`, scripts in `scripts/`, and assets in `resources/`.
- `bin/`, `dist/`, `*.app`, fixture mp4s, and VLC logs are gitignored.

## Build, Test, and Development Commands

Needs VLC 3.0.24 at `/Applications/VLC.app`, or `VLC_APP` for another bundle.

- `./scripts/build.sh gridplayer` compiles the CLI to `./bin/gridplayer`.
- `./bin/gridplayer --out testdata/fixtures` runs it on generated fixtures.
- `python3 scripts/make_fixtures.py` regenerates those fixtures.
- `./scripts/build_app.sh` builds the ad-hoc signed `dist/darwin-arm64/GridPlayer.app`.
- `./scripts/smoke_app.sh` is the local Finder-equivalent launch smoke.
- `go test ./... -race` is the full local test run.
- `./scripts/ci.sh` is hosted CI: `go vet`, `go test ./...`, and `go build ./...` on Ubuntu with `libvlc-dev`. It does not exercise the macOS GUI.

## Coding Style & Naming Conventions

Format with `gofmt` (tabs) and run `go vet ./...` before pushing. Packages are lowercase (`grid`, `vlcmac`). Exported names are `PascalCase`; unexported names are `camelCase`. UI files use an `_ui.go` suffix. Keep MyGo at the version pinned in `go.mod`.

## Testing Guidelines

Use the standard `testing` package and `TestXxx` names. Prefer fake backends. Darwin-only real libVLC tests stay beside the engine (`internal/engine/vlc_test.go`). GUI and `.app` smokes are local evidence; do not report them as hosted CI. `not run` is not a pass. There is no numeric coverage gate.

## Commit & Pull Request Guidelines

Commits use Conventional Commits: `feat(scope):`, `fix(scope):`, `test(scope):`, `docs:`, plus merge commits (`Merge pull request #38`). Target `main`. Follow `docs/MERGE_GATES.md`: name the issue, base and head SHAs, tool versions, and exact command results. Add screenshots or decoded-mode logs for GUI changes. Review is bound to that head SHA. No force-push, history rewrite, public release, or Gatekeeper change.

## Working Boundaries

Mutate UI only on the main thread (`mygo.RunOnMain` or `postMain`). Tear down a player as stop, unbind, detach, then release. Use an isolated data directory and generated fixtures, never private media or credentials. Read `docs/ARCHITECTURE.md` before changing engine, surface, or playback behavior. If `README.md` status disagrees with the tree, the code and `docs/BEHAVIOR_MATRIX.md` win.

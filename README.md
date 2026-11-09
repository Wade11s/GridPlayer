# GridPlayer (Go + MyGo)

Go + MyGo migration of [vzhd1701/gridplayer](https://github.com/vzhd1701/gridplayer)
(source `0.5.5`, frozen at `bf943684232d74ed9e271c3c9eeef6e7a7ed6e0f`).
macOS/arm64 is the first (and currently only) platform.

## Status

Working locally:

- 1→4+ native libVLC panes in one MyGo window, hardware decode via
  VideoToolbox, per-cell engine with generation fencing.
- Play/pause, seek, rate, volume, mute, loop, audio/subtitle track and
  chapter selection, per-cell overlay controls, active-cell border.
- Auto rows / auto columns / fixed grids, Fill (cover) and Fit (contain).
- File open/add (native dialog), playback/grid/view/tracks menus, keyboard
  shortcuts (Space, arrows, M, L, B/N, `[`/`]`).
- Local `.app` build with `mygo build` (ad-hoc signed).

Still open (tracked in Issues #2–#5): drag/reorder, free tiles, mosaic,
single mode, playlist/settings persistence, external A/V slaves, stream
resolver/cookies, full settings UI, bundled VLC, complete media matrix.

## Build and run

Building requires VLC 3.0.24 at `/Applications/VLC.app` (or `VLC_APP=...`).
The built `.app` bundles libVLC and its plugins, so running it does not need
VLC installed.

```sh
./scripts/build.sh gridplayer
./bin/gridplayer --out testdata/fixtures

./scripts/build_app.sh          # dist/darwin-arm64/GridPlayer.app
./scripts/smoke_app.sh          # Finder-equivalent launch + 4 HW panes
```

Fixtures are generated deterministically:

```sh
python3 scripts/make_fixtures.py
```

Tests: `go test ./... -race` (engine + grid + real libVLC dummy-output).

## Docs

- `docs/BEHAVIOR_MATRIX.md` — frozen source → behavior → test matrix.
- `docs/ARCHITECTURE.md` — engine/surface boundaries and design decisions.
- `docs/ACCEPTANCE.md` — frozen acceptance criteria and evidence.
- `docs/MERGE_GATES.md` — review/CI/local-GUI merge gates.
- `docs/APP_BUILD.md` — .app build and smoke instructions.

License: GPL-3.0-or-later (see `docs/BEHAVIOR_MATRIX.md`; the Go rewrite does
not change it).

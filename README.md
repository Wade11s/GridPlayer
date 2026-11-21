# GridPlayer (Go + MyGo)

Go + MyGo migration of [vzhd1701/gridplayer](https://github.com/vzhd1701/gridplayer)
(source `0.5.5`, frozen at `bf943684232d74ed9e271c3c9eeef6e7a7ed6e0f`).
macOS/arm64 is the first (and currently only) platform.

## Status

Working locally (macOS/arm64):

- 1–16 native libVLC panes in one MyGo window, hardware decode via
  VideoToolbox, one engine per cell with generation fencing.
- Play/pause, seek, rate, volume, mute, loop, per-cell end and list actions,
  audio/subtitle tracks and chapters, audio/subtitle delay, frame stepping,
  screenshots, bookmarks with a manager window.
- SponsorBlock: skip/show/off categories, seek-bar marks, cache/dedupe,
  per-video enable and category overrides.
- Grid modes: auto rows, auto columns, fixed (incl. six built-in templates),
  Fit Shapes (justified per-video shapes), Free Tiles (drag/resize, snapping,
  optional overlap), Mosaic (lock/arrange), Single Mode.
- Per-cell view state persists in `.gpls`: view mode, aspect, scale, crop,
  anchor, upstream crop margins, transform, audio/subtitle delay, loop points,
  end/list actions, SponsorBlock overrides.
- Playlists: m3u/pls/xspf/folder expansion, `.gpls` v1 save/load, snapshots,
  Settings INI, editable keymap, 19 bundled locales.
- Streams: yt-dlp then streamlink, proxy/cookies/format settings, HLS and
  local RTSP tests, Open URL.
- Local `.app` bundles libVLC and plugins; the smoke test runs from `/tmp`
  with an isolated data directory.

Still open:

- Grid editor: manual lines/weights, user templates; Fit Shapes size steps,
  reorder and crop share; "match shapes" for Auto Rows/Columns.
- Free/mosaic: PUSH overlap, z-order, optimizer/reorder-to-fit, equal-gap and
  video-shape snap.
- Playlist parity: list-end pause/close and upstream `list_end_action`/
  `list_fail_action`/`list_fail_wait_sec` names; external audio/subtitle file
  persistence; track/device/deinterlace/color/header fields; adaptive stream
  quality.
- F11: videos-per-process does not exist on macOS (HW_SP is forced); a
  selectable driver is not exposed. Tray/close-to-tray stay out of macOS scope.
- F13: full menu/option translation coverage.
- F1: frozen performance/media matrix evidence and final acceptance run.

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

# Migration Checkpoint — 2026-11-08 (post-bootstrap review)

## Pinned inputs (frozen, spec §2.2)
- Upstream vzhd1701/gridplayer HEAD: `bf943684232d74ed9e271c3c9eeef6e7a7ed6e0f`
  (0.5.5, 2026-11-03T19:03:42Z), resolved 2026-11-08 14:09 CST (UTC+8).
- MyGo: `46f8473c5753519a951e65c38c0b53872fdf2a7a` (go.mod
  `v0.8.4-0.20261108060507-46f8473c5753`, go 1.27.3, MIT). Locked.
- libVLC: 3.0.24 Vetinari (`/Applications/VLC.app`, Cask), VideoToolbox plugin present.
- Target: Wade11s/GridPlayer, `main` = seed `029ec3c` + bootstrap landing `94753c1`
  + evidence `86eea30`. The PoC landing happened before a PR base existed; it is
  recorded as pre-gate scaffolding, not as a gated merge (docs/MERGE_GATES.md).
- Oracle: `/Applications/GridPlayer.app` 0.1.0 (`com.wade11.gridplayer`) ≠ upstream
  0.5.5 (`com.vzhd1701.gridplayer`); mismatch recorded, oracle comparison blocked.

## Progress
- §3 matrix: docs/BEHAVIOR_MATRIX.md (F1–F13, macOS HW_SP mapping).
- §4.1 PoC-A: isolated AppKit 4×NSView + `set_nsobject` → PASS; VideoToolbox ×4.
- §4.1 PoC-C: MyGo window + 4 HW panes via `internal/nativehost` + `Painter.Hole`
  → PASS; screenshots `testdata/poc-hw-evidence/`, `testdata/gridplayer-4pane.png`.
- Attempt B (libVLC video callbacks): SW (avcodec), rejected as HW evidence
  (docs/ARCHITECTURE.md).
- Post-review hardening on `bootstrap/gates-v2`: atomic closed guard, nil-safe
  libVLC player methods with per-player mutex, exact UTF-8 length for NSString,
  deterministic error-path cleanup, Linux build stubs + CI script.

## Gates
- Independent review via fresh subagent context (not codex exec), SHA-bound.
- CI: `.github/workflows/ci.yml` on ubuntu + libvlc-dev runs `scripts/ci.sh`
  (vet/build/test, no GUI). Same-repo PR workflows run from the PR head.
- Local macOS GUI acceptance runs on the exact PR head before merge.

## Merged
- PR #6 merged 2026-11-08T10:48:43Z at `9291b07` (head `a76e995`), independent
  subagent review PASS on the exact head, CI `vet-build-test` green, local GUI
  acceptance green on the exact head. Issue #1 closed.
- Post-merge main `9291b07`: CI green; local poc-hw PASS; gridplayer 4 panes,
  teardown clean.

## In flight
- Issue #2 (engine core): `internal/engine` session boundary, generation fencing,
  coalesced snapshots, fake-backend tests + real libVLC dummy-output integration
  test; `cmd/gridplayer` now uses one Engine per cell. Local GUI re-acceptance:
  4 panes playing, loop wrapped past 10s, VideoToolbox ×4, clean teardown.
- #2 tracks/chapters: `vlcmac` track/chapter queries + selection, engine snapshot
  and `RefreshTracks`; feature.mkv fixture (3 chapters, 2 audio, 2 subtitle tracks);
  real libVLC test switches audio/subtitle/chapter. CI/local green.
- Remaining #2 work: external A/V slaves, end-of-list actions, recovery/network retry.
- #3 grid UI: internal/grid AutoRows/AutoCols/Fixed with tests; dynamic 1..N cells,
  menu (File/Playback/Grid/View/Tracks), per-cell overlay controls, Fill cover via
  clip view + Fit, shortcuts (Space/arrows/M/L/B/N/[/]), active-cell border.
  Local screenshots: 1-cell, 2-cell fill, 4-cell.
- #3 drag reorder: cells are `Drag(i)` sources and `Drop[int]` targets; a drop
  swaps cell structs (path/engine/view) and re-frames both slots. Unit test for
  swap bounds. Free/mosaic/single mode still open.
- #2 view/external tracks: aspect/scale/crop in engine+vlcmac, `file://` subtitle/audio
  slaves remembered across reloads, View + Tracks file menus, settings persistence
  for `cell_view_mode` (fill/fit/stretch) plus aspect/scale/crop. Real libVLC
  dummy-output test attaches `feature.de.srt` and sees a new subtitle track.
- #5 local .app: `mygo.json` + pinned `go tool mygo`, icon, `scripts/build_app.sh`,
  `scripts/smoke_app.sh`. `dist/darwin-arm64/GridPlayer.app` (ad-hoc signed)
  launches via `open`, plays 4 fixtures, logs VideoToolbox ×4, exits cleanly;
  no residual process. VLC remains external at /Applications/VLC.app (rpath +
  VLC_PLUGIN_PATH), documented in docs/APP_BUILD.md. Not a public release.
- #4 playlist persistence (partial): `internal/playlist` JSON v1 read/write with
  unknown-field preservation + atomic save; app load/save/save-as, startup
  `--playlist`, position/mute/volume/rate/view/grid restore, unsaved prompt.
  Tests: round-trip, validation, relative URIs. Settings.ini/recent/bookmarks/
  lists/legacy text format remain open.
- #4 settings slice: `internal/settings` QSettings-compatible INI, unknown keys
  preserved, atomic save; app restores grid/Fill-Fit/default volume-mute/window
  bounds/recent list, `--user-data-dir` isolation. Full settings dialog/keymap/
  legacy migrations remain open.

No scope, acceptance, or gate lowered. No public release, no notarization,
no Gatekeeper change.

# Migration Checkpoint — 2026-11-09 (post-grid-state-persist merge)

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
- PRs #7–#16 merged through `aedfbcb`: engine core/tracks/chapters/view/external,
  grid+drag+single+file-drop, playlist/settings persistence, local .app +
  smoke. Main `aedfbcb`: CI green, race tests green, .app smoke 4× VideoToolbox.
- PRs #17–#26 merged through `a24fc43`: frame screenshots, HLS/URL resolver,
  free tiles + drag/resize, media lists, dummy backend, bookmarks. Main
  `a24fc43`: CI green, `go vet`/`go test ./... -race`/build green, local GUI
  acceptance 4× VideoToolbox, bookmark add/jump/clear and active-cell menu
  refresh verified.
- PR #27 merged at `040b770`: mosaic lock + wall-aware Arrange. Main
  `040b770`: CI green, full race/vet/build green, GUI acceptance 4×
  VideoToolbox, one- and two-lock arrange verified with persisted boxes.
- PR #28 merged at `9ec176d`: tile snapping and optional free-mode overlap.
  Main `9ec176d`: CI green, full race/vet/build green, GUI acceptance 4×
  VideoToolbox, Grid toggles persisted `grid_snap=false`/`grid_overlap=false`.
- PR #29 merged at `8b1590e`: Settings window (General/Playback/Grid/Video).
  Main `8b1590e`: CI green, full race/vet/build green, GUI acceptance 4×
  VideoToolbox, Settings Apply persisted `grid_overlap=false`, Cancel closed
  without changes.
- PR #30 merged at `fd1a779`: per-file bookmarks manager with rename/delete.
  Main `fd1a779`: CI green, full race/vet/build green, GUI acceptance on a
  `list.m3u` cell (1 VideoToolbox), per-item key and manager render verified.
- PR #31 merged at `f6aca2d`: streamlink fallback, proxy/cookies/quality
  settings and Network tab. Main `f6aca2d`: CI green, full race/vet/build
  green, loopback HTTP→libVLC test PASS, local HLS via yt-dlp played with
  VideoToolbox and clean teardown.
- PR #32 merged at `6ad006e`: libVLC + plugins bundled into the .app. Main
  `6ad006e`: CI green, full race/vet/build green, bundled app smoke PASS
  (4× VideoToolbox), dyld log shows only bundled libvlc/libvlccore.
- PR #33 merged at `6c277a1`: per-item view mode, paused restore, list index
  and end-of-list actions. Main `6c277a1`: CI green, full race/vet/build
  green, GUI `.gpls` round-trip kept paused at 2000 ms, list index 1,
  `list_end=next`, `cell_view_mode=fit`; View menu Fit checked.
- PR #34 merged at `3d52674`: RTSP end-to-end test with a local VLC RTSP
  server. Main `3d52674`: CI green, full race/vet/build green, escalated
  `TestRealVLCRTSP` PASS (3.38 s); in-sandbox it skips at port reservation.
- PR #35 merged at `7df3f65`: named playlist/grid snapshots. Main `7df3f65`:
  CI green, full race/vet/build green, GUI saved a 4-cell snapshot, switched
  to Stretch, restored Fill from Snapshot 1; live-engine restore test PASS.
- PR #36 merged at `b460d1a`: grid layout/free/mosaic/single persistence.
  Main `b460d1a`: CI green, full race/vet/build green, GUI free playlist kept
  4 tiles and Free Tiles checked; mosaic snapshot kept 4 tiles + lock 0 and
  restored Mosaic/Lock Active Tile after switching to Auto Rows.

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
- #3 drag reorder + file drop (merged aedfbcb): internal drag-swap plus
  Finder drops (cell insert / window append, `.gpls` opens)
- #3 drag reorder: cells are `Drag(i)` sources and `Drop[int]` targets; a drop
  targets; a drop swaps cell structs (path/engine/view) and re-frames slots.
- #3 external file drop: cell drop inserts files before that cell, window drop
  appends, a single `.gpls` opens. `insertCells` preserves live engines; unit
  test covers insert/append. Free/mosaic still open.
- #3 free tiles (merged 25d1846): `internal/grid.FreeLayout` auto-arrange
  fractions + gap inset + toggle/`--free`/settings round-trip.
- #3 free drag/resize: cell drag moves (MoveTile), bottom-right handle resizes
  (ResizeTile), clamped to page/min size; unit tests. Overlap/lock/arrange
  still open.
- #3 mosaic initial arrange (merged d2014c3): `internal/grid.Mosaic` tiered
  tiles + Grid toggle/`--mosaic`/`playlist/grid_mosaic`.
- #3 mosaic lock/arrange (merged 040b770): `internal/grid.Arrange` keeps
  locked tiles fixed, cuts the page around them, fills free pieces with
  justified rows, and falls back to `Mosaic(n)` when no room remains; app
  drag/resize, lock/unlock, Arrange Mosaic, and
  `grid_mosaic_tiles`/`grid_mosaic_locked` persistence; exhaustive
  single/double-lock tests.
- #3 snap/overlap (merged 9ec176d): `SnapMove`/`SnapResize` snap page and
  neighbour edges/middles within 8 DIPs; `MoveTileNoOverlap`/
  `ResizeTileNoOverlap` reject overlap-increasing free-mode steps and slide
  on one axis; `Tile.Valid` rejects corrupt persisted boxes; Grid toggles
  persist `grid_snap`/`grid_overlap`. Equal-gap/video-shape/old-place snap,
  PUSH overlap, optimizer/reorder-to-fit and z-order still open.
- #2 dummy backend (merged 96f027c): `internal/dummy` 100s/25fps deterministic
  clock (mutex-guarded, loop wrap, honest stop, PNG screenshot, Tick events);
  engine-load test under `-race`. Full dummy GUI driver and free drag/resize
  still tracked.
  Free drag/resize/arrange/mosaic still open.
- #3 single mode (merged 15bfc12): `internal/grid.SingleLayout` (Auto/Right/
  Left/Bottom/Top, share clamp 5%..40%, STRIP_PER_LINE=4, tiles centered) +
  menu + `playlist/single_mode_strip{,_side}` settings; screenshots 1/2/4/single.
- #2 view/external (merged 15bfc12): aspect/scale/crop persisted, external file://
  subtitle/audio remembered across reloads, real attach test. Free tiles/mosaic
  and remaining parity (streams, settings UI, bundling) still open.
- #2 view/external tracks (merged 467e843): aspect/scale/crop settings, external
  subtitle/audio loaded via file://, remembered across reloads, feature.de.srt
  fixture + real attach test; View/Tracks menus; only pinned pixel/scalable
  fields implemented. Free/mosaic/single mode still open.
- #2 view/external tracks: aspect/scale/crop in engine+vlcmac, `file://` subtitle/audio
  slaves remembered across reloads, View + Tracks file menus, settings persistence
  for `cell_view_mode` (fill/fit/stretch) plus aspect/scale/crop. Real libVLC
  dummy-output test attaches `feature.de.srt` and sees a new subtitle track.
- #2 frame screenshots: `libvlc_video_take_snapshot` through engine, Save Frame
  dialog + overlay Shot, `--screenshot` automation flag verified to write a PNG.
  Zoom/anchor/shift/transform, free tiles/mosaic, streams, settings UI and
  VLC bundling still open.
- #8 streams (partial): deterministic local HLS fixture (3 renditions), URL MRL
  loading via `libvlc_media_new_location`, `file://` HLS GUI playback verified,
  Open URL dialog; scale default fixed to fit (0) after the 1x regression.
  Adaptive mid-playback quality and streamlink cookies still open.
- #8 URL resolver: `internal/resolve` yt-dlp (ignore-config, no cookies,
  skip-download, no-playlist) with direct fallback; Open URL dialog and
  `--url` startup path; local HLS resolution test. Streamlink/cookies/proxy/
  adaptive quality still open.
- #8 stream resolver (merged f6aca2d): yt-dlp first then streamlink;
  proxy/cookies/format for yt-dlp, proxy for streamlink; Settings > Network
  for proxy/cookies/tool paths/quality; `playURL` snapshots options before the
  goroutine; fake-tool, file-skip, real-HLS and loopback HTTP→libVLC tests.
  Adaptive mid-playback quality and streamlink cookies remain open.
- #8 RTSP e2e (merged 3d52674): `TestRealVLCRTSP` starts VLC's RTP/RTSP server
  on a free loopback port and plays it in a second libVLC instance with dummy
  output; skips when VLC.app is missing or the bind is denied.
- #7 media lists: `internal/medialist` m3u/pls/xspf/folder expansion; one-cell
- #12 bookmarks (merged a24fc43): `internal/bookmarks` positions in
  `bookmarks.json` (atomic, dedupe by name, sorted after replacement);
  Bookmarks menu add/list/jump/clear.
- #12 bookmarks manager (merged fd1a779): keys by the current file (playlist
  item), default `Bookmark N` names skip collisions, `Has`/`Rename`/`Clear`,
  and a manager window with add/jump/rename/delete/clear; menu and manager
  follow active-cell and playlist-item changes. Lists/snapshots, colors and
  per-playlist bookmark scopes remain open.
  list advances on Ended (once per generation), wraps, Previous/Next menu,
  badge i/n; list.m3u fixture and state test. Source path is persisted;
  per-item settings and end/fail actions still open.
- #2 frame screenshots (merged c7fa9a2): libVLC `video_take_snapshot` through
  engine, Save Frame dialog + Shot overlay + `--screenshot` automation;
  PNG verified from a playing fixture. Zoom/shift/transform, free tiles/
  mosaic, streams, settings UI, VLC bundling still open.
- #5 local .app (merged 6ad006e): `mygo.json` + pinned `go tool mygo`, icon,
  `scripts/build_app.sh`, `scripts/smoke_app.sh`. The .app bundles
  libvlc/libvlccore and 88 MB of VLC plugins under Contents/Frameworks and
  Contents/Resources/vlc/plugins, uses only the
  `@executable_path/../Frameworks` rpath, is ad-hoc signed, and needs no
  installed VLC at runtime. Smoke: 4× VideoToolbox; dyld shows bundled libs.
  Developer ID signing/notarization and universal binary remain open.
- #4 playlist persistence (partial): `internal/playlist` JSON v1 read/write with
  unknown-field preservation + atomic save; app load/save/save-as, startup
  `--playlist`, position/mute/volume/rate/view/grid restore, unsaved prompt.
  Tests: round-trip, validation, relative URIs.
- #4 playlist per-item settings (merged 6c277a1): per-cell view mode,
  paused/stopped restore, `list_index` and `list_end` (loop/stop/next cell);
  View menu follows the active cell, Playback > End of List persists per cell.
  Per-item aspect/scale/crop, snapshots/lists and legacy text format remain
  open.
- #4 settings slice: `internal/settings` QSettings-compatible INI, unknown keys
  preserved, atomic save; app restores grid/Fill-Fit/default volume-mute/window
  bounds/recent list, `--user-data-dir` isolation. Full settings dialog/keymap/
  legacy migrations remain open.
- #4 settings window (merged 8b1590e): File > Settings… (Cmd+,) with
  General/Playback/Grid/Video tabs; applies layout, snap/overlap, default
  volume/mute/rate/loop, view mode/aspect/scale/crop; Apply/Cancel/Reset;
  persists `video_defaults/rate`/`loop`. Keymap editing, subtitle styling,
  network/proxy and translations remain open.
- #4 snapshots (merged 7df3f65): `internal/snapshots` atomic named store;
  Save Current / restore / Delete menu; restore replaces cells/grid, applies
  per-item state to live engines, and marks the playlist unsaved. Snapshot
  export/import remains open.
- #4 grid state persistence (merged b460d1a): documents store `grid_layout`,
  free/mosaic tiles, mosaic locks, single side/share; restore validates tiles
  and lock bounds and clamps single share; older documents fall back to
  `grid_state`. Mosaic optimizer/PUSH/z-order still open.

No scope, acceptance, or gate lowered. No public release, no notarization,
no Gatekeeper change.

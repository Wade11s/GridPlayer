# Frozen Acceptance Criteria (bootstrap PoC scope)

Frozen 2026-11-08 (bootstrap) against source SHA
`bf943684232d74ed9e271c3c9eeef6e7a7ed6e0f` and tool versions below.
Changes need a new review; thresholds never relax in-implementation.

## Tools (evidence, not claims)

- host: macOS 27.2 arm64 (M2), Go 1.27.3, MyGo `46f8473c5753519a951e65c38c0b53872fdf2a7a`
  (`v0.8.4-0.20261108060507-46f8473c5753`), VLC 3.0.24 Vetinari (Cask), ffmpeg 9.0.2,
  Python 3.15.2 + Pillow 13, Codex CLI 0.167.0.
- fixtures: `scripts/make_fixtures.py` output in `testdata/fixtures/`; `manifest.json`
  carries per-file sha256 (A=`5b17a7d0...`, B=`55067931...`, C=`4585f4e6...`, D=`75f3695d...`).
  Reproducible: rerun the script, compare the manifest.

## PoC pass criteria (all must hold on the exact head SHA)

1. `scripts/build.sh poc-hw && ./bin/poc-hw` prints `poc-hw: PASS`:
   4 players `state=3`, `640x360`, seek landing ≥2500ms after SetTime(3000)+resume,
   teardown stop→unbind→detach→release, + `Using Video Toolbox to decode 'h264'`
   ×4 in the verbose log (hardware path, not software).
2. Frame-number window evidence: two `screencapture` frames ~1s apart show the
   same fixture frame index advancing ~25 frames (continuous motion, not stills),
   no black pane, identities A/B/C/D correct in every pane.
3. `scripts/build.sh gridplayer && ./bin/gridplayer --autoquit N` (window visible):
   4 badges show advancing times, screenshots show 4 panes with correct letters +
   advancing frame numbers + moving ball; all cells `state=3` at teardown;
   `Video Toolbox` in `testdata/gridplayer-vlc.log`; audible tone on cell A only
   (others muted); close completes teardown without crash.
4. Repeat open/close twice: no residual `gridplayer`/`poc-hw` process (checked via
   `pgrep`), no corrupted state dir.
5. No Gatekeeper/signature changes; binary runs with existing security settings.

## Fail criteria (any one blocks merge)

- Any black pane, frozen frame numbers between shots, cross-pane identity error.
- `avcodec` (software) as the only decoder in the HW run, or VideoToolbox lines missing.
- Crash, hang past a 60s wall timeout per run, or residual processes after teardown.
- Fixture manifest mismatch after regeneration.

## Explicitly out of PoC acceptance

- Audio sample-level verification, seek accuracy stats vs oracle, Retina/occlusion figures:
  tracked as later issues against the frozen behavior matrix, not silently skipped.
- Oracle baseline (`0.1.0` app vs `0.5.5` source): version mismatch recorded in
  `docs/BEHAVIOR_MATRIX.md`; oracle comparison is blocked, never downgraded.

## Evidence log (bind the exact head SHA in the PR)

- 2026-11-08, `bootstrap/gates-v2` working tree: `poc-hw` PASS, 4× `state=3`,
  seek 3654ms after `SetTime(3000)`+resume, VideoToolbox ×4; `gridplayer` 4 panes,
  frame 0051 in all panes, teardown clean, exit 0. Screenshot
  `testdata/gridplayer-4pane.png`.
- `go vet ./...` exit 0; `scripts/build.sh all` builds all three binaries.
- 2026-11-23, PR #72 list-end candidate (exact final head is bound in the PR):
  bundled-app runs with a generated one-second H.264/AAC list fixture and
  isolated `/tmp` data verified Pause At Start at 0 ms, paused save/reopen,
  and Close removing the cell. Evidence: `/tmp/gridplayer-listend-pause-reload/`,
  `/tmp/gridplayer-listend-pause-reopen/`, and
  `/tmp/gridplayer-listend-close/` (`saved.gpls` + `vlc.log`, VideoToolbox
  present). These are targeted local app checks, not the full media/GUI
  acceptance matrix.
- Failed list-end evidence, not passes: the fixed three-second autosave
  sampled a one-second clip before its settled end state in two runs; a
  0.2-second probe and a settled one-second probe showed seek+pause (also
  with the late-Playing guard) settling as `idle`. The implementation then
  changed to reopen-at-start with the paused-restore guard, which produced
  the passing settled runs above. No threshold was lowered.
- 2026-11-23, external-track candidate (exact final head is bound in its PR):
  `TestRealVLCExternalFilesSurviveReloadAndClear` passed with real libVLC
  subtitle-track enumeration. Bundled-app save/reopen preserved ordered
  external audio/subtitle arrays plus upstream top-level fields; another
  real list run cleared those files on item 2. Evidence:
  `/tmp/gridplayer-external-app/` and `/tmp/gridplayer-external-list-step/`.
  Both app runs used isolated data, generated fixtures, and VideoToolbox;
  their logs name both external files. This verifies persistence/attachment
  paths, **not sample-level external-audio output or full GUI acceptance**.
- 2026-11-23, F10 candidate: `TestSpaceAllWorksWithFocusedCellButton` passed
  with MyGo input (Tab-focused button, no double toggle). Agent-desktop runs
  verified real two-pane Space-all pause, focused Tab+Space all-cell pause,
  and an editable F12 active-cell key. Corrected `/tmp` runs
  `/tmp/gridplayer-f10-real-tmp/` and `/tmp/gridplayer-f10-focused-tmp/`
  used VideoToolbox and exited cleanly by autoquit with no residual process.
  Default Ctrl+Space produced no state change under the current macOS
  input-source setup; Ctrl+Right was delivered. Earlier attempts mistakenly
  opened generated fixtures through their Desktop paths: a stack sample
  showed libVLC blocked in `open` while teardown waited in `input_Close`.
  Those runs failed and their known isolated PIDs required SIGKILL; they are
  not passes. No system keyboard or security setting was changed. Standard
  smoke passed at `69f25ccbce024234eda650cfc6e6410b05b877d4` and again at
  `96aeb2b54cf6b691838785497ce4a1ff966733d7`; the real media key runs used
  code-identical candidate `cdbd0d03e74aed843ef85253d2f382fb3234ec0f`.
  Later commits changed checkpoint/review wording only.
- 2026-11-23, metadata candidate: cell-local ID/title/unknown-field swap and
  insertion regression tests pass. Real bundled-app save/reopen at
  `/tmp/gridplayer-metadata-app/` preserved a synthetic UUID, title,
  `custom_field`, and raw `audio_selection`. This verifies persistence;
  the swap/insert cases are unit evidence, not a claimed GUI drag run.
- 2026-11-23, z-order candidate: pure permutation/tier/topmost and app
  persistence tests pass. Real overlapping Free Tiles evidence at
  `/tmp/gridplayer-zorder-app/`: `before.png` shows B covering A; Grid →
  Bring to Front produced `after.png` with A covering B. `saved.gpls`
  persisted `grid_free_order:[1,0]`; VideoToolbox played and autoquit left
  no process. Captures contain only the test app window. This is targeted
  native stacking evidence, not PUSH/optimizer or full occlusion acceptance.
- 2026-11-24, F11 keep-awake candidate (`751c7a1`, main `f51faf3`):
  `go vet ./...`, `go test ./... -race -count=1`, `go build ./...`, and
  `scripts/build_app.sh` PASS; `scripts/smoke_app.sh` PASS with 4
  VideoToolbox lines. A bundled-app run with one generated 60 s H.264/AAC
  fixture and isolated data (`/tmp/gridplayer-f11-evidence/`) held
  `/usr/bin/caffeinate -d -i -w <pid>` while the cell played and removed it
  after Pause (`keep awake: none`), with `Video Toolbox` in the log, clean
  autoquit teardown, and no residual `gridplayer`/`caffeinate` process. The
  minimized system-only assertion is unit evidence
  (`TestKeepAwakeModeFollowsPlaybackAndMinimize`,
  `TestCaffeinateCommandModes`), not a claimed GUI run: agent-desktop AX
  minimization delivered no MyGo `OnMinimize` event in this session.
  `pause_minimized` and background-pause key semantics remain open.
- 2026-11-24, F11 minimize-pause candidate (`079ab3b`, main `d6647b6`):
  `go vet ./...`, `go test ./... -race -count=1`, `go build ./...`,
  `scripts/build_app.sh`, and `scripts/smoke_app.sh` PASS with 4
  VideoToolbox lines. A bundled-app run at `/tmp/gridplayer-f11-pause/` with
  isolated data read `pause_minimized=false`, wrote it back, preserved the
  unknown `playlist/pause_background_videos` key, and exited cleanly with no
  residual process. The minimize pause/restore itself is unit evidence
  (`TestMinimizePausesThenRestoresPlayingCells`,
  `TestMinimizePauseCanBeDisabled`); MyGo did not report the minimized state
  in this session, so no GUI minimize run is claimed. Single-mode
  `pause_background_videos` remains open with the single-mode layout work.
- 2026-11-24, F9 proxy-mode candidate (`e136a87`, main `8ef2a54`):
  `go vet ./...`, `go test ./... -race -count=1`, `go build ./...`,
  `scripts/build_app.sh`, and `scripts/smoke_app.sh` PASS with 4
  VideoToolbox lines. Two isolated bundled-app runs at
  `/tmp/gridplayer-f9-proxy/`: the legacy `network/proxy` URL loaded and
  saved back as `proxy_mode=custom` + `proxy_url=http://legacy:8080`, with
  the legacy key preserved; upstream `proxy_mode=none` + a URL stayed
  unchanged. Both exited cleanly with 4 VideoToolbox lines and no residual
  process. Resolver mode/argument/environment behavior is unit evidence
  (`TestProxyModesSelectFlag`, `TestProxyNoneClearsInheritedEnvironment`);
  VLC playback proxying and the relay path remain open.
- 2026-11-24, F12 loop-chapter candidate (`7bcabab`, main `45be973`):
  `go vet ./...`, `go test ./... -race -count=1`, `go build ./...`,
  `scripts/build_app.sh`, and `scripts/smoke_app.sh` PASS with 4
  VideoToolbox lines. A generated 60 s chaptered H.264/AAC fixture
  (chapters at 0/20/40 s) at `/tmp/gridplayer-f12-loop/` was played through
  the bundled app with isolated data. Choosing Playback > Loop Chapter saved
  `loop_end=20000` (start 0 omitted by the empty-field rule); a second 35 s
  run logged `loop segment: seek 20011 -> 0ms` and saved
  `current_position=12524` after wrapping inside that segment. Both logs show
  VideoToolbox, clean teardown, and no residual process. A last-chapter run
  with isolated data and `end_action=stop`, `loop=false` chose chapter Three
  (`loop_start=40000`, no end), logged `loop segment: restart at end` followed
  by `seek 0 -> 40000ms`, and saved at `current_position=56687` while playing
  (`/tmp/gridplayer-f12-loop/last7/`); the ended restart reopens the media, so
  that pass produced two VideoToolbox lines. Cleaning, the 250 ms boundary
  tolerance, start/end/reset, clearing on a new list item, chapters before
  the playhead, and the no-chapter paths are unit evidence.
- 2026-11-24, F2/F12 segment-seek candidate (`145c7c6`, main `ce98aa2`):
  `go vet ./...`, `go test ./... -race -count=1`, `go build ./...`,
  `scripts/build_app.sh`, and `scripts/smoke_app.sh` PASS with 4
  VideoToolbox lines. A bundled-app run with isolated data at
  `/tmp/gridplayer-f12-seek5/` chose Loop Chapter on chapter One and saved
  `loop_end=20000`, `current_position=11792`, after logging
  `loop segment: seek 20007 -> 0ms`; a chapter-Three run
  (`/tmp/gridplayer-f12-seek2/`) saved `loop_start=40000` without a redundant
  `loop_active` but ended during its EOF reload, so only its persistence shape
  is claimed. Both logged VideoToolbox, exited cleanly, and left no residual
  process. Seek clamping is unit evidence (`TestLoopSeekBounds`,
  `TestSeekCellToClampsSegment`, `TestChapterJumpSeeksInsideSegment`) and
  whole-file persistence is `TestWholeFileSegmentSurvivesDocument`,
  `TestLoopActiveFromDocument`, `TestLoopActiveResetDropsExtension`,
  `TestUpstreamWholeFileLoopPresence`; chapter reachability is
  `TestChapterStopSelection`; one run
  is not proof of every seek path.
- 2026-11-24, F12 chapter-stops candidate (`0f9b723`, main `6ee09d7`):
  `go vet ./...`, `go test ./... -race -count=1`, `go build ./...`,
  `scripts/build_app.sh`, and `scripts/smoke_app.sh` PASS with 4
  VideoToolbox lines. A bundled-app run with isolated data at
  `/tmp/gridplayer-f12-chapters12/` used the default loop end action and
  pressed Next Chapter on the last chapter; VLC logged `SET_TIME to 0` and
  the cell saved playing at 10245, so it looped instead of stopping.
  A pause-action run at `/tmp/gridplayer-f12-chapters13/` logged
  `pause at start: 0ms` and saved `playback_state=paused` at time 0 after the
  reopen (two VideoToolbox lines), clean exit, no residual. Stop selection,
  the 250 ms tolerance and the 3 s Previous rule are unit evidence
  (`TestChapterStopSelection`, `TestChapterNextPastLastRunsEndAction`,
  `TestChapterJumpSeeksInsideSegment`).
- 2026-11-25, F12 chapter-stops head (`d4096d0`, main `6ee09d7`):
  `go vet ./...`, `go test ./... -race -count=1`, `go build ./...`,
  `scripts/build_app.sh`, and `scripts/smoke_app.sh` PASS with 4
  VideoToolbox lines at `d4096d0`. The bundled app played the generated 60 s
  chaptered fixture (0/20/40 s) at `/tmp/gridplayer-f12-loop/loopchapter.mkv`
  with isolated data at `/tmp/gridplayer-f12-chapters18/`; three
  Tracks > Next Chapter clicks logged `SET_TIME to 20000000`, `40000000`,
  then `0` at the last stop with the default `end_action=pause`, and the app
  logged `pause at start: 0ms` followed by `keep awake: none`. The saved
  playlist is `playback_state=paused` with `end_action=pause`; the log has
  one VideoToolbox line, so the pause path seeks and pauses the live input
  instead of reopening it. Teardown finished with no residual
  `gridplayer`/`poc-hw` process. The parent code candidate `34ceccb`
  reproduced the sequence at `/tmp/gridplayer-f12-chapters17/`. The frozen
  boundaries are asserted in `TestChapterStopSelection` (250 ms next
  boundary, 3 s Previous boundary) and the end-action helper scope in
  `TestApplyDefaultEndSkipsLoopDefaultForLists`.
- 2026-11-25, F5 snap-kinds candidate (`c7fcbae`, main `78e51f8`):
  `go vet ./...`, `go test ./... -race -count=1`, `go build ./...`,
  `scripts/build_app.sh`, and `scripts/smoke_app.sh` PASS with 4
  VideoToolbox lines. A bundled-app run at `/tmp/gridplayer-snaps2/` started
  from the frozen JSON `tile_snaps=["window_edges","old_place"]`: the Settings
  Grid tab showed only "Page edges" and "Where the tile was" checked
  (`grid-tab.png`); toggling "Neighbour middles" and pressing Apply wrote
  `tile_snaps=["tile_middles","window_edges","old_place"]` with
  `grid_snap=true` (`settings-after-apply.txt`), and teardown left no
  process. The snap maths
  itself (kind filtering, old place, video shape) and the settings round-trip
  are unit evidence; a GUI drag was not driven, and the equal-gaps snap,
  guides, a resize old-place snap, any frozen threshold setting and per-kind
  menu items remain open.
- 2026-11-25, F5 grid-undo candidate (`a11967f`, main `8c8ca7e`):
  `go vet ./...`, `go test ./... -race -count=1`, `go build ./...`,
  `scripts/build_app.sh`, and `scripts/smoke_app.sh` PASS with 4
  VideoToolbox lines. A bundled-app run at `/tmp/gridplayer-undo5/` loaded four
  videos in Free Tiles whose reading order was d,c,b,a and chose Grid > Fixed
  2x2 (`after-fixed.png`), which permutes the cells into that order; Grid >
  Undo Grid Change restored the Free Tiles layout with each video in its own
  tile (`after-undo.png`; `settings-after-undo.ini` `grid_free=true`,
  `grid_mode=auto_rows`), and Grid > Redo Grid Change re-applied the 2x2
  (`after-redo.png`; `settings-after-redo.ini` `grid_free=false`,
  `grid_mode=fixed`). `saved.gpls` holds the four videos with `grid_layout`
  fixed, teardown finished, and no `gridplayer`/`poc-hw` process remained. The
  per-step settings copies (`settings-after-fixed.ini`, `-undo.ini`,
  `-redo.ini`) record the transitions. The snapshot (cell order included), the
  redo trail, the declined-confirm and no-op guards, the trail cleared by a
  closed video, the 32 cap, the tile reset and the other-set restore are unit
  evidence (`gridundo_test.go`).
  Tile drags and line edits are not covered, and there is no shortcut.
- 2026-11-25, F12 screenshot-all candidate (`bea4ce0`, main `1a758c3`):
  `go vet ./...`, `go test ./... -race -count=1`, `go build ./...`,
  `scripts/build_app.sh`, and `scripts/smoke_app.sh` PASS with 4
  VideoToolbox lines on this tree. A bundled-app run at
  `/tmp/gridplayer-shotall5/` with three cells and
  `<run>/data/settings.ini` set to `dir=/tmp/gridplayer-shotall5/out`,
  `filename_template=%F`, `format=jpg`,
  `jpg_quality=60` chose Playback > Screenshot (All): `plane-a.jpg`,
  `plane-b.jpg` and `plane-c.jpg` appeared, all reported by `file` as JPEG
  640x360, with no temporary `.png` left. Teardown finished and no
  `gridplayer`/`poc-hw` process remained. The per-cell loop, the skip for a
  cell without a video, the counted status and the empty-run status are unit
  evidence (`TestTakeScreenshotAllWritesOnePerCell`,
  `TestTakeScreenshotAllSkipsAudioOnlyCells`,
  `TestTakeScreenshotAllWithoutAVideoReportsIt`); the frozen `show_if` gating
  stays open (the menu is snapshotted at launch, so a disabled state would go
  stale), as does the missing all-videos shortcut.
- 2026-11-25, F12 screenshot-names candidate (`864b44a`, main `0ef3300`):
  `go vet ./...`, `go test ./... -race -count=1`, `go build ./...`,
  `scripts/build_app.sh`, and `scripts/smoke_app.sh` PASS with 4
  VideoToolbox lines on this tree. A bundled-app run at
  `/tmp/gridplayer-shots4/` used `<run>/data/settings.ini` with
  `screenshots/dir=/tmp/gridplayer-shots4/out`,
  `filename_template=%F_%n`, `format=jpg` and `jpg_quality=55`; two Shot
  clicks wrote `plane-a_0001.jpg` and `plane-a_0002.jpg`, both reported by
  `file` as JPEG 640x360 with `ffd8` headers, and no temporary `.png` was
  left in the folder. The settings kept all four keys, teardown finished, and
  no `gridplayer`/`poc-hw` process remained.
  Naming, sanitising, the counter and suffix, the folder, the format, the
  character-counted limits and the Shot/shortcut call sites are unit evidence
  (`internal/screenshots`, plus the app settings round-trip and the
  source-level call-site guard);
  this run does not cover the PNG path or the cropped-to-view frame, which the
  frozen `ScreenshotJob` does and Go does not yet. The JPEG re-encode runs
  inline on the main thread in this build, where the frozen job uses a pool.
- 2026-11-25, F5 leave-tiled-for-plain candidate (`9c316ae`, main `73eddd9`):
  `go vet ./...`, `go test ./... -race -count=1`, `go build ./...`,
  `scripts/build_app.sh`, and `scripts/smoke_app.sh` PASS with 4
  VideoToolbox lines. A bundled-app run at `/tmp/gridplayer-leave2/` loaded
  five videos in Free Tiles reading e,d,c,b,a and chose Grid > Auto Rows: the
  flow reads e,d,c / b,a (`auto-rows.png`), and `saved.gpls` holds
  plane-e,d,c,b,a with `grid_layout` auto_rows. All five cells were playing
  at teardown, autoquit exited cleanly, and no `gridplayer`/`poc-hw` process
  remained. The plain-grid re-pack, the Settings Apply that keeps a tiled
  layout untouched, the Settings plain-grid re-read, the declined fixed entry
  that keeps its tiles, and the template entry are unit evidence
  (`TestLeavingTiledForAPlainGridPacksAndClears`,
  `TestSettingsApplyKeepsATiledLayout`,
  `TestSettingsApplyLeavesTiledForPlain`,
  `TestDeclinedFixedEntryKeepsTheTiles`, `TestTemplateEntryPacksAndCloses`).
- 2026-11-25, F5 repack-tiled-order candidate (`84fdbf6`, main `3f0dbc1`):
  `go vet ./...`, `go test ./... -race -count=1`, `go build ./...`,
  `scripts/build_app.sh`, and `scripts/smoke_app.sh` PASS with 4
  VideoToolbox lines. A bundled-app run at `/tmp/gridplayer-repack3/` loaded
  five videos in Free Tiles reading e,d,c,b,a, then chose Grid > Fixed 2x2
  and confirmed the close. The grid reads E,D / C,B (`packed.png`): the
  reading-first tiles keep the top-left places and plane-a, read last, was
  closed. `plane-e.mp4` is a byte-copy of plane-a, so its burnt-in frame
  reads A while the cell label names plane-e. `saved.gpls` holds plane-e,d,c,b
  in that order with `settings.grid_state` 2x2 fixed and no lock key,
  `vlc.log` has the five launch input-creation lines and five VideoToolbox
  decode lines, teardown finished, and no `gridplayer`/`poc-hw` process
  remained. The packing without overflow, the Mosaic branch and the tile/lock
  clearing are unit evidence, not this run
  (`TestFixedEntryPacksTiledOrderWithoutOverflow`,
  `TestFixedEntryClearsTilesAndLocks`,
  `TestFixedEntryClearsDefaultTilesAndLocks`).
- 2026-11-25, F5 tiled-close-order candidate (`5973e25`, main `92c744d`):
  `go vet ./...`, `go test ./... -race -count=1`, `go build ./...`,
  `scripts/build_app.sh`, and `scripts/smoke_app.sh` PASS with 4
  VideoToolbox lines. A bundled-app run at `/tmp/gridplayer-closeorder2/`
  loaded five videos in Free Tiles arranged so the frozen reading order was
  e,d,c,b,a while playlist order was a,b,c,d,e, then chose Grid > Fixed 2x2.
  The confirm showed "Close 1 video?" (`dialog.png`) and Close Videos kept
  plane-b..e, closing plane-a — the spatially last tile and the active cell —
  and left plane-b active (`closed.png`).
  `saved.gpls` holds those four with `settings.grid_state` 2x2 fixed,
  `vlc.log` has the five decode lines from launch, teardown finished, and no
  `gridplayer`/`poc-hw` process remained. The reading order and the kept set
  are unit evidence (`TestReadInRowsReadsTilesRowByRow`,
  `TestOverflowCloseFollowsTheTiledReadingOrder`).
- 2026-11-25, F5 overflow-close candidate (`1fc9f1b`, main `2bfdda8`):
  `go vet ./...`, `go test ./... -race -count=1`, `go build ./...`,
  `scripts/build_app.sh`, and `scripts/smoke_app.sh` PASS with 4
  VideoToolbox lines. A bundled-app run with five generated fixtures at
  `/tmp/gridplayer-overflow2/` chose Grid > Fixed 2x2 and showed the confirm
  with Cancel as the default button and Close Videos marked destructive
  (`dialog.png`: "Close 1 video?", "This grid holds 4 videos."); Cancel kept
  all five panes (`cancelled.png`). Templates > 1 + 2 then asked
  "Close 2 videos?" / "This grid holds 3 videos." (`template-dialog.png`),
  and Close Videos left the three-cell template layout
  (`template-closed.png`). `saved.gpls` holds plane-a..c with
  `settings.grid_state` 2x3 fixed and `grid_template` "1 + 2", `vlc.log`
  carries the five decode lines from launch, teardown finished, and
  no `gridplayer`/`poc-hw` process remained. The dialog text is the
  screenshot above; the ask's count and capacity, the cancel path, the
  kept-cell identity and the non-fixed guard are unit evidence
  (`TestFixedOverflowAsksBeforeClosing`, `TestOverflowCount`,
  `TestGridDraftOverflowCancelKeepsLayout`, `TestNonFixedLayoutsNeverAsk`).
- 2026-11-25, F5 show-all-cells candidate (`865e723`, main `38a54a0`):
  `go vet ./...`, `go test ./... -race -count=1`, `go build ./...`,
  `scripts/build_app.sh`, and `scripts/smoke_app.sh` PASS with 4
  VideoToolbox lines. A bundled-app run with two generated fixtures and a
  saved 3x3 fixed `grid_state` at `/tmp/gridplayer-showall6/` started as two
  full-window panes; choosing Grid > Show All Cells redrew them as two
  one-third cells in the top row and seven empty cells, each with the
  frozen empty-cell plus in its middle (`show-all-cells.png`). The saved playlist kept
  `grid_state.preallocate` true, `settings.ini` kept
  `grid_preallocate=true`, both cells played, autoquit exited cleanly, and
  no `gridplayer`/`poc-hw` process remained. Only the toggle-on direction
  and the save are exercised in the app; the toggle-off path, the disabled
  state outside Fixed, the settings write and the carry across presets are
  unit evidence (`TestShowAllCellsTogglesInFixedMode`,
  `TestShowAllCellsTurnsOffCleanly`, `TestFixedPreallocateShowsEveryCell`,
  `TestEmptyCellsFollowTheLayout`).
- 2026-11-25, F5 fixed-cells candidate (`2eca6f0`, main `5b43599`):
  `go vet ./...`, `go test ./... -race -count=1`, `go build ./...`,
  `scripts/build_app.sh`, and `scripts/smoke_app.sh` PASS with 4
  VideoToolbox lines. A bundled-app run with two generated
  fixtures and a saved 3x3 fixed `grid_state` at `/tmp/gridplayer-fixed2/`
  showed two large panes filling the window (`2in3x3.png`) instead of a 3x3
  grid with seven empty cells; both cells were playing at teardown
  (7158/7410 ms), the saved document kept `grid_state` 3x3, autoquit exited
  cleanly, and no `gridplayer`/`poc-hw` process remained.
  `TestFixedShowsOnlyUsedCells` asserts the extent math and the layout rects.
  Overflow (more videos than cells) is not covered by this run and still
  grows the rows.
- 2026-11-25, F5 flush-cells candidate (`4154c86`, main `3395ab1`):
  `go vet ./...`, `go test ./... -race -count=1`, `go build ./...`,
  `scripts/build_app.sh`, and `scripts/smoke_app.sh` PASS with 4
  VideoToolbox lines. A bundled-app run with four generated fixtures and
  isolated data at `/tmp/gridplayer-flush/` shows the panes meeting edge to
  edge (`4pane.png`); all four cells were still playing at teardown
  (7410/7158/7402/7410 ms), autoquit exited cleanly, and no
  `gridplayer`/`poc-hw` process remained. `TestCellsMeetWithoutGap` asserts
  the flush geometry for the grid, Free/Mosaic tile rects, Single Mode and a
  built-in template. This is one screenshot of one layout, not the full
  occlusion/Retina matrix.

- Linux CI (`scripts/ci.sh`) builds every package with `libvlc-dev` and runs
  platform-independent tests; it is not GUI acceptance.

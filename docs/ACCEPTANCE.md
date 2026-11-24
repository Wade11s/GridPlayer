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
- 2026-11-24, F12 chapter-stops candidate (`d0311ac`, main `6ee09d7`):
  `go vet ./...`, `go test ./... -race -count=1`, `go build ./...`,
  `scripts/build_app.sh`, and `scripts/smoke_app.sh` PASS with 4
  VideoToolbox lines. A bundled-app run with isolated data at
  `/tmp/gridplayer-f12-chapters2/` used a pause end action and pressed Next
  Chapter on the last chapter; it saved `playback_state=idle`, and the VLC
  log has no seek after chapter Three, so it did not wrap to chapter One.
  VideoToolbox, clean exit, no residual. Stop selection, the 250 ms tolerance
  and the 3 s Previous rule are unit evidence (`TestChapterStopSelection`,
  `TestChapterNextPastLastRunsEndAction`, `TestChapterJumpSeeksInsideSegment`).
- Linux CI (`scripts/ci.sh`) builds every package with `libvlc-dev` and runs
  platform-independent tests; it is not GUI acceptance.

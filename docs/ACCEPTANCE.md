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
- Linux CI (`scripts/ci.sh`) builds every package with `libvlc-dev` and runs
  platform-independent tests; it is not GUI acceptance.

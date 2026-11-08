# Migration Checkpoint — 2026-11-08 15:12 CST

## Pinned inputs (frozen, spec §2.2)
- Upstream vzhd1701/gridplayer HEAD: bf943684232d74ed9e271c3c9eeef6e7a7ed6e0f (2026-11-03T19:03:42Z), resolved 2026-11-08 14:09 CST (UTC+8). Source: read-only clone /private/tmp/gridplayer-upstream @detached HEAD. Upstream version 0.5.5.
- MyGo: 46f8473c5753519a951e65c38c0b53872fdf2a7a (v0.8.3-26-g46f8473c), go 1.27.3, MIT. Locked via go.mod v0.8.4-0.20261108060507-46f8473c5753.
- Target: Wade11s/GridPlayer (empty, no branches/protection yet). Oracle: /Applications/GridPlayer.app 0.1.0 (com.wade11.gridplayer ≠ upstream 0.5.5/com.vzhd1701.gridplayer) — mismatch recorded in docs/BEHAVIOR_MATRIX.md.
- libVLC: 3.0.24 Vetinari (/Applications/VLC.app, Cask), videotoolbox plugin present. macOS 27.2 arm64, clang 21, ffmpeg 9.0.2, Codex CLI 0.167.0. gh auth restored (Wade11s, keyring, scopes gist/read:org/repo/workflow).

## Progress
- §3 matrix: docs/BEHAVIOR_MATRIX.md (F1–F13, macOS HW_SP mapping, fixture + oracle duty). AGENTS.md index not yet created.
- §4.1 PoC-A (isolated AppKit 4×NSView + set_nsobject): PASS — state=3, 640×360, seek 3403ms after SetTime(3000), VideoToolbox ×4 logged; screenshots in testdata/poc-hw-evidence/ (frame 0042→0061).
- §4.1 PoC-C (MyGo window + 4 HW panes via internal/nativehost + Painter.Hole): PASS — badges advancing, frame number 0086→0116 in 1s window (continuous motion), 4 panes correct identities, teardown clean, VideoToolbox ×4. Screenshot: testdata/gridplayer-4pane.png. Callback path (Attempt B) uses avcodec SW → rejected for HW (docs/ARCHITECTURE.md).
- Bootstrap docs: docs/ACCEPTANCE.md (frozen criteria), docs/MERGE_GATES.md, .github/workflows/ci.yml (ubuntu free tier), fixture manifest in testdata/fixtures/manifest.json.
- Local re-acceptance on HEAD 1352559 (2026-11-08 ~15:25 CST): poc-hw PASS (seek 3409ms, state=3 ×4), gridplayer 4 panes state=3, screenshot testdata/gridplayer-4pane.png, VideoToolbox ×4. Push + independent review (escalated codex exec / git push) timed out in auto-approval review 3×; awaiting user approval in client — see chat report.
- Next: commit bootstrap (branch bootstrap/poc-vlc-mygo), Issue, PR, independent review (codex exec fresh context), local GUI re-run on head, merge per gates.

## Blockers
- None currently. Standing risks: CI on first PR cannot report (empty base, no required checks — see MERGE_GATES bootstrap exception); full-matrix implementation remains (engine, grid UI, persistence, .app packaging).

No scope, acceptance, or gate lowered. No public release, no notarization, no Gatekeeper change.

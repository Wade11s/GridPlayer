# GridPlayer Behavior Matrix (frozen source scope)

Source: vzhd1701/gridplayer @ `bf943684232d74ed9e271c3c9eeef6e7a7ed6e0f` (master HEAD resolved 2026-11-08 14:09 CST, UTC+8).
Oracle installed: `/Applications/GridPlayer.app` — `0.1.0` (build `1`), identifier `com.wade11.gridplayer`.
**Mismatch recorded:** oracle `0.1.0` != frozen source `0.5.5` (`gridplayer/version.py`: `__version__ = "0.5.5"`, `__app_id__ = "com.vzhd1701.gridplayer"`). Per MIGRATION_SPEC §3, the frozen source defines the target scope; the oracle is a runnable reference only. Items that cannot be reconciled from the oracle stay blocked, not silently re-scoped.

Platform scope: macOS/arm64 only (PRIMARY_PLATFORM). Windows/Linux/Intel Mac out of first-phase acceptance.
macOS driver mapping (from source, verified):
- `player/managers/video_driver.py`: `VLC_HW` -> forced to `VLC_HW_SP` on macOS (multi-process hardware impossible).
- `widgets/video_frame_vlc_hw_sp.py`: `PlayerProcessSingleVLCHWSP` (QThread + `VlcPlayerBase`), `init_player` calls `_media_player.set_nsobject(win_id)` on macOS; `ui_video_surface()` wraps a QWidget in `QMacCocoaViewContainer(0, video_clip)`; `driver_setup` passes `int(video_surface.winId())`; `load_video_finish` does an explicit +1 resize (macOS quirk); unload/cleanup releases from the player thread, not the GUI thread.
- `vlc_player/instance.py` (`InstanceVLC.init_options`): `--quiet --no-disable-screensaver --no-sub-autodetect-file --no-lua --no-osd --no-snapshot-preview --no-interact --no-keyboard-events --no-mouse-events` + per-video options + subtitle style + settings options; plugin cache (`--no-plugins-scan`) when present.
- `vlc_player/player_base.py`: loop via `--input-repeat` passes (INPUT_REPEAT_PASSES=10000), dead-input detection, slave A/V attach, hardware-decode-keep-alive notes.

Decoders/output: `VideoDriver` enum `{VLC_SW, VLC_SW_SP, VLC_HW, VLC_HW_SP, DUMMY}` (`params/static.py`). SW modes use shared-memory/callback frames; HW modes use native surfaces. Hardware decode evidence must come from libVLC (VideoToolbox `libvideotoolbox_plugin.dylib` present in VLC 3.0.24), never from AVFoundation substitution.

## Feature -> source -> expected -> target test -> evidence (summary)

| # | Feature (verified in source) | Source | Expected (macOS arm64) | Target test | Evidence required |
|---|---|---|---|---|---|
| F1 | Play 1->4+ videos side-by-side, continuous motion | widgets/video_frame_vlc_hw_sp.py, player/manager.py | N panes decode + play concurrently; no black frames, no cross-talk | PoC 1-4 headless + GUI | ffmpeg-fps / frame counter, screenshot motion diff, CPU/GPU mem log |
| F2 | Play/pause, seek (ms), rate, loop file/fragment | widgets/video_block*.py, vlc_player/player_base.py | Seek lands within fixture threshold; loop wraps without restart | Engine unit + GUI seek test | time before/after, loop counter |
| F3 | Volume/mute, audio track, subtitles, external A/V beside file | video_block_audio/subtitles.py, player_base TracksManager | Audible tone fixture; track switches reload | GUI audio level + track switch | engine state + sample-level tone check (no "log says" only) |
| F4 | Aspect/scale/zoom/crop/transform/anchor/shift | video_frame_vlc_base.py + utils/aspect_calc.py | Picture geometry matches calc (crop borders accounted) | Geom unit tests + resize GUI | geometry numbers + screenshot |
| F5 | Grid layout: modes (auto rows, fit, match shapes), lines, templates, free tiles, single mode | models/grid_state.py, player/managers/grid*.py, widgets/grid_editor.py | Layout math preserved; UI restyled for macOS but semantics kept | Layout unit + GUI resize/occlusion/Retina | DIPs vs pixels at scale 2, occlusion test |
| F6 | Drag-n-drop swap, move, add | player/managers/drag_n_drop.py, layout_drag.py | Swap semantics identical | GUI drag test | pane contents before/after |
| F7 | Playlists: m3u/pls/xspf, folders, .gpls save/load, per-video settings, snapshots/bookmarks/lists | models/playlist.py, video.py, media_list.py | .gpls v1 round-trip; old import only on copies, never real data | Persistence unit (temp dir) | JSON round-trip diff |
| F8 | Streaming URLs via streamlink/yt-dlp, cookies, proxy/network, quality | vlc_player/static.py, models/stream.py, utils network | Out of PoC; LATER phase — local HLS fixture only first | Local-only HLS serve test | reserved; no silent cut |
| F9 | Settings + persisted paths (settings.ini, cookies.txt, data dir) | settings.py, params/settings_*.py | macOS data dir `~/Library/Application Support/...` equivalent (Go app name); import only test copies | Settings unit (temp dir) | ini round-trip |
| F10 | Shortcuts (Space, B/N, PgUp/Dn, 0-9, T/R/S/D, arrows, , . /, [/], C/X/Z, +/-/*, K/J/H/G, F4/F5, Ctrl+Space etc. per params/actions.py) | params/actions.py | Same keys where macOS allows; Cmd vs Ctrl mapping documented | Keymap unit + GUI key test | dispatched action log |
| F11 | Hardware vs software driver setting, videos-per-process, screensaver/tray/close behaviors | settings.py player/*, widgets/video_frame_vlc_*.py | macOS exposes HW_SP truthfully; no fake HW claim | Settings + HW evidence | libVLC decode counters, not UI label |
| F12 | Chapters, bookmarks, sponsorblock, screenshots, recovery | widgets/video_block_*.py, models/*.py | Later phase; fixtures for chapters first | Chapter seek test | chapter times |
| F13 | i18n (Crowdin .ts/.qm), icons/fonts | resources/, params/languages.py | Polish/English first; translations bundled, not hand-edited sources | Resource presence | bundled files list |

macOS F11 note: screensaver inhibition uses `/usr/bin/caffeinate -d -i -w <app pid>`
rather than an in-process IOKit assertion; the `-w` makes the child exit with the
app. Tray/menu-bar and close-to-tray behaviors remain out of scope on macOS.

## Non-goals / prohibitions (from spec)

- No AVFoundation/`ui/video` substitution for libVLC; no external VLC window, fake frames, or browser player.
- No reinterpretation of "side-by-side playback" as frame-locked sync; no invented sync semantics.
- No reading user real config/media or private browser cookies; network/cookies only via explicit user file.
- No performance promises from test档位; report measured fps/drop/CPU/GPU/mem vs frozen baseline only.

## Fixtures (frozen before implementation where relevant)

`scripts/make_fixtures.py` generates non-private reproducible media (testsrc + burned frame-number/timecode + sine tone). Manifest records hash. Upstream `tests/real_app/media.py` recipe is the reference (folder/a,b,c 20s, feature.mkv 60s chapters, HLS 3 sizes); ours is a smaller deterministic subset for PoC (1-4 planes). Full-matrix fixtures extend in later phases; thresholds frozen with the baseline runner-up oracle comparison.

## Oracle baseline duty

Run the installed oracle (`0.1.0`) on the same fixtures in an isolated data dir, freeze visual/performance/seek thresholds BEFORE the matching target implementation. If a threshold depends on the oracle being `0.5.5` (which it is not), record as blocked/unverifiable, do not relax.

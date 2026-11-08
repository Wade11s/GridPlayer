# Target Architecture (Go + MyGo, macOS/arm64)

Pinned inputs:
- Upstream source: `bf943684232d74ed9e271c3c9eeef6e7a7ed6e0f` (2026-11-03T19:03:42Z upstream, resolved 2026-11-08 14:09 CST).
- MyGo: `github.com/egoist/mygo` @ `46f8473c5753519a951e65c38c0b53872fdf2a7a` (`v0.8.3-26-g46f8473c`, `Version 0.8.3`, go1.27.3, PUREGO/no-cgo core). Locked; no auto-follow.
- libVLC: VLC `3.0.24 Vetinari` from `/Applications/VLC.app` (`lib/libvlc.dylib` + `libvlccore.dylib` @rpath, plugins in `plugins/`, headers in `include/vlc/`). Homebrew Cask `vlc 3.0.24`. HW plugin `libvideotoolbox_plugin.dylib` present.
- System: macOS 27.2 (26B123) arm64, Go 1.27.3, clang 21, ffmpeg 9.0.2, Codex CLI 0.167.0.

## Dependency DAG

```
MyGo (purego, CGO_ENABLED=0 core)          <- window, menus, lifecycle, ui (Metal)
  ^ (app links mygo; app itself MAY use cgo for libVLC -- spec §4.1)
libVLC (cgo)  [internal/vlcmac]
  ^ 
MediaEngine [internal/engine]  session/generation, bounded callback queue, main-thread posting
  ^
VideoSurface [internal/surface? + ui grid view]  Retina-aware layout, clip, overlay, resize
  ^
App shell [cmd/gridplayer]  MyGo App, menus, shortcuts, settings, persistence
Tests: engine unit / integration (real libVLC) / MyGo GUI (MYGO_AUTOMATION) / .app smoke
```

MyGo core forbids cgo, but that constrains MyGo itself, not an app linking it. Our app builds with `CGO_ENABLED=1` for the `vlcmac` package only; `mygo` and `ui` remain purego. Cross-compile for non-darwin is out of scope (first phase).

## Why libVLC native surface is the primary path

Spec §4.1 demands: MyGo window carries 1->4 libVLC dynamic video, real audio, motion, play/pause/seek/resize/close/release, and on macOS first proves the same-process native hardware output path. It explicitly allows cgo/libVLC and forbids external VLC windows, fake frames, browser players, and masking HW failure with software decode.

MyGo survey (46f8473c) findings:
- `ui/video` decodes with AVFoundation (hardware via VideoToolbox, frames as Metal textures zero-copy). This is a framework substitution for libVLC: loses libVLC format coverage (e.g. MKV chapters, SRT, sponsorblock inputs, streamlink handoff). REJECTED as the engine; never claimed as libVLC evidence.
- `ui.VideoPlayer` over the public `ui.VideoDecoder` (`Next()/Rewind()` only) has no Seek/Play/Pause/Rate/Volume/Track forwarding; libVLC keeps its own clock => drift and wrong seek landing. REJECTED as the playback core. A raw `ui.VideoDecoder` is still useful for fixture decoding (no audio) but not for the engine.
- MyGo public API exposes no generic native-view hole (only `ui.WebView` for WKWebView). `Content` (internal/surface.Conn) is internal-only; apps cannot implement it. Hosting foreign NSViews inside a MyGo `Content` window therefore needs either a MyGo fork (extend the webview-hole machinery to generic native views) or separate AppKit windows. Both are tracked as substantive attempts below.
- `Bitmap.Update` + `Painter`/`Draw` on the public API can show libVLC callback frames as textures with correct Retina/clip. libVLC docs warn callback rendering disables HW decode or forces a slow copy and blends subs on CPU — it cannot prove HW.

## Substantive attempts (max 3, per spec)

1. **A — isolated HW proof (no MyGo).** Minimal AppKit window + NSView + `libvlc_media_player_set_nsobject` (purego, mirrors Qt `QMacCocoaViewContainer` + `set_nsobject` in `video_frame_vlc_hw_sp.py`). 1->4 in one window, verify hardware decode evidence (libVLC stats, codec path, CPU/GPU), stop/unbind/detach/release order. Proves the same-process HW path exists on this machine. If the macosx vout needs a `VLCOpenGLVideoViewEmbedding` view, create a small custom embedding view (addVoutSubview:/removeVoutSubview:) as variant A2.
2. **B — MyGo callback textures (fallback, HW caveat).** libVLC `libvlc_video_set_callbacks` (RV32) -> bounded queue -> `Bitmap.Update` in the MyGo grid. Shipped only with honest decode-mode evidence; if decode is software, it NEVER satisfies the HW requirement and the migration blocks rather than silently downgrades.
3. **C — MyGo fork with native-view holes (preferred long-term if A succeeds).** Minimal patch reusing `PlaceWebViews` machinery for generic native views + a public `ui` element. Fork is pinned (replace directive + commit recorded); upstream MyGo stays locked at 46f8473c and the fork diff is reviewable. Used only if A proves HW and B alone cannot satisfy HW.

Failed genuine HW attempt after 3 substantive tries => blocked, reported, no scope cut.

## Boundaries & guarantees (design contract)

- `internal/vlcmac`: cgo wrapper only (instance/media/player/callbacks/events/tracks). No UI, no goroutine policy. All entry points callable from any thread; internal libVLC calls serialized per player where libVLC requires it.
- `internal/engine`: session/generation fencing. Every callback carries a generation; stale events are dropped. Bounded channel (e.g. depth 8 frames) between libVLC thread and UI; overflow drops oldest, never blocks libVLC. `stop -> unbind -> detach -> release` is an explicit, tested sequence (mirrors Qt teardown, including "release from player side, not GUI thread").
- Main-thread rule: UI mutation only via `mygo.RunOnMain`/`postMain` (loop.go) or inside view builders (`ui.Context` on the main thread). Callback goroutines never touch `ui` directly.
- Resolution/Retina: MyGo `Surface.Size()` returns DIPs + scale; the layout works in DIPs, VideoSurface converts to device pixels for libVLC/`Bitmap`. Occlusion honors `OccludableSurface` (hold muted/background work).
- VLC SDK discovery: prefer `$VLC_APP_PATH`, else `/Applications/VLC.app`; require `libvlc.dylib`, `libvlccore.dylib`, `plugins/`, `include/vlc/`. Support a bundled-copy layout for the `.app` (Frameworks + dylib @rpath + plugin path). No sudo, no Gatekeeper changes.

## CI / gate design (§4.2, §5)

- Local: `go vet`, unit tests (incl. `-race` where applicable), vendored fixture generation (deterministic hash manifest), `mygo build` .app smoke without dev shell env.
- GitHub free tier only: required checks must be real (Go build/vet/test on ubuntu for non-GUI code; NO fake status). GUI/macOS acceptance runs locally (Wade's Mac) and is bound to exact SHAs in PR evidence; it is never claimed as a hosted check.
- Independent review = fresh Codex context (`codex exec` with spec + frozen baseline + exact diff + repro entry), output a SHA-bound report. Writer never self-reviews. No reliable entry => fail closed (no merge).
- Branch protection: check what the repo plan supports; if required reviews/checks cannot be enforced, merge waits (report limit), never replaced by local green.
- Bootstrap PR itself goes through the same review + local acceptance + checks; "no checks yet" does not bypass gates.

## Risks

- MyGo fork maintenance (Attempt C) — keep the patch tiny, version it, and prefer a upstream-able design (generic native views mirroring webviews).
- Threading bugs (libVLC callbacks vs MyGo main thread) — generation fencing + bounded queue + teardown tests.
- Plugin discovery for bundled libVLC — set `--plugin-path`/`VLC_PLUGIN_PATH` explicitly; test `.app` without `DYLD_*`.
- GUI test permissions (Screen Recording for capture, Accessibility for automation) — use MyGo automation socket + fixture frame/analyzer expectations first; request capture permission via OS flow only if needed; never fabricate capability.

# Building and running the local .app

## Prerequisites (this Mac)

- macOS arm64, Go 1.27.3, Xcode command line tools.
- VLC 3.0.24 at `/Applications/VLC.app` (Homebrew Cask or the official app)
  to build. `scripts/build_app.sh` copies `libvlc`/`libvlccore` and the
  plugins into the bundle, rewrites the rpath to
  `@executable_path/../Frameworks`, and re-signs. The built app does not need
  VLC installed at runtime. Override the build source with
  `VLC_APP=/path/to/VLC.app`.
- No sudo, no Gatekeeper change, no notarization. The bundle is ad-hoc
  signed by `mygo build`; it runs with the machine's existing settings.

## Build

```sh
./scripts/build_app.sh
```

Outputs:

- `dist/darwin-arm64/GridPlayer.app`
- `dist/darwin-arm64/gridplayer-0.1.0-arm64.dmg`

The app bundle contains `Contents/Frameworks/libvlc.dylib`,
`Contents/Frameworks/libvlccore.dylib`, and
`Contents/Resources/vlc/plugins` (~90 MB total).

The CLI is pinned in `go.mod` as `tool github.com/egoist/mygo/cmd/mygo`.

## Smoke test (Finder-equivalent launch)

```sh
./scripts/smoke_app.sh
```

It copies the app to `/tmp` (outside the TCC-protected Desktop), checks the
bundled libs/plugins and the `@executable_path/../Frameworks` rpath, opens
four fixtures with `open`, waits for the app to quit, and requires four
VideoToolbox decode lines in the log. A screenshot is taken by the caller
when a GUI session is available.

## Notes

- The default `--files` is `testdata/fixtures/*.mp4` relative to the
  process working directory. From Finder, pass absolute paths:
  `open -a GridPlayer.app --args --files /abs/a.mp4,/abs/b.mp4`.
- VLC's libraries/plugins are bundled; the build still needs the installed
  VLC SDK, but the app does not.
- The app is a local development build, not a public release.

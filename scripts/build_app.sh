#!/bin/bash
# Build the local macOS .app with the pinned MyGo CLI and bundle libVLC.
set -euo pipefail
cd "$(dirname "$0")/.."

VLC_APP="${VLC_APP:-/Applications/VLC.app}"
BASE="$VLC_APP/Contents/MacOS"
export CGO_ENABLED=1
export CGO_CFLAGS="-I$BASE/include"
export CGO_LDFLAGS="-L$BASE/lib -Wl,-rpath,$BASE/lib"

go tool mygo build -skip-dmg "$@"

APP="dist/darwin-arm64/GridPlayer.app"
[ -d "$APP" ] || { echo "missing $APP" >&2; exit 1; }
FRAMEWORKS="$APP/Contents/Frameworks"
PLUGINS="$APP/Contents/Resources/vlc/plugins"
BIN="$APP/Contents/MacOS/GridPlayer"

rm -rf "$FRAMEWORKS" "$PLUGINS"
mkdir -p "$FRAMEWORKS" "$PLUGINS"
ditto "$BASE/lib" "$FRAMEWORKS"
ditto "$BASE/plugins" "$PLUGINS"

# The app must resolve libvlc from its own bundle, not the build machine's
# /Applications/VLC.app. The dylibs use @rpath install names, so one rpath
# covers both libvlc and libvlccore.
install_name_tool -delete_rpath "$BASE/lib" "$BIN" 2>/dev/null || true
install_name_tool -add_rpath "@executable_path/../Frameworks" "$BIN" 2>/dev/null || true

codesign --force --deep --sign - "$APP"

DMG="dist/darwin-arm64/gridplayer-0.1.0-arm64.dmg"
rm -f "$DMG"
if hdiutil create -volname "GridPlayer" -srcfolder "$APP" -ov -format UDZO "$DMG" >/dev/null; then
  echo "wrote $DMG"
else
  echo "warning: DMG creation failed; the .app bundle is ready" >&2
fi

echo "bundled VLC into $APP"

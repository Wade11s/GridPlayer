#!/bin/bash
# Launch the built .app from /tmp with four fixtures, then verify decode/exit.
set -euo pipefail
cd "$(dirname "$0")/.."

APP="dist/darwin-arm64/GridPlayer.app"
[ -d "$APP" ] || { echo "missing $APP; run ./scripts/build_app.sh" >&2; exit 1; }

SMOKE=/tmp/GridPlayer-smoke.app
FIX=/tmp/gridplayer-smoke-fixtures
LOG=/tmp/gridplayer-smoke-vlc.log
rm -rf "$SMOKE" "$FIX"
mkdir -p "$FIX"
cp -R "$APP" "$SMOKE"
cp testdata/fixtures/plane-a.mp4 testdata/fixtures/plane-b.mp4 \
   testdata/fixtures/plane-c.mp4 testdata/fixtures/plane-d.mp4 "$FIX/"

open -n -W -a "$SMOKE" --args \
  --files "$FIX/plane-a.mp4,$FIX/plane-b.mp4,$FIX/plane-c.mp4,$FIX/plane-d.mp4" \
  --autoquit 8 --log "$LOG"

hw=$(grep -c "Using Video Toolbox" "$LOG" || true)
echo "VideoToolbox decode lines: $hw"
[ "$hw" -ge 4 ] || { echo "FAIL: expected 4 hardware decode lines" >&2; exit 1; }
echo "app smoke: PASS"

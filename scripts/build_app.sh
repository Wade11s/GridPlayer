#!/bin/bash
# Build the local macOS .app with the pinned MyGo CLI and libVLC SDK.
set -euo pipefail
cd "$(dirname "$0")/.."

VLC_APP="${VLC_APP:-/Applications/VLC.app}"
BASE="$VLC_APP/Contents/MacOS"
export CGO_ENABLED=1
export CGO_CFLAGS="-I$BASE/include"
export CGO_LDFLAGS="-L$BASE/lib -Wl,-rpath,$BASE/lib"

go tool mygo build "$@"

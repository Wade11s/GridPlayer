#!/bin/bash
# Builds gridplayer-go with libVLC SDK discovery (macOS/arm64 only, first phase).
set -euo pipefail
cd "$(dirname "$0")/.."

VLC_APP="${VLC_APP:-/Applications/VLC.app}"
BASE="$VLC_APP/Contents/MacOS"
export CGO_ENABLED=1
export CGO_CFLAGS="-I$BASE/include"
export CGO_LDFLAGS="-L$BASE/lib -Wl,-rpath,$BASE/lib"

cmd="${1:-all}"
mkdir -p bin
case "$cmd" in
  poc-hw) go build -o bin/poc-hw ./cmd/poc-hw ;;
  cb) go build -o bin/poc-cb ./cmd/poc-cb ;;
  gridplayer) go build -o bin/gridplayer ./cmd/gridplayer ;;
  all) go build -o bin/poc-hw ./cmd/poc-hw ;;
  *) echo "unknown target: $cmd"; exit 1 ;;
esac
echo "built bin/$cmd"

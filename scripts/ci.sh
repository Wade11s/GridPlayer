#!/bin/bash
# CI checks that do not need a Mac desktop: Linux + libvlc-dev builds every
# package, including the darwin stubs, and runs the platform-independent tests.
set -euo pipefail
cd "$(dirname "$0")/.."

export CGO_ENABLED=1
export CGO_CFLAGS="$(pkg-config --cflags libvlc)"
export CGO_LDFLAGS="$(pkg-config --libs libvlc)"

go vet ./...
go test ./...
go build ./...

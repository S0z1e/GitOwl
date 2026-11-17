#!/bin/sh
# Builds GitOwl's macOS app bundle and disk image into dist/darwin-<arch>/.
# The bundle is ad-hoc signed by MyGo and not notarized; see README.md.
set -eu
cd "$(dirname "$0")/.."
arch="${GITOWL_ARCH:-$(go env GOARCH)}"
go tool mygo generate >/dev/null
go tool mygo build -platform "darwin/${arch}" "$@"
echo "Built dist/darwin-${arch}/"
ls "dist/darwin-${arch}/"

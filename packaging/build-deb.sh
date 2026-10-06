#!/bin/sh
# Builds the headless service for Debian / Raspberry Pi OS and packages it.
# Usage: packaging/build-deb.sh [arch]     arch: arm64 (default), armhf, amd64
# Runs anywhere Go does, including Git Bash on Windows; needs no Debian tools.
set -e
cd "$(dirname "$0")/.."
ARCH=${1:-arm64}
case "$ARCH" in
	arm64) GOARCH=arm64 ;;
	armhf) GOARCH=arm GOARM=6; export GOARM ;;
	amd64) GOARCH=amd64 ;;
	*) echo "unknown arch $ARCH" >&2; exit 1 ;;
esac
VERSION=$(go run ./cmd/broadcastwedged -version)
BIN=build/linux-$ARCH/broadcastwedged
CGO_ENABLED=0 GOOS=linux GOARCH=$GOARCH go build -trimpath -ldflags "-s -w" -o "$BIN" ./cmd/broadcastwedged
mkdir -p build/bin
go run ./packaging/mkdeb -arch "$ARCH" -version "$VERSION" -bin "$BIN" \
	-out "build/bin/broadcastwedge_${VERSION}_${ARCH}.deb"

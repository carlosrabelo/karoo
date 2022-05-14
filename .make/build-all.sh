#!/bin/bash
set -euo pipefail

BINARY_NAME="karoo"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

VERSION="$(cd "${ROOT_DIR}" && git describe --tags --always --dirty 2>/dev/null || echo dev)"
BUILD_TIME="$(date +%Y-%m-%dT%H:%M:%S%z)"
LDFLAGS="-s -w -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME}"

mkdir -p "${ROOT_DIR}/bin"
cd "${ROOT_DIR}"
echo "Building for multiple platforms..."
GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -tags netgo -ldflags="${LDFLAGS}" -o "${ROOT_DIR}/bin/${BINARY_NAME}-linux-amd64"       ./karoo/cmd/karoo
GOOS=linux   GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -tags netgo -ldflags="${LDFLAGS}" -o "${ROOT_DIR}/bin/${BINARY_NAME}-linux-arm64"       ./karoo/cmd/karoo
GOOS=darwin  GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -tags netgo -ldflags="${LDFLAGS}" -o "${ROOT_DIR}/bin/${BINARY_NAME}-darwin-amd64"      ./karoo/cmd/karoo
GOOS=darwin  GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -tags netgo -ldflags="${LDFLAGS}" -o "${ROOT_DIR}/bin/${BINARY_NAME}-darwin-arm64"      ./karoo/cmd/karoo
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -tags netgo -ldflags="${LDFLAGS}" -o "${ROOT_DIR}/bin/${BINARY_NAME}-windows-amd64.exe" ./karoo/cmd/karoo
echo "Multi-platform build completed. Binaries in ${ROOT_DIR}/bin/"

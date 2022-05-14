#!/bin/bash
set -euo pipefail

BINARY_NAME="karoo"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

VERSION="$(cd "${ROOT_DIR}" && git describe --tags --always --dirty 2>/dev/null || echo dev)"
BUILD_TIME="$(date +%Y-%m-%dT%H:%M:%S%z)"
LDFLAGS="-s -w -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME}"

mkdir -p "${ROOT_DIR}/bin"
cd "${ROOT_DIR}"
CGO_ENABLED=0 go build -trimpath -tags netgo -ldflags="${LDFLAGS}" -o "${ROOT_DIR}/bin/${BINARY_NAME}" ./karoo/cmd/karoo
echo "Binary ready at: ${ROOT_DIR}/bin/${BINARY_NAME} (version: ${VERSION})"

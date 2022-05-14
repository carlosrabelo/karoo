#!/bin/bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

mkdir -p "${ROOT_DIR}/bin"
cd "${ROOT_DIR}"
go test -coverprofile="${ROOT_DIR}/bin/coverage.out" ./...
go tool cover -html="${ROOT_DIR}/bin/coverage.out" -o "${ROOT_DIR}/bin/coverage.html"
echo "Coverage report: ${ROOT_DIR}/bin/coverage.html"

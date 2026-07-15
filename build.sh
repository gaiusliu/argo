#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"

# Windows 下 Go 产物自动带 .exe 后缀，用 GOOS 判断
EXT=""
[ "${GOOS:-$(go env GOOS)}" = "windows" ] && EXT=".exe"

echo "Building CLI..."
go build -o "argo${EXT}" ./src/cli/

echo "Building server..."
go build -o "argo-server${EXT}" ./src/argo-server/

echo "Done: argo${EXT} + argo-server${EXT}"

#!/bin/sh
set -eu

cd "$(dirname "$0")"
GOROOT="$(go env GOROOT)"
cp -f "$GOROOT/lib/wasm/wasm_exec.js" ./wasm_exec.js
GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" -o peekd.wasm .
# Optimize with wasm-opt if available (from binaryen)
if command -v wasm-opt >/dev/null 2>&1; then
  wasm-opt -O2 peekd.wasm -o peekd.wasm
fi
echo "Built wasm/peekd.wasm ($(du -h peekd.wasm | cut -f1)). Serve this directory over HTTP and open index.html."

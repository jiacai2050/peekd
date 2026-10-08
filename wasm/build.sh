#!/bin/sh
set -eu

cd "$(dirname "$0")"
GOROOT="$(go env GOROOT)"
cp -f "$GOROOT/lib/wasm/wasm_exec.js" ./wasm_exec.js
GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" -o peekd.wasm .
# Optimize with wasm-opt if available (from binaryen)
if command -v wasm-opt >/dev/null 2>&1; then
  echo "Try optimize with wasm-opt..."
  if wasm-opt -O2 --all-features peekd.wasm -o peekd.opt.wasm; then
    mv peekd.opt.wasm peekd.wasm
  else
    echo "Warning: wasm-opt failed, keeping unoptimized binary."
    rm -f peekd.opt.wasm
  fi
fi
echo "Built wasm/peekd.wasm ($(du -h peekd.wasm | cut -f1)). Serve this directory over HTTP and open index.html."

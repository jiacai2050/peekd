#!/bin/sh
set -eu

cd "$(dirname "$0")"
GOROOT="$(go env GOROOT)"
cp "$GOROOT/lib/wasm/wasm_exec.js" ./wasm_exec.js
GOOS=js GOARCH=wasm go build -trimpath -o peekd.wasm .
echo "Built wasm/peekd.wasm. Serve this directory over HTTP and open index.html."

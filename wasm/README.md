# Peekd Local WASM

This is the browser-local version of Peekd. Files are read through the browser
File API and parsed locally; they are not uploaded to a Peekd server.

Build it with Go 1.26:

```sh
./build.sh
python3 -m http.server 8091
```

Open <http://127.0.0.1:8091/>. Browsers block the WASM worker when this page is
opened directly as `file://`.

The build copies the version-matched `wasm_exec.js` from the Go toolchain and
creates `peekd.wasm`; both generated files are ignored by Git. The browser
entry point owns file selection, directory permissions, media/blob handling,
and the UI. Go WASM owns format detection, text formatting, Markdown/Org
rendering, CSV/TSV parsing, and archive listing.

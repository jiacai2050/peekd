# Peekd Local

Peekd Local runs entirely in your browser. Open files or directories, drag
files onto the page, or enter a remote file URL to preview them without
uploading local files to a server.

Try it online: <https://jiacai2050.github.io/peekd/wasm/>.

Supported previews include text and code, Markdown, Org, JSON, XML, CSV/TSV,
images, audio/video, PDF, and ZIP/TAR archive listings.

## Run locally

Build it with:

```sh
./build.sh
python3 -m http.server 8091
```

Open <http://127.0.0.1:8091/>. The page must be served over HTTP; opening
`index.html` directly as `file://` will not start the WASM worker.

Remote URLs must be accessible through browser CORS. Files selected from your
computer stay in the browser and are processed locally.

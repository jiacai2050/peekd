# HTTP Caching

Peekd uses revalidation rather than long-lived strong caching. Browsers may
keep responses locally, but they must ask Peekd whether the resource changed
before using a cached response.

## Response headers

For directory pages, generated previews, and direct files, Peekd sends:

```http
Cache-Control: no-cache
Last-Modified: <file modification time>
ETag: W/"<version>-<request path>-<modification time>"
```

`no-cache` does not prevent the browser from storing a response. It requires
the browser to revalidate the response before reusing it.

The file ETag is a weak validator based on the application version, request
path, and modification time. The request path ensures that switching the served
directory never produces identical ETags for different resources. Modification
time avoids reading and hashing the complete file on every request, which is
important for large files.

Embedded assets use an ETag based on the application version and request path.
They do not have a useful filesystem modification time because they are loaded
from the embedded filesystem.

## Conditional requests

On a later request, the browser may send the validator from its cache:

```http
If-None-Match: W/"e-18f2a6c4b3d"
If-Modified-Since: Tue, 15 Sep 2026 11:10:25 GMT
```

Peekd checks `If-None-Match` first. If the ETag matches, it returns:

```http
HTTP/1.1 304 Not Modified
```

The response has no body, so the browser uses its cached content.

`If-Modified-Since` is only used when the client does not send `If-None-Match`.
When both headers are present, ETag alone decides. This prevents a stale
`If-Modified-Since` from incorrectly returning `304` for a different resource
that happens to have an older modification time (for example, after restarting
the server with a different root directory).

The `Last-Modified` value uses HTTP's second-level timestamp precision. ETags
provide finer-grained validation with the added request-path component, but
they still cannot detect a change when a file's content changes while its
modification time is preserved exactly.

## Direct files and previews

Direct file responses continue to use Go's `http.FileServer`, preserving range
requests and efficient file transfers. Peekd adds the same cache headers and
conditional validation before handing the response to `http.FileServer`.

Directory pages and generated previews use the modification time of the
directory or source file. This avoids regenerating unchanged Markdown, JSON,
CSV/TSV, text, and archive previews after a successful revalidation.

## Design trade-off

Peekd intentionally does not calculate a full content hash for every file.
Hashing would provide stronger change detection, but would add file I/O and
latency to ordinary browsing, especially for large files. The current
path-plus-modification-time validator is a practical compromise for a local
file server.

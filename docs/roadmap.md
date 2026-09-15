# Peekd Roadmap

These are practical follow-up features identified for the file server. They
are intentionally separate from the current preview implementation.

## High priority

### Graceful shutdown

Handle process signals with `http.Server.Shutdown` so active requests can
finish before the listener closes.

### HTTPS

Add optional `-tls-cert` and `-tls-key` settings and use `ServeTLS`. This is
important when Basic Auth is used outside a trusted network because Basic Auth
only encodes credentials with Base64.

### Archive preview limits

Bound archive entry counts, metadata scanning, and compressed input processing.
Reject or mark unsafe absolute and `../` entry paths without extracting files.

### HTTP caching

Implemented in [`caching.md`](caching.md). Peekd uses `no-cache`
revalidation with `Last-Modified` and ETag validators for files, directories,
generated previews, and embedded assets. A future enhancement could add
content-hash ETags for applications that need detection when both file size and
modification time are preserved.

## Medium priority

### Directory search and filtering

Add client-side filtering for the current directory, with server-side search
as a later option for large trees.

### Breadcrumb navigation

Show the current directory path as links on directory and preview pages.

### Copy actions

Provide buttons to copy a file URL and local path, using the Clipboard API
with a browser-compatible fallback.

### XML preview

Add a bounded, safe XML text or tree preview. Disable external entities and
network access, and never render embedded HTML.

### Health check

Add a small `/healthz` endpoint for reverse proxies, service managers, and
container probes.

## Lower priority

### Structured access logs

Offer an optional JSON access-log mode while preserving the current
human-readable format.

# Go `sendfile` Optimization

## How It Works

Go's `net.TCPConn` implements the `io.ReaderFrom` interface. When a file is
transferred with `io.Copy(conn, file)`:

1. `io.Copy` first checks whether the source implements `io.WriterTo`.
2. Otherwise, it checks whether the destination implements `io.ReaderFrom`.
3. The optimized `net.TCPConn` path is selected.
4. On Linux, Go checks whether the source is a regular disk file represented by
   `*os.File`.
5. If the conditions are met, Go calls the `sendfile` system call.

This allows the operating system to transfer file data directly from the file
descriptor to the socket without copying it through a user-space buffer.
`http.FileServer` and `http.ServeFile` can use the same optimized path when the
conditions are satisfied.

`sendfile` is not guaranteed. Its availability depends on the operating
system, file type, network connection, and whether the response requires
additional processing such as compression, templating, or encryption.

## Usage in Peekd

Peekd uses `http.FileServer` to serve regular files directly, so these
responses may use the `sendfile` optimization. Text and Markdown files must be
read and rendered as HTML. Image, audio, and video preview pages also require
template rendering, so the preview pages themselves do not use zero-copy
transfer.

Direct file URLs use the direct file-serving path:

```bash
curl -o large-file.iso \
  'http://127.0.0.1:8090/large-file.iso'
```

## Verify on Linux

Build the server and trace both normal writes and `sendfile`:

```bash
go build -o peekd .
strace -ttt -f -e trace=write,writev,sendfile ./peekd -root /path/to/files
```

From another terminal, request a regular binary file:

```bash
curl -o /dev/null http://127.0.0.1:8090/large-file.bin
```

If Go uses the optimization, you should see output similar to:

```text
write(5, "HTTP/1.1 200 OK\r\n...", 702) = 702
sendfile(5, 8, NULL, 1165) = 1165
```

The first `write` includes the HTTP response headers and usually the first
512 bytes of the file, which Go reads before switching to `sendfile`. The
`sendfile` count is therefore only the remaining part of the response body.
For example, `512 + 1165 = 1677` is the complete file body, while the `702`
bytes reported by `write` also include the response headers. Peekd's access
log counts only response-body bytes.

Use a regular non-text file for this test. If no `sendfile` call appears, check
that the file is served directly, the response is not being transformed, and
the current connection and operating system support the optimization.

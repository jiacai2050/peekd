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

Requesting a raw file with `raw=1` uses the direct file-serving path:

```bash
curl -o large-file.iso \
  'http://127.0.0.1:8090/large-file.iso?raw=1'
```

## Verify on Linux

Build the server and use `strace` to trace only `sendfile`:

```bash
go build -o peekd .
strace -f -e trace=sendfile ./peekd -root /path/to/files
```

From another terminal, request a regular binary file:

```bash
curl -o /dev/null http://127.0.0.1:8090/large-file.bin
```

If Go uses the optimization, you should see output similar to:

```text
sendfile(8, 7, NULL, 4194304) = 4194304
```

Use a regular non-text file for this test. If no `sendfile` call appears, check
that the file is served directly, the response is not being transformed, and
the current connection and operating system support the optimization.

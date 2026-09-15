# Preview Expansion Plan

This document describes a preview strategy for file types that are useful in a
local file browser while keeping Peekd small, safe, and fast.

## Goals

- Keep direct file serving unchanged for unsupported or oversized files.
- Select a preview type from the file extension first.
- Use the first 512 bytes only when the extension is inconclusive.
- Keep every preview bounded by the existing maximum preview size.
- Treat preview rendering as a read-only operation.
- Preserve a simple fallback to the original file.

## Common Preview Pipeline

1. Check the request context. Only document requests are eligible for a
   rendered preview; subresource requests and `raw=1` are served directly.
2. Determine the `previewType` from the extension.
3. If the extension is inconclusive, detect the content type from the first
   512 bytes.
4. Check the file size before reading the complete file.
5. Read and validate the input using the type-specific limit.
6. Render the preview with an appropriate template.
7. Fall back to direct file serving when the format is invalid, unsupported, or
   too large.

The existing text and Markdown preview limit should remain the default limit
for textual formats. Structured and archive formats may use a smaller limit
because parsing can expand memory usage.

## High Priority

### PDF

PDF files should use the browser's native PDF viewer rather than a PDF parsing
library.

- Recognize `.pdf` and `application/pdf`.
- Render a minimal preview page containing an embedded PDF resource.
- Use the original file URL with `raw=1` for the embedded resource.
- Keep the download and back-to-directory actions consistent with other
  previews.
- Do not parse or modify PDF contents in the server.

This adds a useful preview with almost no new dependency or parser attack
surface. Browsers that do not support inline PDF viewing can still download the
original file.

### CSV and TSV (implemented)

CSV and TSV files should be rendered as bounded HTML tables.

- Recognize `.csv` and `.tsv` by extension.
- Use the standard library CSV reader for CSV.
- Use tab-separated parsing for TSV.
- Read at most the configured preview size and cap the number of rows and
  columns.
- Escape every cell through `html/template`.
- Display a truncation notice when rows or columns exceed the limit.
- Fall back to the text preview when parsing fails.

The parser must not load an unbounded file into memory. A table preview should
not attempt to infer or execute formulas.

### JSON

JSON should keep its own preview type instead of being treated only as plain
text.

- Recognize `.json` and `.jsonc` where the input is valid JSON.
- Parse with the standard library.
- Render a formatted, escaped view with collapsible objects and arrays.
- Preserve strings, numbers, booleans, and null values without evaluation.
- Enforce the preview size before parsing and cap nesting or rendered output if
  necessary.
- Fall back to the existing text preview for invalid JSON or JSONC syntax.

JSONC can initially use the text preview because comments are not accepted by
the standard JSON parser. Supporting comments should be a separate, explicit
feature rather than silently stripping input.

## Medium Priority

### XML

XML can use a formatted text or tree view.

- Recognize `.xml` and `application/xml` or `text/xml`.
- Prefer a safe formatted text view first.
- Escape all source content and never render embedded HTML.
- Set a parser depth and input-size limit if a tree view is added.
- Disable external entity resolution and network access.
- Fall back to the text preview when parsing fails.

A safe text view is preferable to introducing a full XML browser until there is
a clear need for tree navigation.

### ZIP and TAR Archives

Archives should show metadata and a file listing without extracting files.

- Recognize `.zip`, `.tar`, `.tar.gz`, and `.tgz`.
- List entry names, sizes, modification times, and compression status.
- Do not extract entries to disk.
- Cap the number of entries and total metadata read.
- Reject or clearly mark unsafe paths such as absolute paths and `../`.
- Keep archive entries non-clickable initially.

ZIP can use the standard library. TAR can use the standard library, while gzip
should be layered around the TAR reader. Archive content previews should be a
separate later feature because nested archives and decompression bombs require
additional limits.

### Mermaid in Markdown

Mermaid support should remain optional and client-side.

- Detect fenced `mermaid` blocks during Markdown rendering.
- Render them as escaped code or a dedicated placeholder by default.
- If client-side rendering is added, use a pinned and bundled asset.
- Do not execute arbitrary scripts from Markdown.
- Apply a maximum diagram count and source size.

Mermaid is useful, but it should not introduce a runtime dependency or weaken
the current Markdown raw-HTML safety guarantees.

## Rendering and Security Rules

- All generated HTML must use `html/template` or explicitly trusted,
  sanitized output.
- Never execute formulas in CSV or embedded scripts in XML, SVG, PDF, or
  archive entries.
- Preview pages must not change the original file.
- Use `raw=1` for resources embedded by preview templates.
- Keep direct file serving, range requests, and `sendfile` available whenever
  no preview transformation is required.
- Add focused tests for extension detection, content fallback, size limits,
  malformed input, escaping, and raw resource requests.

## Suggested Implementation Order

1. Add PDF preview using the browser's native viewer.
2. Add CSV and TSV table previews with strict row and column limits.
3. Add formatted JSON previews with text fallback.
4. Add safe XML formatting.
5. Add bounded ZIP and TAR listings.
6. Consider optional Mermaid rendering only after the security and asset
   strategy is settled.

package internal

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

var testEmbeddedFiles = fstest.MapFS{
	"assets/text.html":      {Data: []byte("<!doctype html><body>{{range .Lines}}{{.}}\n{{end}}</body>")},
	"assets/directory.html": {Data: []byte("<!doctype html><body><a href=\"/\">Peekd</a>{{if .HasParent}}<a href=\"../\">Parent directory</a>{{end}}{{range .Entries}}<div class=\"entry\">{{.Name}}|{{.URL}}|{{.FileModeBits}}|{{.Modified}}</div>{{end}}</body>")},
	"assets/image.html":     {Data: []byte("<!doctype html><body>image {{.FileName}}</body>")},
	"assets/media.html":     {Data: []byte("<!doctype html><body>media {{.FileName}}</body>")},
	"assets/json.html":      {Data: []byte("<!doctype html><body>json {{.FileName}}<pre>{{.Content}}</pre></body>")},
	"assets/csv.html":       {Data: []byte("<!doctype html><body>csv {{.FileName}} {{range $i, $row := .Rows}}{{range $row}}{{.}}|{{end}}{{end}}{{if .Truncated}}truncated{{end}}</body>")},
	"assets/pdf.html":       {Data: []byte("<!doctype html><body>pdf {{.FileName}} {{.RawURL}}</body>")},
	"assets/archive.html":   {Data: []byte("<!doctype html><body>archive {{.FileName}} {{range .Entries}}{{.Name}}|{{.Size}}{{end}}</body>")},
	"assets/markdown.html":  {Data: []byte("<!doctype html><body>{{.HTML}}</body>")},
	"assets/preview.css":    {Data: []byte("body{}")},
	"assets/directory.css":  {Data: []byte("body{}")},
}

func mustNewHandler(t *testing.T, rootDir string, maxPreviewSize int64) http.Handler {
	t.Helper()

	handler, err := NewHandler(Config{
		RootDir:            rootDir,
		Addr:               ":0",
		MaxTextPreviewSize: maxPreviewSize,
		Version:            "test",
		ProjectURL:         "https://example.com",
		EmbeddedFiles:      testEmbeddedFiles,
	})
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}
	return handler
}

func TestParseByteSize(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{input: "4M", want: 4 << 20},
		{input: "512K", want: 512 << 10},
		{input: "4MiB", want: 4 << 20},
		{input: "4194304", want: 4194304},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			got, err := ParseByteSize(test.input)
			if err != nil {
				t.Fatalf("ParseByteSize(%q): %v", test.input, err)
			}
			if got != test.want {
				t.Fatalf("ParseByteSize(%q) = %d, want %d", test.input, got, test.want)
			}
		})
	}
}

func TestPreviewBreadcrumbs(t *testing.T) {
	directory := previewBreadcrumbs("/docs/guide", true)
	if len(directory) != 3 {
		t.Fatalf("directory breadcrumb count = %d, want 3", len(directory))
	}
	if directory[0].Name != "Peekd" || directory[0].URL != "/" || directory[0].Current {
		t.Fatalf("unexpected root breadcrumb: %+v", directory[0])
	}
	if directory[1].Name != "docs" || directory[1].URL != "/docs/" || directory[1].Current {
		t.Fatalf("unexpected parent breadcrumb: %+v", directory[1])
	}
	if directory[2].Name != "guide" || directory[2].URL != "/docs/guide/" || !directory[2].Current {
		t.Fatalf("unexpected current breadcrumb: %+v", directory[2])
	}

	file := previewBreadcrumbs("/docs/my guide.txt", false)
	if file[2].URL != "/docs/my%20guide.txt" || !file[2].Current {
		t.Fatalf("unexpected file breadcrumb: %+v", file[2])
	}
}

func TestListenWithFallbackUsesNextPortWhenOccupied(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("occupy port: %v", err)
	}
	defer occupied.Close()

	_, occupiedPort, err := net.SplitHostPort(occupied.Addr().String())
	if err != nil {
		t.Fatalf("split occupied address: %v", err)
	}
	port, err := strconv.Atoi(occupiedPort)
	if err != nil {
		t.Fatalf("parse occupied port: %v", err)
	}
	if port == 65535 {
		t.Skip("no port available after occupied port")
	}

	fallback, err := listenWithFallback(occupied.Addr().String())
	if err != nil {
		t.Fatalf("listen with fallback: %v", err)
	}
	defer fallback.Close()

	_, fallbackPort, err := net.SplitHostPort(fallback.Addr().String())
	if err != nil {
		t.Fatalf("split fallback address: %v", err)
	}
	if fallbackPort != strconv.Itoa(port+1) {
		t.Fatalf("fallback port = %s, want %d", fallbackPort, port+1)
	}
}

func TestReadTextPreviewEnforcesLimit(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "large.txt")
	if err := os.WriteFile(filePath, []byte("12345"), 0o600); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	content, previewable, err := readTextPreview(filePath, 4)
	if err != nil {
		t.Fatalf("read text preview: %v", err)
	}
	if previewable {
		t.Fatal("expected oversized text file to be unavailable for preview")
	}
	if content != nil {
		t.Fatalf("oversized preview content = %q, want nil", content)
	}
}

func TestPreviewUsesConditionalCache(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rootDir, "notes.txt"), []byte("cached content"), 0o600); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	handler := mustNewHandler(t, rootDir, 4<<20)
	request := httptest.NewRequest(http.MethodGet, "/notes.txt", nil)
	request.Header.Set("Sec-Fetch-Dest", "document")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusOK)
	}
	etag := response.Header().Get("ETag")
	if etag == "" {
		t.Fatal("expected ETag")
	}
	if response.Header().Get("Last-Modified") == "" {
		t.Fatal("expected Last-Modified")
	}
	if response.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("Cache-Control = %q, want no-cache", response.Header().Get("Cache-Control"))
	}

	cachedRequest := httptest.NewRequest(http.MethodGet, "/notes.txt", nil)
	cachedRequest.Header.Set("Sec-Fetch-Dest", "document")
	cachedRequest.Header.Set("If-None-Match", etag)
	cachedResponse := httptest.NewRecorder()
	handler.ServeHTTP(cachedResponse, cachedRequest)

	if cachedResponse.Code != http.StatusNotModified {
		t.Fatalf("conditional status code = %d, want %d", cachedResponse.Code, http.StatusNotModified)
	}
	if cachedResponse.Body.Len() != 0 {
		t.Fatalf("conditional response body = %q, want empty", cachedResponse.Body.String())
	}
}

func TestContentDetectedTextFileIsPreviewed(t *testing.T) {
	rootDir := t.TempDir()
	filePath := filepath.Join(rootDir, "notes")
	if err := os.WriteFile(filePath, []byte("content without an extension"), 0o600); err != nil {
		t.Fatalf("write text file: %v", err)
	}

	handler := mustNewHandler(t, rootDir, 4<<20)
	request := httptest.NewRequest(http.MethodGet, "/notes", nil)
	request.Header.Set("Sec-Fetch-Dest", "document")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusOK)
	}
	if !strings.Contains(response.Body.String(), "content without an extension") {
		t.Fatalf("expected extensionless text file preview, got %q", response.Body.String())
	}
}

func TestEmptyFetchDestWithoutUpgradeRequestServesRawFile(t *testing.T) {
	rootDir := t.TempDir()
	filePath := filepath.Join(rootDir, "notes.txt")
	if err := os.WriteFile(filePath, []byte("raw response"), 0o600); err != nil {
		t.Fatalf("write text file: %v", err)
	}

	handler := mustNewHandler(t, rootDir, 4<<20)
	request := httptest.NewRequest(http.MethodGet, "/notes.txt", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Body.String() != "raw response" {
		t.Fatalf("expected raw file response, got %q", response.Body.String())
	}
}

func TestEmptyFetchDestWithUpgradeRequestPreviewsFile(t *testing.T) {
	rootDir := t.TempDir()
	filePath := filepath.Join(rootDir, "notes.txt")
	if err := os.WriteFile(filePath, []byte("top-level navigation"), 0o600); err != nil {
		t.Fatalf("write text file: %v", err)
	}

	handler := mustNewHandler(t, rootDir, 4<<20)
	request := httptest.NewRequest(http.MethodGet, "/notes.txt", nil)
	request.Header.Set("Upgrade-Insecure-Requests", "1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if !strings.Contains(response.Body.String(), "top-level navigation") {
		t.Fatalf("expected navigation request to preview file, got %q", response.Body.String())
	}
	if response.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("expected HTML preview response, got %q", response.Header().Get("Content-Type"))
	}
}

func TestContentDetectedImageFileIsPreviewed(t *testing.T) {
	rootDir := t.TempDir()
	filePath := filepath.Join(rootDir, "image")
	pngSignature := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}
	if err := os.WriteFile(filePath, pngSignature, 0o600); err != nil {
		t.Fatalf("write image file: %v", err)
	}

	handler := mustNewHandler(t, rootDir, 4<<20)
	request := httptest.NewRequest(http.MethodGet, "/image", nil)
	request.Header.Set("Sec-Fetch-Dest", "document")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusOK)
	}
	if !strings.Contains(response.Body.String(), "image image") {
		t.Fatalf("expected extensionless image preview, got %q", response.Body.String())
	}
}

func TestPDFFileIsPreviewed(t *testing.T) {
	rootDir := t.TempDir()
	filePath := filepath.Join(rootDir, "manual.pdf")
	if err := os.WriteFile(filePath, []byte("%PDF-1.7\n"), 0o600); err != nil {
		t.Fatalf("write PDF file: %v", err)
	}

	handler := mustNewHandler(t, rootDir, 4<<20)
	request := httptest.NewRequest(http.MethodGet, "/manual.pdf", nil)
	request.Header.Set("Sec-Fetch-Dest", "document")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusOK)
	}
	if !strings.Contains(response.Body.String(), "pdf manual.pdf /manual.pdf?raw=1") {
		t.Fatalf("expected PDF preview, got %q", response.Body.String())
	}
}

func TestJSONFileIsFormatted(t *testing.T) {
	rootDir := t.TempDir()
	filePath := filepath.Join(rootDir, "config.json")
	if err := os.WriteFile(filePath, []byte(`{"name":"peekd","enabled":true}`), 0o600); err != nil {
		t.Fatalf("write JSON file: %v", err)
	}

	handler := mustNewHandler(t, rootDir, 4<<20)
	request := httptest.NewRequest(http.MethodGet, "/config.json", nil)
	request.Header.Set("Sec-Fetch-Dest", "document")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()
	if !strings.Contains(body, "&#34;name&#34;: &#34;peekd&#34;") || !strings.Contains(body, "&#34;enabled&#34;: true") {
		t.Fatalf("expected formatted JSON preview, got %q", body)
	}
}

func TestCSVFileIsRenderedAsTable(t *testing.T) {
	rootDir := t.TempDir()
	filePath := filepath.Join(rootDir, "data.csv")
	if err := os.WriteFile(filePath, []byte("Name,Count\nPeekd,2\n"), 0o600); err != nil {
		t.Fatalf("write CSV file: %v", err)
	}

	handler := mustNewHandler(t, rootDir, 4<<20)
	request := httptest.NewRequest(http.MethodGet, "/data.csv", nil)
	request.Header.Set("Sec-Fetch-Dest", "document")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()
	if !strings.Contains(body, "Name|Count|Peekd|2|") {
		t.Fatalf("expected CSV table preview, got %q", body)
	}
}

func TestTSVFileIsRenderedAsTable(t *testing.T) {
	rootDir := t.TempDir()
	filePath := filepath.Join(rootDir, "data.tsv")
	if err := os.WriteFile(filePath, []byte("Name\tCount\nPeekd\t2\n"), 0o600); err != nil {
		t.Fatalf("write TSV file: %v", err)
	}

	handler := mustNewHandler(t, rootDir, 4<<20)
	request := httptest.NewRequest(http.MethodGet, "/data.tsv", nil)
	request.Header.Set("Sec-Fetch-Dest", "document")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if !strings.Contains(response.Body.String(), "Name|Count|Peekd|2|") {
		t.Fatalf("expected TSV table preview, got %q", response.Body.String())
	}
}

func TestCSVPreviewIsBounded(t *testing.T) {
	content := strings.Builder{}
	for row := 0; row < csvPreviewMaxRows+1; row++ {
		content.WriteString("a,b,c,d\n")
	}

	rows, truncated, err := parseCSVPreview([]byte(content.String()), ',')
	if err != nil {
		t.Fatalf("parse CSV preview: %v", err)
	}
	if len(rows) != csvPreviewMaxRows {
		t.Fatalf("row count = %d, want %d", len(rows), csvPreviewMaxRows)
	}
	if !truncated {
		t.Fatal("expected truncated CSV preview")
	}

	wideRow := strings.TrimSuffix(strings.Repeat("a,", csvPreviewMaxColumns), ",") + ",a\n"
	rows, truncated, err = parseCSVPreview([]byte(wideRow), ',')
	if err != nil {
		t.Fatalf("parse wide CSV preview: %v", err)
	}
	if len(rows[0]) != csvPreviewMaxColumns {
		t.Fatalf("column count = %d, want %d", len(rows[0]), csvPreviewMaxColumns)
	}
	if !truncated {
		t.Fatal("expected truncated wide CSV preview")
	}
}

func TestInvalidCSVFallsBackToTextPreview(t *testing.T) {
	rootDir := t.TempDir()
	filePath := filepath.Join(rootDir, "invalid.csv")
	if err := os.WriteFile(filePath, []byte("Name,Count\n\"unterminated\n"), 0o600); err != nil {
		t.Fatalf("write invalid CSV file: %v", err)
	}

	handler := mustNewHandler(t, rootDir, 4<<20)
	request := httptest.NewRequest(http.MethodGet, "/invalid.csv", nil)
	request.Header.Set("Sec-Fetch-Dest", "document")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if strings.Contains(response.Body.String(), "truncated") || !strings.Contains(response.Body.String(), "unterminated") {
		t.Fatalf("expected invalid CSV text fallback, got %q", response.Body.String())
	}
}

func TestZIPFileListsEntries(t *testing.T) {
	rootDir := t.TempDir()
	filePath := filepath.Join(rootDir, "bundle.zip")
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("create ZIP file: %v", err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("docs/readme.txt")
	if err != nil {
		t.Fatalf("create ZIP entry: %v", err)
	}
	if _, err := entry.Write([]byte("hello")); err != nil {
		t.Fatalf("write ZIP entry: %v", err)
	}
	if _, err := writer.Create("images/"); err != nil {
		t.Fatalf("create ZIP directory: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close ZIP writer: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close ZIP file: %v", err)
	}

	handler := mustNewHandler(t, rootDir, 4<<20)
	request := httptest.NewRequest(http.MethodGet, "/bundle.zip", nil)
	request.Header.Set("Sec-Fetch-Dest", "document")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()
	if !strings.Contains(body, "docs/readme.txt") || !strings.Contains(body, "images/") {
		t.Fatalf("expected ZIP entries in preview, got %q", body)
	}
}

func TestTARFileListsEntries(t *testing.T) {
	rootDir := t.TempDir()
	filePath := filepath.Join(rootDir, "bundle.tar")
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("create TAR file: %v", err)
	}
	writer := tar.NewWriter(file)
	if err := writer.WriteHeader(&tar.Header{Name: "docs/readme.txt", Size: 5, Mode: 0o600}); err != nil {
		t.Fatalf("create TAR entry: %v", err)
	}
	if _, err := writer.Write([]byte("hello")); err != nil {
		t.Fatalf("write TAR entry: %v", err)
	}
	if err := writer.WriteHeader(&tar.Header{Name: "images/", Typeflag: tar.TypeDir, Mode: 0o755}); err != nil {
		t.Fatalf("create TAR directory: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close TAR writer: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close TAR file: %v", err)
	}

	assertArchivePreviewContains(t, rootDir, "/bundle.tar", "docs/readme.txt", "images/")
}

func TestTARGZFileListsEntries(t *testing.T) {
	rootDir := t.TempDir()
	filePath := filepath.Join(rootDir, "bundle.tar.gz")
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("create TAR.GZ file: %v", err)
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: "docs/readme.txt", Size: 5, Mode: 0o600}); err != nil {
		t.Fatalf("create TAR.GZ entry: %v", err)
	}
	if _, err := tarWriter.Write([]byte("hello")); err != nil {
		t.Fatalf("write TAR.GZ entry: %v", err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close TAR.GZ TAR writer: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("close TAR.GZ gzip writer: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close TAR.GZ file: %v", err)
	}

	assertArchivePreviewContains(t, rootDir, "/bundle.tar.gz", "docs/readme.txt")
}

func assertArchivePreviewContains(t *testing.T, rootDir, requestPath string, entries ...string) {
	t.Helper()
	handler := mustNewHandler(t, rootDir, 4<<20)
	request := httptest.NewRequest(http.MethodGet, requestPath, nil)
	request.Header.Set("Sec-Fetch-Dest", "document")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()
	for _, entry := range entries {
		if !strings.Contains(body, entry) {
			t.Fatalf("expected archive entry %q in preview, got %q", entry, body)
		}
	}
}

func TestMarkdownPreviewForBrowserDisablesRawHTML(t *testing.T) {
	rootDir := t.TempDir()
	filePath := filepath.Join(rootDir, "README.md")
	content := "# Hello\n\n<script>alert('xss')</script>\n\n**world**\n"
	if err := os.WriteFile(filePath, []byte(content), 0o600); err != nil {
		t.Fatalf("write markdown file: %v", err)
	}

	handler := mustNewHandler(t, rootDir, 4<<20)
	request := httptest.NewRequest(http.MethodGet, "/README.md", nil)
	request.Header.Set("Sec-Fetch-Dest", "document")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusOK)
	}

	body := response.Body.String()
	if !strings.Contains(body, "<h1>Hello</h1>") {
		t.Fatalf("expected markdown heading in response body, got %q", body)
	}
	if !strings.Contains(body, "<strong>world</strong>") {
		t.Fatalf("expected markdown emphasis in response body, got %q", body)
	}
	if strings.Contains(body, "<script>alert('xss')</script>") {
		t.Fatalf("raw HTML was rendered in markdown output: %q", body)
	}
}

func TestMarkdownPreviewRendersMermaid(t *testing.T) {
	rendered, err := renderMarkdown([]byte("```mermaid\ngraph LR\n    A --> B\n```\n"))
	if err != nil {
		t.Fatalf("render Mermaid markdown: %v", err)
	}
	body := string(rendered)
	if !strings.Contains(body, `<pre class="mermaid">`) {
		t.Fatalf("expected Mermaid block, got %q", body)
	}
	if !strings.Contains(body, `<script src="https://cdn.jsdelivr.net/npm/mermaid/dist/mermaid.min.js"></script>`) {
		t.Fatalf("expected Mermaid script, got %q", body)
	}
}

func TestMarkdownPreviewRendersExtendedSyntax(t *testing.T) {
	rendered, err := renderMarkdown([]byte("Term\n: Definition\n\nText[^1]\n\n[^1]: Note\n"))
	if err != nil {
		t.Fatalf("render extended Markdown: %v", err)
	}
	body := string(rendered)
	if !strings.Contains(body, "<dl>") || !strings.Contains(body, "<dt>Term</dt>") {
		t.Fatalf("expected definition list, got %q", body)
	}
	if !strings.Contains(body, "footnote") {
		t.Fatalf("expected footnote output, got %q", body)
	}
}

func TestMarkdownFileDefaultsToRaw(t *testing.T) {
	rootDir := t.TempDir()
	filePath := filepath.Join(rootDir, "README.markdown")
	content := "# plain\n\ncontent\n"
	if err := os.WriteFile(filePath, []byte(content), 0o600); err != nil {
		t.Fatalf("write markdown file: %v", err)
	}

	handler := mustNewHandler(t, rootDir, 4<<20)
	request := httptest.NewRequest(http.MethodGet, "/README.markdown?raw=1", nil)
	request.Header.Set("Sec-Fetch-Dest", "document")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Body.String(); got != content {
		t.Fatalf("non-browser response = %q, want %q", got, content)
	}
}

func TestDirectoryPreviewContainsParentAndRootLinksAndSecondPrecision(t *testing.T) {
	rootDir := t.TempDir()
	subDir := filepath.Join(rootDir, "sub")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatalf("create sub directory: %v", err)
	}

	filePath := filepath.Join(subDir, "entry.txt")
	if err := os.WriteFile(filePath, []byte("entry"), 0o600); err != nil {
		t.Fatalf("write directory entry file: %v", err)
	}
	modTime := time.Date(2025, 1, 2, 3, 4, 5, 123456789, time.UTC)
	if err := os.Chtimes(filePath, modTime, modTime); err != nil {
		t.Fatalf("set modtime: %v", err)
	}

	handler := mustNewHandler(t, rootDir, 4<<20)
	request := httptest.NewRequest(http.MethodGet, "/sub/", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusOK)
	}

	body := response.Body.String()
	if !strings.Contains(body, "href=\"../\"") {
		t.Fatalf("expected parent link in directory preview, got %q", body)
	}
	if !strings.Contains(body, "href=\"/\"") {
		t.Fatalf("expected root link in directory preview, got %q", body)
	}
	if !strings.Contains(body, "-rw-------") {
		t.Fatalf("expected file mode bits in directory preview, got %q", body)
	}
	if !strings.Contains(body, "/sub/entry.txt") {
		t.Fatalf("expected directory entry URL, got %q", body)
	}

	re := regexp.MustCompile(`\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}`)
	if !re.MatchString(body) {
		t.Fatalf("expected second-precision timestamp in directory preview, got %q", body)
	}
	if strings.Contains(body, ".123") {
		t.Fatalf("timestamp should not contain sub-second precision: %q", body)
	}
}

func TestAssetRouteServesEmbeddedFiles(t *testing.T) {
	rootDir := t.TempDir()
	handler := mustNewHandler(t, rootDir, 4<<20)

	request := httptest.NewRequest(http.MethodGet, "/__peekd_assets/preview.css", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusOK)
	}
	if !strings.Contains(response.Body.String(), "body{}") {
		t.Fatalf("unexpected asset response: %q", response.Body.String())
	}
	etag := response.Header().Get("ETag")
	if etag == "" {
		t.Fatal("expected asset ETag")
	}
	if response.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("asset Cache-Control = %q, want no-cache", response.Header().Get("Cache-Control"))
	}

	cachedRequest := httptest.NewRequest(http.MethodGet, "/__peekd_assets/preview.css", nil)
	cachedRequest.Header.Set("If-None-Match", etag)
	cachedResponse := httptest.NewRecorder()
	handler.ServeHTTP(cachedResponse, cachedRequest)
	if cachedResponse.Code != http.StatusNotModified {
		t.Fatalf("conditional asset status code = %d, want %d", cachedResponse.Code, http.StatusNotModified)
	}
}

func TestNewHandlerRequiresMarkdownTemplate(t *testing.T) {
	missingMarkdownTemplate := fstest.MapFS{
		"assets/text.html":      {Data: []byte("text")},
		"assets/directory.html": {Data: []byte("directory")},
		"assets/image.html":     {Data: []byte("image")},
		"assets/media.html":     {Data: []byte("media")},
		"assets/json.html":      {Data: []byte("json")},
		"assets/csv.html":       {Data: []byte("csv")},
		"assets/pdf.html":       {Data: []byte("pdf")},
		"assets/archive.html":   {Data: []byte("archive")},
	}

	_, err := NewHandler(Config{
		RootDir:            ".",
		Addr:               ":0",
		MaxTextPreviewSize: 4 << 20,
		Version:            "test",
		ProjectURL:         "https://example.com",
		EmbeddedFiles:      fs.FS(missingMarkdownTemplate),
	})
	if err == nil {
		t.Fatal("expected markdown template parse error, got nil")
	}
	if !strings.Contains(err.Error(), "markdown template") {
		t.Fatalf("unexpected error: %v", err)
	}
}

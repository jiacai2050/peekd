package server

import (
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
	"assets/directory.html": {Data: []byte("<!doctype html><body><a href=\"/\">Peekd</a>{{if .HasParent}}<a href=\"../\">Parent directory</a>{{end}}{{range .Entries}}<div class=\"entry\">{{.Name}}|{{.Modified}}</div>{{end}}</body>")},
	"assets/image.html":     {Data: []byte("<!doctype html><body>image {{.FileName}}</body>")},
	"assets/media.html":     {Data: []byte("<!doctype html><body>media {{.FileName}}</body>")},
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

func TestIsBrowserUserAgent(t *testing.T) {
	tests := []struct {
		name      string
		userAgent string
		want      bool
	}{
		{name: "Chrome", userAgent: "Mozilla/5.0 Chrome/120.0.0.0 Safari/537.36", want: true},
		{name: "Firefox", userAgent: "Mozilla/5.0 Firefox/121.0", want: true},
		{name: "Safari", userAgent: "Mozilla/5.0 Version/17.0 Safari/605.1.15", want: true},
		{name: "curl", userAgent: "curl/8.5.0", want: false},
		{name: "empty", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isBrowserUserAgent(test.userAgent); got != test.want {
				t.Fatalf("isBrowserUserAgent(%q) = %v, want %v", test.userAgent, got, test.want)
			}
		})
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

func TestMarkdownPreviewForBrowserDisablesRawHTML(t *testing.T) {
	rootDir := t.TempDir()
	filePath := filepath.Join(rootDir, "README.md")
	content := "# Hello\n\n<script>alert('xss')</script>\n\n**world**\n"
	if err := os.WriteFile(filePath, []byte(content), 0o600); err != nil {
		t.Fatalf("write markdown file: %v", err)
	}

	handler := mustNewHandler(t, rootDir, 4<<20)
	request := httptest.NewRequest(http.MethodGet, "/README.md", nil)
	request.Header.Set("User-Agent", "Mozilla/5.0")
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

func TestMarkdownPreviewFallsBackToRawForNonBrowser(t *testing.T) {
	rootDir := t.TempDir()
	filePath := filepath.Join(rootDir, "README.markdown")
	content := "# plain\n\ncontent\n"
	if err := os.WriteFile(filePath, []byte(content), 0o600); err != nil {
		t.Fatalf("write markdown file: %v", err)
	}

	handler := mustNewHandler(t, rootDir, 4<<20)
	request := httptest.NewRequest(http.MethodGet, "/README.markdown", nil)
	request.Header.Set("User-Agent", "curl/8.5.0")
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
	request.Header.Set("User-Agent", "Mozilla/5.0")
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
}

func TestNewHandlerRequiresMarkdownTemplate(t *testing.T) {
	missingMarkdownTemplate := fstest.MapFS{
		"assets/text.html":      {Data: []byte("text")},
		"assets/directory.html": {Data: []byte("directory")},
		"assets/image.html":     {Data: []byte("image")},
		"assets/media.html":     {Data: []byte("media")},
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

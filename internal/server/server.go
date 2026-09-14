package server

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

type Config struct {
	RootDir            string
	Addr               string
	MaxTextPreviewSize int64
	Version            string
	ProjectURL         string
	EmbeddedFiles      fs.FS
}

type accessLogResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (w *accessLogResponseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *accessLogResponseWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(data)
	w.bytes += int64(n)
	return n, err
}

func (w *accessLogResponseWriter) ReadFrom(reader io.Reader) (int64, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}

	if readerFrom, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		n, err := readerFrom.ReadFrom(reader)
		w.bytes += n
		return n, err
	}

	return io.Copy(struct{ io.Writer }{Writer: w}, reader)
}

func (w *accessLogResponseWriter) Flush() {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func withAccessLog(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/__peekd_assets/") {
			handler.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		response := &accessLogResponseWriter{ResponseWriter: w}
		handler.ServeHTTP(response, r)
		if response.status == 0 {
			response.status = http.StatusOK
		}
		log.Printf("%s %s %d %d %s", r.Method, r.URL.RequestURI(), response.status, response.bytes, time.Since(start).Round(time.Microsecond))
	})
}

type directoryEntry struct {
	Name         string
	URL          string
	Icon         string
	FileModeBits string
	Size         string
	Modified     string
	IsDir        bool
	ModTime      time.Time
}

type directoryData struct {
	Path       string
	HasParent  bool
	Entries    []directoryEntry
	ProjectURL string
	Version    string
}

type textPreviewData struct {
	FileName   string
	Lines      []string
	RawURL     string
	Size       string
	Modified   string
	ProjectURL string
	Version    string
}

type markdownPreviewData struct {
	FileName   string
	HTML       template.HTML
	RawURL     string
	Size       string
	Modified   string
	ProjectURL string
	Version    string
}

type mediaPreviewData struct {
	FileName   string
	RawURL     string
	MediaKind  previewType
	Size       string
	Modified   string
	ProjectURL string
	Version    string
}

type previewType string

const (
	previewTypeNone     previewType = ""
	previewTypeText     previewType = "text"
	previewTypeMarkdown previewType = "markdown"
	previewTypeImage    previewType = "image"
	previewTypeAudio    previewType = "audio"
	previewTypeVideo    previewType = "video"
)

var markdownRenderer = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
)

func detectContentType(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if n == 0 {
		return "", nil
	}
	return http.DetectContentType(buffer[:n]), nil
}

func previewTypeByExtension(filePath string) previewType {
	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".md", ".markdown":
		return previewTypeMarkdown
	case ".avif", ".bmp", ".gif", ".ico", ".jpeg", ".jpg", ".png", ".svg", ".tif", ".tiff", ".webp":
		return previewTypeImage
	case ".aac", ".flac", ".m4a", ".mp3", ".oga", ".ogg", ".opus", ".wav", ".weba":
		return previewTypeAudio
	case ".avi", ".m4v", ".mkv", ".mov", ".mp4", ".ogv", ".webm":
		return previewTypeVideo
	case ".txt", ".log", ".conf", ".ini", ".properties",
		".json", ".jsonc", ".yaml", ".yml", ".toml", ".xml",
		".rst",
		".html", ".htm", ".css", ".scss", ".sass", ".less",
		".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx",
		".vue", ".svelte",
		".go", ".rs", ".zig", ".py", ".rb", ".php",
		".java", ".kt", ".kts", ".swift",
		".c", ".h", ".cc", ".cpp", ".cxx", ".hpp",
		".cs", ".fs", ".fsx",
		".sh", ".bash", ".zsh", ".fish", ".ps1",
		".sql":
		return previewTypeText
	default:
		switch strings.ToLower(filepath.Base(filePath)) {
		case "makefile", "dockerfile", "jenkinsfile", "justfile", "license":
			return previewTypeText
		default:
			return previewTypeNone
		}
	}
}

func previewTypeByContent(contentType string) previewType {
	switch {
	case strings.HasPrefix(contentType, "image/"):
		return previewTypeImage
	case strings.HasPrefix(contentType, "audio/"):
		return previewTypeAudio
	case strings.HasPrefix(contentType, "video/"):
		return previewTypeVideo
	case strings.HasPrefix(contentType, "text/"):
		return previewTypeText
	default:
		return previewTypeNone
	}
}

func fileIcon(path string, isDir bool) string {
	if isDir {
		return "📁"
	}

	switch previewTypeByExtension(path) {
	case previewTypeImage:
		return "🖼️"
	case previewTypeAudio:
		return "🎵"
	case previewTypeVideo:
		return "🎬"
	case previewTypeText, previewTypeMarkdown:
		return "📄"
	}

	switch strings.ToLower(filepath.Ext(path)) {
	case ".7z", ".bz2", ".gz", ".rar", ".tar", ".xz", ".zip":
		return "📦"
	default:
		return "📄"
	}
}

func formatFileSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}

	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

func splitLines(content string) []string {
	lines := strings.Split(content, "\n")
	if len(lines) > 1 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func ParseByteSize(value string) (int64, error) {
	value = strings.TrimSpace(value)
	numberEnd := 0
	for numberEnd < len(value) && (value[numberEnd] == '.' || value[numberEnd] >= '0' && value[numberEnd] <= '9') {
		numberEnd++
	}
	if numberEnd == 0 {
		return 0, fmt.Errorf("invalid size %q", value)
	}

	number, err := strconv.ParseFloat(value[:numberEnd], 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, fmt.Errorf("invalid size %q", value)
	}

	multiplier := int64(1)
	switch strings.ToUpper(strings.TrimSpace(value[numberEnd:])) {
	case "":
	case "B":
	case "K", "KB", "KIB":
		multiplier = 1 << 10
	case "M", "MB", "MIB":
		multiplier = 1 << 20
	case "G", "GB", "GIB":
		multiplier = 1 << 30
	case "T", "TB", "TIB":
		multiplier = 1 << 40
	default:
		return 0, fmt.Errorf("invalid size suffix in %q", value)
	}

	size := number * float64(multiplier)
	if size <= 0 || size >= float64(1<<63) || math.Trunc(size) != size {
		return 0, fmt.Errorf("size out of range %q", value)
	}
	return int64(size), nil
}

func readTextPreview(filePath string, maxSize int64) ([]byte, bool, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()

	content, err := io.ReadAll(io.LimitReader(file, maxSize+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(content)) > maxSize {
		return nil, false, nil
	}
	return content, true, nil
}

func renderMarkdown(content []byte) (template.HTML, error) {
	var output bytes.Buffer
	if err := markdownRenderer.Convert(content, &output); err != nil {
		return "", err
	}
	return template.HTML(output.String()), nil
}

func directoryEntryURL(requestPath, name string, isDir bool) string {
	entryPath := path.Join(requestPath, name)
	if isDir {
		entryPath += "/"
	}
	return (&url.URL{Path: entryPath}).String()
}

func renderDirectory(w http.ResponseWriter, r *http.Request, rootDir, requestPath string, tmpl *template.Template, projectURL string, version string) {
	fullPath := filepath.Join(rootDir, filepath.FromSlash(requestPath))
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		http.Error(w, "unable to read directory", http.StatusInternalServerError)
		return
	}

	displayPath := rootDir
	if requestPath != "/" {
		displayPath = filepath.Join(displayPath, filepath.FromSlash(strings.TrimPrefix(requestPath, "/")))
	}

	data := directoryData{
		Path:       displayPath,
		HasParent:  requestPath != "/",
		ProjectURL: projectURL,
		Version:    version,
	}

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			http.Error(w, "unable to read directory entry", http.StatusInternalServerError)
			return
		}

		size := "-"
		if !entry.IsDir() {
			size = formatFileSize(info.Size())
		}
		data.Entries = append(data.Entries, directoryEntry{
			Name:         entry.Name(),
			URL:          directoryEntryURL(requestPath, entry.Name(), entry.IsDir()),
			Icon:         fileIcon(entry.Name(), entry.IsDir()),
			FileModeBits: info.Mode().String(),
			Size:         size,
			Modified:     info.ModTime().Format("2006-01-02 15:04:05"),
			IsDir:        entry.IsDir(),
			ModTime:      info.ModTime(),
		})
	}

	sort.Slice(data.Entries, func(i, j int) bool {
		left, right := data.Entries[i], data.Entries[j]
		if !left.ModTime.Equal(right.ModTime) {
			return left.ModTime.After(right.ModTime)
		}
		return strings.ToLower(left.Name) < strings.ToLower(right.Name)
	})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("failed to render directory %s: %v", r.URL.Path, err)
	}
}

func printServerAddresses(addr net.Addr) {
	host, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		log.Printf("local file server started at %s", addr)
		return
	}

	if host != "" && host != "0.0.0.0" && host != "::" {
		log.Printf("open http://%s", net.JoinHostPort(host, port))
		return
	}

	log.Printf("open http://localhost:%s", port)

	interfaceAddrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Printf("unable to enumerate network interfaces: %v", err)
		return
	}

	seen := make(map[string]struct{})
	for _, interfaceAddr := range interfaceAddrs {
		ipNet, ok := interfaceAddr.(*net.IPNet)
		if !ok {
			continue
		}

		ip := ipNet.IP
		if !ip.IsGlobalUnicast() {
			continue
		}

		host := ip.String()
		if _, ok := seen[host]; ok {
			continue
		}
		seen[host] = struct{}{}
		log.Printf("open http://%s", net.JoinHostPort(host, port))
	}
}

func listenWithFallback(addr string) (net.Listener, error) {
	currentAddr := addr
	for {
		listener, err := net.Listen("tcp", currentAddr)
		if err == nil {
			return listener, nil
		}
		if !errors.Is(err, syscall.EADDRINUSE) {
			return nil, err
		}

		host, port, splitErr := net.SplitHostPort(currentAddr)
		if splitErr != nil {
			return nil, err
		}
		portNumber, parseErr := strconv.Atoi(port)
		if parseErr != nil || portNumber >= 65535 {
			return nil, err
		}
		currentAddr = net.JoinHostPort(host, strconv.Itoa(portNumber+1))
	}
}

func NewHandler(config Config) (http.Handler, error) {
	textTemplate, err := template.ParseFS(config.EmbeddedFiles, "assets/text.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse embedded template: %w", err)
	}
	directoryTemplate, err := template.ParseFS(config.EmbeddedFiles, "assets/directory.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse directory template: %w", err)
	}
	imageTemplate, err := template.ParseFS(config.EmbeddedFiles, "assets/image.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse image template: %w", err)
	}
	mediaTemplate, err := template.ParseFS(config.EmbeddedFiles, "assets/media.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse media template: %w", err)
	}
	markdownTemplate, err := template.ParseFS(config.EmbeddedFiles, "assets/markdown.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse markdown template: %w", err)
	}

	assetsFS, err := fs.Sub(config.EmbeddedFiles, "assets")
	if err != nil {
		return nil, fmt.Errorf("failed to access embedded subdirectory: %w", err)
	}

	fileServer := http.FileServer(http.Dir(config.RootDir))
	mux := http.NewServeMux()
	mux.Handle("/__peekd_assets/", http.StripPrefix("/__peekd_assets/", http.FileServer(http.FS(assetsFS))))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		cleanedPath := filepath.Clean(r.URL.Path)
		fullPath := filepath.Join(config.RootDir, cleanedPath)

		info, err := os.Stat(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				http.NotFound(w, r)
			} else {
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
			return
		}

		if info.IsDir() {
			renderDirectory(w, r, config.RootDir, cleanedPath, directoryTemplate, config.ProjectURL, config.Version)
			return
		}

		if r.URL.Query().Get("raw") == "1" || r.Header.Get("Sec-Fetch-Dest") != "document" {
			fileServer.ServeHTTP(w, r)
			return
		}

		preview := previewTypeByExtension(fullPath)
		if preview == previewTypeNone {
			contentType, detectErr := detectContentType(fullPath)
			if detectErr != nil {
				http.Error(w, "unable to inspect file", http.StatusInternalServerError)
				return
			}
			preview = previewTypeByContent(contentType)
		}

		switch preview {
		case previewTypeImage:
			data := mediaPreviewData{
				FileName:   filepath.Base(fullPath),
				RawURL:     (&url.URL{Path: r.URL.Path, RawQuery: "raw=1"}).String(),
				Size:       formatFileSize(info.Size()),
				Modified:   info.ModTime().Format("2006-01-02 15:04:05"),
				ProjectURL: config.ProjectURL,
				Version:    config.Version,
			}

			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if err := imageTemplate.Execute(w, data); err != nil {
				log.Printf("failed to render image preview %s: %v", r.URL.Path, err)
			}
			return

		case previewTypeAudio, previewTypeVideo:
			data := mediaPreviewData{
				FileName:   filepath.Base(fullPath),
				RawURL:     (&url.URL{Path: r.URL.Path, RawQuery: "raw=1"}).String(),
				MediaKind:  preview,
				Size:       formatFileSize(info.Size()),
				Modified:   info.ModTime().Format("2006-01-02 15:04:05"),
				ProjectURL: config.ProjectURL,
				Version:    config.Version,
			}

			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if err := mediaTemplate.Execute(w, data); err != nil {
				log.Printf("failed to render media preview %s: %v", r.URL.Path, err)
			}
			return

		case previewTypeText, previewTypeMarkdown:
			if info.Size() > config.MaxTextPreviewSize {
				fileServer.ServeHTTP(w, r)
				return
			}

			content, previewable, err := readTextPreview(fullPath, config.MaxTextPreviewSize)
			if err != nil {
				http.Error(w, "unable to read file", http.StatusInternalServerError)
				return
			}
			if !previewable || !utf8.Valid(content) {
				fileServer.ServeHTTP(w, r)
				return
			}

			if preview == previewTypeMarkdown {
				markdownHTML, err := renderMarkdown(content)
				if err != nil {
					http.Error(w, "unable to render markdown", http.StatusInternalServerError)
					return
				}

				data := markdownPreviewData{
					FileName:   filepath.Base(fullPath),
					HTML:       markdownHTML,
					RawURL:     (&url.URL{Path: r.URL.Path, RawQuery: "raw=1"}).String(),
					Size:       formatFileSize(info.Size()),
					Modified:   info.ModTime().Format("2006-01-02 15:04:05"),
					ProjectURL: config.ProjectURL,
					Version:    config.Version,
				}

				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				if err := markdownTemplate.Execute(w, data); err != nil {
					log.Printf("failed to render markdown preview %s: %v", r.URL.Path, err)
				}
				return
			}

			data := textPreviewData{
				FileName:   filepath.Base(fullPath),
				Lines:      splitLines(string(content)),
				RawURL:     (&url.URL{Path: r.URL.Path, RawQuery: "raw=1"}).String(),
				Size:       formatFileSize(info.Size()),
				Modified:   info.ModTime().Format("2006-01-02 15:04:05"),
				ProjectURL: config.ProjectURL,
				Version:    config.Version,
			}

			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if err := textTemplate.Execute(w, data); err != nil {
				log.Printf("failed to render text preview %s: %v", r.URL.Path, err)
			}
			return
		}

		fileServer.ServeHTTP(w, r)
	})

	return withAccessLog(mux), nil
}

func Run(config Config) error {
	handler, err := NewHandler(config)
	if err != nil {
		return err
	}

	listener, err := listenWithFallback(config.Addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", config.Addr, err)
	}
	defer listener.Close()

	log.Printf("serving %s", config.RootDir)
	printServerAddresses(listener.Addr())
	return http.Serve(listener, handler)
}

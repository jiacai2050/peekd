package main

import (
	"embed"
	"errors"
	"flag"
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
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

// 1. Core magic: use go:embed to bundle all files in the assets directory into the binary.
//
//go:embed assets/*
var embeddedFiles embed.FS

var Version = "dev"

const ProjectURL = "https://github.com/jiacai2050/peekd"

type directoryEntry struct {
	Name     string
	URL      string
	Icon     string
	Size     string
	Modified string
	IsDir    bool
	ModTime  time.Time
}

type directoryData struct {
	Path       string
	HasParent  bool
	Entries    []directoryEntry
	ProjectURL string
	Version    string
}

type previewData struct {
	FileName   string
	Lines      []string
	RawURL     string
	MediaKind  string
	Size       string
	Modified   string
	ProjectURL string
	Version    string
}

func isTextFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".txt", ".log", ".conf", ".ini", ".properties",
		".json", ".jsonc", ".yaml", ".yml", ".toml", ".xml",
		".md", ".markdown", ".rst",
		".html", ".htm", ".css", ".scss", ".sass", ".less",
		".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx",
		".vue", ".svelte",
		".go", ".rs", ".zig", ".py", ".rb", ".php",
		".java", ".kt", ".kts", ".swift",
		".c", ".h", ".cc", ".cpp", ".cxx", ".hpp",
		".cs", ".fs", ".fsx",
		".sh", ".bash", ".zsh", ".fish", ".ps1",
		".sql":
		return true
	}

	switch strings.ToLower(filepath.Base(path)) {
	case "makefile", "dockerfile", "jenkinsfile", "justfile":
		return true
	default:
		return false
	}
}

func isImageFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".avif", ".bmp", ".gif", ".ico", ".jpeg", ".jpg", ".png", ".svg", ".tif", ".tiff", ".webp":
		return true
	default:
		return false
	}
}

func mediaKind(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".aac", ".flac", ".m4a", ".mp3", ".oga", ".ogg", ".opus", ".wav", ".weba":
		return "audio"
	case ".avi", ".m4v", ".mkv", ".mov", ".mp4", ".ogv", ".webm":
		return "video"
	default:
		return ""
	}
}

func fileIcon(path string, isDir bool) string {
	if isDir {
		return "📁"
	}
	if isImageFile(path) {
		return "🖼️"
	}
	if mediaKind(path) == "audio" {
		return "🎵"
	}
	if mediaKind(path) == "video" {
		return "🎬"
	}
	if isTextFile(path) {
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

func isBrowserUserAgent(userAgent string) bool {
	return strings.Contains(strings.ToLower(userAgent), "mozilla")
}

func parseByteSize(value string) (int64, error) {
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

func directoryEntryURL(requestPath, name string, isDir bool) string {
	entryPath := path.Join(requestPath, name)
	if isDir {
		entryPath += "/"
	}
	return (&url.URL{Path: entryPath}).String()
}

func renderDirectory(w http.ResponseWriter, r *http.Request, rootDir, requestPath string, tmpl *template.Template) {
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
		ProjectURL: ProjectURL,
		Version:    Version,
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
			Name:     entry.Name(),
			URL:      directoryEntryURL(requestPath, entry.Name(), entry.IsDir()),
			Icon:     fileIcon(entry.Name(), entry.IsDir()),
			Size:     size,
			Modified: info.ModTime().Format("2006-01-02 15:04:05"),
			IsDir:    entry.IsDir(),
			ModTime:  info.ModTime(),
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

func main() {
	rootDir := flag.String("root", ".", "directory to serve")
	addr := flag.String("addr", ":8090", "HTTP server address")
	maxTextPreviewSize := int64(4 << 20)
	flag.Func("max-preview-size", "maximum text preview size (default 4M; e.g. 512K or 4194304)", func(value string) error {
		parsed, err := parseByteSize(value)
		if err != nil {
			return err
		}
		maxTextPreviewSize = parsed
		return nil
	})
	showVersion := flag.Bool("version", false, "print version")
	flag.Parse()

	if *showVersion {
		fmt.Printf("peekd %s\n", Version)
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, setting := range info.Settings {
				switch setting.Key {
				case "vcs.revision":
					fmt.Printf("Revision: %s\n", setting.Value)
				case "vcs.time":
					fmt.Printf("Build Time: %s\n", setting.Value)
				}
			}
		}
		return
	}

	var err error
	*rootDir, err = filepath.Abs(*rootDir)
	if err != nil {
		log.Fatalf("failed to resolve root directory: %v", err)
	}

	// 2. Read and parse the HTML template from the embedded file system.
	tmpl, err := template.ParseFS(embeddedFiles, "assets/text.html")
	if err != nil {
		log.Fatalf("failed to parse embedded template: %v", err)
	}
	directoryTmpl, err := template.ParseFS(embeddedFiles, "assets/directory.html")
	if err != nil {
		log.Fatalf("failed to parse directory template: %v", err)
	}
	imageTmpl, err := template.ParseFS(embeddedFiles, "assets/image.html")
	if err != nil {
		log.Fatalf("failed to parse image template: %v", err)
	}
	mediaTmpl, err := template.ParseFS(embeddedFiles, "assets/media.html")
	if err != nil {
		log.Fatalf("failed to parse media template: %v", err)
	}

	// 3. Extract the "assets" subdirectory from the embedded file system
	// so it can be served as HTTP static resources.
	assetsFS, err := fs.Sub(embeddedFiles, "assets")
	if err != nil {
		log.Fatalf("failed to access embedded subdirectory: %v", err)
	}

	// Serve files from the physical disk for large downloads, with sendfile support.
	fileServer := http.FileServer(http.Dir(*rootDir))

	// 4. Provide a dedicated route for frontend CSS resources.
	// The /__peekd_assets/ route maps directly to the assetsFS bundled inside the binary.
	http.Handle("/__peekd_assets/", http.StripPrefix("/__peekd_assets/", http.FileServer(http.FS(assetsFS))))

	// Main route for browsing files.
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		cleanedPath := filepath.Clean(r.URL.Path)
		fullPath := filepath.Join(*rootDir, cleanedPath)

		info, err := os.Stat(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				http.NotFound(w, r)
			} else {
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
			return
		}

		browserUserAgent := isBrowserUserAgent(r.UserAgent())
		if !info.IsDir() && (r.URL.Query().Get("raw") == "1" || !browserUserAgent) {
			fileServer.ServeHTTP(w, r)
			return
		}

		if !info.IsDir() && isImageFile(fullPath) {
			data := previewData{
				FileName:   filepath.Base(fullPath),
				RawURL:     (&url.URL{Path: r.URL.Path, RawQuery: "raw=1"}).String(),
				Size:       formatFileSize(info.Size()),
				Modified:   info.ModTime().Format("2006-01-02 15:04:05"),
				ProjectURL: ProjectURL,
				Version:    Version,
			}

			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if err := imageTmpl.Execute(w, data); err != nil {
				log.Printf("failed to render image preview %s: %v", r.URL.Path, err)
			}
			return
		}

		if !info.IsDir() {
			if kind := mediaKind(fullPath); kind != "" {
				data := previewData{
					FileName:   filepath.Base(fullPath),
					RawURL:     (&url.URL{Path: r.URL.Path, RawQuery: "raw=1"}).String(),
					MediaKind:  kind,
					Size:       formatFileSize(info.Size()),
					Modified:   info.ModTime().Format("2006-01-02 15:04:05"),
					ProjectURL: ProjectURL,
					Version:    Version,
				}

				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				if err := mediaTmpl.Execute(w, data); err != nil {
					log.Printf("failed to render media preview %s: %v", r.URL.Path, err)
				}
				return
			}
		}

		// Read and render text files using the template.
		if !info.IsDir() && isTextFile(fullPath) {
			if info.Size() > maxTextPreviewSize {
				fileServer.ServeHTTP(w, r)
				return
			}

			content, previewable, err := readTextPreview(fullPath, maxTextPreviewSize)
			if err != nil {
				http.Error(w, "unable to read file", http.StatusInternalServerError)
				return
			}
			if !previewable || !utf8.Valid(content) {
				fileServer.ServeHTTP(w, r)
				return
			}

			data := previewData{
				FileName:   filepath.Base(fullPath),
				Lines:      splitLines(string(content)),
				RawURL:     (&url.URL{Path: r.URL.Path, RawQuery: "raw=1"}).String(),
				Size:       formatFileSize(info.Size()),
				Modified:   info.ModTime().Format("2006-01-02 15:04:05"),
				ProjectURL: ProjectURL,
				Version:    Version,
			}

			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			tmpl.Execute(w, data)
			return
		}

		if info.IsDir() {
			renderDirectory(w, r, *rootDir, cleanedPath, directoryTmpl)
			return
		}

		// Let other files use the underlying high-performance sendfile path.
		fileServer.ServeHTTP(w, r)
	})

	listener, err := listenWithFallback(*addr)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", *addr, err)
	}
	defer listener.Close()

	log.Printf("serving %s", *rootDir)
	printServerAddresses(listener.Addr())
	log.Fatal(http.Serve(listener, nil))
}

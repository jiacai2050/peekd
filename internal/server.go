package internal

import (
	"cmp"
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
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jiacai2050/peekd/internal/middleware"
	"github.com/jiacai2050/peekd/internal/preview"
)

type Config struct {
	RootDir            string
	Addr               string
	MaxTextPreviewSize int64
	Version            string
	ProjectURL         string
	EmbeddedFiles      fs.FS
	AuthUsername       string
	AuthPassword       string
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
	HasParent   bool
	Entries     []directoryEntry
	Breadcrumbs []preview.Breadcrumb
	LocalPath   string
	ProjectURL  string
	Version     string
}

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

func previewTypeByContent(contentType string) preview.PreviewType {
	switch {
	case strings.HasPrefix(contentType, "image/"):
		return preview.PreviewTypeImage
	case strings.HasPrefix(contentType, "audio/"):
		return preview.PreviewTypeAudio
	case strings.HasPrefix(contentType, "video/"):
		return preview.PreviewTypeVideo
	case contentType == "application/pdf":
		return preview.PreviewTypePDF
	case contentType == "application/zip":
		return preview.PreviewTypeZIP
	case contentType == "application/x-tar":
		return preview.PreviewTypeTAR
	case contentType == "application/json":
		return preview.PreviewTypeJSON
	case strings.HasPrefix(contentType, "text/"):
		return preview.PreviewTypeText
	default:
		return preview.PreviewTypeNone
	}
}

func isDocumentRequest(r *http.Request) bool {
	dest := r.Header.Get("Sec-Fetch-Dest")
	if dest == "document" {
		return true
	}
	if dest == "" {
		// Some browsers omit Sec-Fetch-Dest for LAN navigations. This header
		// usually appears only on top-level navigations, not subresources.
		return r.Header.Get("Upgrade-Insecure-Requests") == "1"
	}
	return false
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

func directoryEntryURL(requestPath, name string, isDir bool) string {
	entryPath := path.Join(requestPath, name)
	if isDir {
		entryPath += "/"
	}
	return (&url.URL{Path: entryPath}).String()
}

func setCacheHeaders(w http.ResponseWriter, info os.FileInfo, path, version string) {
	w.Header().Set("Cache-Control", "no-cache")
	if info.ModTime().IsZero() {
		return
	}

	modified := info.ModTime().UTC()
	w.Header().Set("Last-Modified", modified.Format(http.TimeFormat))
	w.Header().Set("ETag", fmt.Sprintf("W/\"%s-%s-%x\"", version, path, modified.UnixNano()))
}

func etagMatches(header, etag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || strings.TrimPrefix(candidate, "W/") == strings.TrimPrefix(etag, "W/") {
			return true
		}
	}
	return false
}

func validateETag(w http.ResponseWriter, r *http.Request, etag string) bool {
	w.Header().Set("ETag", etag)
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	return false
}

func validateCache(w http.ResponseWriter, r *http.Request, info os.FileInfo, version string) bool {
	setCacheHeaders(w, info, r.URL.Path, version)
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}

	etag := w.Header().Get("ETag")
	if etag != "" && validateETag(w, r, etag) {
		return true
	}

	// Only use If-Modified-Since when the client does not send an ETag.
	// When both are present, ETag alone decides — If-Modified-Since can
	// incorrectly return 304 for a different resource that has an older
	// modification time (e.g. serving a different directory after restart).
	if r.Header.Get("If-None-Match") == "" {
		if modifiedSince, err := http.ParseTime(r.Header.Get("If-Modified-Since")); err == nil &&
			!info.ModTime().After(modifiedSince.Add(time.Second)) {
			w.WriteHeader(http.StatusNotModified)
			return true
		}
	}
	return false
}

func renderDirectory(w http.ResponseWriter, r *http.Request, rootDir, requestPath string, tmpl *template.Template, projectURL string, version string) {
	fullPath := filepath.Join(rootDir, filepath.FromSlash(requestPath))
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		http.Error(w, "unable to read directory", http.StatusInternalServerError)
		return
	}

	data := directoryData{
		HasParent:   requestPath != "/",
		ProjectURL:  projectURL,
		Version:     version,
		Breadcrumbs: preview.Breadcrumbs(requestPath, true),
		LocalPath:   filepath.Clean(fullPath),
	}

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			http.Error(w, "unable to read directory entry", http.StatusInternalServerError)
			return
		}

		size := "-"
		if !entry.IsDir() {
			size = preview.FormatFileSize(info.Size())
		}
		data.Entries = append(data.Entries, directoryEntry{
			Name:         entry.Name(),
			URL:          directoryEntryURL(requestPath, entry.Name(), entry.IsDir()),
			Icon:         preview.FileIcon(entry.Name(), entry.IsDir()),
			FileModeBits: info.Mode().String(),
			Size:         size,
			Modified:     info.ModTime().Format("2006-01-02 15:04:05"),
			IsDir:        entry.IsDir(),
			ModTime:      info.ModTime(),
		})
	}

	slices.SortFunc(data.Entries, func(a, b directoryEntry) int {
		if c := b.ModTime.Compare(a.ModTime); c != 0 {
			return c // descending by time
		}
		return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
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
	textTemplate, err := template.ParseFS(config.EmbeddedFiles, "assets/text.html", "assets/preview-common.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse embedded template: %w", err)
	}
	codeTemplate, err := template.ParseFS(config.EmbeddedFiles, "assets/code.html", "assets/preview-common.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse code template: %w", err)
	}
	htmlTemplate, err := template.ParseFS(config.EmbeddedFiles, "assets/html.html", "assets/preview-common.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML template: %w", err)
	}
	directoryTemplate, err := template.ParseFS(config.EmbeddedFiles, "assets/directory.html", "assets/preview-common.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse directory template: %w", err)
	}
	imageTemplate, err := template.ParseFS(config.EmbeddedFiles, "assets/image.html", "assets/preview-common.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse image template: %w", err)
	}
	mediaTemplate, err := template.ParseFS(config.EmbeddedFiles, "assets/media.html", "assets/preview-common.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse media template: %w", err)
	}
	jsonTemplate, err := template.ParseFS(config.EmbeddedFiles, "assets/json.html", "assets/preview-common.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON template: %w", err)
	}
	xmlTemplate, err := template.ParseFS(config.EmbeddedFiles, "assets/xml.html", "assets/preview-common.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse XML template: %w", err)
	}
	csvTemplate, err := template.ParseFS(config.EmbeddedFiles, "assets/csv.html", "assets/preview-common.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse CSV template: %w", err)
	}
	pdfTemplate, err := template.ParseFS(config.EmbeddedFiles, "assets/pdf.html", "assets/preview-common.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse PDF template: %w", err)
	}
	archiveTemplate, err := template.ParseFS(config.EmbeddedFiles, "assets/archive.html", "assets/preview-common.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse archive template: %w", err)
	}
	markdownTemplate, err := template.ParseFS(config.EmbeddedFiles, "assets/markdown.html", "assets/preview-common.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse markdown template: %w", err)
	}
	mobiTemplate, err := template.ParseFS(config.EmbeddedFiles, "assets/mobi.html", "assets/preview-common.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse MOBI template: %w", err)
	}

	assetsFS, err := fs.Sub(config.EmbeddedFiles, "assets")
	if err != nil {
		return nil, fmt.Errorf("failed to access embedded subdirectory: %w", err)
	}

	fileServer := http.FileServer(http.Dir(config.RootDir))
	previewConfig := preview.Config{
		ProjectURL: config.ProjectURL,
		Version:    config.Version,
	}
	assetServer := http.StripPrefix("/__peekd_assets/", http.FileServer(http.FS(assetsFS)))
	mux := http.NewServeMux()
	mux.Handle("/__peekd_assets/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		assetETag := fmt.Sprintf("\"asset-%x-%x\"", config.Version, r.URL.Path)
		if validateETag(w, r, assetETag) {
			return
		}
		assetServer.ServeHTTP(w, r)
	}))
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

		if validateCache(w, r, info, config.Version) {
			return
		}
		if info.IsDir() {
			renderDirectory(w, r, config.RootDir, cleanedPath, directoryTemplate, config.ProjectURL, config.Version)
			return
		}

		if r.URL.Query().Get("raw") == "1" || !isDocumentRequest(r) {
			fileServer.ServeHTTP(w, r)
			return
		}

		previewKind := preview.PreviewTypeByExtension(fullPath)
		if previewKind == preview.PreviewTypeNone {
			contentType, detectErr := detectContentType(fullPath)
			if detectErr != nil {
				http.Error(w, "unable to inspect file", http.StatusInternalServerError)
				return
			}
			previewKind = previewTypeByContent(contentType)
		}

		prepared, preparedPreviewType, prepareErr := preview.PrepareTextPreview(previewKind, fullPath, info.Size(), config.MaxTextPreviewSize)
		if prepareErr != nil {
			http.Error(w, "unable to read file", http.StatusInternalServerError)
			return
		}
		if preparedPreviewType == preview.PreviewTypeNone {
			fileServer.ServeHTTP(w, r)
			return
		}
		previewKind = preparedPreviewType

		switch previewKind {
		case preview.PreviewTypeImage:
			if err := preview.RenderImagePreview(w, imageTemplate, r.URL.Path, fullPath, info, previewConfig); err != nil {
				log.Printf("failed to render image preview %s: %v", r.URL.Path, err)
			}

		case preview.PreviewTypeAudio, preview.PreviewTypeVideo:
			if err := preview.RenderMediaPreview(w, mediaTemplate, r.URL.Path, fullPath, info, previewConfig, previewKind); err != nil {
				log.Printf("failed to render media preview %s: %v", r.URL.Path, err)
			}

		case preview.PreviewTypePDF:
			if err := preview.RenderPDFPreview(w, pdfTemplate, r.URL.Path, fullPath, info, previewConfig); err != nil {
				log.Printf("failed to render PDF preview %s: %v", r.URL.Path, err)
			}

		case preview.PreviewTypeZIP, preview.PreviewTypeTAR, preview.PreviewTypeTARGZ:
			if err := preview.RenderArchivePreview(w, archiveTemplate, r.URL.Path, fullPath, info, previewConfig); err != nil {
				fileServer.ServeHTTP(w, r)
			}

		case preview.PreviewTypeHTML:
			if err := preview.RenderHTMLPreview(w, htmlTemplate, r.URL.Path, fullPath, info, previewConfig, prepared.Formatted); err != nil {
				log.Printf("failed to render HTML preview %s: %v", r.URL.Path, err)
			}

		case preview.PreviewTypeJSON:
			if err := preview.RenderJSONPreview(w, jsonTemplate, r.URL.Path, fullPath, info, previewConfig, prepared.Formatted); err != nil {
				log.Printf("failed to render JSON preview %s: %v", r.URL.Path, err)
			}

		case preview.PreviewTypeXML:
			if err := preview.RenderXMLPreview(w, xmlTemplate, r.URL.Path, fullPath, info, previewConfig, prepared.Formatted); err != nil {
				log.Printf("failed to render XML preview %s: %v", r.URL.Path, err)
			}

		case preview.PreviewTypeMarkdown:
			if err := preview.RenderMarkdownPreview(w, markdownTemplate, r.URL.Path, fullPath, info, previewConfig, prepared.Formatted); err != nil {
				log.Printf("failed to render markdown preview %s: %v", r.URL.Path, err)
			}

		case preview.PreviewTypeText:
			if err := preview.RenderTextPreview(w, textTemplate, r.URL.Path, fullPath, info, previewConfig, prepared.Formatted); err != nil {
				log.Printf("failed to render text preview %s: %v", r.URL.Path, err)
			}
		case preview.PreviewTypeCode:
			if err := preview.RenderTextPreview(w, codeTemplate, r.URL.Path, fullPath, info, previewConfig, prepared.Formatted); err != nil {
				log.Printf("failed to render text preview %s: %v", r.URL.Path, err)
			}

		case preview.PreviewTypeCSV, preview.PreviewTypeTSV:
			if err := preview.RenderCSVPreview(w, csvTemplate, r.URL.Path, fullPath, info, previewConfig, prepared.Rows); err != nil {
				log.Printf("failed to render delimited preview %s: %v", r.URL.Path, err)
			}

		case preview.PreviewTypeMOBI:
			if err := preview.RenderMOBIPreview(w, mobiTemplate, r.URL.Path, fullPath, info, previewConfig); err != nil {
				log.Printf("failed to render MOBI preview %s: %v", r.URL.Path, err)
			}

		default:
			fileServer.ServeHTTP(w, r)
		}
	})

	authMiddleware, err := middleware.NewBasicAuth(config.AuthUsername, config.AuthPassword)
	if err != nil {
		return nil, err
	}
	// Requests pass through access logging, then Basic Auth, then the mux.
	// Responses return in reverse order, so access logging records the result last.
	return middleware.WithAccessLog(authMiddleware(mux)), nil
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

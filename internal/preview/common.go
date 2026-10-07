package preview

import (
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// PreviewTypeByExtension returns the preview type for a file path based on its extension.
func PreviewTypeByExtension(filePath string) PreviewType {
	lowerPath := strings.ToLower(filePath)
	switch {
	case strings.HasSuffix(lowerPath, ".tar.gz"), strings.HasSuffix(lowerPath, ".tgz"):
		return PreviewTypeTARGZ
	}

	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".md", ".markdown":
		return PreviewTypeMarkdown
	case ".org":
		return PreviewTypeOrg
	case ".avif", ".bmp", ".gif", ".ico", ".jpeg", ".jpg", ".png", ".svg", ".tif", ".tiff", ".webp":
		return PreviewTypeImage
	case ".aac", ".flac", ".m4a", ".mp3", ".oga", ".ogg", ".opus", ".wav", ".weba":
		return PreviewTypeAudio
	case ".avi", ".m4v", ".mkv", ".mov", ".mp4", ".ogv", ".webm":
		return PreviewTypeVideo
	case ".pdf":
		return PreviewTypePDF
	case ".zip":
		return PreviewTypeZIP
	case ".tar":
		return PreviewTypeTAR
	case ".json":
		return PreviewTypeJSON
	case ".xml":
		return PreviewTypeXML
	case ".html", ".htm":
		return PreviewTypeHTML
	case ".csv":
		return PreviewTypeCSV
	case ".tsv":
		return PreviewTypeTSV
	case ".mobi":
		return PreviewTypeMOBI
	case ".txt", ".log", ".conf", ".ini", ".properties", ".rst":
		return PreviewTypeText
	case ".jsonc", ".yaml", ".yml", ".toml",
		".css", ".scss", ".sass", ".less",
		".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx",
		".vue", ".svelte",
		".go", ".rs", ".zig", ".py", ".rb", ".php",
		".java", ".kt", ".kts", ".swift",
		".c", ".h", ".cc", ".cpp", ".cxx", ".hpp",
		".cs", ".fs", ".fsx",
		".sh", ".bash", ".zsh", ".fish", ".ps1",
		".sql":
		return PreviewTypeCode
	default:
		switch strings.ToLower(filepath.Base(filePath)) {
		case "makefile", "dockerfile", "jenkinsfile", "justfile":
			return PreviewTypeCode
		case "license":
			return PreviewTypeText
		default:
			return PreviewTypeNone
		}
	}
}

// PreviewTypeByContent maps a detected MIME type to a preview type.
func PreviewTypeByContent(contentType string) PreviewType {
	switch {
	case strings.HasPrefix(contentType, "image/"):
		return PreviewTypeImage
	case strings.HasPrefix(contentType, "audio/"):
		return PreviewTypeAudio
	case strings.HasPrefix(contentType, "video/"):
		return PreviewTypeVideo
	case contentType == "application/pdf":
		return PreviewTypePDF
	case contentType == "application/zip":
		return PreviewTypeZIP
	case contentType == "application/x-tar":
		return PreviewTypeTAR
	case contentType == "application/json":
		return PreviewTypeJSON
	case strings.HasPrefix(contentType, "text/"):
		return PreviewTypeText
	default:
		return PreviewTypeNone
	}
}

type Breadcrumb struct {
	Name    string
	URL     string
	Current bool
}

type Config struct {
	ProjectURL  string
	Version     string
	CurrentRoot int
}

// PreviewCommon holds fields shared by every preview data struct.
type PreviewCommon struct {
	FileName    string
	Icon        string
	ExtraMeta   string
	Size        string
	Modified    string
	RawURL      string
	Breadcrumbs []Breadcrumb
	LocalPath   string
	ProjectURL  string
	Version     string
	CurrentRoot int
}

func FileIcon(path string, isDir bool) string {
	if isDir {
		return "📁"
	}

	switch PreviewTypeByExtension(path) {
	case PreviewTypeImage:
		return "🖼️"
	case PreviewTypeAudio:
		return "🎵"
	case PreviewTypeVideo:
		return "🎬"
	case PreviewTypeText, PreviewTypeCode, PreviewTypeMarkdown, PreviewTypeOrg, PreviewTypeCSV, PreviewTypeTSV:
		return "📄"
	case PreviewTypeHTML:
		return "🌐"
	case PreviewTypeMOBI:
		return "📚"
	case PreviewTypeJSON:
		return "🧾"
	case PreviewTypeXML:
		return "🧾"
	case PreviewTypePDF:
		return "📚"
	case PreviewTypeZIP, PreviewTypeTAR, PreviewTypeTARGZ:
		return "📦"
	default:
		return "📄"
	}
}

func newPreviewCommon(requestPath, filePath string, info os.FileInfo, config Config, extraMeta string) PreviewCommon {
	rootParam := ""
	if config.CurrentRoot > 0 {
		rootParam = strconv.Itoa(config.CurrentRoot)
	}
	return PreviewCommon{
		FileName:    previewFileName(filePath),
		Icon:        FileIcon(filePath, info.IsDir()),
		ExtraMeta:   extraMeta,
		Size:        previewFileSize(info),
		Modified:    previewModified(info),
		RawURL:      previewRawURL(requestPath, rootParam),
		Breadcrumbs: Breadcrumbs(requestPath, false, rootParam),
		LocalPath:   previewLocalPath(filePath),
		ProjectURL:  config.ProjectURL,
		Version:     config.Version,
		CurrentRoot: config.CurrentRoot,
	}
}

func executeTemplate(w http.ResponseWriter, tmpl *template.Template, data any) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return tmpl.Execute(w, data)
}

func Breadcrumbs(requestPath string, isDir bool, rootParam ...string) []Breadcrumb {
	root := ""
	if len(rootParam) > 0 {
		root = rootParam[0]
	}

	peekdURL := "/"
	if root != "" {
		peekdURL = "/?root=" + url.QueryEscape(root)
	}
	breadcrumbs := []Breadcrumb{{Name: "Peekd", URL: peekdURL, Current: strings.Trim(requestPath, "/") == "" && isDir}}
	parts := strings.Split(strings.Trim(requestPath, "/"), "/")
	if len(parts) == 1 && parts[0] == "" {
		return breadcrumbs
	}

	currentPath := "/"
	for index, part := range parts {
		currentPath = path.Join(currentPath, part)
		current := index == len(parts)-1
		p := currentPath
		if !current || isDir {
			p += "/"
		}
		u := &url.URL{Path: p}
		if root != "" {
			u.RawQuery = url.Values{"root": []string{root}}.Encode()
		}
		breadcrumbs = append(breadcrumbs, Breadcrumb{
			Name:    part,
			URL:     u.String(),
			Current: current,
		})
	}
	return breadcrumbs
}

type PreviewType string

const (
	PreviewTypeNone     PreviewType = ""
	PreviewTypeText     PreviewType = "text"
	PreviewTypeCode     PreviewType = "code"
	PreviewTypeHTML     PreviewType = "html"
	PreviewTypeMarkdown PreviewType = "markdown"
	PreviewTypeOrg      PreviewType = "org"
	PreviewTypeImage    PreviewType = "image"
	PreviewTypeAudio    PreviewType = "audio"
	PreviewTypeVideo    PreviewType = "video"
	PreviewTypePDF      PreviewType = "pdf"
	PreviewTypeJSON     PreviewType = "json"
	PreviewTypeXML      PreviewType = "xml"
	PreviewTypeCSV      PreviewType = "csv"
	PreviewTypeTSV      PreviewType = "tsv"
	PreviewTypeZIP      PreviewType = "zip"
	PreviewTypeTAR      PreviewType = "tar"
	PreviewTypeTARGZ    PreviewType = "tar-gz"
	PreviewTypeMOBI     PreviewType = "mobi"
)

func previewRawURL(requestPath, rootParam string) string {
	q := url.Values{"raw": []string{"1"}}
	if rootParam != "" {
		q.Set("root", rootParam)
	}
	return (&url.URL{Path: requestPath, RawQuery: q.Encode()}).String()
}

func previewFileName(filePath string) string {
	return filepath.Base(filePath)
}

func previewLocalPath(filePath string) string {
	return filepath.Clean(filePath)
}

func previewModified(info os.FileInfo) string {
	return info.ModTime().Format("2006-01-02 15:04:05")
}

func previewFileSize(info os.FileInfo) string {
	return FormatFileSize(info.Size())
}

func FormatFileSize(size int64) string {
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

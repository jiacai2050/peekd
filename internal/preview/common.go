package preview

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type Breadcrumb struct {
	Name    string
	URL     string
	Current bool
}

type Config struct {
	ProjectURL string
	Version    string
}

func Breadcrumbs(requestPath string, isDir bool) []Breadcrumb {
	breadcrumbs := []Breadcrumb{{Name: "Peekd", URL: "/", Current: strings.Trim(requestPath, "/") == "" && isDir}}
	parts := strings.Split(strings.Trim(requestPath, "/"), "/")
	if len(parts) == 1 && parts[0] == "" {
		return breadcrumbs
	}

	currentPath := "/"
	for index, part := range parts {
		currentPath = path.Join(currentPath, part)
		current := index == len(parts)-1
		breadcrumbURL := (&url.URL{Path: currentPath}).String()
		if !current || isDir {
			breadcrumbURL += "/"
		}
		breadcrumbs = append(breadcrumbs, Breadcrumb{
			Name:    part,
			URL:     breadcrumbURL,
			Current: current,
		})
	}
	return breadcrumbs
}

type PreviewType string

const (
	PreviewTypeNone     PreviewType = ""
	PreviewTypeText     PreviewType = "text"
	PreviewTypeHTML     PreviewType = "html"
	PreviewTypeMarkdown PreviewType = "markdown"
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
)

func previewRawURL(requestPath string) string {
	return (&url.URL{Path: requestPath, RawQuery: "raw=1"}).String()
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

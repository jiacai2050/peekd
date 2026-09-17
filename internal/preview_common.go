package internal

import (
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type breadcrumb struct {
	Name    string
	URL     string
	Current bool
}

func previewBreadcrumbs(requestPath string, isDir bool) []breadcrumb {
	breadcrumbs := []breadcrumb{{Name: "Peekd", URL: "/", Current: strings.Trim(requestPath, "/") == "" && isDir}}
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
		breadcrumbs = append(breadcrumbs, breadcrumb{
			Name:    part,
			URL:     breadcrumbURL,
			Current: current,
		})
	}
	return breadcrumbs
}

type previewType string

const (
	previewTypeNone     previewType = ""
	previewTypeText     previewType = "text"
	previewTypeHTML     previewType = "html"
	previewTypeMarkdown previewType = "markdown"
	previewTypeImage    previewType = "image"
	previewTypeAudio    previewType = "audio"
	previewTypeVideo    previewType = "video"
	previewTypePDF      previewType = "pdf"
	previewTypeJSON     previewType = "json"
	previewTypeCSV      previewType = "csv"
	previewTypeTSV      previewType = "tsv"
	previewTypeZIP      previewType = "zip"
	previewTypeTAR      previewType = "tar"
	previewTypeTARGZ    previewType = "tar-gz"
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
	return formatFileSize(info.Size())
}

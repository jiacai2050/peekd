package internal

import (
	"net/url"
	"os"
	"path/filepath"
)

type previewType string

const (
	previewTypeNone     previewType = ""
	previewTypeText     previewType = "text"
	previewTypeMarkdown previewType = "markdown"
	previewTypeImage    previewType = "image"
	previewTypeAudio    previewType = "audio"
	previewTypeVideo    previewType = "video"
	previewTypePDF      previewType = "pdf"
	previewTypeJSON     previewType = "json"
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

func previewModified(info os.FileInfo) string {
	return info.ModTime().Format("2006-01-02 15:04:05")
}

func previewFileSize(info os.FileInfo) string {
	return formatFileSize(info.Size())
}

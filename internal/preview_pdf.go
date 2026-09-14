package internal

import (
	"html/template"
	"net/http"
	"os"
)

type pdfPreviewData struct {
	FileName   string
	RawURL     string
	Size       string
	Modified   string
	ProjectURL string
	Version    string
}

func renderPDFPreview(w http.ResponseWriter, tmpl *template.Template, requestPath, filePath string, info os.FileInfo, config Config) error {
	data := pdfPreviewData{
		FileName:   previewFileName(filePath),
		RawURL:     previewRawURL(requestPath),
		Size:       previewFileSize(info),
		Modified:   previewModified(info),
		ProjectURL: config.ProjectURL,
		Version:    config.Version,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return tmpl.Execute(w, data)
}

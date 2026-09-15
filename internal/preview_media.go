package internal

import (
	"html/template"
	"net/http"
	"os"
)

type mediaPreviewData struct {
	FileName    string
	RawURL      string
	MediaKind   previewType
	Size        string
	Modified    string
	Breadcrumbs []breadcrumb
	ProjectURL  string
	Version     string
}

func renderMediaPreview(w http.ResponseWriter, tmpl *template.Template, requestPath, filePath string, info os.FileInfo, config Config, mediaKind previewType) error {
	data := mediaPreviewData{
		FileName:    previewFileName(filePath),
		RawURL:      previewRawURL(requestPath),
		MediaKind:   mediaKind,
		Size:        previewFileSize(info),
		Modified:    previewModified(info),
		Breadcrumbs: previewBreadcrumbs(requestPath, false),
		ProjectURL:  config.ProjectURL,
		Version:     config.Version,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return tmpl.Execute(w, data)
}

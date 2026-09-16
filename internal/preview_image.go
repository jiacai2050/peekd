package internal

import (
	"html/template"
	"net/http"
	"os"
)

type imagePreviewData struct {
	FileName    string
	RawURL      string
	Size        string
	Modified    string
	Breadcrumbs []breadcrumb
	LocalPath   string
	ProjectURL  string
	Version     string
}

func renderImagePreview(w http.ResponseWriter, tmpl *template.Template, requestPath, filePath string, info os.FileInfo, config Config) error {
	data := imagePreviewData{
		FileName:    previewFileName(filePath),
		RawURL:      previewRawURL(requestPath),
		Size:        previewFileSize(info),
		Modified:    previewModified(info),
		Breadcrumbs: previewBreadcrumbs(requestPath, false),
		LocalPath:   previewLocalPath(filePath),
		ProjectURL:  config.ProjectURL,
		Version:     config.Version,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return tmpl.Execute(w, data)
}

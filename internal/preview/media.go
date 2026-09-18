package preview

import (
	"html/template"
	"net/http"
	"os"
)

type mediaPreviewData struct {
	FileName    string
	RawURL      string
	MediaKind   PreviewType
	Size        string
	Modified    string
	Breadcrumbs []Breadcrumb
	LocalPath   string
	ProjectURL  string
	Version     string
}

func RenderMediaPreview(w http.ResponseWriter, tmpl *template.Template, requestPath, filePath string, info os.FileInfo, config Config, mediaKind PreviewType) error {
	data := mediaPreviewData{
		FileName:    previewFileName(filePath),
		RawURL:      previewRawURL(requestPath),
		MediaKind:   mediaKind,
		Size:        previewFileSize(info),
		Modified:    previewModified(info),
		Breadcrumbs: Breadcrumbs(requestPath, false),
		LocalPath:   previewLocalPath(filePath),
		ProjectURL:  config.ProjectURL,
		Version:     config.Version,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return tmpl.Execute(w, data)
}

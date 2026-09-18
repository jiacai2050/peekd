package preview

import (
	"html/template"
	"net/http"
	"os"
)

type htmlPreviewData struct {
	FileName    string
	Content     string
	RawURL      string
	Size        string
	Modified    string
	Breadcrumbs []Breadcrumb
	LocalPath   string
	ProjectURL  string
	Version     string
}

func RenderHTMLPreview(w http.ResponseWriter, tmpl *template.Template, requestPath, filePath string, info os.FileInfo, config Config, content string) error {
	data := htmlPreviewData{
		FileName:    previewFileName(filePath),
		Content:     content,
		RawURL:      previewRawURL(requestPath),
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

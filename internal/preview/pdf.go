package preview

import (
	"html/template"
	"net/http"
	"os"
)

type pdfPreviewData struct {
	FileName    string
	RawURL      string
	Size        string
	Modified    string
	Breadcrumbs []Breadcrumb
	LocalPath   string
	ProjectURL  string
	Version     string
}

func RenderPDFPreview(w http.ResponseWriter, tmpl *template.Template, requestPath, filePath string, info os.FileInfo, config Config) error {
	data := pdfPreviewData{
		FileName:    previewFileName(filePath),
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

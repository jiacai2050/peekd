package internal

import (
	"bytes"
	"encoding/json"
	"html/template"
	"net/http"
	"os"
)

type jsonPreviewData struct {
	FileName    string
	Content     string
	RawURL      string
	Size        string
	Modified    string
	Breadcrumbs []breadcrumb
	LocalPath   string
	ProjectURL  string
	Version     string
}

func formatJSON(content []byte) (string, error) {
	var output bytes.Buffer
	if err := json.Indent(&output, content, "", "  "); err != nil {
		return "", err
	}
	return output.String(), nil
}

func renderJSONPreview(w http.ResponseWriter, tmpl *template.Template, requestPath, filePath string, info os.FileInfo, config Config, content string) error {
	data := jsonPreviewData{
		FileName:    previewFileName(filePath),
		Content:     content,
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

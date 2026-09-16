package internal

import (
	"bytes"
	"html/template"
	"net/http"
	"os"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

type markdownPreviewData struct {
	FileName    string
	HTML        template.HTML
	RawURL      string
	Size        string
	Modified    string
	Breadcrumbs []breadcrumb
	LocalPath   string
	ProjectURL  string
	Version     string
}

var markdownRenderer = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
)

func renderMarkdown(content []byte) (template.HTML, error) {
	var output bytes.Buffer
	if err := markdownRenderer.Convert(content, &output); err != nil {
		return "", err
	}
	return template.HTML(output.String()), nil
}

func renderMarkdownPreview(w http.ResponseWriter, tmpl *template.Template, requestPath, filePath string, info os.FileInfo, config Config, content []byte) error {
	markdownHTML, err := renderMarkdown(content)
	if err != nil {
		return err
	}
	data := markdownPreviewData{
		FileName:    previewFileName(filePath),
		HTML:        markdownHTML,
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

package internal

import (
	"html/template"
	"io"
	"net/http"
	"os"
	"strings"
	"unicode/utf8"
)

type textPreviewData struct {
	FileName    string
	Lines       []string
	RawURL      string
	Size        string
	Modified    string
	Breadcrumbs []breadcrumb
	ProjectURL  string
	Version     string
}

func readTextPreview(filePath string, maxSize int64) ([]byte, bool, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()

	content, err := io.ReadAll(io.LimitReader(file, maxSize+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(content)) > maxSize {
		return nil, false, nil
	}
	return content, true, nil
}

func readPreviewContent(filePath string, fileSize, maxSize int64) ([]byte, bool, error) {
	if fileSize > maxSize {
		return nil, false, nil
	}

	content, previewable, err := readTextPreview(filePath, maxSize)
	if err != nil || !previewable || !utf8.Valid(content) {
		return content, false, err
	}
	return content, true, nil
}

func splitLines(content string) []string {
	lines := strings.Split(content, "\n")
	if len(lines) > 1 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func renderTextPreview(w http.ResponseWriter, tmpl *template.Template, requestPath, filePath string, info os.FileInfo, config Config, content []byte) error {
	data := textPreviewData{
		FileName:    previewFileName(filePath),
		Lines:       splitLines(string(content)),
		RawURL:      previewRawURL(requestPath),
		Size:        previewFileSize(info),
		Modified:    previewModified(info),
		Breadcrumbs: previewBreadcrumbs(requestPath, false),
		ProjectURL:  config.ProjectURL,
		Version:     config.Version,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return tmpl.Execute(w, data)
}

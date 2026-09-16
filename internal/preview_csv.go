package internal

import (
	"encoding/csv"
	"html/template"
	"io"
	"net/http"
	"os"
	"strings"
)

const (
	csvPreviewMaxRows    = 1000
	csvPreviewMaxColumns = 50
)

type csvPreviewData struct {
	FileName    string
	Rows        [][]string
	Truncated   bool
	RawURL      string
	Size        string
	Modified    string
	Breadcrumbs []breadcrumb
	LocalPath   string
	ProjectURL  string
	Version     string
}

func parseCSVPreview(content []byte, delimiter rune) ([][]string, bool, error) {
	reader := csv.NewReader(strings.NewReader(string(content)))
	reader.Comma = delimiter
	reader.FieldsPerRecord = -1

	rows := make([][]string, 0, csvPreviewMaxRows)
	truncated := false
	for len(rows) < csvPreviewMaxRows {
		record, err := reader.Read()
		if err == io.EOF {
			return rows, truncated, nil
		}
		if err != nil {
			return nil, false, err
		}

		if len(record) > csvPreviewMaxColumns {
			record = record[:csvPreviewMaxColumns]
			truncated = true
		}
		rows = append(rows, record)
	}

	if _, err := reader.Read(); err == nil {
		truncated = true
	} else if err != io.EOF {
		return nil, false, err
	}
	return rows, truncated, nil
}

func renderCSVPreview(w http.ResponseWriter, tmpl *template.Template, requestPath, filePath string, info os.FileInfo, config Config, content []byte, delimiter rune) error {
	rows, truncated, err := parseCSVPreview(content, delimiter)
	if err != nil {
		return err
	}
	data := csvPreviewData{
		FileName:    previewFileName(filePath),
		Rows:        rows,
		Truncated:   truncated,
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

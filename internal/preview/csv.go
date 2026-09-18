package preview

import (
	"bytes"
	"encoding/csv"
	"html/template"
	"io"
	"net/http"
	"os"
)

type csvPreviewData struct {
	FileName    string
	Rows        [][]string
	RawURL      string
	Size        string
	Modified    string
	Breadcrumbs []Breadcrumb
	LocalPath   string
	ProjectURL  string
	Version     string
}

func ParseCSVPreview(content []byte, delimiter rune) ([][]string, error) {
	reader := csv.NewReader(bytes.NewReader(content))
	reader.Comma = delimiter
	reader.FieldsPerRecord = -1

	var rows [][]string
	for {
		record, err := reader.Read()
		if err == io.EOF {
			return rows, nil
		}
		if err != nil {
			return nil, err
		}
		rows = append(rows, record)
	}
}

func RenderCSVPreview(w http.ResponseWriter, tmpl *template.Template, requestPath, filePath string, info os.FileInfo, config Config, rows [][]string) error {
	data := csvPreviewData{
		FileName:    previewFileName(filePath),
		Rows:        rows,
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

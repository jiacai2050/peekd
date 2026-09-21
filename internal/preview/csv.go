package preview

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
)

type csvPreviewData struct {
	PreviewCommon
	Rows [][]string
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
	return executeTemplate(w, tmpl, csvPreviewData{
		PreviewCommon: newPreviewCommon(requestPath, filePath, info, config, fmt.Sprintf("%d rows", len(rows))),
		Rows:          rows,
	})
}

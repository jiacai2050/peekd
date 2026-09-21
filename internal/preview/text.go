package preview

import (
	"html/template"
	"io"
	"net/http"
	"os"
	"strings"
	"unicode/utf8"
)

type textPreviewData struct {
	PreviewCommon
	Lines []string
}

type PreparedTextPreview struct {
	// Formatted is used by text, HTML, JSON, XML, and Markdown previews.
	Formatted string
	// Rows is used by CSV and TSV previews after parsing.
	Rows [][]string
}

func formatPreviewContent(preview PreviewType, content []byte) (PreparedTextPreview, error) {
	var prepared PreparedTextPreview
	var err error
	switch preview {
	case PreviewTypeHTML, PreviewTypeText:
		prepared.Formatted = string(content)
	case PreviewTypeJSON:
		prepared.Formatted, err = formatJSON(content)
	case PreviewTypeXML:
		prepared.Formatted, err = formatXML(content)
	case PreviewTypeMarkdown:
		prepared.Formatted, err = RenderMarkdown(content)
	case PreviewTypeCSV:
		prepared.Formatted = string(content)
		prepared.Rows, err = ParseCSVPreview(content, ',')
	case PreviewTypeTSV:
		prepared.Formatted = string(content)
		prepared.Rows, err = ParseCSVPreview(content, '\t')
	default:
		return prepared, nil
	}
	return prepared, err
}

func ReadTextPreview(filePath string, maxSize int64) ([]byte, bool, error) {
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

func PrepareTextPreview(preview PreviewType, filePath string, fileSize, maxSize int64) (PreparedTextPreview, PreviewType, error) {
	if !previewNeedsContent(preview) {
		return PreparedTextPreview{}, preview, nil
	}
	if fileSize > maxSize {
		return PreparedTextPreview{}, PreviewTypeNone, nil
	}

	content, previewable, err := ReadTextPreview(filePath, maxSize)
	if err != nil || !previewable || !utf8.Valid(content) {
		return PreparedTextPreview{}, PreviewTypeNone, err
	}

	prepared, err := formatPreviewContent(preview, content)
	if err != nil {
		return PreparedTextPreview{Formatted: string(content)}, PreviewTypeText, nil
	}
	return prepared, preview, nil
}

func previewNeedsContent(preview PreviewType) bool {
	switch preview {
	case PreviewTypeHTML, PreviewTypeCSV, PreviewTypeTSV, PreviewTypeJSON, PreviewTypeXML, PreviewTypeText, PreviewTypeMarkdown:
		return true
	default:
		return false
	}
}

func splitLines(content string) []string {
	lines := strings.Split(content, "\n")
	if len(lines) > 1 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func RenderTextPreview(w http.ResponseWriter, tmpl *template.Template, requestPath, filePath string, info os.FileInfo, config Config, content string) error {
	return executeTemplate(w, tmpl, textPreviewData{
		PreviewCommon: newPreviewCommon(requestPath, filePath, info, config, ""),
		Lines:         splitLines(content),
	})
}

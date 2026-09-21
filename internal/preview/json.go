package preview

import (
	"bytes"
	"encoding/json"
	"html/template"
	"net/http"
	"os"
)

type jsonPreviewData struct {
	PreviewCommon
	Content string
}

func formatJSON(content []byte) (string, error) {
	var output bytes.Buffer
	if err := json.Indent(&output, content, "", "  "); err != nil {
		return "", err
	}
	return output.String(), nil
}

func RenderJSONPreview(w http.ResponseWriter, tmpl *template.Template, requestPath, filePath string, info os.FileInfo, config Config, content string) error {
	return executeTemplate(w, tmpl, jsonPreviewData{
		PreviewCommon: newPreviewCommon(requestPath, filePath, info, config, ""),
		Content:       content,
	})
}

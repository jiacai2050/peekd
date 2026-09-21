package preview

import (
	"html/template"
	"net/http"
	"os"
)

type htmlPreviewData struct {
	PreviewCommon
	Content string
}

func RenderHTMLPreview(w http.ResponseWriter, tmpl *template.Template, requestPath, filePath string, info os.FileInfo, config Config, content string) error {
	return executeTemplate(w, tmpl, htmlPreviewData{
		PreviewCommon: newPreviewCommon(requestPath, filePath, info, config, ""),
		Content:       content,
	})
}

package preview

import (
	"html/template"
	"net/http"
	"os"
)

type imagePreviewData struct {
	PreviewCommon
}

func RenderImagePreview(w http.ResponseWriter, tmpl *template.Template, requestPath, filePath string, info os.FileInfo, config Config) error {
	return executeTemplate(w, tmpl, imagePreviewData{
		PreviewCommon: newPreviewCommon(requestPath, filePath, info, config, "🖼️", ""),
	})
}

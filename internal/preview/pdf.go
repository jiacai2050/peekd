package preview

import (
	"html/template"
	"net/http"
	"os"
)

type pdfPreviewData struct {
	PreviewCommon
}

func RenderPDFPreview(w http.ResponseWriter, tmpl *template.Template, requestPath, filePath string, info os.FileInfo, config Config) error {
	return executeTemplate(w, tmpl, pdfPreviewData{
		PreviewCommon: newPreviewCommon(requestPath, filePath, info, config, ""),
	})
}

package preview

import (
	"html/template"
	"net/http"
	"os"
)

type mediaPreviewData struct {
	PreviewCommon
	MediaKind PreviewType
}

func RenderMediaPreview(w http.ResponseWriter, tmpl *template.Template, requestPath, filePath string, info os.FileInfo, config Config, mediaKind PreviewType) error {
	icon := "🎬"
	if mediaKind == PreviewTypeAudio {
		icon = "🎵"
	}
	return executeTemplate(w, tmpl, mediaPreviewData{
		PreviewCommon: newPreviewCommon(requestPath, filePath, info, config, icon, ""),
		MediaKind:     mediaKind,
	})
}

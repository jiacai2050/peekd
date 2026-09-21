package preview

import (
	"bytes"
	"html/template"
	"net/http"
	"os"

	diagram "github.com/yuin/goldmark-diagram"
	"github.com/yuin/goldmark/v2/extension"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/renderer/html"
)

type markdownPreviewData struct {
	PreviewCommon
	HTML template.HTML
}

var markdownParser = parser.New(
	parser.WithExtensions(
		extension.GFMParser,
		extension.FootnoteParser,
		extension.DefinitionListParser,
	),
	parser.WithEscapedSpace(),
)
var markdownRenderer = html.New(html.WithExtensions(
	extension.GFMHTMLRenderer,
	extension.FootnoteHTMLRenderer,
	extension.DefinitionListHTMLRenderer,
	diagram.NewHTMLRenderer(diagram.WithRenderer(
		diagram.LanguageMermaid,
		diagram.NewMermaidClientRenderer(diagram.WithMermaidUMDURL(
			"https://cdn.jsdelivr.net/npm/mermaid/dist/mermaid.min.js",
		)),
	)),
), html.WithLineBreakStrategy(html.SimpleEastAsianLineBreakStrategy))

func RenderMarkdown(content []byte) (string, error) {
	var output bytes.Buffer
	node := markdownParser.Parse(content)
	if err := markdownRenderer.Render(&output, content, node); err != nil {
		return "", err
	}
	return output.String(), nil
}

func RenderMarkdownPreview(w http.ResponseWriter, tmpl *template.Template, requestPath, filePath string, info os.FileInfo, config Config, markdownHTML string) error {
	return executeTemplate(w, tmpl, markdownPreviewData{
		PreviewCommon: newPreviewCommon(requestPath, filePath, info, config, ""),
		HTML:          template.HTML(markdownHTML),
	})
}

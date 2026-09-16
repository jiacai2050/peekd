package internal

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
	FileName    string
	HTML        template.HTML
	RawURL      string
	Size        string
	Modified    string
	Breadcrumbs []breadcrumb
	LocalPath   string
	ProjectURL  string
	Version     string
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

func renderMarkdown(content []byte) (template.HTML, error) {
	var output bytes.Buffer
	node := markdownParser.Parse(content)
	if err := markdownRenderer.Render(&output, content, node); err != nil {
		return "", err
	}
	return template.HTML(output.String()), nil
}

func renderMarkdownPreview(w http.ResponseWriter, tmpl *template.Template, requestPath, filePath string, info os.FileInfo, config Config, content []byte) error {
	markdownHTML, err := renderMarkdown(content)
	if err != nil {
		return err
	}
	data := markdownPreviewData{
		FileName:    previewFileName(filePath),
		HTML:        markdownHTML,
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

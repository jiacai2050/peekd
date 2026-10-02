package preview

import (
	"bytes"
	"fmt"
	"html"
	"html/template"
	"net/http"
	"os"
	"strings"

	"github.com/niklasfasching/go-org/org"
)

type orgPreviewData struct {
	PreviewCommon
	HTML template.HTML
}

// RenderOrg converts Org mode content into HTML.
func RenderOrg(content []byte) (string, error) {
	writer := org.NewHTMLWriter()
	hasMermaid := false
	writer.HighlightCodeBlock = func(source, lang string, inline bool, params map[string]string) string {
		if strings.EqualFold(lang, "mermaid") {
			hasMermaid = true
			return fmt.Sprintf("<pre class=\"mermaid\">%s</pre>", html.EscapeString(source))
		}
		escaped := html.EscapeString(source)
		if inline {
			if lang != "" {
				return fmt.Sprintf("<code class=\"language-%s\">%s</code>", html.EscapeString(lang), escaped)
			}
			return fmt.Sprintf("<code>%s</code>", escaped)
		}
		if lang != "" {
			return fmt.Sprintf("<pre><code class=\"language-%s\">%s</code></pre>", html.EscapeString(lang), escaped)
		}
		return fmt.Sprintf("<pre><code>%s</code></pre>", escaped)
	}

	doc := org.New().Silent().Parse(bytes.NewReader(content), "")
	rendered, err := doc.Write(writer)
	if err != nil {
		return "", err
	}
	if hasMermaid {
		rendered += "\n<script src=\"https://cdn.jsdelivr.net/npm/mermaid/dist/mermaid.min.js\"></script>\n"
	}
	return rendered, nil
}

// RenderOrgPreview renders the HTML template for an Org mode preview.
func RenderOrgPreview(w http.ResponseWriter, tmpl *template.Template, requestPath, filePath string, info os.FileInfo, config Config, orgHTML string) error {
	return executeTemplate(w, tmpl, orgPreviewData{
		PreviewCommon: newPreviewCommon(requestPath, filePath, info, config, ""),
		HTML:          template.HTML(orgHTML),
	})
}

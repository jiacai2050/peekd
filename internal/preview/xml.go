package preview

import (
	"bytes"
	"encoding/xml"
	"html/template"
	"io"
	"net/http"
	"os"
	"strings"
)

type xmlPreviewData struct {
	PreviewCommon
	Content string
}

func formatXML(content []byte) (string, error) {
	var output bytes.Buffer
	decoder := xml.NewDecoder(bytes.NewReader(content))
	encoder := xml.NewEncoder(&output)
	encoder.Indent("", "  ")

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		// Keep the XML declaration on its own line; Encoder.Indent does not
		// insert a separator before the root element in this case.
		if instruction, ok := token.(xml.ProcInst); ok && instruction.Target == "xml" {
			if _, err := output.WriteString("<?" + instruction.Target); err != nil {
				return "", err
			}
			if len(instruction.Inst) > 0 {
				if err := output.WriteByte(' '); err != nil {
					return "", err
				}
				if _, err := output.Write(instruction.Inst); err != nil {
					return "", err
				}
			}
			if _, err := output.WriteString("?>\n"); err != nil {
				return "", err
			}
			continue
		}
		// Ignore formatting-only whitespace so indentation is generated once.
		if text, ok := token.(xml.CharData); ok && strings.TrimSpace(string(text)) == "" {
			continue
		}
		if err := encoder.EncodeToken(token); err != nil {
			return "", err
		}
	}
	if err := encoder.Flush(); err != nil {
		return "", err
	}
	return output.String(), nil
}

func RenderXMLPreview(w http.ResponseWriter, tmpl *template.Template, requestPath, filePath string, info os.FileInfo, config Config, content string) error {
	return executeTemplate(w, tmpl, xmlPreviewData{
		PreviewCommon: newPreviewCommon(requestPath, filePath, info, config, ""),
		Content:       content,
	})
}

package preview

import (
	"bytes"
	"encoding/xml"
	"html/template"
	"io"
	"net/http"
	"os"
)

type xmlPreviewData struct {
	FileName    string
	Content     string
	RawURL      string
	Size        string
	Modified    string
	Breadcrumbs []Breadcrumb
	LocalPath   string
	ProjectURL  string
	Version     string
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
	data := xmlPreviewData{
		FileName:    previewFileName(filePath),
		Content:     content,
		RawURL:      previewRawURL(requestPath),
		Size:        previewFileSize(info),
		Modified:    previewModified(info),
		Breadcrumbs: Breadcrumbs(requestPath, false),
		LocalPath:   previewLocalPath(filePath),
		ProjectURL:  config.ProjectURL,
		Version:     config.Version,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return tmpl.Execute(w, data)
}

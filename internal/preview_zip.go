package internal

import (
	"archive/zip"
	"html/template"
	"net/http"
	"os"
)

type zipPreviewEntry struct {
	Name     string
	Size     string
	Modified string
	IsDir    bool
}

type zipPreviewData struct {
	FileName   string
	Entries    []zipPreviewEntry
	RawURL     string
	Size       string
	Modified   string
	ProjectURL string
	Version    string
}

func readZipPreview(filePath string) ([]zipPreviewEntry, error) {
	reader, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	entries := make([]zipPreviewEntry, 0, len(reader.File))
	for _, file := range reader.File {
		isDir := file.FileInfo().IsDir()
		size := "-"
		if !isDir {
			size = formatFileSize(int64(file.UncompressedSize64))
		}
		entries = append(entries, zipPreviewEntry{
			Name:     file.Name,
			Size:     size,
			Modified: file.Modified.Format("2006-01-02 15:04:05"),
			IsDir:    isDir,
		})
	}
	return entries, nil
}

func renderZIPPreview(w http.ResponseWriter, tmpl *template.Template, requestPath, filePath string, info os.FileInfo, config Config) error {
	entries, err := readZipPreview(filePath)
	if err != nil {
		return err
	}
	data := zipPreviewData{
		FileName:   previewFileName(filePath),
		Entries:    entries,
		RawURL:     previewRawURL(requestPath),
		Size:       previewFileSize(info),
		Modified:   previewModified(info),
		ProjectURL: config.ProjectURL,
		Version:    config.Version,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return tmpl.Execute(w, data)
}

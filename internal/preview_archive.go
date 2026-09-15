package internal

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"html/template"
	"io"
	"net/http"
	"os"
	"strings"
)

type archivePreviewEntry struct {
	Name     string
	Size     string
	Modified string
	IsDir    bool
}

type archivePreviewData struct {
	FileName    string
	Entries     []archivePreviewEntry
	RawURL      string
	Size        string
	Modified    string
	Breadcrumbs []breadcrumb
	ProjectURL  string
	Version     string
}

func readZIPPreview(filePath string) ([]archivePreviewEntry, error) {
	reader, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	entries := make([]archivePreviewEntry, 0, len(reader.File))
	for _, file := range reader.File {
		isDir := file.FileInfo().IsDir()
		size := "-"
		if !isDir {
			size = formatFileSize(int64(file.UncompressedSize64))
		}
		entries = append(entries, archivePreviewEntry{
			Name:     file.Name,
			Size:     size,
			Modified: file.Modified.Format("2006-01-02 15:04:05"),
			IsDir:    isDir,
		})
	}
	return entries, nil
}

func readTARPreview(filePath string) ([]archivePreviewEntry, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var reader *tar.Reader
	if strings.HasSuffix(strings.ToLower(filePath), ".gz") || strings.HasSuffix(strings.ToLower(filePath), ".tgz") {
		gzipReader, err := gzip.NewReader(file)
		if err != nil {
			return nil, err
		}
		defer gzipReader.Close()
		reader = tar.NewReader(gzipReader)
	} else {
		reader = tar.NewReader(file)
	}

	entries := make([]archivePreviewEntry, 0)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return entries, nil
		}
		if err != nil {
			return nil, err
		}

		isDir := header.Typeflag == tar.TypeDir
		size := "-"
		if !isDir {
			size = formatFileSize(header.Size)
		}
		entries = append(entries, archivePreviewEntry{
			Name:     header.Name,
			Size:     size,
			Modified: header.ModTime.Format("2006-01-02 15:04:05"),
			IsDir:    isDir,
		})
	}
}

func readArchivePreview(filePath string) ([]archivePreviewEntry, error) {
	if strings.HasSuffix(strings.ToLower(filePath), ".zip") {
		return readZIPPreview(filePath)
	}
	return readTARPreview(filePath)
}

func renderArchivePreview(w http.ResponseWriter, tmpl *template.Template, requestPath, filePath string, info os.FileInfo, config Config) error {
	entries, err := readArchivePreview(filePath)
	if err != nil {
		return err
	}
	data := archivePreviewData{
		FileName:    previewFileName(filePath),
		Entries:     entries,
		RawURL:      previewRawURL(requestPath),
		Size:        previewFileSize(info),
		Modified:    previewModified(info),
		Breadcrumbs: previewBreadcrumbs(requestPath, false),
		ProjectURL:  config.ProjectURL,
		Version:     config.Version,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return tmpl.Execute(w, data)
}

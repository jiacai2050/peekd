package preview

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
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
	PreviewCommon
	Entries []archivePreviewEntry
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
			size = FormatFileSize(int64(file.UncompressedSize64))
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
			size = FormatFileSize(header.Size)
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

func RenderArchivePreview(w http.ResponseWriter, tmpl *template.Template, requestPath, filePath string, info os.FileInfo, config Config) error {
	entries, err := readArchivePreview(filePath)
	if err != nil {
		return err
	}
	return executeTemplate(w, tmpl, archivePreviewData{
		PreviewCommon: newPreviewCommon(requestPath, filePath, info, config, "📦", fmt.Sprintf("%d entries", len(entries))),
		Entries:       entries,
	})
}

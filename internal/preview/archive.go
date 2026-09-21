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

func zipMethodString(method uint16) string {
	switch method {
	case zip.Store:
		return "Store"
	case zip.Deflate:
		return "Deflate"
	default:
		return fmt.Sprintf("Unknown(%d)", method)
	}
}

type archivePreviewEntry struct {
	Name             string
	CompressedSize   string
	UncompressedSize string
	Ratio            string
	Method           string
	Permissions      string
	Modified         string
	IsDir            bool
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
		compressedSize := "-"
		uncompressedSize := "-"
		ratio := "-"
		method := "-"
		permissions := "-"
		if isDir {
			method = "Dir"
		} else {
			compressedSize = FormatFileSize(int64(file.CompressedSize64))
			uncompressedSize = FormatFileSize(int64(file.UncompressedSize64))
			if file.UncompressedSize64 > 0 {
				ratio = fmt.Sprintf("%.0f%%", float64(file.CompressedSize64)/float64(file.UncompressedSize64)*100)
			}
			method = zipMethodString(file.Method)
			permissions = file.Mode().String()
		}
		entries = append(entries, archivePreviewEntry{
			Name:             file.Name,
			CompressedSize:   compressedSize,
			UncompressedSize: uncompressedSize,
			Ratio:            ratio,
			Method:           method,
			Permissions:      permissions,
			Modified:         file.Modified.Format("2006-01-02 15:04:05"),
			IsDir:            isDir,
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
		compressedSize := "-"
		uncompressedSize := "-"
		ratio := "-"
		method := "-"
		permissions := "-"
		if isDir {
			method = "Dir"
		} else {
			uncompressedSize = FormatFileSize(header.Size)
			method = "tar"
			permissions = os.FileMode(header.Mode).String()
		}
		entries = append(entries, archivePreviewEntry{
			Name:             header.Name,
			CompressedSize:   compressedSize,
			UncompressedSize: uncompressedSize,
			Ratio:            ratio,
			Method:           method,
			Permissions:      permissions,
			Modified:         header.ModTime.Format("2006-01-02 15:04:05"),
			IsDir:            isDir,
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
		PreviewCommon: newPreviewCommon(requestPath, filePath, info, config, fmt.Sprintf("%d entries", len(entries))),
		Entries:       entries,
	})
}

package preview

import (
	"archive/tar"
	"archive/zip"
	"bytes"
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

// ArchiveEntry is the portable representation of an archive entry.
type ArchiveEntry struct {
	Name             string `json:"name"`
	CompressedSize   string `json:"compressedSize"`
	UncompressedSize string `json:"uncompressedSize"`
	Ratio            string `json:"ratio"`
	Method           string `json:"method"`
	Permissions      string `json:"permissions"`
	Modified         string `json:"modified"`
	IsDir            bool   `json:"isDir"`
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
		entries = append(entries, archiveEntryFromZIP(file))
	}
	return entries, nil
}

func archiveEntryFromZIP(file *zip.File) archivePreviewEntry {
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
	return archivePreviewEntry{
		Name:             file.Name,
		CompressedSize:   compressedSize,
		UncompressedSize: uncompressedSize,
		Ratio:            ratio,
		Method:           method,
		Permissions:      permissions,
		Modified:         file.Modified.Format("2006-01-02 15:04:05"),
		IsDir:            isDir,
	}
}

func readZIPPreviewBytes(content []byte) ([]archivePreviewEntry, error) {
	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return nil, err
	}

	entries := make([]archivePreviewEntry, 0, len(reader.File))
	for _, file := range reader.File {
		entries = append(entries, archiveEntryFromZIP(file))
	}
	return entries, nil
}

func readTARPreview(filePath string) ([]archivePreviewEntry, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var source io.Reader = file
	var gzipReader *gzip.Reader
	if strings.HasSuffix(strings.ToLower(filePath), ".gz") || strings.HasSuffix(strings.ToLower(filePath), ".tgz") {
		gzipReader, err = gzip.NewReader(source)
		if err != nil {
			return nil, err
		}
		defer gzipReader.Close()
		source = gzipReader
	}

	return readTARPreviewReader(source)
}

func readTARPreviewReader(source io.Reader) ([]archivePreviewEntry, error) {
	reader := tar.NewReader(source)
	entries := make([]archivePreviewEntry, 0)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return entries, nil
		}
		if err != nil {
			return nil, err
		}

		entries = append(entries, archiveEntryFromTAR(header))
	}
}

func readTARPreviewBytes(filePath string, content []byte) ([]archivePreviewEntry, error) {
	var source io.Reader = bytes.NewReader(content)
	var gzipReader *gzip.Reader
	var err error
	if strings.HasSuffix(strings.ToLower(filePath), ".gz") || strings.HasSuffix(strings.ToLower(filePath), ".tgz") {
		gzipReader, err = gzip.NewReader(source)
		if err != nil {
			return nil, err
		}
		defer gzipReader.Close()
		source = gzipReader
	}

	return readTARPreviewReader(source)
}

func archiveEntryFromTAR(header *tar.Header) archivePreviewEntry {
	isDir := header.Typeflag == tar.TypeDir
	uncompressedSize := "-"
	method := "-"
	permissions := "-"
	if isDir {
		method = "Dir"
	} else {
		uncompressedSize = FormatFileSize(header.Size)
		method = "tar"
		permissions = os.FileMode(header.Mode).String()
	}
	return archivePreviewEntry{
		Name:             header.Name,
		CompressedSize:   "-",
		UncompressedSize: uncompressedSize,
		Ratio:            "-",
		Method:           method,
		Permissions:      permissions,
		Modified:         header.ModTime.Format("2006-01-02 15:04:05"),
		IsDir:            isDir,
	}
}

func readArchivePreview(filePath string) ([]archivePreviewEntry, error) {
	if strings.HasSuffix(strings.ToLower(filePath), ".zip") {
		return readZIPPreview(filePath)
	}
	return readTARPreview(filePath)
}

// ReadArchivePreviewBytes lists an archive without extracting it.
func ReadArchivePreviewBytes(filePath string, content []byte) ([]ArchiveEntry, error) {
	var entries []archivePreviewEntry
	var err error
	if strings.HasSuffix(strings.ToLower(filePath), ".zip") {
		entries, err = readZIPPreviewBytes(content)
	} else {
		entries, err = readTARPreviewBytes(filePath, content)
	}
	if err != nil {
		return nil, err
	}

	result := make([]ArchiveEntry, len(entries))
	for i, entry := range entries {
		result[i] = ArchiveEntry{
			Name:             entry.Name,
			CompressedSize:   entry.CompressedSize,
			UncompressedSize: entry.UncompressedSize,
			Ratio:            entry.Ratio,
			Method:           entry.Method,
			Permissions:      entry.Permissions,
			Modified:         entry.Modified,
			IsDir:            entry.IsDir,
		}
	}
	return result, nil
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

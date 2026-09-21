package preview

import (
	"encoding/binary"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"strings"
	"unicode/utf8"
)

const (
	exthRecordTitle          = 100
	exthRecordAuthor         = 101
	exthRecordISBN           = 103
	exthRecordPublisher      = 104
	exthRecordDescription    = 105
	exthRecordPublishingDate = 106
	exthRecordSubject        = 109
)

type mobiMetadata struct {
	Title          string
	Author         string
	Publisher      string
	ISBN           string
	Subject        string
	Description    string
	PublishingDate string
}

type mobiPreviewData struct {
	PreviewCommon
	mobiMetadata
}

func parseMOBI(filePath string) (mobiMetadata, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return mobiMetadata{}, err
	}
	defer file.Close()

	// PDB header: 76 bytes. Record count at offset 76.
	var pdbHeader [78]byte
	if _, err := io.ReadFull(file, pdbHeader[:]); err != nil {
		return mobiMetadata{}, fmt.Errorf("read PDB header: %w", err)
	}
	numRecords := int(binary.BigEndian.Uint16(pdbHeader[76:78]))
	if numRecords < 1 {
		return mobiMetadata{}, fmt.Errorf("no PDB records")
	}

	meta := mobiMetadata{}

	// Read first record offset.
	var rec0Entry [8]byte
	if _, err := io.ReadFull(file, rec0Entry[:]); err != nil {
		return mobiMetadata{}, fmt.Errorf("read record entry: %w", err)
	}
	rec0Offset := binary.BigEndian.Uint32(rec0Entry[0:4])

	// Seek to record 0. The MOBI header may be preceded by a 16-byte
	// PalmDOC compression header, so read enough to find the "MOBI" magic.
	if _, err := file.Seek(int64(rec0Offset), io.SeekStart); err != nil {
		return mobiMetadata{}, fmt.Errorf("seek record 0: %w", err)
	}

	var probe [40]byte // PalmDOC (16) + "MOBI" (4) + header-length (4) + padding
	if _, err := io.ReadFull(file, probe[:]); err != nil {
		return mobiMetadata{}, fmt.Errorf("read MOBI probe: %w", err)
	}

	mobiMagicOffset := -1
	for i := 0; i <= len(probe)-4; i++ {
		if string(probe[i:i+4]) == "MOBI" {
			mobiMagicOffset = i
			break
		}
	}
	if mobiMagicOffset < 0 {
		return mobiMetadata{}, fmt.Errorf("MOBI magic not found")
	}

	mobiHeaderLen := binary.BigEndian.Uint32(probe[mobiMagicOffset+4 : mobiMagicOffset+8])
	if mobiHeaderLen < 104 {
		return mobiMetadata{}, fmt.Errorf("MOBI header too short: %d", mobiHeaderLen)
	}

	// Read the full MOBI header from the "MOBI" magic.
	mobiStart := int64(rec0Offset) + int64(mobiMagicOffset)
	if _, err := file.Seek(mobiStart, io.SeekStart); err != nil {
		return mobiMetadata{}, fmt.Errorf("seek MOBI header: %w", err)
	}
	mobiHeader := make([]byte, mobiHeaderLen)
	if _, err := io.ReadFull(file, mobiHeader); err != nil {
		return mobiMetadata{}, fmt.Errorf("read MOBI header: %w", err)
	}

	// Read the full name from the MOBI header.  The fields are at fixed
	// byte offsets inside the MOBI header (not the PalmDOC preamble):
	//   68–71  full_name_offset   (relative to record 0 start)
	//   72–75  full_name_length
	if mobiHeaderLen >= 76 {
		fullNameOff := binary.BigEndian.Uint32(mobiHeader[68:72])
		fullNameLen := binary.BigEndian.Uint32(mobiHeader[72:76])
		if fullNameLen > 0 && fullNameLen < 1<<20 {
			absOff := int64(rec0Offset) + int64(fullNameOff)
			if _, err := file.Seek(absOff, io.SeekStart); err == nil {
				buf := make([]byte, fullNameLen)
				if _, err := io.ReadFull(file, buf); err == nil {
					meta.Title = bestString(buf)
				}
			}
		}
	}

	// EXTH header starts right after MOBI header.
	exthStart := mobiStart + int64(mobiHeaderLen)
	exthMeta, err := parseEXTH(file, exthStart)
	if err == nil {
		// Prefer MOBI header title over EXTH 100 (some files store
		// author in EXTH 100 instead of the title).
		exth100Title := exthMeta.Title
		if exthMeta.Title != "" && meta.Title == "" {
			meta.Title = exthMeta.Title
		}
		if exthMeta.Author != "" {
			meta.Author = exthMeta.Author
		} else if exth100Title != "" && meta.Author == "" {
			// EXTH 100 contained the author, not the title.
			meta.Author = exth100Title
		}
		if exthMeta.Publisher != "" {
			meta.Publisher = exthMeta.Publisher
		}
		if exthMeta.ISBN != "" {
			meta.ISBN = exthMeta.ISBN
		}
		if exthMeta.Subject != "" {
			meta.Subject = exthMeta.Subject
		}
		if exthMeta.Description != "" {
			meta.Description = exthMeta.Description
		}
		if exthMeta.PublishingDate != "" {
			meta.PublishingDate = exthMeta.PublishingDate
		}
	}
	return meta, nil
}

func parseEXTH(file *os.File, offset int64) (mobiMetadata, error) {
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return mobiMetadata{}, fmt.Errorf("seek EXTH: %w", err)
	}

	var exthHeader [12]byte
	if _, err := io.ReadFull(file, exthHeader[:]); err != nil {
		return mobiMetadata{}, fmt.Errorf("read EXTH header: %w", err)
	}
	if string(exthHeader[0:4]) != "EXTH" {
		return mobiMetadata{}, fmt.Errorf("EXTH magic not found")
	}

	exthLen := int(binary.BigEndian.Uint32(exthHeader[4:8]))
	numRecords := int(binary.BigEndian.Uint32(exthHeader[8:12]))

	bodyLen := exthLen - 12
	if bodyLen < 0 {
		return mobiMetadata{}, fmt.Errorf("invalid EXTH length")
	}
	body := make([]byte, bodyLen)
	if _, err := io.ReadFull(file, body); err != nil {
		return mobiMetadata{}, fmt.Errorf("read EXTH body: %w", err)
	}

	meta := mobiMetadata{}
	pos := 0
	for i := 0; i < numRecords && pos+8 <= len(body); i++ {
		recType := binary.BigEndian.Uint32(body[pos : pos+4])
		recLen := int(binary.BigEndian.Uint32(body[pos+4 : pos+8]))
		if recLen < 8 || pos+recLen > len(body) {
			break
		}
		data := body[pos+8 : pos+recLen]
		switch recType {
		case exthRecordTitle:
			meta.Title = bestString(data)
		case exthRecordAuthor:
			meta.Author = bestString(data)
		case exthRecordPublisher:
			meta.Publisher = bestString(data)
		case exthRecordISBN:
			meta.ISBN = bestString(data)
		case exthRecordPublishingDate:
			meta.PublishingDate = bestString(data)
		case exthRecordSubject:
			meta.Subject = bestString(data)
		case exthRecordDescription:
			meta.Description = bestString(data)
		}
		pos += recLen
	}
	return meta, nil
}

func bestString(data []byte) string {
	if utf8.Valid(data) {
		return strings.TrimSpace(string(data))
	}
	// Strip nulls and non-ASCII padding; try again.
	cleaned := make([]byte, 0, len(data))
	for _, b := range data {
		if b >= 0x20 {
			cleaned = append(cleaned, b)
		}
	}
	return strings.TrimSpace(string(cleaned))
}

func RenderMOBIPreview(w http.ResponseWriter, tmpl *template.Template, requestPath, filePath string, info os.FileInfo, config Config) error {
	meta, _ := parseMOBI(filePath) // ignore errors; show whatever we get
	data := mobiPreviewData{
		PreviewCommon: newPreviewCommon(requestPath, filePath, info, config, ""),
		mobiMetadata:  meta,
	}
	return executeTemplate(w, tmpl, data)
}

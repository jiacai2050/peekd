package preview

import (
	"archive/tar"
	"bytes"
	"testing"
)

func TestPrepareBytesPreview(t *testing.T) {
	t.Parallel()

	prepared, kind, err := PrepareBytesPreview(PreviewTypeJSON, []byte(`{"name":"peekd"}`), 1<<20)
	if err != nil {
		t.Fatalf("prepare JSON preview: %v", err)
	}
	if kind != PreviewTypeJSON {
		t.Fatalf("preview kind = %q, want %q", kind, PreviewTypeJSON)
	}
	if prepared.Formatted != "{\n  \"name\": \"peekd\"\n}" {
		t.Fatalf("formatted JSON = %q", prepared.Formatted)
	}

	_, kind, err = PrepareBytesPreview(PreviewTypeText, bytes.Repeat([]byte("x"), 5), 4)
	if err != nil {
		t.Fatalf("prepare oversized preview: %v", err)
	}
	if kind != PreviewTypeNone {
		t.Fatalf("oversized preview kind = %q, want none", kind)
	}
}

func TestPreviewTypeByContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		contentType string
		want        PreviewType
	}{
		{"image/png", PreviewTypeImage},
		{"audio/mpeg", PreviewTypeAudio},
		{"video/mp4", PreviewTypeVideo},
		{"application/pdf", PreviewTypePDF},
		{"application/x-tar", PreviewTypeTAR},
		{"application/json", PreviewTypeJSON},
		{"text/plain", PreviewTypeText},
		{"application/octet-stream", PreviewTypeNone},
	}
	for _, test := range tests {
		if got := PreviewTypeByContent(test.contentType); got != test.want {
			t.Errorf("PreviewTypeByContent(%q) = %q, want %q", test.contentType, got, test.want)
		}
	}
}

func TestReadArchivePreviewBytesTAR(t *testing.T) {
	t.Parallel()

	var content bytes.Buffer
	writer := tar.NewWriter(&content)
	if err := writer.WriteHeader(&tar.Header{Name: "README.md", Size: 4, Mode: 0o644}); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := writer.Write([]byte("peek")); err != nil {
		t.Fatalf("write tar content: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}

	entries, err := ReadArchivePreviewBytes("files.tar", content.Bytes())
	if err != nil {
		t.Fatalf("read tar preview: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "README.md" {
		t.Fatalf("entries = %#v, want README.md", entries)
	}
}

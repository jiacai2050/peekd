package main

import (
	"net"
	"os"
	"strconv"
	"testing"
)

func TestParseByteSize(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{input: "4M", want: 4 << 20},
		{input: "512K", want: 512 << 10},
		{input: "4MiB", want: 4 << 20},
		{input: "4194304", want: 4194304},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			got, err := parseByteSize(test.input)
			if err != nil {
				t.Fatalf("parseByteSize(%q): %v", test.input, err)
			}
			if got != test.want {
				t.Fatalf("parseByteSize(%q) = %d, want %d", test.input, got, test.want)
			}
		})
	}
}

func TestIsBrowserUserAgent(t *testing.T) {
	tests := []struct {
		name      string
		userAgent string
		want      bool
	}{
		{name: "Chrome", userAgent: "Mozilla/5.0 Chrome/120.0.0.0 Safari/537.36", want: true},
		{name: "Firefox", userAgent: "Mozilla/5.0 Firefox/121.0", want: true},
		{name: "Safari", userAgent: "Mozilla/5.0 Version/17.0 Safari/605.1.15", want: true},
		{name: "curl", userAgent: "curl/8.5.0", want: false},
		{name: "empty", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isBrowserUserAgent(test.userAgent); got != test.want {
				t.Fatalf("isBrowserUserAgent(%q) = %v, want %v", test.userAgent, got, test.want)
			}
		})
	}
}

func TestListenWithFallbackUsesNextPortWhenOccupied(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("occupy port: %v", err)
	}
	defer occupied.Close()

	_, occupiedPort, err := net.SplitHostPort(occupied.Addr().String())
	if err != nil {
		t.Fatalf("split occupied address: %v", err)
	}
	port, err := strconv.Atoi(occupiedPort)
	if err != nil {
		t.Fatalf("parse occupied port: %v", err)
	}
	if port == 65535 {
		t.Skip("no port available after occupied port")
	}

	fallback, err := listenWithFallback(occupied.Addr().String())
	if err != nil {
		t.Fatalf("listen with fallback: %v", err)
	}
	defer fallback.Close()

	_, fallbackPort, err := net.SplitHostPort(fallback.Addr().String())
	if err != nil {
		t.Fatalf("split fallback address: %v", err)
	}
	if fallbackPort != strconv.Itoa(port+1) {
		t.Fatalf("fallback port = %s, want %d", fallbackPort, port+1)
	}
}

func TestReadTextPreviewEnforcesLimit(t *testing.T) {
	filePath := t.TempDir() + "/large.txt"
	if err := os.WriteFile(filePath, []byte("12345"), 0o600); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	content, previewable, err := readTextPreview(filePath, 4)
	if err != nil {
		t.Fatalf("read text preview: %v", err)
	}
	if previewable {
		t.Fatal("expected oversized text file to be unavailable for preview")
	}
	if content != nil {
		t.Fatalf("oversized preview content = %q, want nil", content)
	}
}

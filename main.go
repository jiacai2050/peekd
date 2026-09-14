package main

import (
	"embed"
	"flag"
	"fmt"
	"log"
	"path/filepath"
	"runtime/debug"

	"github.com/jiacai2050/peekd/internal"
)

//go:embed assets/*
var embeddedFiles embed.FS

var Version = "dev"

const ProjectURL = "https://github.com/jiacai2050/peekd"

func main() {
	rootDir := flag.String("root", ".", "directory to serve")
	addr := flag.String("addr", ":8090", "HTTP server address")
	maxTextPreviewSize := int64(4 << 20)
	flag.Func("max-preview-size", "maximum text preview size (default 4M; e.g. 512K or 4194304)", func(value string) error {
		parsed, err := internal.ParseByteSize(value)
		if err != nil {
			return err
		}
		maxTextPreviewSize = parsed
		return nil
	})
	showVersion := flag.Bool("version", false, "print version")
	flag.Parse()

	if *showVersion {
		fmt.Printf("peekd %s\n", Version)
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, setting := range info.Settings {
				switch setting.Key {
				case "vcs.revision":
					fmt.Printf("Revision: %s\n", setting.Value)
				case "vcs.time":
					fmt.Printf("Build Time: %s\n", setting.Value)
				}
			}
		}
		return
	}

	absoluteRootDir, err := filepath.Abs(*rootDir)
	if err != nil {
		log.Fatalf("failed to resolve root directory: %v", err)
	}

	if err := internal.Run(internal.Config{
		RootDir:            absoluteRootDir,
		Addr:               *addr,
		MaxTextPreviewSize: maxTextPreviewSize,
		Version:            Version,
		ProjectURL:         ProjectURL,
		EmbeddedFiles:      embeddedFiles,
	}); err != nil {
		log.Fatal(err)
	}
}

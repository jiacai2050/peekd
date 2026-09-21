package main

import (
	"embed"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/jiacai2050/peekd/internal"
)

//go:embed assets/*
var embeddedFiles embed.FS

var Version = "dev-" + time.Now().Format("20060102-150405")

const ProjectURL = "https://github.com/jiacai2050/peekd"

func main() {
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

	rootDir := "."
	if flag.NArg() > 0 {
		rootDir = flag.Arg(0)
	}

	absoluteRootDir, err := filepath.Abs(rootDir)
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
		AuthUsername:       os.Getenv("PEEKD_AUTH_USER"),
		AuthPassword:       os.Getenv("PEEKD_AUTH_PASSWORD"),
	}); err != nil {
		log.Fatal(err)
	}
}

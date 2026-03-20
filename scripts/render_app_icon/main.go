package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/xuzhougeng/citebox/internal/appicon"
)

func main() {
	var (
		pngPath = flag.String("png", "", "write PNG icon to this path")
		icoPath = flag.String("ico", "", "write ICO icon to this path")
		size    = flag.Int("size", appicon.DefaultSize, "render size in pixels")
	)

	flag.Parse()

	if *pngPath == "" && *icoPath == "" {
		fmt.Fprintln(os.Stderr, "at least one of -png or -ico must be provided")
		os.Exit(2)
	}
	if *size <= 0 {
		fmt.Fprintln(os.Stderr, "-size must be greater than zero")
		os.Exit(2)
	}

	img := appicon.Render(*size)

	if *pngPath != "" {
		if err := writeIcon(*pngPath, func(path string) error {
			return appicon.WritePNG(path, img)
		}); err != nil {
			fmt.Fprintf(os.Stderr, "write png icon: %v\n", err)
			os.Exit(1)
		}
	}

	if *icoPath != "" {
		if err := writeIcon(*icoPath, func(path string) error {
			return appicon.WriteICO(path, img)
		}); err != nil {
			fmt.Fprintf(os.Stderr, "write ico icon: %v\n", err)
			os.Exit(1)
		}
	}
}

func writeIcon(path string, write func(path string) error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return write(path)
}

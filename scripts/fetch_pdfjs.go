package main

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultPDFJSVersion = "5.5.207"

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: go run ./scripts/fetch_pdfjs.go <target-dir>\n")
		os.Exit(1)
	}

	targetDir := filepath.Clean(os.Args[1])
	version := strings.TrimSpace(os.Getenv("PDFJS_VERSION"))
	if version == "" {
		version = defaultPDFJSVersion
	}

	if ready, err := assetsReady(targetDir); err == nil && ready {
		fmt.Printf("pdf.js assets already present at %s\n", targetDir)
		return
	}

	source := strings.TrimSpace(os.Getenv("PDFJS_TARBALL"))
	if source == "" {
		source = fmt.Sprintf("https://registry.npmjs.org/pdfjs-dist/-/pdfjs-dist-%s.tgz", version)
	}

	reader, closer, err := openSource(source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open pdf.js source: %v\n", err)
		os.Exit(1)
	}
	defer closer.Close()

	if err := os.RemoveAll(targetDir); err != nil {
		fmt.Fprintf(os.Stderr, "remove existing target dir: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "create target dir: %v\n", err)
		os.Exit(1)
	}

	if err := extractAssets(reader, targetDir); err != nil {
		fmt.Fprintf(os.Stderr, "extract pdf.js assets: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("pdf.js assets prepared at %s\n", targetDir)
}

func openSource(source string) (io.Reader, io.Closer, error) {
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		client := &http.Client{Timeout: 2 * time.Minute}
		resp, err := client.Get(source)
		if err != nil {
			return nil, nil, err
		}
		if resp.StatusCode != http.StatusOK {
			defer resp.Body.Close()
			return nil, nil, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, source)
		}
		return resp.Body, resp.Body, nil
	}

	file, err := os.Open(source)
	if err != nil {
		return nil, nil, err
	}
	return file, file, nil
}

func extractAssets(source io.Reader, targetDir string) error {
	gzipReader, err := gzip.NewReader(source)
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	required := map[string]bool{
		"LICENSE":                         false,
		"legacy/build/pdf.min.mjs":        false,
		"legacy/build/pdf.worker.min.mjs": false,
	}
	copied := 0

	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}

		relativePath, ok := selectedPath(header.Name)
		if !ok {
			continue
		}

		destination := filepath.Join(targetDir, relativePath)
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}

		file, err := os.Create(destination)
		if err != nil {
			return err
		}
		if _, err := io.Copy(file, tarReader); err != nil {
			file.Close()
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
		copied++

		if _, exists := required[relativePath]; exists {
			required[relativePath] = true
		}
	}

	for path, found := range required {
		if !found {
			return fmt.Errorf("missing required asset %s", path)
		}
	}
	if copied == 0 {
		return errors.New("no pdf.js assets were extracted")
	}

	return nil
}

func selectedPath(tarPath string) (string, bool) {
	switch {
	case tarPath == "package/LICENSE":
		return "LICENSE", true
	case tarPath == "package/legacy/build/pdf.min.mjs":
		return "legacy/build/pdf.min.mjs", true
	case tarPath == "package/legacy/build/pdf.worker.min.mjs":
		return "legacy/build/pdf.worker.min.mjs", true
	case strings.HasPrefix(tarPath, "package/cmaps/"):
		return strings.TrimPrefix(tarPath, "package/"), true
	case strings.HasPrefix(tarPath, "package/standard_fonts/"):
		return strings.TrimPrefix(tarPath, "package/"), true
	case strings.HasPrefix(tarPath, "package/wasm/"):
		return strings.TrimPrefix(tarPath, "package/"), true
	default:
		return "", false
	}
}

func assetsReady(targetDir string) (bool, error) {
	required := []string{
		filepath.Join(targetDir, "LICENSE"),
		filepath.Join(targetDir, "legacy/build/pdf.min.mjs"),
		filepath.Join(targetDir, "legacy/build/pdf.worker.min.mjs"),
		filepath.Join(targetDir, "cmaps/LICENSE"),
		filepath.Join(targetDir, "standard_fonts/LiberationSans-Regular.ttf"),
		filepath.Join(targetDir, "wasm/qcms_bg.wasm"),
	}

	for _, path := range required {
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return false, nil
			}
			return false, err
		}
		if info.IsDir() {
			return false, fmt.Errorf("expected file but got directory: %s", path)
		}
	}

	return true, nil
}

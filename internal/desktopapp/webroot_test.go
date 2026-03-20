package desktopapp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveWebRootFromExecutable(t *testing.T) {
	t.Run("next to executable", func(t *testing.T) {
		root := t.TempDir()
		executablePath := filepath.Join(root, "citebox-desktop")
		webRoot := filepath.Join(root, "web")
		mustWriteIndex(t, webRoot)

		got, err := ResolveWebRootFromExecutable(executablePath)
		if err != nil {
			t.Fatalf("ResolveWebRootFromExecutable() error = %v", err)
		}
		if got != webRoot {
			t.Fatalf("ResolveWebRootFromExecutable() = %q, want %q", got, webRoot)
		}
	})

	t.Run("inside macOS app bundle", func(t *testing.T) {
		root := t.TempDir()
		executablePath := filepath.Join(root, "CiteBox.app", "Contents", "MacOS", "CiteBox")
		webRoot := filepath.Join(root, "CiteBox.app", "Contents", "Resources", "web")
		mustWriteIndex(t, webRoot)

		got, err := ResolveWebRootFromExecutable(executablePath)
		if err != nil {
			t.Fatalf("ResolveWebRootFromExecutable() error = %v", err)
		}
		if got != webRoot {
			t.Fatalf("ResolveWebRootFromExecutable() = %q, want %q", got, webRoot)
		}
	})

	t.Run("current working directory fallback", func(t *testing.T) {
		root := t.TempDir()
		executablePath := filepath.Join(root, "bin", "citebox-desktop")
		webRoot := filepath.Join(root, "web")
		mustWriteIndex(t, webRoot)
		t.Chdir(root)

		got, err := ResolveWebRootFromExecutable(executablePath)
		if err != nil {
			t.Fatalf("ResolveWebRootFromExecutable() error = %v", err)
		}
		if got != "web" {
			t.Fatalf("ResolveWebRootFromExecutable() = %q, want %q", got, "web")
		}
	})
}

func mustWriteIndex(t *testing.T, webRoot string) {
	t.Helper()

	if err := os.MkdirAll(webRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(webRoot, "index.html"), []byte("<!doctype html>"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

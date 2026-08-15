package zotero

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsPathToWSL(t *testing.T) {
	got := windowsPathToWSL(`E:\zotero-source\storage\ABC\paper.pdf`)
	want := "/mnt/e/zotero-source/storage/ABC/paper.pdf"
	if got != want {
		t.Fatalf("windowsPathToWSL() = %q, want %q", got, want)
	}
}

func TestResolveLocalFilePathUsesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "paper.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	got, err := ResolveLocalFilePath("file://" + path)
	if err != nil {
		t.Fatalf("ResolveLocalFilePath() error = %v", err)
	}
	if got != path {
		t.Fatalf("ResolveLocalFilePath() = %q, want %q", got, path)
	}
}

func TestResolveLocalFilePathRejectsRemoteURL(t *testing.T) {
	if _, err := ResolveLocalFilePath("https://example.com/paper.pdf"); err == nil {
		t.Fatal("expected remote URL to be rejected")
	}
}

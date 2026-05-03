package ai_image_gen

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveImage_WritesPNGToConversationDir(t *testing.T) {
	tmp := t.TempDir()
	store := NewStorage(tmp)

	pngBytes := []byte("\x89PNG\r\n\x1a\nfake")
	rel, err := store.Save(42, pngBytes)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if filepath.Dir(rel) == "." {
		t.Fatalf("rel path should be nested: %q", rel)
	}

	abs := filepath.Join(tmp, rel)
	got, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != string(pngBytes) {
		t.Fatalf("content mismatch")
	}

	parent := filepath.Dir(abs)
	if filepath.Base(parent) != "42" {
		t.Fatalf("expected conversation_id dir '42', got %q", parent)
	}
}

func TestSaveImage_RejectsEmptyBytes(t *testing.T) {
	tmp := t.TempDir()
	store := NewStorage(tmp)
	if _, err := store.Save(1, nil); err == nil {
		t.Fatalf("expected error for empty bytes")
	}
}

package ai_image_gen

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Storage writes generated PNG bytes to disk, namespaced by conversation_id.
// Returned paths are relative to the rootDir given at construction so callers
// can store them verbatim and rebuild absolute paths via filepath.Join.
type Storage struct {
	rootDir string
}

func NewStorage(rootDir string) *Storage {
	return &Storage{rootDir: rootDir}
}

func (s *Storage) Save(conversationID int64, pngBytes []byte) (string, error) {
	if len(pngBytes) == 0 {
		return "", errors.New("image bytes are empty")
	}
	dir := filepath.Join(s.rootDir, fmt.Sprintf("%d", conversationID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name, err := randomULID()
	if err != nil {
		return "", err
	}
	abs := filepath.Join(dir, name+".png")
	if err := os.WriteFile(abs, pngBytes, 0o644); err != nil {
		return "", err
	}
	rel, err := filepath.Rel(s.rootDir, abs)
	if err != nil {
		return "", err
	}
	return rel, nil
}

// Delete removes the file at relPath (relative to rootDir) from disk.
// It is a no-op if relPath is empty or the file does not exist.
func (s *Storage) Delete(relPath string) error {
	if relPath == "" {
		return nil
	}
	abs := filepath.Join(s.rootDir, relPath)
	if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// DeleteConversationFiles removes the conversation's directory and all PNGs in
// it. Idempotent — no error if the directory does not exist.
func (s *Storage) DeleteConversationFiles(conversationID int64) error {
	if conversationID <= 0 {
		return nil
	}
	dir := filepath.Join(s.rootDir, fmt.Sprintf("%d", conversationID))
	err := os.RemoveAll(dir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// randomULID returns a 16-byte hex token; we don't need true ULID monotonicity.
func randomULID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

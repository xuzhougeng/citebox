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

// randomULID returns a 16-byte hex token; we don't need true ULID monotonicity.
func randomULID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

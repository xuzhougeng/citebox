package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/xuzhougeng/citebox/internal/repository"
)

type fakeImageRepo struct{ rows map[int64]repository.AIGeneratedImage }

func (f *fakeImageRepo) GetByID(id int64) (repository.AIGeneratedImage, error) {
	row, ok := f.rows[id]
	if !ok {
		return repository.AIGeneratedImage{}, repository.ErrAIGeneratedImageNotFound
	}
	return row, nil
}

func TestAIGeneratedImageHandler_ServesFile(t *testing.T) {
	tmp := t.TempDir()
	rel := "1/abc.png"
	abs := filepath.Join(tmp, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(abs, []byte("PNGDATA"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	repo := &fakeImageRepo{rows: map[int64]repository.AIGeneratedImage{
		7: {ID: 7, ConversationID: 1, FilePath: rel},
	}}
	h := NewAIGeneratedImageHandler(repo, tmp)

	req := httptest.NewRequest(http.MethodGet, "/api/ai-generated-images/7/file", nil)
	rec := httptest.NewRecorder()
	h.GetFile(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d body = %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "PNGDATA" {
		t.Fatalf("body mismatch: %q", rec.Body.String())
	}
}

func TestAIGeneratedImageHandler_NotFound(t *testing.T) {
	tmp := t.TempDir()
	repo := &fakeImageRepo{rows: map[int64]repository.AIGeneratedImage{}}
	h := NewAIGeneratedImageHandler(repo, tmp)

	req := httptest.NewRequest(http.MethodGet, "/api/ai-generated-images/9999/file", nil)
	rec := httptest.NewRecorder()
	h.GetFile(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

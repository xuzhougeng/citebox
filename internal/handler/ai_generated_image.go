package handler

import (
	"errors"
	"net/http"
	"path/filepath"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/repository"
)

type aiGeneratedImageReader interface {
	GetByID(id int64) (repository.AIGeneratedImage, error)
}

type AIGeneratedImageHandler struct {
	repo    aiGeneratedImageReader
	rootDir string
}

func NewAIGeneratedImageHandler(repo aiGeneratedImageReader, rootDir string) *AIGeneratedImageHandler {
	return &AIGeneratedImageHandler{repo: repo, rootDir: rootDir}
}

func (h *AIGeneratedImageHandler) GetFile(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDWithSuffix(r.URL.Path, "/api/ai-generated-images/", "/file")
	if err != nil || id <= 0 {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "id 无效"))
		return
	}
	row, err := h.repo.GetByID(id)
	if errors.Is(err, repository.ErrAIGeneratedImageNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		sendError(w, err)
		return
	}
	abs := filepath.Join(h.rootDir, row.FilePath)
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, abs)
}

package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/xuzhougeng/citebox/internal/config"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service"
)

func newAIHandlerForTest(t *testing.T) (*AIHandler, *service.AIService, *repository.LibraryRepository, *config.Config) {
	t.Helper()

	root := t.TempDir()
	cfg := &config.Config{
		StorageDir:              filepath.Join(root, "storage"),
		DatabasePath:            filepath.Join(root, "library.db"),
		AdminUsername:           "citebox",
		AdminPassword:           "citebox123",
		ExtractorTimeoutSeconds: 1,
		ExtractorPollInterval:   1,
		ExtractorFileField:      "file",
	}

	repo, err := repository.NewLibraryRepository(cfg.DatabasePath)
	if err != nil {
		t.Fatalf("NewLibraryRepository() error = %v", err)
	}
	t.Cleanup(func() {
		_ = repo.Close()
	})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if _, err := service.NewLibraryService(repo, cfg, service.WithLogger(logger), service.WithoutBackgroundJobs()); err != nil {
		t.Fatalf("NewLibraryService() error = %v", err)
	}

	aiService := service.NewAIService(repo, cfg, logger)
	return NewAIHandler(aiService), aiService, repo, cfg
}

func TestReadStreamReturnsJSONErrorBeforeStreamingStarts(t *testing.T) {
	handler, aiService, repo, _ := newAIHandlerForTest(t)

	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Quota Exhausted",
		OriginalFilename: "quota-exhausted.pdf",
		StoredPDFName:    "quota-exhausted.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		PDFText:          "full text",
		ExtractionStatus: "completed",
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = fmt.Fprint(w, `{"error":{"message":"insufficient_quota"}}`)
	}))
	defer upstream.Close()

	if _, err := aiService.UpdateSettings(model.AISettings{
		Models: []model.AIModelConfig{
			{
				ID:              "qa",
				Name:            "QA",
				Provider:        model.AIProviderOpenAI,
				APIKey:          "test-key",
				BaseURL:         upstream.URL,
				Model:           "gpt-test",
				MaxOutputTokens: 1200,
			},
		},
		SceneModels: model.AISceneModelSelection{
			DefaultModelID: "qa",
			QAModelID:      "qa",
		},
		SystemPrompt: "system",
		QAPrompt:     "qa",
	}); err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/ai/read/stream", bytes.NewBufferString(fmt.Sprintf(`{"paper_id":%d,"action":"paper_qa","question":"请总结"}`, paper.ID)))
	w := httptest.NewRecorder()

	handler.ReadStream(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("ReadStream() status = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("ReadStream() content-type = %q, want application/json", got)
	}

	var payload struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
		Error   string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if payload.Success {
		t.Fatalf("ReadStream() success = true, want false")
	}
	if payload.Code == "" || payload.Error == "" {
		t.Fatalf("ReadStream() payload = %+v, want code and error", payload)
	}
}

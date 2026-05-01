package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/config"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service"
)

func newSettingsHandlerForTest(t *testing.T) (*SettingsHandler, *repository.LibraryRepository) {
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
	t.Cleanup(func() { _ = repo.Close() })
	librarySvc, err := service.NewLibraryService(repo, cfg, service.WithoutBackgroundJobs())
	if err != nil {
		t.Fatalf("NewLibraryService() error = %v", err)
	}
	return NewSettingsHandler(librarySvc, service.NewVersionService()), repo
}

func TestGetAIExternalSearchSettingsDefaultsToPubMed(t *testing.T) {
	h, _ := newSettingsHandlerForTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/settings/ai-external-search", nil)
	rec := httptest.NewRecorder()

	h.GetAIExternalSearchSettings(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var body model.AIExternalSearchSettings
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if len(body.Sources) != 1 || body.Sources[0] != model.AIExternalSourcePubMed {
		t.Fatalf("Sources = %#v, want [pubmed]", body.Sources)
	}
}

func TestPutAIExternalSearchSettingsPersistsNormalizedValues(t *testing.T) {
	h, repo := newSettingsHandlerForTest(t)
	req := httptest.NewRequest(http.MethodPut, "/api/settings/ai-external-search", bytes.NewBufferString(`{
		"sources":["semantic_scholar","pubmed","pubmed"],
		"pubmed_api_key":" key ",
		"pubmed_email":" user@example.org ",
		"pubmed_tool":" CiteBox "
	}`))
	rec := httptest.NewRecorder()

	h.PutAIExternalSearchSettings(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	raw, err := repo.GetAppSetting("ai_external_search_settings")
	if err != nil {
		t.Fatalf("GetAppSetting() error = %v", err)
	}
	if !strings.Contains(raw, `"sources":["semantic_scholar","pubmed"]`) {
		t.Fatalf("raw = %s", raw)
	}
}

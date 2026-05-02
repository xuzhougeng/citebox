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

func TestPutAIExternalSearchSettingsPreservesPubMedFieldsWhenOnlySourcesSaved(t *testing.T) {
	h, repo := newSettingsHandlerForTest(t)
	seedReq := httptest.NewRequest(http.MethodPut, "/api/settings/ai-external-search", bytes.NewBufferString(`{
		"sources":["pubmed"],
		"pubmed_api_key":"saved-pubmed-key",
		"pubmed_email":"saved@example.org",
		"pubmed_tool":"saved-tool"
	}`))
	seedRec := httptest.NewRecorder()
	h.PutAIExternalSearchSettings(seedRec, seedReq)
	if seedRec.Code != http.StatusOK {
		t.Fatalf("seed status = %d body = %s", seedRec.Code, seedRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodPut, "/api/settings/ai-external-search", bytes.NewBufferString(`{"sources":["semantic_scholar"]}`))
	rec := httptest.NewRecorder()
	h.PutAIExternalSearchSettings(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	raw, err := repo.GetAppSetting("ai_external_search_settings")
	if err != nil {
		t.Fatalf("GetAppSetting() error = %v", err)
	}
	if !strings.Contains(raw, `"sources":["semantic_scholar"]`) || !strings.Contains(raw, `"pubmed_api_key":"saved-pubmed-key"`) {
		t.Fatalf("raw = %s", raw)
	}
}

func TestGetResearchSettingsIncludesPubMedSettings(t *testing.T) {
	h, _ := newSettingsHandlerForTest(t)
	if err := h.libraryService.UpsertAppSetting("s2_api_key", "saved-s2-key"); err != nil {
		t.Fatalf("UpsertAppSetting() error = %v", err)
	}
	if _, err := h.libraryService.UpdateAIExternalSearchSettings(model.AIExternalSearchSettings{
		Sources:      []string{model.AIExternalSourcePubMed},
		PubMedAPIKey: "saved-pubmed-key",
		PubMedEmail:  "saved@example.org",
		PubMedTool:   "saved-tool",
	}); err != nil {
		t.Fatalf("UpdateAIExternalSearchSettings() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/settings/research", nil)
	rec := httptest.NewRecorder()
	h.GetResearchSettings(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body["s2_api_key"] != "saved-s2-key" || body["pubmed_api_key"] != "saved-pubmed-key" || body["pubmed_tool"] != "saved-tool" {
		t.Fatalf("body = %+v", body)
	}
	sources, _ := body["sources"].([]any)
	if len(sources) != 1 || sources[0] != model.AIExternalSourcePubMed {
		t.Fatalf("sources = %#v, want [pubmed]", body["sources"])
	}
}

func TestPutResearchSettingsAcceptsSourcesAndPersistsThem(t *testing.T) {
	h, _ := newSettingsHandlerForTest(t)
	client := &stubResearchSettingsClient{}
	h.SetResearchClient(client, ResearchRuntimeSettings{})
	req := httptest.NewRequest(http.MethodPut, "/api/settings/research", bytes.NewBufferString(`{"s2_api_key":"key","sources":[]}`))
	rec := httptest.NewRecorder()

	h.PutResearchSettings(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	settings, err := h.libraryService.GetAIExternalSearchSettings()
	if err != nil {
		t.Fatalf("GetAIExternalSearchSettings() error = %v", err)
	}
	for _, source := range settings.Sources {
		if source == model.AIExternalSourcePubMed {
			t.Fatalf("Sources = %#v, should be empty after explicit unchecking", settings.Sources)
		}
	}
}

func TestPutResearchSettingsPersistsSavedKeyButKeepsConfiguredLiveKey(t *testing.T) {
	h, repo := newSettingsHandlerForTest(t)
	client := &stubResearchSettingsClient{}
	h.SetResearchClient(client, ResearchRuntimeSettings{APIKey: "env-s2-key", APIKeySet: true})
	req := httptest.NewRequest(http.MethodPut, "/api/settings/research", bytes.NewBufferString(`{"s2_api_key":" saved-s2-key "}`))
	rec := httptest.NewRecorder()

	h.PutResearchSettings(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	raw, err := repo.GetAppSetting("s2_api_key")
	if err != nil {
		t.Fatalf("GetAppSetting() error = %v", err)
	}
	if raw != " saved-s2-key " {
		t.Fatalf("stored key = %q, want saved key", raw)
	}
	if client.apiKey != "env-s2-key" {
		t.Fatalf("live key = %q, want env-s2-key", client.apiKey)
	}
}

func TestPutResearchSettingsHotReloadsSavedKeyWhenNoConfiguredKey(t *testing.T) {
	h, _ := newSettingsHandlerForTest(t)
	client := &stubResearchSettingsClient{}
	h.SetResearchClient(client, ResearchRuntimeSettings{})
	req := httptest.NewRequest(http.MethodPut, "/api/settings/research", bytes.NewBufferString(`{"s2_api_key":" saved-s2-key "}`))
	rec := httptest.NewRecorder()

	h.PutResearchSettings(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if client.apiKey != "saved-s2-key" {
		t.Fatalf("live key = %q, want saved-s2-key", client.apiKey)
	}
}

func TestPutResearchSettingsPersistsPubMedSettingsAndHotReloads(t *testing.T) {
	h, repo := newSettingsHandlerForTest(t)
	client := &stubPubMedSettingsClient{}
	h.SetPubMedClient(client, PubMedRuntimeSettings{})
	req := httptest.NewRequest(http.MethodPut, "/api/settings/research", bytes.NewBufferString(`{
		"s2_api_key":"saved-s2-key",
		"pubmed_api_key":" saved-pubmed-key ",
		"pubmed_email":" saved@example.org ",
		"pubmed_tool":" saved-tool "
	}`))
	rec := httptest.NewRecorder()

	h.PutResearchSettings(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	raw, err := repo.GetAppSetting("ai_external_search_settings")
	if err != nil {
		t.Fatalf("GetAppSetting() error = %v", err)
	}
	if !strings.Contains(raw, `"pubmed_api_key":"saved-pubmed-key"`) || !strings.Contains(raw, `"pubmed_email":"saved@example.org"`) {
		t.Fatalf("raw = %s", raw)
	}
	if client.apiKey != "saved-pubmed-key" || client.email != "saved@example.org" || client.tool != "saved-tool" {
		t.Fatalf("live settings = key %q email %q tool %q", client.apiKey, client.email, client.tool)
	}
}

func TestPutAIExternalSearchSettingsPersistsSavedSettingsButKeepsConfiguredLiveValues(t *testing.T) {
	h, repo := newSettingsHandlerForTest(t)
	client := &stubPubMedSettingsClient{}
	h.SetPubMedClient(client, PubMedRuntimeSettings{
		APIKey:    "env-pubmed-key",
		APIKeySet: true,
		Tool:      "env-tool",
		ToolSet:   true,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/settings/ai-external-search", bytes.NewBufferString(`{
		"sources":["pubmed"],
		"pubmed_api_key":" saved-pubmed-key ",
		"pubmed_email":" saved@example.org ",
		"pubmed_tool":" saved-tool "
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
	if !strings.Contains(raw, `"pubmed_api_key":"saved-pubmed-key"`) || !strings.Contains(raw, `"pubmed_tool":"saved-tool"`) {
		t.Fatalf("raw = %s", raw)
	}
	if client.apiKey != "env-pubmed-key" || client.email != "saved@example.org" || client.tool != "env-tool" {
		t.Fatalf("live settings = key %q email %q tool %q", client.apiKey, client.email, client.tool)
	}
}

func TestPutAIExternalSearchSettingsHotReloadsSavedSettingsWhenNoConfiguredValues(t *testing.T) {
	h, _ := newSettingsHandlerForTest(t)
	client := &stubPubMedSettingsClient{}
	h.SetPubMedClient(client, PubMedRuntimeSettings{})
	req := httptest.NewRequest(http.MethodPut, "/api/settings/ai-external-search", bytes.NewBufferString(`{
		"sources":["pubmed"],
		"pubmed_api_key":" saved-pubmed-key ",
		"pubmed_email":" saved@example.org ",
		"pubmed_tool":" saved-tool "
	}`))
	rec := httptest.NewRecorder()

	h.PutAIExternalSearchSettings(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if client.apiKey != "saved-pubmed-key" || client.email != "saved@example.org" || client.tool != "saved-tool" {
		t.Fatalf("live settings = key %q email %q tool %q", client.apiKey, client.email, client.tool)
	}
}

type stubResearchSettingsClient struct {
	apiKey string
}

func (s *stubResearchSettingsClient) SetAPIKey(apiKey string) {
	s.apiKey = apiKey
}

type stubPubMedSettingsClient struct {
	apiKey string
	email  string
	tool   string
}

func (s *stubPubMedSettingsClient) SetSettings(apiKey, email, tool string) {
	s.apiKey = apiKey
	s.email = email
	s.tool = tool
}

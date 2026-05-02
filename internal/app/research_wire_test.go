package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/xuzhougeng/citebox/internal/config"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service"
	"github.com/xuzhougeng/citebox/internal/service/ai_external"
)

func newAIExternalSettingsShimForTest(t *testing.T) (aiExternalSettingsShim, *service.LibraryService) {
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
	return aiExternalSettingsShim{librarySvc: librarySvc}, librarySvc
}

func sourcesContain(sources []ai_external.SourceID, want ai_external.SourceID) bool {
	for _, s := range sources {
		if s == want {
			return true
		}
	}
	return false
}

func TestEnabledExternalSourcesAlwaysIncludesPubMed(t *testing.T) {
	shim, _ := newAIExternalSettingsShimForTest(t)

	got, err := shim.EnabledExternalSources(context.Background())
	if err != nil {
		t.Fatalf("EnabledExternalSources() error = %v", err)
	}
	if !sourcesContain(got, ai_external.SourcePubMed) {
		t.Fatalf("EnabledExternalSources() = %v, want pubmed always enabled", got)
	}
}

func TestEnabledExternalSourcesIncludesSemanticScholarWhenAPIKeySet(t *testing.T) {
	shim, svc := newAIExternalSettingsShimForTest(t)
	if err := svc.UpsertAppSetting("s2_api_key", "user-s2-key"); err != nil {
		t.Fatalf("UpsertAppSetting() error = %v", err)
	}

	got, err := shim.EnabledExternalSources(context.Background())
	if err != nil {
		t.Fatalf("EnabledExternalSources() error = %v", err)
	}
	if !sourcesContain(got, ai_external.SourceSemanticScholar) {
		t.Fatalf("EnabledExternalSources() = %v, want to include semantic_scholar", got)
	}
}

func TestEnabledExternalSourcesExcludesSemanticScholarWhenAPIKeyEmpty(t *testing.T) {
	shim, _ := newAIExternalSettingsShimForTest(t)

	got, err := shim.EnabledExternalSources(context.Background())
	if err != nil {
		t.Fatalf("EnabledExternalSources() error = %v", err)
	}
	if sourcesContain(got, ai_external.SourceSemanticScholar) {
		t.Fatalf("EnabledExternalSources() = %v, should not include semantic_scholar without API key", got)
	}
}

func TestEnabledExternalSourcesIgnoresLegacySourcesField(t *testing.T) {
	shim, svc := newAIExternalSettingsShimForTest(t)
	// Legacy data: a stored Sources slice (whatever it contains) is no longer
	// the source of truth — credentials drive gating now.
	if _, err := svc.UpdateAIExternalSearchSettings(model.AIExternalSearchSettings{
		Sources: []string{model.AIExternalSourceSemanticScholar},
	}); err != nil {
		t.Fatalf("UpdateAIExternalSearchSettings() error = %v", err)
	}

	got, err := shim.EnabledExternalSources(context.Background())
	if err != nil {
		t.Fatalf("EnabledExternalSources() error = %v", err)
	}
	if sourcesContain(got, ai_external.SourceSemanticScholar) {
		t.Fatalf("EnabledExternalSources() = %v, should ignore legacy semantic_scholar in Sources without API key", got)
	}
	if !sourcesContain(got, ai_external.SourcePubMed) {
		t.Fatalf("EnabledExternalSources() = %v, want pubmed always enabled", got)
	}
}

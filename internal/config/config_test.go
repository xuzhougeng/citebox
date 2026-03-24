package config

import (
	"path/filepath"
	"testing"
)

func TestEffectiveExtractorURLFromBaseURL(t *testing.T) {
	cfg := &Config{ExtractorURL: "http://127.0.0.1:8000"}

	if got := cfg.EffectiveExtractorURL(); got != "http://127.0.0.1:8000/api/v1/extract" {
		t.Fatalf("unexpected extract url: %s", got)
	}
	if got := cfg.EffectiveExtractorJobsURL(); got != "http://127.0.0.1:8000/api/v1/jobs" {
		t.Fatalf("unexpected jobs url: %s", got)
	}
}

func TestEffectiveExtractorJobsURLIgnoresSeparateOverride(t *testing.T) {
	cfg := &Config{
		ExtractorURL:     "http://127.0.0.1:8000/api/v1/extract",
		ExtractorJobsURL: "http://127.0.0.1:9000",
	}

	if got := cfg.EffectiveExtractorJobsURL(); got != "http://127.0.0.1:8000/api/v1/jobs" {
		t.Fatalf("unexpected jobs url: %s", got)
	}
}

func TestEffectiveExtractorURLKeepsExplicitEndpoint(t *testing.T) {
	cfg := &Config{ExtractorURL: "http://127.0.0.1:8000/api/analyze"}

	if got := cfg.EffectiveExtractorURL(); got != "http://127.0.0.1:8000/api/analyze" {
		t.Fatalf("unexpected explicit extract url: %s", got)
	}
	if got := cfg.EffectiveExtractorJobsURL(); got != "" {
		t.Fatalf("unexpected jobs url for legacy endpoint: %s", got)
	}
}

func TestApplyDesktopDefaultsUsesUserConfigDir(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	cfg := Load()
	if err := cfg.ApplyDesktopDefaults("CiteBox"); err != nil {
		t.Fatalf("ApplyDesktopDefaults() error = %v", err)
	}

	baseDir := filepath.Join(configHome, "CiteBox")
	if cfg.UploadDir != filepath.Join(baseDir, "uploads") {
		t.Fatalf("unexpected upload dir: %s", cfg.UploadDir)
	}
	if cfg.StorageDir != filepath.Join(baseDir, "library") {
		t.Fatalf("unexpected storage dir: %s", cfg.StorageDir)
	}
	if cfg.DatabasePath != filepath.Join(baseDir, "library.db") {
		t.Fatalf("unexpected database path: %s", cfg.DatabasePath)
	}
}

func TestApplyDesktopDefaultsKeepsExplicitEnv(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("UPLOAD_DIR", "/tmp/citebox-upload")
	t.Setenv("STORAGE_DIR", "/tmp/citebox-storage")
	t.Setenv("DATABASE_PATH", "/tmp/citebox.db")

	cfg := Load()
	if err := cfg.ApplyDesktopDefaults("CiteBox"); err != nil {
		t.Fatalf("ApplyDesktopDefaults() error = %v", err)
	}

	if cfg.UploadDir != "/tmp/citebox-upload" {
		t.Fatalf("unexpected upload dir: %s", cfg.UploadDir)
	}
	if cfg.StorageDir != "/tmp/citebox-storage" {
		t.Fatalf("unexpected storage dir: %s", cfg.StorageDir)
	}
	if cfg.DatabasePath != "/tmp/citebox.db" {
		t.Fatalf("unexpected database path: %s", cfg.DatabasePath)
	}
}

func TestLoadReadsDisableAuth(t *testing.T) {
	t.Setenv("DISABLE_AUTH", "1")

	cfg := Load()
	if !cfg.DisableAuth {
		t.Fatal("Load() DisableAuth = false, want true")
	}
}

package ai

import (
	"testing"

	"github.com/xuzhougeng/citebox/internal/model"
)

func TestNormalizeImageGenSettings_Defaults(t *testing.T) {
	defaults := model.DefaultAISettings().ImageGen
	got := normalizeImageGenSettings(model.AIImageGenSettings{Enabled: true}, defaults)

	if got.BaseURL != defaults.BaseURL {
		t.Errorf("BaseURL: got %q want %q", got.BaseURL, defaults.BaseURL)
	}
	if got.Model != "gpt-image-2" {
		t.Errorf("Model: got %q want gpt-image-2", got.Model)
	}
	if got.Size != "1024x1024" {
		t.Errorf("Size: got %q want 1024x1024", got.Size)
	}
	if got.Quality != "high" {
		t.Errorf("Quality: got %q want high", got.Quality)
	}
}

func TestNormalizeImageGenSettings_RejectsInvalidEnums(t *testing.T) {
	defaults := model.DefaultAISettings().ImageGen
	got := normalizeImageGenSettings(model.AIImageGenSettings{
		Enabled: true,
		Size:    "9999x9999",
		Quality: "ULTRA",
	}, defaults)

	if got.Size != defaults.Size {
		t.Errorf("Size should fall back: got %q", got.Size)
	}
	if got.Quality != defaults.Quality {
		t.Errorf("Quality should fall back: got %q", got.Quality)
	}
}

func TestNormalizeImageGenSettings_TrimsAPIKey(t *testing.T) {
	defaults := model.DefaultAISettings().ImageGen
	got := normalizeImageGenSettings(model.AIImageGenSettings{
		Enabled: true,
		APIKey:  "  sk-test  ",
		BaseURL: "https://api.example.com/",
	}, defaults)

	if got.APIKey != "sk-test" {
		t.Errorf("APIKey should be trimmed: got %q", got.APIKey)
	}
	if got.BaseURL != "https://api.example.com" {
		t.Errorf("BaseURL trailing slash should be trimmed: got %q", got.BaseURL)
	}
}

package app

import (
	"testing"

	"github.com/xuzhougeng/citebox/internal/config"
	"github.com/xuzhougeng/citebox/internal/model"
)

func TestStartupPubMedSettingsUsesSavedToolWhenEnvUnset(t *testing.T) {
	cfg := &config.Config{
		PubMedAPIKey: "cfg-key",
		PubMedEmail:  "cfg@example.org",
		PubMedTool:   "citebox",
	}
	settings := &model.AIExternalSearchSettings{
		PubMedAPIKey: "saved-key",
		PubMedEmail:  "saved@example.org",
		PubMedTool:   "saved-tool",
	}

	apiKey, email, tool := startupPubMedSettings(cfg, settings, envLookup(nil))

	if apiKey != "cfg-key" {
		t.Fatalf("apiKey = %q, want cfg-key", apiKey)
	}
	if email != "cfg@example.org" {
		t.Fatalf("email = %q, want cfg@example.org", email)
	}
	if tool != "saved-tool" {
		t.Fatalf("tool = %q, want saved-tool", tool)
	}
}

func TestStartupPubMedSettingsEnvToolWinsOverSavedTool(t *testing.T) {
	cfg := &config.Config{PubMedTool: "citebox"}
	settings := &model.AIExternalSearchSettings{PubMedTool: "saved-tool"}

	_, _, tool := startupPubMedSettings(cfg, settings, envLookup(map[string]string{
		"PUBMED_TOOL": "env-tool",
	}))

	if tool != "env-tool" {
		t.Fatalf("tool = %q, want env-tool", tool)
	}
}

func TestStartupPubMedSettingsUsesConfigAPIKeyAndEmailOverSaved(t *testing.T) {
	cfg := &config.Config{
		PubMedAPIKey: "cfg-key",
		PubMedEmail:  "cfg@example.org",
	}
	settings := &model.AIExternalSearchSettings{
		PubMedAPIKey: "saved-key",
		PubMedEmail:  "saved@example.org",
	}

	apiKey, email, _ := startupPubMedSettings(cfg, settings, envLookup(nil))

	if apiKey != "cfg-key" {
		t.Fatalf("apiKey = %q, want cfg-key", apiKey)
	}
	if email != "cfg@example.org" {
		t.Fatalf("email = %q, want cfg@example.org", email)
	}
}

func TestStartupPubMedSettingsDefaultsToolWhenUnsetEverywhere(t *testing.T) {
	_, _, tool := startupPubMedSettings(&config.Config{}, &model.AIExternalSearchSettings{}, envLookup(nil))

	if tool != "citebox" {
		t.Fatalf("tool = %q, want citebox", tool)
	}
}

func TestStartupS2APIKeyUsesConfigOverSaved(t *testing.T) {
	if got := startupS2APIKey(&config.Config{S2APIKey: "cfg-key"}, "saved-key"); got != "cfg-key" {
		t.Fatalf("startupS2APIKey() = %q, want cfg-key", got)
	}
}

func TestStartupS2APIKeyFallsBackToSaved(t *testing.T) {
	if got := startupS2APIKey(&config.Config{}, "saved-key"); got != "saved-key" {
		t.Fatalf("startupS2APIKey() = %q, want saved-key", got)
	}
}

func envLookup(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

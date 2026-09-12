package service

import (
	"testing"

	"github.com/xuzhougeng/citebox/internal/model"
)

func TestImageCapabilityUnknownSurvivesSettingsRoundTrip(t *testing.T) {
	_, repo, cfg := newTestService(t)
	svc := NewAIService(repo, cfg, nil)
	settings := model.DefaultAISettings()
	settings.Models[0].Model = "custom-text-model"
	settings.Models[0].SupportsImages = nil
	if _, err := svc.UpdateSettings(settings); err != nil {
		t.Fatal(err)
	}
	loaded, err := svc.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Models[0].SupportsImages != nil {
		t.Fatal("unknown model capability became explicit support")
	}
	for _, flag := range []bool{false, true} {
		settings.Models[0].SupportsImages = &flag
		if _, err := svc.UpdateSettings(settings); err != nil {
			t.Fatal(err)
		}
		loaded, err := svc.GetSettings()
		if err != nil {
			t.Fatal(err)
		}
		if loaded.Models[0].SupportsImages == nil || *loaded.Models[0].SupportsImages != flag {
			t.Fatal("explicit capability lost")
		}
	}
}

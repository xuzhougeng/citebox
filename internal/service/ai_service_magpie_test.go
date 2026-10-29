package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/xuzhougeng/citebox/internal/model"
)

func TestMagpieRuntimeUsesGatewayAndRestoresSavedSettings(t *testing.T) {
	_, repo, cfg := newTestService(t)
	cfg.MagpieConfigPath = filepath.Join(t.TempDir(), "magpie.json")
	svc := NewAIService(repo, cfg, nil)
	original, err := svc.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	original.Models[0].APIKey = "personal-key"
	original, err = svc.UpdateSettings(*original)
	if err != nil {
		t.Fatal(err)
	}
	storedBefore, _ := repo.GetAppSetting(aiSettingsKey)
	var seenModel string
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("wrong path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer fixture-key" {
			t.Error("wrong gateway key")
		}
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		seenModel = body.Model
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"fixture-ok"}}]}`))
	}))
	defer gateway.Close()
	for _, m := range []string{"fixture/first", "fixture/second"} {
		data, _ := json.Marshal(map[string]string{"baseURL": gateway.URL + "/v1", "apiKey": "fixture-key", "model": m})
		if err := os.WriteFile(cfg.MagpieConfigPath, data, 0600); err != nil {
			t.Fatal(err)
		}
		settings, err := svc.RuntimeSettings().GetSettings()
		if err != nil {
			t.Fatal(err)
		}
		text, _, err := svc.CallProviderGeneric(context.Background(), *settings, "system", "hello")
		if err != nil || text != "fixture-ok" || seenModel != m {
			t.Fatalf("gateway request: text=%s model=%s error=%v", text, seenModel, err)
		}
		ui, _ := svc.GetSettings()
		if !reflect.DeepEqual(ui, original) {
			t.Fatal("managed settings leaked into the settings editor")
		}
		// An ordinary settings save cannot write the external model/key.
		if _, err := svc.UpdateSettings(*ui); err != nil {
			t.Fatal(err)
		}
		stored, _ := repo.GetAppSetting(aiSettingsKey)
		if stored != storedBefore {
			t.Fatal("personal settings changed")
		}
	}
	if err := os.WriteFile(cfg.MagpieConfigPath, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	restored, err := svc.GetRuntimeSettings()
	if err != nil || !reflect.DeepEqual(restored, original) {
		t.Fatal("disconnect did not restore personal settings")
	}
	if restored.Provider != model.AIProviderOpenAI {
		t.Fatal("unexpected original provider")
	}
}

// Opt-in probe used by magpie-agents' real-gateway test after it writes the file.
func TestMagpieExternalGateway(t *testing.T) {
	path := os.Getenv("MAGPIE_TEST_CONFIG")
	if path == "" {
		t.Skip("requires an isolated Magpie fixture")
	}
	_, repo, cfg := newTestService(t)
	cfg.MagpieConfigPath = path
	svc := NewAIService(repo, cfg, nil)
	settings, err := svc.GetRuntimeSettings()
	if err != nil {
		t.Fatal(err)
	}
	text, _, err := svc.CallProviderGeneric(context.Background(), *settings, "system", "fixture")
	if err != nil || text != "fixture-ok" {
		t.Fatalf("Magpie gateway: %v, %s", err, text)
	}
}

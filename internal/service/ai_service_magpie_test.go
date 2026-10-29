package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/apperr"
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
	// Stands in for a level saved in the settings page, possibly under another
	// model, so it is stored as is rather than validated against this one.
	if effort := os.Getenv("MAGPIE_TEST_REASONING_EFFORT"); effort != "" {
		payload, _ := json.Marshal(magpieSettings{ReasoningEffort: effort})
		if err := repo.UpsertAppSetting(magpieSettingsKey, string(payload)); err != nil {
			t.Fatal(err)
		}
	}
	settings, err := svc.GetRuntimeSettings()
	if err != nil {
		t.Fatal(err)
	}
	text, _, err := svc.CallProviderGeneric(context.Background(), *settings, "system", "fixture")
	if err != nil || text != "fixture-ok" {
		t.Fatalf("Magpie gateway: %v, %s", err, text)
	}
}

func TestMagpieRuntimeSendsModelOutputAndSelectedEffort(t *testing.T) {
	_, repo, cfg := newTestService(t)
	cfg.MagpieConfigPath = filepath.Join(t.TempDir(), "magpie.json")
	svc := NewAIService(repo, cfg, nil)
	storedBefore, _ := repo.GetAppSetting(aiSettingsKey)
	var body map[string]any
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body = nil
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"fixture-ok"}}]}`))
	}))
	defer gateway.Close()
	connect := func(model string, info any) {
		t.Helper()
		values := map[string]any{"baseURL": gateway.URL + "/v1", "apiKey": "fixture-key", "model": model}
		if info != nil {
			values["modelInfo"] = info
		}
		data, _ := json.Marshal(values)
		if err := os.WriteFile(cfg.MagpieConfigPath, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	send := func(wantEffort any, wantMaxTokens float64) {
		t.Helper()
		settings, err := svc.RuntimeSettings().GetSettings()
		if err != nil {
			t.Fatal(err)
		}
		if text, _, err := svc.CallProviderGeneric(context.Background(), *settings, "system", "hello"); err != nil || text != "fixture-ok" {
			t.Fatalf("gateway request: %s %v", text, err)
		}
		if body["reasoning_effort"] != wantEffort || body["max_tokens"] != wantMaxTokens {
			t.Fatalf("reasoning_effort=%v max_tokens=%v, want %v %v", body["reasoning_effort"], body["max_tokens"], wantEffort, wantMaxTokens)
		}
		if _, ok := body["thinking"]; ok {
			t.Fatal("vendor thinking switch sent to Magpie")
		}
	}

	reasoner := map[string]any{"id": "p/reasoner", "context": 400000, "output": 128000, "efforts": []string{"low", "medium", "high"}}
	connect("p/reasoner", reasoner)
	send("medium", 128000)
	runtime, err := svc.MagpieRuntime()
	if err != nil || !reflect.DeepEqual(runtime, &model.AIMagpieRuntime{Connected: true, Model: "p/reasoner", ContextTokens: 400000,
		MaxOutputTokens: 128000, ReasoningEfforts: []string{"low", "medium", "high"}, DefaultReasoningEffort: "medium", ReasoningEffort: "medium"}) {
		t.Fatalf("runtime: %+v %v", runtime, err)
	}
	if _, err := svc.UpdateMagpieReasoningEffort("max"); apperr.CodeOf(err) != apperr.CodeInvalidArgument {
		t.Fatalf("unsupported effort accepted: %v", err)
	}
	if runtime, err = svc.UpdateMagpieReasoningEffort("high"); err != nil || runtime.ReasoningEffort != "high" {
		t.Fatalf("select high: %+v %v", runtime, err)
	}
	send("high", 128000)
	if settings, _ := svc.GetRuntimeSettings(); settings.LightReasoningEffort != "low" || settings.HelperTimeout != magpieHelperTimeout {
		t.Fatalf("helper calls: %q %v", settings.LightReasoningEffort, settings.HelperTimeout)
	}
	// The choice is kept apart from the personal settings and never holds the gateway key.
	if stored, _ := repo.GetAppSetting(aiSettingsKey); stored != storedBefore {
		t.Fatal("personal settings changed")
	}
	if saved, _ := repo.GetAppSetting(magpieSettingsKey); saved != `{"reasoning_effort":"high"}` {
		t.Fatalf("saved choice: %s", saved)
	}
	if ui, _ := svc.GetSettings(); ui.ReasoningEffort != "" || strings.Contains(ui.APIKey, "fixture") {
		t.Fatal("managed values leaked into the settings editor")
	}

	// A model without the saved level falls back to its default; the choice
	// returns when a model supports it again.
	connect("p/fast", map[string]any{"id": "p/fast", "output": 32000, "efforts": []string{"minimal", "low"}})
	send("minimal", 32000)
	connect("p/reasoner", reasoner)
	send("high", 128000)

	// Unknown or stale facts leave the provider default and the previous limit.
	connect("p/opaque", nil)
	send(nil, 8192)
	connect("p/edited", reasoner)
	send(nil, 8192)
	connect("p/reasoner", map[string]any{"id": "p/reasoner", "output": "128000", "efforts": []string{"high"}})
	send(nil, 8192)
	if runtime, _ := svc.MagpieRuntime(); len(runtime.ReasoningEfforts) != 0 || runtime.ReasoningEffort != "" {
		t.Fatalf("unknown model offers levels: %+v", runtime)
	}
	if _, err := svc.UpdateMagpieReasoningEffort("high"); err == nil {
		t.Fatal("level saved for a model without levels")
	}
	if _, err := svc.UpdateMagpieReasoningEffort(""); err != nil {
		t.Fatal(err)
	}
	if saved, _ := repo.GetAppSetting(magpieSettingsKey); saved != "" {
		t.Fatalf("reset kept %s", saved)
	}

	if err := os.WriteFile(cfg.MagpieConfigPath, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if runtime, _ := svc.MagpieRuntime(); runtime.Connected {
		t.Fatal("disconnected runtime reported")
	}
	if _, err := svc.UpdateMagpieReasoningEffort("high"); err == nil {
		t.Fatal("level saved without a connection")
	}
}

func TestMagpieLightEffortForDeadlineCalls(t *testing.T) {
	for _, tc := range []struct {
		efforts []string
		effort  string
		want    string
	}{
		{[]string{"low", "medium", "high"}, "high", "low"},
		{[]string{"minimal", "medium"}, "medium", "minimal"},
		{[]string{"medium", "high"}, "high", "high"},
		{[]string{"minimal", "low", "high"}, "minimal", "minimal"},
		{nil, "", ""},
	} {
		if got := magpieLightEffort(tc.efforts, tc.effort); got != tc.want {
			t.Fatalf("%v %s: got %s, want %s", tc.efforts, tc.effort, got, tc.want)
		}
	}
}

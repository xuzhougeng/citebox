package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

func TestCheckModelCallsProviderSuccessfully(t *testing.T) {
	_, repo, cfg := newTestService(t)
	aiSvc := NewAIService(repo, cfg, nil)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output_text":"OK"}`))
	}))
	defer server.Close()

	result, err := aiSvc.CheckModel(context.Background(), model.AIModelConfig{
		ID:       "check-openai",
		Name:     "Check OpenAI",
		Provider: model.AIProviderOpenAI,
		APIKey:   "test-key",
		BaseURL:  server.URL,
		Model:    "gpt-test",
	})
	if err != nil {
		t.Fatalf("CheckModel() error = %v", err)
	}
	if !result.Success || result.Model != "gpt-test" || result.Mode != "responses" {
		t.Fatalf("CheckModel() = %+v, want success for responses mode", result)
	}
}

func TestCheckModelOmitsTemperatureForGPT5Family(t *testing.T) {
	_, repo, cfg := newTestService(t)
	aiSvc := NewAIService(repo, cfg, nil)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if _, exists := payload["temperature"]; exists {
			t.Fatalf("request payload includes unsupported temperature: %+v", payload)
		}
		if payload["model"] != "gpt-5.5" {
			t.Fatalf("request model = %v, want gpt-5.5", payload["model"])
		}
		reasoning, ok := payload["reasoning"].(map[string]interface{})
		if !ok || reasoning["effort"] != "high" {
			t.Fatalf("reasoning payload = %+v, want high effort", payload["reasoning"])
		}
		if _, exists := payload["thinking"]; exists {
			t.Fatalf("responses payload should not include raw thinking parameter: %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output_text":"OK"}`))
	}))
	defer server.Close()

	result, err := aiSvc.CheckModel(context.Background(), model.AIModelConfig{
		ID:              "check-gpt55",
		Name:            "Check GPT 5.5",
		Provider:        model.AIProviderOpenAI,
		APIKey:          "test-key",
		BaseURL:         server.URL,
		Model:           "gpt-5.5",
		MaxOutputTokens: 1200,
		ThinkingEnabled: true,
		ReasoningEffort: "high",
	})
	if err != nil {
		t.Fatalf("CheckModel() error = %v", err)
	}
	if !result.Success || result.Model != "gpt-5.5" {
		t.Fatalf("CheckModel() = %+v, want success for gpt-5.5", result)
	}
}

func TestCheckModelUnsupportedTemperatureErrorIncludesHint(t *testing.T) {
	_, repo, cfg := newTestService(t)
	aiSvc := NewAIService(repo, cfg, nil)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"Unsupported parameter: 'temperature' is not supported with this model."}}`))
	}))
	defer server.Close()

	_, err := aiSvc.CheckModel(context.Background(), model.AIModelConfig{
		ID:       "check-custom-no-temp",
		Name:     "Check Custom No Temperature",
		Provider: model.AIProviderOpenAI,
		APIKey:   "test-key",
		BaseURL:  server.URL,
		Model:    "custom-no-temperature",
	})
	if err == nil {
		t.Fatal("CheckModel() error = nil, want unsupported temperature error")
	}
	message := err.Error()
	if !strings.Contains(message, "Unsupported parameter") || !strings.Contains(message, "不发送 temperature 参数") {
		t.Fatalf("CheckModel() error = %q, want original provider error and actionable temperature hint", message)
	}
}

func TestCheckModelResponsesThinkingDefaultsReasoningEffort(t *testing.T) {
	_, repo, cfg := newTestService(t)
	aiSvc := NewAIService(repo, cfg, nil)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		reasoning, ok := payload["reasoning"].(map[string]interface{})
		if !ok || reasoning["effort"] != "medium" {
			t.Fatalf("reasoning payload = %+v, want default medium effort", payload["reasoning"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output_text":"OK"}`))
	}))
	defer server.Close()

	if _, err := aiSvc.CheckModel(context.Background(), model.AIModelConfig{
		ID:              "check-gpt55-thinking",
		Name:            "Check GPT 5.5 Thinking",
		Provider:        model.AIProviderOpenAI,
		APIKey:          "test-key",
		BaseURL:         server.URL,
		Model:           "gpt-5.5",
		MaxOutputTokens: 1200,
		ThinkingEnabled: true,
	}); err != nil {
		t.Fatalf("CheckModel() error = %v", err)
	}
}

func TestCheckModelSendsChatCompletionsThinkingOptions(t *testing.T) {
	_, repo, cfg := newTestService(t)
	aiSvc := NewAIService(repo, cfg, nil)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		thinking, ok := payload["thinking"].(map[string]interface{})
		if !ok || thinking["type"] != "enabled" {
			t.Fatalf("thinking payload = %+v, want enabled", payload["thinking"])
		}
		if payload["reasoning_effort"] != "high" {
			t.Fatalf("reasoning_effort = %v, want high", payload["reasoning_effort"])
		}
		if _, exists := payload["temperature"]; !exists {
			t.Fatalf("request payload missing temperature: %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"OK"}}]}`))
	}))
	defer server.Close()

	result, err := aiSvc.CheckModel(context.Background(), model.AIModelConfig{
		ID:               "deepseek-pro",
		Name:             "DeepSeek Pro",
		Provider:         model.AIProviderOpenAI,
		APIKey:           "test-key",
		BaseURL:          server.URL,
		Model:            "deepseek-v4-pro",
		MaxOutputTokens:  4096,
		OpenAILegacyMode: true,
		ThinkingEnabled:  true,
		ReasoningEffort:  "high",
	})
	if err != nil {
		t.Fatalf("CheckModel() error = %v", err)
	}
	if !result.Success || result.Mode != "chat_completions" {
		t.Fatalf("CheckModel() = %+v, want chat completions success", result)
	}
}

func TestJoinProviderURLUsesDeepSeekRootChatCompletionsEndpoint(t *testing.T) {
	got := joinProviderURL("https://api.deepseek.com", "https://api.openai.com", "/v1/chat/completions")
	if got != "https://api.deepseek.com/chat/completions" {
		t.Fatalf("joinProviderURL() = %q, want DeepSeek root chat completions endpoint", got)
	}
}

func TestReadPaperStreamSupportsPaperQA(t *testing.T) {
	_, repo, cfg := newTestService(t)
	aiSvc := NewAIService(repo, cfg, nil)
	paper := createTestPaper(t, repo)
	writeFigureFixture(t, filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename), 320, 220)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		bodyText := string(body)
		if !strings.Contains(bodyText, "\"stream\":true") {
			t.Fatalf("request body = %s, want streaming payload", bodyText)
		}
		if strings.Contains(bodyText, "JSON 必须包含 answer") {
			t.Fatalf("request body = %s, want plain text stream prompt for paper_qa", bodyText)
		}
		if !strings.Contains(bodyText, "不要返回 JSON") {
			t.Fatalf("request body = %s, want plain text output requirements", bodyText)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"第一段\"}\n\n")
		_, _ = fmt.Fprint(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"第二段\"}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	if _, err := aiSvc.UpdateSettings(model.AISettings{
		Models: []model.AIModelConfig{
			{
				ID:              "qa",
				Name:            "QA",
				Provider:        model.AIProviderOpenAI,
				APIKey:          "test-key",
				BaseURL:         server.URL,
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

	var events []model.AIReadStreamEvent
	err := aiSvc.ReadPaperStream(context.Background(), model.AIReadRequest{
		PaperID:  paper.ID,
		Action:   model.AIActionPaperQA,
		Question: "请总结这篇文献。",
	}, func(event model.AIReadStreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("ReadPaperStream() error = %v", err)
	}

	if len(events) != 5 {
		t.Fatalf("event count = %d, want 5", len(events))
	}
	if events[0].Type != "meta" || events[1].Type != "delta" || events[2].Type != "delta" || events[3].Type != "final" || events[4].Type != "done" {
		t.Fatalf("event types = %#v, want meta/delta/delta/final/done", []string{events[0].Type, events[1].Type, events[2].Type, events[3].Type, events[4].Type})
	}
	if events[0].Result == nil || events[0].Result.Action != model.AIActionPaperQA {
		t.Fatalf("meta result = %#v, want paper_qa metadata", events[0].Result)
	}
	if events[3].Result == nil {
		t.Fatal("final result = nil, want normalized response")
	}
	if events[3].Result.Answer != "第一段第二段" {
		t.Fatalf("final answer = %q, want merged stream text", events[3].Result.Answer)
	}
	if events[3].Result.Question != "请总结这篇文献。" {
		t.Fatalf("final question = %q, want original question", events[3].Result.Question)
	}
}

func TestPrepareReadPaperQACanOmitFigures(t *testing.T) {
	_, repo, cfg := newTestService(t)
	aiSvc := NewAIService(repo, cfg, nil)
	paper := createTestPaper(t, repo)
	writeFigureFixture(t, filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename), 320, 220)

	if _, err := aiSvc.UpdateSettings(model.AISettings{
		Models: []model.AIModelConfig{
			{
				ID:              "qa",
				Name:            "QA Text",
				Provider:        model.AIProviderOpenAI,
				APIKey:          "test-key",
				BaseURL:         "https://api.openai.com",
				Model:           "text-only-model",
				MaxOutputTokens: 1200,
			},
		},
		SceneModels: model.AISceneModelSelection{
			DefaultModelID: "qa",
			QAModelID:      "qa",
		},
		MaxFigures:   1,
		SystemPrompt: "system",
		QAPrompt:     "qa",
	}); err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}

	includeFigures := false
	prepared, err := aiSvc.prepareRead(model.AIReadRequest{
		PaperID:        paper.ID,
		Action:         model.AIActionPaperQA,
		Question:       "请解释划选内容。",
		IncludeFigures: &includeFigures,
	}, true)
	if err != nil {
		t.Fatalf("prepareRead() error = %v", err)
	}

	if prepared.includedFigures != 0 || len(prepared.images) != 0 {
		t.Fatalf("prepareRead() included=%d images=%d, want 0/0", prepared.includedFigures, len(prepared.images))
	}
	if strings.Contains(prepared.userPrompt, "caption=Figure") {
		t.Fatalf("prepareRead() prompt includes figure summaries despite include_figures=false\n%s", prepared.userPrompt)
	}
}

func TestPrepareReadPaperQAOmitImagesForTextOnlyModel(t *testing.T) {
	_, repo, cfg := newTestService(t)
	aiSvc := NewAIService(repo, cfg, nil)
	paper := createTestPaper(t, repo)
	writeFigureFixture(t, filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename), 320, 220)

	supportsImages := false
	if _, err := aiSvc.UpdateSettings(model.AISettings{
		Models: []model.AIModelConfig{
			{
				ID:              "qa",
				Name:            "QA Text",
				Provider:        model.AIProviderOpenAI,
				APIKey:          "test-key",
				BaseURL:         "https://api.openai.com",
				Model:           "text-only-model",
				MaxOutputTokens: 1200,
				SupportsImages:  &supportsImages,
			},
		},
		SceneModels: model.AISceneModelSelection{
			DefaultModelID: "qa",
			QAModelID:      "qa",
		},
		MaxFigures:   1,
		SystemPrompt: "system",
		QAPrompt:     "qa",
	}); err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}

	prepared, err := aiSvc.prepareRead(model.AIReadRequest{
		PaperID:  paper.ID,
		Action:   model.AIActionPaperQA,
		Question: "请总结这篇文献。",
	}, true)
	if err != nil {
		t.Fatalf("prepareRead() error = %v", err)
	}

	if prepared.includedFigures != 0 || len(prepared.images) != 0 {
		t.Fatalf("prepareRead() included=%d images=%d, want 0/0 for text-only model", prepared.includedFigures, len(prepared.images))
	}
}

func TestPrepareReadRejectsImageRequiredActionForTextOnlyModel(t *testing.T) {
	_, repo, cfg := newTestService(t)
	aiSvc := NewAIService(repo, cfg, nil)
	paper := createTestPaper(t, repo)
	writeFigureFixture(t, filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename), 320, 220)

	supportsImages := false
	if _, err := aiSvc.UpdateSettings(model.AISettings{
		Models: []model.AIModelConfig{
			{
				ID:              "figure",
				Name:            "Figure Text",
				Provider:        model.AIProviderOpenAI,
				APIKey:          "test-key",
				BaseURL:         "https://api.openai.com",
				Model:           "text-only-model",
				MaxOutputTokens: 1200,
				SupportsImages:  &supportsImages,
			},
		},
		SceneModels: model.AISceneModelSelection{
			DefaultModelID: "figure",
			FigureModelID:  "figure",
		},
		SystemPrompt: "system",
		FigurePrompt: "figure",
	}); err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}

	_, err := aiSvc.prepareRead(model.AIReadRequest{
		PaperID:  paper.ID,
		FigureID: paper.Figures[0].ID,
		Action:   model.AIActionFigureInterpretation,
		Question: "请解读这张图。",
	}, true)
	if !apperr.IsCode(err, apperr.CodeFailedPrecondition) {
		t.Fatalf("prepareRead() code = %q, want %q", apperr.CodeOf(err), apperr.CodeFailedPrecondition)
	}
	if !strings.Contains(err.Error(), "图片输入") {
		t.Fatalf("prepareRead() error = %v, want image input hint", err)
	}
}

func TestCallProviderStreamGenericUsesResponsesDoneSnapshot(t *testing.T) {
	_, repo, cfg := newTestService(t)
	aiSvc := NewAIService(repo, cfg, nil)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: response.output_text.done\ndata: {\"type\":\"response.output_text.done\",\"text\":\"完整回答\"}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	settings := model.DefaultAISettings()
	settings.Provider = model.AIProviderOpenAI
	settings.APIKey = "test-key"
	settings.BaseURL = server.URL
	settings.Model = "gpt-test"
	settings.MaxOutputTokens = 1200

	var deltas []string
	raw, mode, err := aiSvc.CallProviderStreamGeneric(context.Background(), settings, "system", "question", nil, func(delta string) error {
		deltas = append(deltas, delta)
		return nil
	})
	if err != nil {
		t.Fatalf("CallProviderStreamGeneric() error = %v", err)
	}
	if raw != "完整回答" || mode != "responses" {
		t.Fatalf("raw/mode = %q/%q, want 完整回答/responses", raw, mode)
	}
	if strings.Join(deltas, "") != "完整回答" {
		t.Fatalf("deltas = %v", deltas)
	}
}

func TestCallProviderStreamGenericPersistsLongerResponsesSnapshot(t *testing.T) {
	_, repo, cfg := newTestService(t)
	aiSvc := NewAIService(repo, cfg, nil)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"部分\"}\n\n")
		_, _ = fmt.Fprint(w, "event: response.output_text.done\ndata: {\"type\":\"response.output_text.done\",\"text\":\"部分但完整得多\"}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	settings := model.DefaultAISettings()
	settings.Provider = model.AIProviderOpenAI
	settings.APIKey = "test-key"
	settings.BaseURL = server.URL
	settings.Model = "gpt-test"
	settings.MaxOutputTokens = 1200

	var deltas []string
	raw, mode, err := aiSvc.CallProviderStreamGeneric(context.Background(), settings, "system", "question", nil, func(delta string) error {
		deltas = append(deltas, delta)
		return nil
	})
	if err != nil {
		t.Fatalf("CallProviderStreamGeneric() error = %v", err)
	}
	if raw != "部分但完整得多" || mode != "responses" {
		t.Fatalf("raw/mode = %q/%q, want snapshot/responses", raw, mode)
	}
	if strings.Join(deltas, "") != "部分" {
		t.Fatalf("deltas = %v, want only streamed delta", deltas)
	}
}

func TestCallProviderStreamGenericErrorsOnEmptyResponsesStream(t *testing.T) {
	_, repo, cfg := newTestService(t)
	aiSvc := NewAIService(repo, cfg, nil)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	settings := model.DefaultAISettings()
	settings.Provider = model.AIProviderOpenAI
	settings.APIKey = "test-key"
	settings.BaseURL = server.URL
	settings.Model = "gpt-test"
	settings.MaxOutputTokens = 1200

	_, _, err := aiSvc.CallProviderStreamGeneric(context.Background(), settings, "system", "question", nil, func(string) error { return nil })
	if !apperr.IsCode(err, apperr.CodeUnavailable) {
		t.Fatalf("CallProviderStreamGeneric() code = %q, want %q", apperr.CodeOf(err), apperr.CodeUnavailable)
	}
}

func TestReadPaperStreamDoesNotEmitMetaWhenProviderFailsBeforeStreaming(t *testing.T) {
	_, repo, cfg := newTestService(t)
	aiSvc := NewAIService(repo, cfg, nil)
	paper := createTestPaper(t, repo)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = fmt.Fprint(w, `{"error":{"message":"insufficient_quota"}}`)
	}))
	defer server.Close()

	if _, err := aiSvc.UpdateSettings(model.AISettings{
		Models: []model.AIModelConfig{
			{
				ID:              "qa",
				Name:            "QA",
				Provider:        model.AIProviderOpenAI,
				APIKey:          "test-key",
				BaseURL:         server.URL,
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

	var events []model.AIReadStreamEvent
	err := aiSvc.ReadPaperStream(context.Background(), model.AIReadRequest{
		PaperID:  paper.ID,
		Action:   model.AIActionPaperQA,
		Question: "请总结这篇文献。",
	}, func(event model.AIReadStreamEvent) error {
		events = append(events, event)
		return nil
	})
	if !apperr.IsCode(err, apperr.CodeUnavailable) {
		t.Fatalf("ReadPaperStream() code = %q, want %q", apperr.CodeOf(err), apperr.CodeUnavailable)
	}
	if len(events) != 0 {
		t.Fatalf("event count = %d, want 0 before upstream stream starts", len(events))
	}
}

func TestPrepareReadUsesSceneModelMaxOutputTokens(t *testing.T) {
	svc, repo, _ := newTestService(t)
	aiSvc := NewAIService(repo, svc.config, nil)
	paper := createTestPaper(t, repo)

	_, err := aiSvc.UpdateSettings(model.AISettings{
		Models: []model.AIModelConfig{
			{
				ID:              "default",
				Name:            "Default",
				Provider:        model.AIProviderOpenAI,
				APIKey:          "key-default",
				BaseURL:         "https://api.openai.com",
				Model:           "gpt-default",
				MaxOutputTokens: 1200,
			},
			{
				ID:              "qa",
				Name:            "QA",
				Provider:        model.AIProviderAnthropic,
				APIKey:          "key-qa",
				BaseURL:         "https://api.anthropic.com",
				Model:           "claude-qa",
				MaxOutputTokens: 2048,
			},
		},
		SceneModels: model.AISceneModelSelection{
			DefaultModelID: "default",
			QAModelID:      "qa",
		},
		SystemPrompt: "system",
		QAPrompt:     "qa",
	})
	if err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}

	prepared, err := aiSvc.prepareRead(model.AIReadRequest{
		PaperID:  paper.ID,
		Action:   model.AIActionPaperQA,
		Question: "总结一下",
	}, true)
	if err != nil {
		t.Fatalf("prepareRead() error = %v", err)
	}

	if prepared.settings.Provider != model.AIProviderAnthropic || prepared.settings.Model != "claude-qa" {
		t.Fatalf("prepareRead() model = %s/%s, want anthropic/claude-qa", prepared.settings.Provider, prepared.settings.Model)
	}
	if prepared.settings.MaxOutputTokens != 2048 {
		t.Fatalf("prepareRead() max_output_tokens = %d, want 2048", prepared.settings.MaxOutputTokens)
	}
}

func TestPrepareReadFigureInterpretationUsesRequestedFigureOnly(t *testing.T) {
	_, repo, cfg := newTestService(t)
	aiSvc := NewAIService(repo, cfg, nil)

	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Figure Scoped AI",
		OriginalFilename: "figure-scoped-ai.pdf",
		StoredPDFName:    "figure-scoped-ai.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		PDFText:          "Full text for figure scoped AI.",
		ExtractionStatus: "completed",
		Figures: []repository.FigureUpsertInput{
			{Filename: "figure_a.png", ContentType: "image/png", PageNumber: 1, FigureIndex: 1, Caption: "First"},
			{Filename: "figure_b.png", ContentType: "image/png", PageNumber: 2, FigureIndex: 2, Caption: "Second"},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	writeFigureFixture(t, filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename), 320, 220)
	writeFigureFixture(t, filepath.Join(cfg.FiguresDir(), paper.Figures[1].Filename), 360, 240)

	if _, err := aiSvc.UpdateSettings(model.AISettings{
		Models: []model.AIModelConfig{
			{
				ID:              "figure",
				Name:            "Figure",
				Provider:        model.AIProviderOpenAI,
				APIKey:          "test-key",
				BaseURL:         "https://api.openai.com",
				Model:           "gpt-test",
				MaxOutputTokens: 1200,
			},
		},
		SceneModels: model.AISceneModelSelection{
			DefaultModelID: "figure",
			FigureModelID:  "figure",
			TagModelID:     "figure",
		},
		SystemPrompt: "system",
		FigurePrompt: "figure",
		TagPrompt:    "tag",
	}); err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}

	prepared, err := aiSvc.prepareRead(model.AIReadRequest{
		PaperID:  paper.ID,
		FigureID: paper.Figures[1].ID,
		Action:   model.AIActionFigureInterpretation,
		Question: "请解读当前图片。",
	}, true)
	if err != nil {
		t.Fatalf("prepareRead() error = %v", err)
	}

	if prepared.includedFigures != 1 || len(prepared.images) != 1 {
		t.Fatalf("prepareRead() included=%d images=%d, want 1/1", prepared.includedFigures, len(prepared.images))
	}
	if !strings.Contains(prepared.userPrompt, "caption=Second") {
		t.Fatalf("prepareRead() prompt missing selected figure summary\n%s", prepared.userPrompt)
	}
	if strings.Contains(prepared.userPrompt, "caption=First") {
		t.Fatalf("prepareRead() prompt leaked unselected figure summary\n%s", prepared.userPrompt)
	}
}

func TestDetectFigureRegionsUsesFigureSceneModel(t *testing.T) {
	_, repo, cfg := newTestService(t)
	aiSvc := NewAIService(repo, cfg, nil)

	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Region Detection",
		OriginalFilename: "region-detection.pdf",
		StoredPDFName:    "region-detection.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		bodyText := string(body)
		if !strings.Contains(bodyText, "Composite figures with subpanels A/B/C/D should usually be returned as one larger figure box") {
			t.Fatalf("request body missing composite-figure instruction: %s", bodyText)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output_text":"{\"figures\":[{\"bbox\":[100,120,700,820],\"confidence\":0.93}]}"}`))
	}))
	defer server.Close()

	if _, err := aiSvc.UpdateSettings(model.AISettings{
		Models: []model.AIModelConfig{
			{
				ID:              "figure",
				Name:            "Figure",
				Provider:        model.AIProviderOpenAI,
				APIKey:          "test-key",
				BaseURL:         server.URL,
				Model:           "gpt-figure",
				MaxOutputTokens: 900,
			},
		},
		SceneModels: model.AISceneModelSelection{
			DefaultModelID: "figure",
			FigureModelID:  "figure",
		},
		SystemPrompt: "system",
		FigurePrompt: "figure",
	}); err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}

	result, err := aiSvc.DetectFigureRegions(context.Background(), model.AIFigureRegionDetectRequest{
		PaperID:    paper.ID,
		PageNumber: 2,
		PageWidth:  1200,
		PageHeight: 1800,
		ImageData:  "data:image/png;base64," + base64.StdEncoding.EncodeToString(testFigurePNGBytes(t, 640, 960)),
	})
	if err != nil {
		t.Fatalf("DetectFigureRegions() error = %v", err)
	}
	if !result.Success || result.Model != "gpt-figure" || result.Provider != model.AIProviderOpenAI {
		t.Fatalf("DetectFigureRegions() = %+v, want figure scene model metadata", result)
	}
	if len(result.Regions) != 1 {
		t.Fatalf("DetectFigureRegions() regions = %+v, want 1 item", result.Regions)
	}
	if result.Regions[0].X <= 0 || result.Regions[0].Width <= 0 || result.Regions[0].Height <= 0 {
		t.Fatalf("DetectFigureRegions() region = %+v, want normalized bbox", result.Regions[0])
	}
}

func TestLoadFigureInputsCompressesOversizedFigure(t *testing.T) {
	_, repo, cfg := newTestService(t)
	aiSvc := NewAIService(repo, cfg, nil)

	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Large Figure AI",
		OriginalFilename: "large-figure-ai.pdf",
		StoredPDFName:    "large-figure-ai.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		PDFText:          "Full text for image compression.",
		ExtractionStatus: "completed",
		Figures: []repository.FigureUpsertInput{
			{Filename: "large_figure.png", ContentType: "image/png", PageNumber: 3, FigureIndex: 1, Caption: "Large"},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	writeFigureFixture(t, filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename), 2600, 1600)

	images, summaries, err := aiSvc.loadFigureInputs(paper, paper.Figures, model.AIActionPaperQA)
	if err != nil {
		t.Fatalf("loadFigureInputs() error = %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("loadFigureInputs() summaries = %d, want 1", len(summaries))
	}
	if !strings.Contains(summaries[0], "figure://") || !strings.Contains(summaries[0], "figure_id=") {
		t.Fatalf("loadFigureInputs() summary = %q, want figure reference instructions", summaries[0])
	}
	if len(images) != 1 {
		t.Fatalf("loadFigureInputs() images = %d, want 1", len(images))
	}
	if images[0].MIMEType != "image/jpeg" {
		t.Fatalf("loadFigureInputs() mime = %q, want image/jpeg", images[0].MIMEType)
	}

	decoded, err := base64.StdEncoding.DecodeString(images[0].Data)
	if err != nil {
		t.Fatalf("DecodeString() error = %v", err)
	}
	img, _, err := image.Decode(bytes.NewReader(decoded))
	if err != nil {
		t.Fatalf("Decode(compressed) error = %v", err)
	}
	if maxInt(img.Bounds().Dx(), img.Bounds().Dy()) > aiFigureImageMaxDimension {
		t.Fatalf("compressed image bounds = %dx%d, want max <= %d", img.Bounds().Dx(), img.Bounds().Dy(), aiFigureImageMaxDimension)
	}
}

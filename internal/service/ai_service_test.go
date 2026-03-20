package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

func testFigurePNGBytes(t *testing.T, width, height int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8((x*37 + y*11) % 256),
				G: uint8((x*19 + y*29) % 256),
				B: uint8((x*13 + y*7) % 256),
				A: 255,
			})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode() error = %v", err)
	}
	return buf.Bytes()
}

func writeFigureFixture(t *testing.T, path string, width, height int) []byte {
	t.Helper()

	data := testFigurePNGBytes(t, width, height)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
	return data
}

func TestAISettingsDefaultsAndPersistence(t *testing.T) {
	_, repo, cfg := newTestService(t)
	aiSvc := NewAIService(repo, cfg, nil)

	defaults, err := aiSvc.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings() default error = %v", err)
	}
	if defaults.Provider != model.AIProviderOpenAI || defaults.Model == "" || defaults.SystemPrompt == "" {
		t.Fatalf("GetSettings() defaults = %+v, want populated defaults", defaults)
	}
	if len(defaults.Models) != 1 || defaults.SceneModels.DefaultModelID == "" {
		t.Fatalf("GetSettings() defaults = %+v, want default model pool and scene bindings", defaults)
	}
	if len(defaults.RolePrompts) != 0 {
		t.Fatalf("GetSettings() defaults role_prompts = %+v, want empty list", defaults.RolePrompts)
	}

	updated, err := aiSvc.UpdateSettings(model.AISettings{
		Provider:        model.AIProviderAnthropic,
		APIKey:          "test-key",
		BaseURL:         "https://api.anthropic.com",
		Model:           "claude-test",
		Temperature:     0.1,
		MaxOutputTokens: 900,
		MaxFigures:      4,
		QAPrompt:        "custom qa",
		FigurePrompt:    "custom figure",
		TagPrompt:       "custom tag",
		GroupPrompt:     "custom group",
		TranslatePrompt: "custom translate",
		Translation: model.AITranslationConfig{
			PrimaryLanguage: "中文",
			TargetLanguage:  "英文",
		},
		SystemPrompt: "custom system",
	})
	if err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}
	if updated.Provider != model.AIProviderAnthropic || updated.MaxFigures != 4 || updated.OpenAILegacyMode {
		t.Fatalf("UpdateSettings() = %+v, want anthropic settings persisted", updated)
	}
	if len(updated.Models) != 1 || updated.Models[0].Provider != model.AIProviderAnthropic {
		t.Fatalf("UpdateSettings() models = %+v, want migrated legacy model", updated.Models)
	}
	if updated.Models[0].MaxOutputTokens != 900 {
		t.Fatalf("UpdateSettings() model max_output_tokens = %d, want 900", updated.Models[0].MaxOutputTokens)
	}

	reloaded, err := aiSvc.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings() reload error = %v", err)
	}
	if reloaded.Provider != model.AIProviderAnthropic || reloaded.APIKey != "test-key" || reloaded.QAPrompt != "custom qa" {
		t.Fatalf("GetSettings() reload = %+v, want updated settings", reloaded)
	}
	if reloaded.SceneModels.QAModelID == "" || len(reloaded.Models) != 1 {
		t.Fatalf("GetSettings() reload scene/models = %+v %+v, want populated values", reloaded.SceneModels, reloaded.Models)
	}
	if reloaded.Models[0].MaxOutputTokens != 900 {
		t.Fatalf("GetSettings() reload model max_output_tokens = %d, want 900", reloaded.Models[0].MaxOutputTokens)
	}
	if reloaded.TranslatePrompt != "custom translate" || reloaded.Translation.PrimaryLanguage != "中文" || reloaded.Translation.TargetLanguage != "英文" {
		t.Fatalf("GetSettings() reload translate settings = %+v, want persisted translate config", reloaded)
	}
}

func TestAIRolePromptsPersistence(t *testing.T) {
	_, repo, cfg := newTestService(t)
	aiSvc := NewAIService(repo, cfg, nil)

	saved, err := aiSvc.UpdateRolePrompts([]model.AIRolePrompt{
		{
			Name:   "严格证据模式",
			Prompt: "你是一名严格审稿人，优先检查证据链和结论边界。",
		},
	})
	if err != nil {
		t.Fatalf("UpdateRolePrompts() error = %v", err)
	}
	if len(saved) != 1 || saved[0].Name != "严格证据模式" {
		t.Fatalf("UpdateRolePrompts() = %+v, want single normalized role prompt", saved)
	}

	settings, err := aiSvc.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings() error = %v", err)
	}
	if len(settings.RolePrompts) != 1 || settings.RolePrompts[0].Prompt != "你是一名严格审稿人，优先检查证据链和结论边界。" {
		t.Fatalf("GetSettings() role_prompts = %+v, want persisted role prompts", settings.RolePrompts)
	}

	reloaded, err := aiSvc.GetRolePrompts()
	if err != nil {
		t.Fatalf("GetRolePrompts() error = %v", err)
	}
	if len(reloaded) != 1 || reloaded[0].Prompt != "你是一名严格审稿人，优先检查证据链和结论边界。" {
		t.Fatalf("GetRolePrompts() = %+v, want persisted role prompt list", reloaded)
	}
}

func TestGetRolePromptsMigratesLegacyPromptPresets(t *testing.T) {
	_, repo, cfg := newTestService(t)
	aiSvc := NewAIService(repo, cfg, nil)

	if err := repo.UpsertAppSetting(aiRolePromptsKey, `[{"name":"严格证据模式","system_prompt":"优先引用原文","qa_prompt":"先回答结论","translate_prompt":"只返回译文"}]`); err != nil {
		t.Fatalf("UpsertAppSetting() error = %v", err)
	}

	rolePrompts, err := aiSvc.GetRolePrompts()
	if err != nil {
		t.Fatalf("GetRolePrompts() error = %v", err)
	}
	if len(rolePrompts) != 1 || rolePrompts[0].Name != "严格证据模式" {
		t.Fatalf("GetRolePrompts() = %+v, want migrated legacy role prompt", rolePrompts)
	}
	for _, want := range []string{"System Prompt", "优先引用原文", "通用问答 Prompt", "只返回译文"} {
		if !strings.Contains(rolePrompts[0].Prompt, want) {
			t.Fatalf("migrated role prompt missing %q\n%s", want, rolePrompts[0].Prompt)
		}
	}
}

func TestSplitAISettingsUpdatesPreserveOtherSections(t *testing.T) {
	_, repo, cfg := newTestService(t)
	aiSvc := NewAIService(repo, cfg, nil)

	if _, err := aiSvc.UpdateSettings(model.AISettings{
		Models: []model.AIModelConfig{
			{ID: "qa", Name: "QA", Provider: model.AIProviderOpenAI, APIKey: "key-1", BaseURL: "https://api.openai.com", Model: "gpt-4.1-mini", MaxOutputTokens: 1200},
		},
		SceneModels: model.AISceneModelSelection{
			DefaultModelID: "qa",
			QAModelID:      "qa",
		},
		Temperature:     0.2,
		MaxFigures:      2,
		SystemPrompt:    "base system",
		QAPrompt:        "base qa",
		FigurePrompt:    "base figure",
		TagPrompt:       "base tag",
		GroupPrompt:     "base group",
		TranslatePrompt: "base translate",
		Translation: model.AITranslationConfig{
			PrimaryLanguage: "中文",
			TargetLanguage:  "英文",
		},
		RolePrompts: []model.AIRolePrompt{
			{Name: "导师", Prompt: "你是一名导师。"},
		},
	}); err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}

	if _, err := aiSvc.UpdateModelSettings(model.AIModelSettingsUpdate{
		Models: []model.AIModelConfig{
			{ID: "qa", Name: "QA Fast", Provider: model.AIProviderAnthropic, APIKey: "key-2", BaseURL: "https://api.anthropic.com", Model: "claude-test", MaxOutputTokens: 900},
		},
		SceneModels: model.AISceneModelSelection{
			DefaultModelID: "qa",
			QAModelID:      "qa",
		},
		Temperature: 0.3,
		MaxFigures:  4,
		Translation: model.AITranslationConfig{
			PrimaryLanguage: "中文",
			TargetLanguage:  "日文",
		},
	}); err != nil {
		t.Fatalf("UpdateModelSettings() error = %v", err)
	}

	afterModelUpdate, err := aiSvc.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings() after model update error = %v", err)
	}
	if afterModelUpdate.SystemPrompt != "base system" || afterModelUpdate.QAPrompt != "base qa" {
		t.Fatalf("model update should preserve prompt settings, got %+v", afterModelUpdate)
	}
	if len(afterModelUpdate.RolePrompts) != 1 || afterModelUpdate.RolePrompts[0].Name != "导师" {
		t.Fatalf("model update should preserve role prompts, got %+v", afterModelUpdate.RolePrompts)
	}

	if _, err := aiSvc.UpdatePromptSettings(model.AIPromptSettingsUpdate{
		SystemPrompt:    "updated system",
		QAPrompt:        "updated qa",
		FigurePrompt:    "updated figure",
		TagPrompt:       "updated tag",
		GroupPrompt:     "updated group",
		TranslatePrompt: "updated translate",
	}); err != nil {
		t.Fatalf("UpdatePromptSettings() error = %v", err)
	}

	afterPromptUpdate, err := aiSvc.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings() after prompt update error = %v", err)
	}
	if afterPromptUpdate.Models[0].Provider != model.AIProviderAnthropic || afterPromptUpdate.Translation.TargetLanguage != "日文" {
		t.Fatalf("prompt update should preserve model settings, got %+v", afterPromptUpdate)
	}
	if len(afterPromptUpdate.RolePrompts) != 1 || afterPromptUpdate.RolePrompts[0].Prompt != "你是一名导师。" {
		t.Fatalf("prompt update should preserve role prompts, got %+v", afterPromptUpdate.RolePrompts)
	}
}

func TestResolveModelForActionUsesSceneSpecificModel(t *testing.T) {
	settings := model.DefaultAISettings()
	settings.Models = []model.AIModelConfig{
		{ID: "default", Name: "Default", Provider: model.AIProviderOpenAI, APIKey: "key-1", BaseURL: "https://api.openai.com", Model: "gpt-4.1-mini"},
		{ID: "figure", Name: "Figure", Provider: model.AIProviderAnthropic, APIKey: "key-2", BaseURL: "https://api.anthropic.com", Model: "claude-test"},
		{ID: "translate", Name: "Translate", Provider: model.AIProviderGemini, APIKey: "key-3", BaseURL: "https://generativelanguage.googleapis.com", Model: "gemini-test"},
	}
	settings.SceneModels = model.AISceneModelSelection{
		DefaultModelID:   "default",
		QAModelID:        "default",
		FigureModelID:    "figure",
		TagModelID:       "default",
		GroupModelID:     "default",
		TranslateModelID: "translate",
	}

	resolved, err := resolveModelForAction(settings, model.AIActionFigureInterpretation)
	if err != nil {
		t.Fatalf("resolveModelForAction() error = %v", err)
	}
	if resolved.ID != "figure" || resolved.Provider != model.AIProviderAnthropic {
		t.Fatalf("resolveModelForAction() = %+v, want figure-scoped model", resolved)
	}

	translated, err := resolveModelForAction(settings, model.AIActionTranslate)
	if err != nil {
		t.Fatalf("resolveModelForAction(translate) error = %v", err)
	}
	if translated.ID != "translate" || translated.Provider != model.AIProviderGemini {
		t.Fatalf("resolveModelForAction(translate) = %+v, want translate-scoped model", translated)
	}
}

func TestResolveTranslationDirection(t *testing.T) {
	config := model.AITranslationConfig{
		PrimaryLanguage: "中文",
		TargetLanguage:  "英文",
	}

	source, target := resolveTranslationDirection(config, "这是中文句子。")
	if source != "中文" || target != "英文" {
		t.Fatalf("resolveTranslationDirection(chinese) = %q -> %q, want 中文 -> 英文", source, target)
	}

	source, target = resolveTranslationDirection(config, "This is an English sentence.")
	if source != "其他语言" || target != "中文" {
		t.Fatalf("resolveTranslationDirection(english) = %q -> %q, want 其他语言 -> 中文", source, target)
	}
}

func TestTranslateUsesSceneModelAndReturnsTranslation(t *testing.T) {
	_, repo, cfg := newTestService(t)
	aiSvc := NewAIService(repo, cfg, nil)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		bodyText := string(body)
		if !strings.Contains(bodyText, "任务类型: translate") {
			t.Fatalf("request body missing translate prompt: %s", bodyText)
		}
		if !strings.Contains(bodyText, "原文语言: 中文") {
			t.Fatalf("request body missing source language: %s", bodyText)
		}
		if !strings.Contains(bodyText, "目标语言: 英文") {
			t.Fatalf("request body missing target language: %s", bodyText)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output_text":"Translated output"}`))
	}))
	defer server.Close()

	if _, err := aiSvc.UpdateSettings(model.AISettings{
		Models: []model.AIModelConfig{
			{
				ID:              "default",
				Name:            "Default",
				Provider:        model.AIProviderOpenAI,
				APIKey:          "default-key",
				BaseURL:         "https://api.openai.com",
				Model:           "gpt-default",
				MaxOutputTokens: 1200,
			},
			{
				ID:              "translate",
				Name:            "Translate",
				Provider:        model.AIProviderOpenAI,
				APIKey:          "translate-key",
				BaseURL:         server.URL,
				Model:           "gpt-translate",
				MaxOutputTokens: 800,
			},
		},
		SceneModels: model.AISceneModelSelection{
			DefaultModelID:   "default",
			TranslateModelID: "translate",
		},
		SystemPrompt:    "system",
		TranslatePrompt: "translate prompt",
		Translation: model.AITranslationConfig{
			PrimaryLanguage: "中文",
			TargetLanguage:  "英文",
		},
	}); err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}

	result, err := aiSvc.Translate(context.Background(), model.AITranslateRequest{
		Text: "这是一个测试。",
	})
	if err != nil {
		t.Fatalf("Translate() error = %v", err)
	}
	if !result.Success || result.Model != "gpt-translate" || result.SourceLanguage != "中文" || result.TargetLanguage != "英文" {
		t.Fatalf("Translate() = %+v, want translate-scoped result metadata", result)
	}
	if result.Translation != "Translated output" {
		t.Fatalf("Translate() translation = %q, want translated text", result.Translation)
	}
}

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

func TestExportReadMarkdownBundlesAssetsAndRewritesFigureRefs(t *testing.T) {
	_, repo, cfg := newTestService(t)
	aiSvc := NewAIService(repo, cfg, nil)

	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Export Markdown Study",
		OriginalFilename: "export-markdown.pdf",
		StoredPDFName:    "export-markdown.pdf",
		FileSize:         512,
		ContentType:      "application/pdf",
		PDFText:          "Full text for export markdown.",
		ExtractionStatus: "completed",
		Figures: []repository.FigureUpsertInput{
			{Filename: "figure_one.png", ContentType: "image/png", PageNumber: 3, FigureIndex: 1, Caption: "Figure one"},
			{Filename: "figure_two.png", ContentType: "image/png", PageNumber: 5, FigureIndex: 2, Caption: "Figure two"},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	figureOneBytes := writeFigureFixture(t, filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename), 280, 180)
	figureTwoBytes := writeFigureFixture(t, filepath.Join(cfg.FiguresDir(), paper.Figures[1].Filename), 300, 200)

	filename, archive, err := aiSvc.ExportReadMarkdown(context.Background(), model.AIReadExportRequest{
		PaperID:   paper.ID,
		TurnIndex: 2,
		Answer: strings.Join([]string{
			"这是第一张图：",
			"",
			fmt.Sprintf("![第 3 页图 1](figure://%d)", paper.Figures[0].ID),
			"",
			"第二次再引用第一张图：",
			fmt.Sprintf("![第 3 页图 1](figure://%d)", paper.Figures[0].ID),
			"",
			fmt.Sprintf("![第 5 页图 2](figure://%d)", paper.Figures[1].ID),
		}, "\n"),
	})
	if err != nil {
		t.Fatalf("ExportReadMarkdown() error = %v", err)
	}

	if filename != fmt.Sprintf("paper_%d_ai_reader_turn_02.zip", paper.ID) {
		t.Fatalf("ExportReadMarkdown() filename = %q, want turn-specific zip name", filename)
	}

	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatalf("zip.NewReader() error = %v", err)
	}

	entries := map[string][]byte{}
	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("zip entry %s open error = %v", file.Name, err)
		}
		content, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatalf("zip entry %s read error = %v", file.Name, err)
		}
		entries[file.Name] = content
	}

	answer, ok := entries["answer.md"]
	if !ok {
		t.Fatalf("zip entries missing answer.md: %#v", entries)
	}

	firstAsset := fmt.Sprintf("assets/figure-p3-n1-%d.png", paper.Figures[0].ID)
	secondAsset := fmt.Sprintf("assets/figure-p5-n2-%d.png", paper.Figures[1].ID)

	answerText := string(answer)
	if strings.Contains(answerText, "figure://") {
		t.Fatalf("answer.md = %q, want rewritten asset references", answerText)
	}
	for _, want := range []string{firstAsset, secondAsset} {
		if !strings.Contains(answerText, want) {
			t.Fatalf("answer.md missing %q\n%s", want, answerText)
		}
	}

	if got := entries[firstAsset]; !bytes.Equal(got, figureOneBytes) {
		t.Fatalf("first asset bytes mismatch: got=%d want=%d", len(got), len(figureOneBytes))
	}
	if got := entries[secondAsset]; !bytes.Equal(got, figureTwoBytes) {
		t.Fatalf("second asset bytes mismatch: got=%d want=%d", len(got), len(figureTwoBytes))
	}
	if len(entries) != 3 {
		t.Fatalf("zip entry count = %d, want 3 (answer + 2 assets)", len(entries))
	}
}

func TestExportReadMarkdownRejectsUnknownFigureReference(t *testing.T) {
	_, repo, cfg := newTestService(t)
	aiSvc := NewAIService(repo, cfg, nil)
	paper := createTestPaper(t, repo)

	_, _, err := aiSvc.ExportReadMarkdown(context.Background(), model.AIReadExportRequest{
		PaperID: paper.ID,
		Answer:  "![不存在的图](figure://999999)",
	})
	if err == nil {
		t.Fatal("ExportReadMarkdown() error = nil, want invalid figure reference error")
	}
	if got := apperr.CodeOf(err); got != apperr.CodeInvalidArgument {
		t.Fatalf("ExportReadMarkdown() code = %q, want %q", got, apperr.CodeInvalidArgument)
	}
}

func TestExportReadMarkdownConversationScopeUsesConversationFilenames(t *testing.T) {
	_, repo, cfg := newTestService(t)
	aiSvc := NewAIService(repo, cfg, nil)

	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Conversation Export Study",
		OriginalFilename: "conversation-export.pdf",
		StoredPDFName:    "conversation-export.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		PDFText:          "Full text for conversation export.",
		ExtractionStatus: "completed",
		Figures: []repository.FigureUpsertInput{
			{Filename: "conversation_figure.png", ContentType: "image/png", PageNumber: 4, FigureIndex: 1, Caption: "Conversation figure"},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	assetBytes := writeFigureFixture(t, filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename), 240, 160)

	filename, archive, err := aiSvc.ExportReadMarkdown(context.Background(), model.AIReadExportRequest{
		PaperID: paper.ID,
		Scope:   "conversation",
		Content: strings.Join([]string{
			"# 第 1 轮",
			"",
			"## 用户提问",
			"请结合图说明结论。",
			"",
			"## AI 回答",
			fmt.Sprintf("见第 4 页图 1：![第 4 页图 1](figure://%d)", paper.Figures[0].ID),
		}, "\n"),
	})
	if err != nil {
		t.Fatalf("ExportReadMarkdown(conversation) error = %v", err)
	}

	if filename != fmt.Sprintf("paper_%d_ai_reader_conversation.zip", paper.ID) {
		t.Fatalf("ExportReadMarkdown(conversation) filename = %q, want conversation zip", filename)
	}

	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatalf("zip.NewReader() error = %v", err)
	}

	entries := map[string][]byte{}
	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("zip entry %s open error = %v", file.Name, err)
		}
		content, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatalf("zip entry %s read error = %v", file.Name, err)
		}
		entries[file.Name] = content
	}

	conversation, ok := entries["conversation.md"]
	if !ok {
		t.Fatalf("zip entries missing conversation.md: %#v", entries)
	}

	assetPath := fmt.Sprintf("assets/figure-p4-n1-%d.png", paper.Figures[0].ID)
	if !strings.Contains(string(conversation), assetPath) {
		t.Fatalf("conversation.md missing %q\n%s", assetPath, string(conversation))
	}
	if got := entries[assetPath]; !bytes.Equal(got, assetBytes) {
		t.Fatalf("conversation asset bytes mismatch: got=%d want=%d", len(got), len(assetBytes))
	}
}

func TestBuildAIPromptsIncludePaperContext(t *testing.T) {
	settings := model.DefaultAISettings()
	paper := &model.Paper{
		ID:               7,
		Title:            "Atlas Study",
		OriginalFilename: "atlas-study.pdf",
		PDFText:          "Full paper text for AI reading.",
		AbstractText:     "Atlas abstract",
		NotesText:        "Atlas notes",
		GroupName:        "Atlas Group",
		Tags: []model.Tag{
			{Name: "Microscopy"},
		},
	}

	systemPrompt, userPrompt := buildAIPrompts(
		settings,
		paper,
		[]model.Group{{Name: "Atlas Group", Description: "single-cell atlas"}},
		[]model.Tag{{Name: "Microscopy"}},
		model.AIActionFigureInterpretation,
		"请解释关键图片。",
		"请解释关键图片。",
		nil,
		[]string{"- 第 1 页图 1：caption=Overview"},
		1,
		nil,
		true,
	)

	if !strings.Contains(systemPrompt, "科研论文") {
		t.Fatalf("systemPrompt = %q, want default system instructions", systemPrompt)
	}
	for _, want := range []string{
		"Atlas Study",
		"Atlas abstract",
		"Atlas notes",
		"Atlas Group",
		"Microscopy",
		"请解释关键图片。",
		"Full paper text for AI reading.",
		"第 1 页图 1",
	} {
		if !strings.Contains(userPrompt, want) {
			t.Fatalf("userPrompt missing %q\n%s", want, userPrompt)
		}
	}
}

func TestBuildAIPromptsIncludeConversationHistoryForPaperQA(t *testing.T) {
	settings := model.DefaultAISettings()
	paper := &model.Paper{
		ID:               9,
		Title:            "Conversation Study",
		OriginalFilename: "conversation-study.pdf",
		PDFText:          "Conversation full text.",
		AbstractText:     "Conversation abstract",
	}

	_, userPrompt := buildAIPrompts(
		settings,
		paper,
		nil,
		nil,
		model.AIActionPaperQA,
		"这篇文章最关键的证据是什么？",
		"这篇文章最关键的证据是什么？",
		[]model.AIConversationTurn{
			{Question: "先概括一下这篇文章。", Answer: "它主要研究细胞图谱。"},
		},
		nil,
		0,
		nil,
		true,
	)

	for _, want := range []string{
		"历史对话:",
		"第 1 轮用户: 先概括一下这篇文章。",
		"第 1 轮助手: 它主要研究细胞图谱。",
		"这篇文章最关键的证据是什么？",
		"answer 支持使用 Markdown",
		"figure://<figure_id>",
	} {
		if !strings.Contains(userPrompt, want) {
			t.Fatalf("userPrompt missing %q\n%s", want, userPrompt)
		}
	}
}

func TestBuildAIPromptsIncludeActiveRolePromptsForPaperQA(t *testing.T) {
	settings := model.DefaultAISettings()
	paper := &model.Paper{
		ID:               10,
		Title:            "Role Study",
		OriginalFilename: "role-study.pdf",
		PDFText:          "Role full text.",
	}

	systemPrompt, userPrompt := buildAIPrompts(
		settings,
		paper,
		nil,
		nil,
		model.AIActionPaperQA,
		"@严格证据模式 请总结结论。",
		"请总结结论。",
		nil,
		nil,
		0,
		[]model.AIRolePrompt{
			{Name: "严格证据模式", Prompt: "优先引用原文证据，并明确不确定性。"},
		},
		true,
	)

	for _, want := range []string{"@严格证据模式", "角色调用:", "请总结结论。"} {
		if !strings.Contains(userPrompt, want) {
			t.Fatalf("userPrompt missing %q\n%s", want, userPrompt)
		}
	}
	for _, want := range []string{"当前用户通过 @ 调用的角色 Prompt", "严格证据模式", "优先引用原文证据"} {
		if !strings.Contains(systemPrompt, want) {
			t.Fatalf("systemPrompt missing %q\n%s", want, systemPrompt)
		}
	}
}

func TestBuildAIPromptsIncludeFigureReferenceFormatForPaperQA(t *testing.T) {
	settings := model.DefaultAISettings()
	paper := &model.Paper{
		ID:               13,
		Title:            "Figure Ref Study",
		OriginalFilename: "figure-ref-study.pdf",
		PDFText:          "Full text",
	}

	_, userPrompt := buildAIPrompts(
		settings,
		paper,
		nil,
		nil,
		model.AIActionPaperQA,
		"请结合图片说明主要发现。",
		"请结合图片说明主要发现。",
		nil,
		[]string{"- figure_id=182；标签=第 3 页图 1；caption=Signal map；如需插图请使用 ![第 3 页图 1](figure://182)"},
		1,
		nil,
		true,
	)

	for _, want := range []string{
		"figure_id=182",
		"![第 3 页图 1](figure://182)",
		"不要伪造本地文件路径",
	} {
		if !strings.Contains(userPrompt, want) {
			t.Fatalf("userPrompt missing %q\n%s", want, userPrompt)
		}
	}
}

func TestNormalizeConversationHistoryRejectsMoreThanFourTurns(t *testing.T) {
	_, err := normalizeConversationHistory(model.AIActionPaperQA, []model.AIConversationTurn{
		{Question: "q1", Answer: "a1"},
		{Question: "q2", Answer: "a2"},
		{Question: "q3", Answer: "a3"},
		{Question: "q4", Answer: "a4"},
		{Question: "q5", Answer: "a5"},
	})
	if err == nil {
		t.Fatal("normalizeConversationHistory() error = nil, want limit error")
	}
}

func TestBuildAIPromptsUsePlainTextRequirementsForStreamingInterpretation(t *testing.T) {
	settings := model.DefaultAISettings()
	paper := &model.Paper{
		ID:               11,
		Title:            "Figure Stream Study",
		OriginalFilename: "figure-stream-study.pdf",
		PDFText:          "Full text",
	}

	_, userPrompt := buildAIPrompts(
		settings,
		paper,
		nil,
		nil,
		model.AIActionFigureInterpretation,
		"请解读这张图。",
		"请解读这张图。",
		nil,
		[]string{"- 第 2 页图 3：caption=Signal map"},
		1,
		nil,
		false,
	)

	if strings.Contains(userPrompt, "JSON 必须包含 answer") {
		t.Fatalf("userPrompt = %q, want plain text output requirements", userPrompt)
	}
	if !strings.Contains(userPrompt, "不要返回 JSON") {
		t.Fatalf("userPrompt = %q, want plain text stream instruction", userPrompt)
	}
}

func TestExtractStructuredAIResultParsesCodeFenceJSON(t *testing.T) {
	result := extractStructuredAIResult("```json\n{\"answer\":\"ok\",\"suggested_tags\":[\"TagA\",\"TagB\"],\"suggested_group\":\"GroupA\"}\n```")

	if result.Answer != "ok" {
		t.Fatalf("Answer = %q, want %q", result.Answer, "ok")
	}
	if len(result.SuggestedTags) != 2 || result.SuggestedTags[0] != "TagA" {
		t.Fatalf("SuggestedTags = %#v, want parsed tags", result.SuggestedTags)
	}
	if result.SuggestedGroup != "GroupA" {
		t.Fatalf("SuggestedGroup = %q, want %q", result.SuggestedGroup, "GroupA")
	}
}

func TestExtractStructuredAIResultParsesPartialJSONAnswer(t *testing.T) {
	result := extractStructuredAIResult("{\"answer\":\"FT 很重要\\n\\n### 1) 核心结论\\n可直接看原图：第 3 页图 1")

	if !strings.Contains(result.Answer, "FT 很重要") {
		t.Fatalf("Answer = %q, want salvaged answer text", result.Answer)
	}
	if !strings.Contains(result.Answer, "### 1) 核心结论") {
		t.Fatalf("Answer = %q, want decoded markdown heading", result.Answer)
	}
	if strings.Contains(result.Answer, "{\"answer\"") {
		t.Fatalf("Answer = %q, want parsed content instead of raw JSON", result.Answer)
	}
}

func TestExtractStructuredAIResultParsesPartialJSONEscapesAndTags(t *testing.T) {
	result := extractStructuredAIResult("{\"answer\":\"他说：\\\"FT 是关键\\\"。\\n第二行\",\"suggested_tags\":[\"FT\",\"FAC\"")

	if result.Answer != "他说：\"FT 是关键\"。\n第二行" {
		t.Fatalf("Answer = %q, want decoded escaped content", result.Answer)
	}
	if len(result.SuggestedTags) != 2 || result.SuggestedTags[0] != "FT" || result.SuggestedTags[1] != "FAC" {
		t.Fatalf("SuggestedTags = %#v, want salvaged partial tags", result.SuggestedTags)
	}
}

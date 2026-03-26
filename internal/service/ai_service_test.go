package service

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/model"
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

func containsKeyword(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
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
	if defaults.SceneModels.IMIntentModelID == "" {
		t.Fatalf("GetSettings() defaults scene_models = %+v, want IM intent model default", defaults.SceneModels)
	}
	if defaults.SceneModels.TTSModelID == "" || strings.TrimSpace(defaults.TTSPrompt) == "" {
		t.Fatalf("GetSettings() defaults = %+v, want TTS rewrite scene and prompt", defaults)
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
		TTSPrompt:       "base tts",
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
		TTSPrompt:       "updated tts",
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
	if afterPromptUpdate.TTSPrompt != "updated tts" {
		t.Fatalf("prompt update should persist TTS prompt, got %+v", afterPromptUpdate)
	}
}

func TestResolveModelForActionUsesSceneSpecificModel(t *testing.T) {
	settings := model.DefaultAISettings()
	settings.Models = []model.AIModelConfig{
		{ID: "default", Name: "Default", Provider: model.AIProviderOpenAI, APIKey: "key-1", BaseURL: "https://api.openai.com", Model: "gpt-4.1-mini"},
		{ID: "figure", Name: "Figure", Provider: model.AIProviderAnthropic, APIKey: "key-2", BaseURL: "https://api.anthropic.com", Model: "claude-test"},
		{ID: "translate", Name: "Translate", Provider: model.AIProviderGemini, APIKey: "key-3", BaseURL: "https://generativelanguage.googleapis.com", Model: "gemini-test"},
		{ID: "tts", Name: "TTS", Provider: model.AIProviderOpenAI, APIKey: "key-4", BaseURL: "https://api.openai.com", Model: "gpt-tts"},
	}
	settings.SceneModels = model.AISceneModelSelection{
		DefaultModelID:   "default",
		QAModelID:        "default",
		FigureModelID:    "figure",
		TagModelID:       "default",
		GroupModelID:     "default",
		TranslateModelID: "translate",
		TTSModelID:       "tts",
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

	ttsModel, err := resolveModelForAction(settings, model.AIActionTTSRewrite)
	if err != nil {
		t.Fatalf("resolveModelForAction(tts) error = %v", err)
	}
	if ttsModel.ID != "tts" || ttsModel.Provider != model.AIProviderOpenAI {
		t.Fatalf("resolveModelForAction(tts) = %+v, want tts-scoped model", ttsModel)
	}
}

func TestUpdateModelSettingsPersistsIMIntentModelSelection(t *testing.T) {
	_, repo, cfg := newTestService(t)
	aiSvc := NewAIService(repo, cfg, nil)

	if _, err := aiSvc.UpdateModelSettings(model.AIModelSettingsUpdate{
		Models: []model.AIModelConfig{
			{ID: "default", Name: "Default", Provider: model.AIProviderOpenAI, APIKey: "key-1", BaseURL: "https://api.openai.com", Model: "gpt-4.1-mini", MaxOutputTokens: 1200},
			{ID: "intent", Name: "Intent", Provider: model.AIProviderAnthropic, APIKey: "key-2", BaseURL: "https://api.anthropic.com", Model: "claude-test", MaxOutputTokens: 800},
		},
		SceneModels: model.AISceneModelSelection{
			DefaultModelID:  "default",
			QAModelID:       "default",
			IMIntentModelID: "intent",
		},
		Temperature: 0.2,
		MaxFigures:  0,
		Translation: model.AITranslationConfig{
			PrimaryLanguage: "中文",
			TargetLanguage:  "英文",
		},
	}); err != nil {
		t.Fatalf("UpdateModelSettings() error = %v", err)
	}

	settings, err := aiSvc.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings() error = %v", err)
	}
	if settings.SceneModels.IMIntentModelID != "intent" {
		t.Fatalf("GetSettings() scene_models = %+v, want im_intent_model_id persisted", settings.SceneModels)
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

func TestRewriteTextForTTSUsesSceneModelAndPrompt(t *testing.T) {
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
		if !strings.Contains(bodyText, "适合中文 TTS 直接朗读的版本") {
			t.Fatalf("request body missing TTS rewrite instruction: %s", bodyText)
		}
		if !strings.Contains(bodyText, "custom tts prompt") {
			t.Fatalf("request body missing custom TTS prompt: %s", bodyText)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output_text":"这是整理后的朗读稿。见 Figure 5。"}`))
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
				ID:              "tts",
				Name:            "TTS Rewrite",
				Provider:        model.AIProviderOpenAI,
				APIKey:          "tts-key",
				BaseURL:         server.URL,
				Model:           "gpt-tts",
				MaxOutputTokens: 900,
			},
		},
		SceneModels: model.AISceneModelSelection{
			DefaultModelID: "default",
			TTSModelID:     "tts",
		},
		SystemPrompt: "system",
		TTSPrompt:    "custom tts prompt",
	}); err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}

	rewritten, err := aiSvc.RewriteTextForTTS(context.Background(), "原始内容 **带 Markdown**")
	if err != nil {
		t.Fatalf("RewriteTextForTTS() error = %v", err)
	}
	if rewritten != "这是整理后的朗读稿。见 Figure 5。" {
		t.Fatalf("RewriteTextForTTS() = %q, want rewritten text", rewritten)
	}
}

func TestSanitizeMarkdownForTTSRemovesFigureSyntax(t *testing.T) {
	input := "见图形摘要 ![第 1 页图 1](figure://309)\n\n一句话概括：**这篇论文**构建了统一图谱。"
	got := sanitizeMarkdownForTTS(input)
	want := "见图形摘要 第 1 页图 1\n\n一句话概括：这篇论文构建了统一图谱。"
	if got != want {
		t.Fatalf("sanitizeMarkdownForTTS() = %q, want %q", got, want)
	}
}

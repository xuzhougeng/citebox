package service

import (
	"strings"
	"testing"

	"paper_image_db/internal/model"
)

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
		SystemPrompt:    "custom system",
	})
	if err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}
	if updated.Provider != model.AIProviderAnthropic || updated.MaxFigures != 4 || updated.OpenAILegacyMode {
		t.Fatalf("UpdateSettings() = %+v, want anthropic settings persisted", updated)
	}

	reloaded, err := aiSvc.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings() reload error = %v", err)
	}
	if reloaded.Provider != model.AIProviderAnthropic || reloaded.APIKey != "test-key" || reloaded.QAPrompt != "custom qa" {
		t.Fatalf("GetSettings() reload = %+v, want updated settings", reloaded)
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
		nil,
		[]string{"- 第 1 页图 1：caption=Overview"},
		1,
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
		[]model.AIConversationTurn{
			{Question: "先概括一下这篇文章。", Answer: "它主要研究细胞图谱。"},
		},
		nil,
		0,
		true,
	)

	for _, want := range []string{
		"历史对话:",
		"第 1 轮用户: 先概括一下这篇文章。",
		"第 1 轮助手: 它主要研究细胞图谱。",
		"这篇文章最关键的证据是什么？",
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
		nil,
		[]string{"- 第 2 页图 3：caption=Signal map"},
		1,
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

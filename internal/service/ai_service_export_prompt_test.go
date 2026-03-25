package service

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

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

package service

import (
	"bytes"
	"context"
	"encoding/base64"
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

func TestPlanWeixinSearchUsesIntentSceneModel(t *testing.T) {
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
		if !strings.Contains(bodyText, "微信 IM 检索请求改写成 JSON") {
			t.Fatalf("request body missing IM search planning prompt: %s", bodyText)
		}
		if !strings.Contains(bodyText, "我想找单细胞图谱综述") {
			t.Fatalf("request body missing original query: %s", bodyText)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output_text":"{\"target\":\"paper\",\"keywords_zh\":[\"单细胞图谱\",\"综述\"],\"keywords_en\":[\"single-cell atlas\",\"review\",\"atlas\"]}"}`))
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
				ID:              "intent",
				Name:            "Intent",
				Provider:        model.AIProviderOpenAI,
				APIKey:          "intent-key",
				BaseURL:         server.URL,
				Model:           "gpt-intent",
				MaxOutputTokens: 600,
			},
		},
		SceneModels: model.AISceneModelSelection{
			DefaultModelID:  "default",
			IMIntentModelID: "intent",
		},
		SystemPrompt: "system",
	}); err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}

	plan, err := aiSvc.PlanWeixinSearch(context.Background(), "我想找单细胞图谱综述", "")
	if err != nil {
		t.Fatalf("PlanWeixinSearch() error = %v", err)
	}
	if plan.Target != weixinSearchTargetPaper {
		t.Fatalf("PlanWeixinSearch() target = %q, want %q", plan.Target, weixinSearchTargetPaper)
	}
	if len(plan.KeywordsZH) == 0 || len(plan.KeywordsEN) == 0 {
		t.Fatalf("PlanWeixinSearch() zh/en keywords = %v / %v, want bilingual keyword groups", plan.KeywordsZH, plan.KeywordsEN)
	}
	if !containsKeyword(plan.KeywordsZH, "单细胞图谱") || !containsKeyword(plan.KeywordsEN, "single-cell atlas") {
		t.Fatalf("PlanWeixinSearch() zh/en keywords = %v / %v, want requested bilingual search terms", plan.KeywordsZH, plan.KeywordsEN)
	}
}

func TestPlanWeixinCommandUsesIntentSceneModel(t *testing.T) {
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
		if !strings.Contains(bodyText, "微信 IM 普通文本改写成最合适的 slash 命令 JSON") {
			t.Fatalf("request body missing IM command planning prompt: %s", bodyText)
		}
		if !strings.Contains(bodyText, "第一问") {
			t.Fatalf("request body missing original IM text: %s", bodyText)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output_text":"{\"command\":\"/ask\",\"arg\":\"第一问\"}"}`))
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
				ID:              "intent",
				Name:            "Intent",
				Provider:        model.AIProviderOpenAI,
				APIKey:          "intent-key",
				BaseURL:         server.URL,
				Model:           "gpt-intent",
				MaxOutputTokens: 400,
			},
		},
		SceneModels: model.AISceneModelSelection{
			DefaultModelID:  "default",
			IMIntentModelID: "intent",
		},
		SystemPrompt: "system",
	}); err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}

	plan, err := aiSvc.PlanWeixinCommand(context.Background(), "第一问", weixinIntentContext{
		CurrentPaperID:    7,
		CurrentPaperTitle: "Help Atlas",
	})
	if err != nil {
		t.Fatalf("PlanWeixinCommand() error = %v", err)
	}
	if plan.Command != "/ask" || plan.Arg != "第一问" {
		t.Fatalf("PlanWeixinCommand() = %+v, want /ask 第一问", plan)
	}
}

func TestPlanWeixinCommandSupportsContextualPaperSelection(t *testing.T) {
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
		if !strings.Contains(bodyText, "/paper") || !strings.Contains(bodyText, "/figure") {
			t.Fatalf("request body missing contextual selection commands: %s", bodyText)
		}
		if !strings.Contains(bodyText, "search_paper_count: 3") {
			t.Fatalf("request body missing recent search context: %s", bodyText)
		}
		if !strings.Contains(bodyText, "第三篇文献") {
			t.Fatalf("request body missing original selection text: %s", bodyText)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output_text":"{\"command\":\"/paper\",\"arg\":\"3\"}"}`))
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
				ID:              "intent",
				Name:            "Intent",
				Provider:        model.AIProviderOpenAI,
				APIKey:          "intent-key",
				BaseURL:         server.URL,
				Model:           "gpt-intent",
				MaxOutputTokens: 400,
			},
		},
		SceneModels: model.AISceneModelSelection{
			DefaultModelID:  "default",
			IMIntentModelID: "intent",
		},
		SystemPrompt: "system",
	}); err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}

	plan, err := aiSvc.PlanWeixinCommand(context.Background(), "我想看看第三篇文献", weixinIntentContext{
		SearchPaperCount: 3,
	})
	if err != nil {
		t.Fatalf("PlanWeixinCommand() error = %v", err)
	}
	if plan.Command != "/paper" || plan.Arg != "3" {
		t.Fatalf("PlanWeixinCommand() = %+v, want /paper 3", plan)
	}
}

func TestPlanWeixinCommandSupportsRandomFigureSelection(t *testing.T) {
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
		if !strings.Contains(bodyText, "/random") {
			t.Fatalf("request body missing random figure command: %s", bodyText)
		}
		if !strings.Contains(bodyText, "随机来一张图") {
			t.Fatalf("request body missing original random request: %s", bodyText)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output_text":"{\"command\":\"/random\",\"arg\":\"\"}"}`))
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
				ID:              "intent",
				Name:            "Intent",
				Provider:        model.AIProviderOpenAI,
				APIKey:          "intent-key",
				BaseURL:         server.URL,
				Model:           "gpt-intent",
				MaxOutputTokens: 400,
			},
		},
		SceneModels: model.AISceneModelSelection{
			DefaultModelID:  "default",
			IMIntentModelID: "intent",
		},
		SystemPrompt: "system",
	}); err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}

	plan, err := aiSvc.PlanWeixinCommand(context.Background(), "随机来一张图", weixinIntentContext{})
	if err != nil {
		t.Fatalf("PlanWeixinCommand() error = %v", err)
	}
	if plan.Command != "/random" || plan.Arg != "" {
		t.Fatalf("PlanWeixinCommand() = %+v, want /random with empty arg", plan)
	}
}

func TestReviewWeixinFigureSearchSendsCompressedImages(t *testing.T) {
	_, repo, cfg := newTestService(t)
	aiSvc := NewAIService(repo, cfg, nil)

	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Volcano Figure Study",
		OriginalFilename: "volcano-figure.pdf",
		StoredPDFName:    "volcano-figure.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		PDFText:          "volcano figure full text",
		ExtractionStatus: "completed",
		Figures: []repository.FigureUpsertInput{
			{Filename: "volcano_figure.png", ContentType: "image/png", PageNumber: 4, FigureIndex: 1, Caption: "Volcano plot of DEGs"},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}
	writeFigureFixture(t, filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename), 1800, 1200)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		bodyText := string(body)
		if !strings.Contains(bodyText, "\"input_image\"") {
			t.Fatalf("request body missing compressed image payload: %s", bodyText)
		}
		if !strings.Contains(bodyText, "附加缩略图顺序对应的候选 ID") {
			t.Fatalf("request body missing figure review prompt: %s", bodyText)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{"output_text":"{\"summary\":\"高匹配，图注和缩略图都像火山图\",\"selected_ids\":[%d]}"}`, paper.Figures[0].ID)))
	}))
	defer server.Close()

	if _, err := aiSvc.UpdateSettings(model.AISettings{
		Models: []model.AIModelConfig{
			{
				ID:              "intent",
				Name:            "Intent",
				Provider:        model.AIProviderOpenAI,
				APIKey:          "intent-key",
				BaseURL:         server.URL,
				Model:           "gpt-intent",
				MaxOutputTokens: 700,
			},
		},
		SceneModels: model.AISceneModelSelection{
			DefaultModelID:  "intent",
			IMIntentModelID: "intent",
		},
		SystemPrompt: "system",
	}); err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}

	figure, err := repo.GetFigure(paper.Figures[0].ID)
	if err != nil {
		t.Fatalf("GetFigure() error = %v", err)
	}
	if figure == nil {
		t.Fatal("GetFigure() returned nil figure")
	}

	review, err := aiSvc.ReviewWeixinFigureSearch(context.Background(), "我想要一张火山图", []string{"火山图", "volcano plot"}, []model.FigureListItem{*figure})
	if err != nil {
		t.Fatalf("ReviewWeixinFigureSearch() error = %v", err)
	}
	if len(review.SelectedIDs) != 1 || review.SelectedIDs[0] != paper.Figures[0].ID {
		t.Fatalf("ReviewWeixinFigureSearch() selected_ids = %v, want [%d]", review.SelectedIDs, paper.Figures[0].ID)
	}
	if review.Summary == "" {
		t.Fatal("ReviewWeixinFigureSearch() summary is empty")
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

package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/weixin"
)

type fakeWeixinAIReader struct {
	mu             sync.Mutex
	inputs         []model.AIReadRequest
	answer         string
	ttsRewrite     string
	ttsRewriteErr  error
	commandPlan    *weixinCommandPlan
	commandPlanErr error
	searchPlan     *weixinSearchPlan
	searchPlanErr  error
	searchReview   *weixinSearchReview
}

func (f *fakeWeixinAIReader) ReadPaper(_ context.Context, input model.AIReadRequest) (*model.AIReadResponse, error) {
	f.mu.Lock()
	f.inputs = append(f.inputs, input)
	answer := f.answer
	f.mu.Unlock()

	return &model.AIReadResponse{
		Success:  true,
		Action:   input.Action,
		PaperID:  input.PaperID,
		Question: input.Question,
		Answer:   answer,
	}, nil
}

func (f *fakeWeixinAIReader) PlanWeixinCommand(_ context.Context, text string, context weixinIntentContext) (*weixinCommandPlan, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.commandPlanErr != nil {
		return nil, f.commandPlanErr
	}
	if f.commandPlan != nil {
		plan := *f.commandPlan
		return &plan, nil
	}

	if context.CurrentPaperID > 0 {
		return &weixinCommandPlan{Command: "/ask", Arg: text}, nil
	}
	return &weixinCommandPlan{Command: "/search", Arg: text}, nil
}

func (f *fakeWeixinAIReader) PlanWeixinSearch(_ context.Context, query, forcedTarget string) (*weixinSearchPlan, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.searchPlanErr != nil {
		return nil, f.searchPlanErr
	}
	if f.searchPlan != nil {
		plan := *f.searchPlan
		plan.Keywords = append([]string(nil), plan.Keywords...)
		if normalized := normalizeWeixinSearchTarget(forcedTarget); normalized != "" {
			plan.Target = normalized
		}
		return &plan, nil
	}

	target := normalizeWeixinSearchTarget(forcedTarget)
	if target == "" {
		target = weixinSearchTargetPaper
	}
	plan := heuristicWeixinSearchPlan(query, target)
	plan.Target = target
	return plan, nil
}

func (f *fakeWeixinAIReader) ReviewWeixinPaperSearch(_ context.Context, _ string, _ []string, candidates []model.Paper) (*weixinSearchReview, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.searchReview != nil {
		review := *f.searchReview
		review.SelectedIDs = append([]int64(nil), review.SelectedIDs...)
		return &review, nil
	}

	ids := make([]int64, 0, minInt(len(candidates), weixinSearchResultLimit))
	for _, candidate := range candidates {
		ids = append(ids, candidate.ID)
		if len(ids) >= weixinSearchResultLimit {
			break
		}
	}
	return &weixinSearchReview{
		Summary:     "已按候选顺序保留最可能结果。",
		SelectedIDs: ids,
	}, nil
}

func (f *fakeWeixinAIReader) ReviewWeixinFigureSearch(_ context.Context, _ string, _ []string, candidates []model.FigureListItem) (*weixinSearchReview, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.searchReview != nil {
		review := *f.searchReview
		review.SelectedIDs = append([]int64(nil), review.SelectedIDs...)
		return &review, nil
	}

	ids := make([]int64, 0, minInt(len(candidates), weixinSearchResultLimit))
	for _, candidate := range candidates {
		ids = append(ids, candidate.ID)
		if len(ids) >= weixinSearchResultLimit {
			break
		}
	}
	return &weixinSearchReview{
		Summary:     "已按候选顺序保留最可能结果。",
		SelectedIDs: ids,
	}, nil
}

func (f *fakeWeixinAIReader) RewriteTextForTTS(_ context.Context, text string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.ttsRewriteErr != nil {
		return "", f.ttsRewriteErr
	}
	if strings.TrimSpace(f.ttsRewrite) != "" {
		return f.ttsRewrite, nil
	}
	return text, nil
}

func (f *fakeWeixinAIReader) lastInput() model.AIReadRequest {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.inputs) == 0 {
		return model.AIReadRequest{}
	}
	return f.inputs[len(f.inputs)-1]
}

func createBridgePaper(t *testing.T, repo *repository.LibraryRepository, title, filename string) *model.Paper {
	t.Helper()

	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            title,
		OriginalFilename: filename,
		StoredPDFName:    filename,
		FileSize:         256,
		ContentType:      "application/pdf",
		PDFText:          title + " full text",
		AbstractText:     title + " abstract",
		PaperNotesText:   title + " paper notes",
		ExtractionStatus: "completed",
		Figures: []repository.FigureUpsertInput{
			{
				Filename:     filename + ".png",
				OriginalName: filename + ".png",
				ContentType:  "image/png",
				PageNumber:   2,
				FigureIndex:  1,
				Caption:      title + " figure",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}
	return paper
}

func createBridgePaperWithFigureCaption(t *testing.T, repo *repository.LibraryRepository, title, filename, caption string) *model.Paper {
	t.Helper()

	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            title,
		OriginalFilename: filename,
		StoredPDFName:    filename,
		FileSize:         256,
		ContentType:      "application/pdf",
		PDFText:          title + " full text",
		AbstractText:     title + " abstract",
		PaperNotesText:   title + " paper notes",
		ExtractionStatus: "completed",
		Figures: []repository.FigureUpsertInput{
			{
				Filename:     filename + ".png",
				OriginalName: filename + ".png",
				ContentType:  "image/png",
				PageNumber:   2,
				FigureIndex:  1,
				Caption:      caption,
			},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}
	return paper
}

func newTestWeixinBridge(t *testing.T, svc *LibraryService, aiReader weixinAIReader, storageDir string) *WeixinIMBridge {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewWeixinIMBridge(svc, aiReader, logger, storageDir)
}

func TestWeixinIMBridgeRunReportsDisabledState(t *testing.T) {
	svc, _, cfg := newTestService(t)
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	bridge := NewWeixinIMBridge(svc, &fakeWeixinAIReader{answer: "ok"}, logger, cfg.StorageDir)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- bridge.Run(ctx)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil && err != context.Canceled {
			t.Fatalf("Run() error = %v, want nil or context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not stop after context cancellation")
	}

	if got := logs.String(); !strings.Contains(got, "is disabled; enable it in Settings") {
		t.Fatalf("Run() logs = %q, want disabled bridge hint", got)
	}
}

func TestWeixinIMBridgeRunWarnsWhenBindingMissing(t *testing.T) {
	svc, _, cfg := newTestService(t)
	if _, err := svc.UpdateWeixinBridgeSettings(model.WeixinBridgeSettings{Enabled: true}); err != nil {
		t.Fatalf("UpdateWeixinBridgeSettings() error = %v", err)
	}

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	bridge := NewWeixinIMBridge(svc, &fakeWeixinAIReader{answer: "ok"}, logger, cfg.StorageDir)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- bridge.Run(ctx)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil && err != context.Canceled {
			t.Fatalf("Run() error = %v, want nil or context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not stop after context cancellation")
	}

	if got := logs.String(); !strings.Contains(got, "no active binding found") {
		t.Fatalf("Run() logs = %q, want missing binding warning", got)
	}
}

func TestWeixinIMBridgeSearchAndSelectPaperByResultNumber(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	createBridgePaper(t, repo, "Atlas Alpha", "atlas-alpha.pdf")
	createBridgePaper(t, repo, "Atlas Beta", "atlas-beta.pdf")
	bridge := newTestWeixinBridge(t, svc, &fakeWeixinAIReader{answer: "ok"}, cfg.StorageDir)

	reply := bridge.handleIncomingText(context.Background(), "/search Atlas")
	if reply == "" {
		t.Fatal("search reply is empty")
	}

	state := bridge.getContext()
	if len(state.SearchPaperIDs) != 2 {
		t.Fatalf("search result ids = %v, want 2 ids", state.SearchPaperIDs)
	}

	reply = bridge.handleIncomingText(context.Background(), "/paper 1")
	if state.SearchPaperIDs[0] != bridge.getContext().CurrentPaperID {
		t.Fatalf("current paper = %d, want %d from first search result", bridge.getContext().CurrentPaperID, state.SearchPaperIDs[0])
	}
	if reply == "" {
		t.Fatal("select reply is empty")
	}
}

func TestWeixinIMBridgeSmartPaperSearchUsesPlannedKeywords(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createBridgePaper(t, repo, "Single-cell Atlas Review", "single-cell-atlas-review.pdf")
	aiReader := &fakeWeixinAIReader{
		answer: "ok",
		searchPlan: &weixinSearchPlan{
			Target:     weixinSearchTargetPaper,
			KeywordsZH: []string{"单细胞图谱", "综述"},
			KeywordsEN: []string{"single-cell atlas", "review"},
			Keywords:   []string{"单细胞图谱", "综述", "single-cell atlas", "review"},
		},
	}
	bridge := newTestWeixinBridge(t, svc, aiReader, cfg.StorageDir)

	reply := bridge.handleIncomingText(context.Background(), "/search 我想找单细胞图谱综述")

	if !containsAll(reply, "中文关键词", "英文关键词", "评估", "已自动选中文献") {
		t.Fatalf("smart paper search reply = %q, want planned keyword summary and auto-selected paper", reply)
	}
	if bridge.getContext().CurrentPaperID != paper.ID {
		t.Fatalf("current paper = %d, want %d", bridge.getContext().CurrentPaperID, paper.ID)
	}
}

func TestWeixinIMBridgeFigureSearchFallsBackToHeuristics(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	first := createBridgePaperWithFigureCaption(t, repo, "Differential Expression Study", "de-study.pdf", "Volcano plot of differential expression genes")
	second := createBridgePaperWithFigureCaption(t, repo, "Heatmap Study", "heatmap-study.pdf", "Expression heatmap across samples")

	bridge := newTestWeixinBridge(t, svc, &fakeWeixinAIReader{
		answer:        "ok",
		searchPlanErr: context.DeadlineExceeded,
	}, cfg.StorageDir)

	reply := bridge.handleIncomingText(context.Background(), "/search 我想要一张火山图")
	if !containsAll(reply, "中文关键词", "英文关键词", "汇总后最可能的图片", "Volcano plot") {
		t.Fatalf("figure search reply = %q, want heuristic keyword search result", reply)
	}

	state := bridge.getContext()
	if len(state.SearchFigureIDs) == 0 || state.SearchFigureIDs[0] != first.Figures[0].ID {
		t.Fatalf("search figure ids = %v, want first volcano figure id %d", state.SearchFigureIDs, first.Figures[0].ID)
	}
	if len(state.SearchFigureIDs) > 1 && state.SearchFigureIDs[1] == second.Figures[0].ID {
		t.Fatalf("search figure ids = %v, unexpected heatmap ranked as top fallback result", state.SearchFigureIDs)
	}

	selectReply := bridge.handleIncomingText(context.Background(), "/figure 1")
	if !containsAll(selectReply, "已选中图片", "所属文献") {
		t.Fatalf("select figure reply = %q, want figure selection from search result", selectReply)
	}
	if bridge.getContext().CurrentFigureID != first.Figures[0].ID {
		t.Fatalf("current figure = %d, want %d", bridge.getContext().CurrentFigureID, first.Figures[0].ID)
	}
}

func TestWeixinIMBridgeAppendsPaperNote(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createBridgePaper(t, repo, "Atlas Unique", "atlas-unique.pdf")
	bridge := newTestWeixinBridge(t, svc, &fakeWeixinAIReader{answer: "ok"}, cfg.StorageDir)

	_ = bridge.handleIncomingText(context.Background(), "/search Atlas Unique")
	reply := bridge.handleIncomingText(context.Background(), "/note 这是从微信追加的笔记")
	if reply == "" {
		t.Fatal("note reply is empty")
	}

	reloaded, err := svc.GetPaper(paper.ID)
	if err != nil {
		t.Fatalf("GetPaper() error = %v", err)
	}
	if got := reloaded.PaperNotesText; got == "" || !containsAll(got, "[微信", "这是从微信追加的笔记") {
		t.Fatalf("paper notes = %q, want appended weixin note", got)
	}
}

func TestWeixinIMBridgeInterpretCurrentFigureUsesFigureAction(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	createBridgePaper(t, repo, "Figure Atlas", "figure-atlas.pdf")
	aiReader := &fakeWeixinAIReader{answer: "这是图片解读结果"}
	bridge := newTestWeixinBridge(t, svc, aiReader, cfg.StorageDir)

	_ = bridge.handleIncomingText(context.Background(), "/search Figure Atlas")
	_ = bridge.handleIncomingText(context.Background(), "/figure 1")
	reply := bridge.handleIncomingText(context.Background(), "/interpret 解释这张图")

	if !containsAll(reply, "图片解读", "这是图片解读结果") {
		t.Fatalf("interpret reply = %q, want figure interpretation answer", reply)
	}

	last := aiReader.lastInput()
	if last.Action != model.AIActionFigureInterpretation {
		t.Fatalf("AI action = %q, want %q", last.Action, model.AIActionFigureInterpretation)
	}
	if last.FigureID == 0 || last.PaperID == 0 {
		t.Fatalf("AI request = %+v, want paper and figure ids", last)
	}
}

func TestWeixinIMBridgeSelectFigureResolvesPreviewPath(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createBridgePaper(t, repo, "Preview Atlas", "preview-atlas.pdf")
	bridge := newTestWeixinBridge(t, svc, &fakeWeixinAIReader{answer: "ok"}, cfg.StorageDir)

	figurePath := filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename)
	if err := os.WriteFile(figurePath, []byte("preview-bytes"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_ = bridge.handleIncomingText(context.Background(), "/search Preview Atlas")
	reply := bridge.handleIncomingTextReply(context.Background(), "/figure 1")
	previewPath, err := bridge.selectedFigurePreviewPath(weixin.Message{
		ItemList: []weixin.MessageItem{
			{
				Type:     weixin.ItemTypeText,
				TextItem: &weixin.TextItem{Text: "/figure 1"},
			},
		},
	}, reply)
	if err != nil {
		t.Fatalf("selectedFigurePreviewPath() error = %v", err)
	}
	if previewPath != figurePath {
		t.Fatalf("selectedFigurePreviewPath() path = %q, want %q", previewPath, figurePath)
	}
}

func TestWeixinIMBridgeRandomSelectsFigureAndResolvesPreviewPath(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	firstPaper := createBridgePaper(t, repo, "Random Atlas A", "random-atlas-a.pdf")
	secondPaper := createBridgePaper(t, repo, "Random Atlas B", "random-atlas-b.pdf")
	bridge := newTestWeixinBridge(t, svc, &fakeWeixinAIReader{answer: "ok"}, cfg.StorageDir)

	firstFigurePath := filepath.Join(cfg.FiguresDir(), firstPaper.Figures[0].Filename)
	secondFigurePath := filepath.Join(cfg.FiguresDir(), secondPaper.Figures[0].Filename)
	writeTestPNGFile(t, firstFigurePath)
	writeTestPNGFile(t, secondFigurePath)

	reply := bridge.handleIncomingTextReply(context.Background(), "/random")
	if !containsAll(reply.Text, "已随机选中图片", "所属文献") {
		t.Fatalf("/random reply = %q, want random figure selection summary", reply.Text)
	}

	state := bridge.getContext()
	if state.CurrentPaperID == 0 || state.CurrentFigureID == 0 {
		t.Fatalf("context after /random = %+v, want selected paper and figure", state)
	}
	if state.CurrentFigureID != firstPaper.Figures[0].ID && state.CurrentFigureID != secondPaper.Figures[0].ID {
		t.Fatalf("current figure id = %d, want one of seeded figures", state.CurrentFigureID)
	}

	previewPath, err := bridge.selectedFigurePreviewPath(weixin.Message{
		ItemList: []weixin.MessageItem{
			{
				Type:     weixin.ItemTypeText,
				TextItem: &weixin.TextItem{Text: "/random"},
			},
		},
	}, reply)
	if err != nil {
		t.Fatalf("selectedFigurePreviewPath() error = %v", err)
	}
	if previewPath != firstFigurePath && previewPath != secondFigurePath {
		t.Fatalf("selectedFigurePreviewPath() path = %q, want one of seeded figure paths", previewPath)
	}
}

func TestWeixinIMBridgePlannedRandomResolvesPreviewPath(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	firstPaper := createBridgePaper(t, repo, "Planned Random A", "planned-random-a.pdf")
	secondPaper := createBridgePaper(t, repo, "Planned Random B", "planned-random-b.pdf")
	bridge := newTestWeixinBridge(t, svc, &fakeWeixinAIReader{
		answer:      "ok",
		commandPlan: &weixinCommandPlan{Command: "/random"},
	}, cfg.StorageDir)

	firstFigurePath := filepath.Join(cfg.FiguresDir(), firstPaper.Figures[0].Filename)
	secondFigurePath := filepath.Join(cfg.FiguresDir(), secondPaper.Figures[0].Filename)
	writeTestPNGFile(t, firstFigurePath)
	writeTestPNGFile(t, secondFigurePath)

	reply := bridge.handleIncomingTextReply(context.Background(), "随机来一张图")
	if !reply.PreviewCurrentFigure {
		t.Fatalf("reply preview flag = false, want true for planned /random")
	}
	if !containsAll(reply.Text, "已随机选中图片", "所属文献") {
		t.Fatalf("planned random reply = %q, want random figure selection summary", reply.Text)
	}

	previewPath, err := bridge.selectedFigurePreviewPath(weixin.Message{
		ItemList: []weixin.MessageItem{
			{
				Type:     weixin.ItemTypeText,
				TextItem: &weixin.TextItem{Text: "随机来一张图"},
			},
		},
	}, reply)
	if err != nil {
		t.Fatalf("selectedFigurePreviewPath() error = %v", err)
	}
	if previewPath != firstFigurePath && previewPath != secondFigurePath {
		t.Fatalf("selectedFigurePreviewPath() path = %q, want one of seeded figure paths", previewPath)
	}
}

func TestWeixinIMBridgeQuestionCarriesHistory(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	createBridgePaper(t, repo, "History Atlas", "history-atlas.pdf")
	aiReader := &fakeWeixinAIReader{answer: "ok"}
	bridge := newTestWeixinBridge(t, svc, aiReader, cfg.StorageDir)

	_ = bridge.handleIncomingText(context.Background(), "/search History Atlas")
	_ = bridge.handleIncomingText(context.Background(), "/ask 第一问")
	_ = bridge.handleIncomingText(context.Background(), "/ask 第二问")

	last := aiReader.lastInput()
	if last.Action != model.AIActionPaperQA {
		t.Fatalf("AI action = %q, want %q", last.Action, model.AIActionPaperQA)
	}
	if len(last.History) != 1 || last.History[0].Question != "第一问" {
		t.Fatalf("AI history = %+v, want previous QA turn", last.History)
	}
}

func TestWeixinIMBridgePlainTextSearchRoutesToBestCommand(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	createBridgePaperWithFigureCaption(t, repo, "Volcano Atlas", "volcano-atlas.pdf", "Volcano plot for DE genes")
	aiReader := &fakeWeixinAIReader{
		answer:      "ok",
		commandPlan: &weixinCommandPlan{Command: "/search-figures", Arg: "我想要一张火山图"},
		searchPlan: &weixinSearchPlan{
			Target:     weixinSearchTargetFigure,
			KeywordsZH: []string{"火山图", "差异表达"},
			KeywordsEN: []string{"volcano plot", "differential expression"},
			Keywords:   []string{"火山图", "差异表达", "volcano plot", "differential expression"},
		},
	}
	bridge := newTestWeixinBridge(t, svc, aiReader, cfg.StorageDir)

	reply := bridge.handleIncomingText(context.Background(), "我想要一张火山图")

	if !containsAll(reply, "中文关键词", "英文关键词", "汇总后最可能的图片") {
		t.Fatalf("plain text reply = %q, want auto-routed search result", reply)
	}
	if len(bridge.getContext().SearchFigureIDs) == 0 {
		t.Fatalf("search figure ids = %v, want auto-routed figure search results", bridge.getContext().SearchFigureIDs)
	}
}

func TestWeixinIMBridgePlainTextSelectionRoutesToPaper(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	createBridgePaper(t, repo, "Atlas Alpha", "atlas-alpha.pdf")
	createBridgePaper(t, repo, "Atlas Beta", "atlas-beta.pdf")
	createBridgePaper(t, repo, "Atlas Gamma", "atlas-gamma.pdf")
	aiReader := &fakeWeixinAIReader{answer: "ok"}
	bridge := newTestWeixinBridge(t, svc, aiReader, cfg.StorageDir)

	_ = bridge.handleIncomingText(context.Background(), "/search Atlas")
	state := bridge.getContext()
	if len(state.SearchPaperIDs) < 3 {
		t.Fatalf("search result ids = %v, want at least 3 results", state.SearchPaperIDs)
	}

	aiReader.commandPlan = &weixinCommandPlan{Command: "/paper", Arg: "3"}
	reply := bridge.handleIncomingText(context.Background(), "我想看看第三篇文献")

	if bridge.getContext().CurrentPaperID != state.SearchPaperIDs[2] {
		t.Fatalf("current paper = %d, want third search result %d", bridge.getContext().CurrentPaperID, state.SearchPaperIDs[2])
	}
	selectedPaper, err := svc.GetPaper(state.SearchPaperIDs[2])
	if err != nil {
		t.Fatalf("GetPaper() error = %v", err)
	}
	if selectedPaper == nil {
		t.Fatal("GetPaper() returned nil selected paper")
	}
	if !containsAll(reply, "已选中文献", selectedPaper.Title) {
		t.Fatalf("plain text selection reply = %q, want selected third paper title %q", reply, selectedPaper.Title)
	}
}

func TestWeixinIMBridgePlainTextQuestionRoutesToAsk(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	createBridgePaper(t, repo, "Help Atlas", "help-atlas.pdf")
	aiReader := &fakeWeixinAIReader{answer: "这是问答结果"}
	bridge := newTestWeixinBridge(t, svc, aiReader, cfg.StorageDir)

	_ = bridge.handleIncomingText(context.Background(), "/search Help Atlas")
	reply := bridge.handleIncomingText(context.Background(), "第一问")

	if !containsAll(reply, "文献问答", "这是问答结果") {
		t.Fatalf("plain text reply = %q, want auto-routed ask result", reply)
	}
	last := aiReader.lastInput()
	if last.Action != model.AIActionPaperQA {
		t.Fatalf("AI action = %q, want auto-routed paper QA", last.Action)
	}
}

func TestWeixinIMBridgeUnknownSlashCommandReturnsHelpWithoutIntentRouting(t *testing.T) {
	svc, _, cfg := newTestService(t)
	aiReader := &fakeWeixinAIReader{
		answer:      "ok",
		commandPlan: &weixinCommandPlan{Command: "/ask", Arg: "不应该触发"},
	}
	bridge := newTestWeixinBridge(t, svc, aiReader, cfg.StorageDir)

	reply := bridge.handleIncomingText(context.Background(), "/unknown something")
	if !containsAll(reply, "微信 IM 优先响应 slash 命令", "`/help`") {
		t.Fatalf("unknown slash reply = %q, want help text", reply)
	}

	last := aiReader.lastInput()
	if last.Action != "" {
		t.Fatalf("AI action = %q, want no AI read call for unknown slash command", last.Action)
	}
}

func TestWeixinIMBridgeTestVoiceReturnsVoiceAttachment(t *testing.T) {
	svc, _, cfg := newTestService(t)
	if _, err := svc.UpdateWeixinBridgeSettings(model.WeixinBridgeSettings{Enabled: true}); err != nil {
		t.Fatalf("UpdateWeixinBridgeSettings() error = %v", err)
	}
	if _, err := svc.UpdateTTSSettings(model.TTSSettings{
		AppID:     "app-id",
		AccessKey: "access-key",
		Speaker:   "speaker-id",
	}); err != nil {
		t.Fatalf("UpdateTTSSettings() error = %v", err)
	}
	bridge := newTestWeixinBridge(t, svc, &fakeWeixinAIReader{answer: "ok"}, cfg.StorageDir)

	voiceDir := t.TempDir()
	voicePath := filepath.Join(voiceDir, "testvoice.mp3")
	if err := os.WriteFile(voicePath, []byte("voice-bytes"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	synthCalled := false
	bridge.synthesizeTTS = func(_ context.Context, text, uid string, settings model.TTSSettings) (string, func(), error) {
		synthCalled = true
		if text != ttsTestDemoText {
			t.Fatalf("synthesizeTTS() text = %q, want %q", text, ttsTestDemoText)
		}
		if uid != "user@im.wechat" {
			t.Fatalf("synthesizeTTS() uid = %q, want %q", uid, "user@im.wechat")
		}
		if settings.AppID != "app-id" || settings.Speaker != "speaker-id" {
			t.Fatalf("synthesizeTTS() settings = %+v, want persisted TTS config", settings)
		}
		return voicePath, func() {}, nil
	}

	reply := bridge.handleIncomingText(context.Background(), "/testvoice")
	if !containsAll(reply, "测试语音", "Hello World") {
		t.Fatalf("testvoice reply = %q, want test voice caption", reply)
	}

	replyEnvelope := bridge.handleIncomingTextReply(context.Background(), "/testvoice")
	selectedPath, cleanup, err := bridge.resolveVoiceReply(context.Background(), weixin.Message{
		FromUserID: "user@im.wechat",
		ItemList: []weixin.MessageItem{
			{
				Type:     weixin.ItemTypeText,
				TextItem: &weixin.TextItem{Text: "/testvoice"},
			},
		},
	}, replyEnvelope)
	if err != nil {
		t.Fatalf("resolveVoiceReply() error = %v", err)
	}
	cleanup()
	if !synthCalled {
		t.Fatal("resolveVoiceReply() did not trigger synthesizeTTS for /testvoice")
	}
	if selectedPath == "" {
		t.Fatal("resolveVoiceReply() path is empty")
	}
	if selectedPath != voicePath {
		t.Fatalf("resolveVoiceReply() path = %q, want %q", selectedPath, voicePath)
	}
	if _, statErr := os.Stat(selectedPath); statErr != nil {
		t.Fatalf("Stat(%q) error = %v", selectedPath, statErr)
	}
}

func TestWeixinIMBridgeVoiceToggleCommandsPersistSetting(t *testing.T) {
	svc, _, cfg := newTestService(t)
	bridge := newTestWeixinBridge(t, svc, &fakeWeixinAIReader{answer: "ok"}, cfg.StorageDir)

	reply := bridge.handleIncomingText(context.Background(), "/voiceoff")
	if !containsAll(reply, "已关闭微信 TTS 语音输出", "/ask", "/qa", "/testvoice") {
		t.Fatalf("/voiceoff reply = %q, want disable confirmation", reply)
	}

	settings, err := svc.GetTTSSettings()
	if err != nil {
		t.Fatalf("GetTTSSettings() after /voiceoff error = %v", err)
	}
	if settings.WeixinVoiceOutputEnabled {
		t.Fatalf("GetTTSSettings() after /voiceoff = %+v, want disabled voice output", settings)
	}

	reply = bridge.handleIncomingText(context.Background(), "/voiceon")
	if !containsAll(reply, "已开启微信 TTS 语音输出") {
		t.Fatalf("/voiceon reply = %q, want enable confirmation", reply)
	}

	settings, err = svc.GetTTSSettings()
	if err != nil {
		t.Fatalf("GetTTSSettings() after /voiceon error = %v", err)
	}
	if !settings.WeixinVoiceOutputEnabled {
		t.Fatalf("GetTTSSettings() after /voiceon = %+v, want enabled voice output", settings)
	}
}

func TestWeixinIMBridgeAskReplyReturnsSynthesizedVoiceWhenTTSConfigured(t *testing.T) {
	svc, _, cfg := newTestService(t)
	if _, err := svc.UpdateWeixinBridgeSettings(model.WeixinBridgeSettings{Enabled: true}); err != nil {
		t.Fatalf("UpdateWeixinBridgeSettings() error = %v", err)
	}
	if _, err := svc.UpdateTTSSettings(model.TTSSettings{
		AppID:     "app-id",
		AccessKey: "access-key",
		Speaker:   "speaker-id",
	}); err != nil {
		t.Fatalf("UpdateTTSSettings() error = %v", err)
	}

	aiReader := &fakeWeixinAIReader{
		answer:     "这是 Ask 的语音内容。",
		ttsRewrite: "这是适合朗读的 Ask 语音内容。",
	}
	bridge := newTestWeixinBridge(t, svc, aiReader, cfg.StorageDir)
	paper := createBridgePaper(t, svc.repo, "Ask TTS Paper", "ask-tts.pdf")
	bridge.activatePaperContext(paper.ID, true)

	voiceDir := t.TempDir()
	voicePath := filepath.Join(voiceDir, "ask-reply.mp3")
	if err := os.WriteFile(voicePath, []byte("tts-audio"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	synthCalled := false
	bridge.synthesizeTTS = func(_ context.Context, text, uid string, settings model.TTSSettings) (string, func(), error) {
		synthCalled = true
		if text != "这是适合朗读的 Ask 语音内容。" {
			t.Fatalf("synthesizeTTS() text = %q, want %q", text, "这是适合朗读的 Ask 语音内容。")
		}
		if uid != "user@im.wechat" {
			t.Fatalf("synthesizeTTS() uid = %q, want %q", uid, "user@im.wechat")
		}
		if settings.AppID != "app-id" || settings.Speaker != "speaker-id" {
			t.Fatalf("synthesizeTTS() settings = %+v, want persisted TTS config", settings)
		}
		return voicePath, func() {}, nil
	}

	reply := bridge.handleIncomingMessageReply(context.Background(), weixin.Message{
		FromUserID: "user@im.wechat",
		ItemList: []weixin.MessageItem{
			{
				Type:     weixin.ItemTypeText,
				TextItem: &weixin.TextItem{Text: "/ask 总结一下这篇文献"},
			},
		},
	})
	if !containsAll(reply.Text, "文献问答", "这是 Ask 的语音内容。") {
		t.Fatalf("handleIncomingMessageReply() = %+v, want ask answer text", reply)
	}
	if reply.VoicePendingNotice != "语音内容生成中，请稍后。" {
		t.Fatalf("handleIncomingMessageReply() pending notice = %q, want %q", reply.VoicePendingNotice, "语音内容生成中，请稍后。")
	}

	selectedPath, cleanup, err := bridge.resolveVoiceReply(context.Background(), weixin.Message{
		FromUserID: "user@im.wechat",
	}, reply)
	if err != nil {
		t.Fatalf("resolveVoiceReply() error = %v", err)
	}
	cleanup()
	if !synthCalled {
		t.Fatal("resolveVoiceReply() did not trigger synthesizeTTS")
	}
	if selectedPath != voicePath {
		t.Fatalf("resolveVoiceReply() path = %q, want %q", selectedPath, voicePath)
	}
}

func TestWeixinIMBridgeAskReplySkipsVoiceWhenWeixinVoiceOutputDisabled(t *testing.T) {
	svc, _, cfg := newTestService(t)
	if _, err := svc.UpdateWeixinBridgeSettings(model.WeixinBridgeSettings{Enabled: true}); err != nil {
		t.Fatalf("UpdateWeixinBridgeSettings() error = %v", err)
	}
	if _, err := svc.UpdateTTSSettings(model.TTSSettings{
		AppID:                       "app-id",
		AccessKey:                   "access-key",
		Speaker:                     "speaker-id",
		WeixinVoiceOutputEnabled:    false,
		WeixinVoiceOutputEnabledSet: true,
	}); err != nil {
		t.Fatalf("UpdateTTSSettings() error = %v", err)
	}

	bridge := newTestWeixinBridge(t, svc, &fakeWeixinAIReader{
		answer:     "这是 Ask 的文字答案。",
		ttsRewrite: "这是 Ask 的语音答案。",
	}, cfg.StorageDir)
	paper := createBridgePaper(t, svc.repo, "Ask Voice Off Paper", "ask-voiceoff.pdf")
	bridge.activatePaperContext(paper.ID, true)

	synthCalled := false
	bridge.synthesizeTTS = func(_ context.Context, text, uid string, settings model.TTSSettings) (string, func(), error) {
		synthCalled = true
		return "", func() {}, nil
	}

	reply := bridge.handleIncomingMessageReply(context.Background(), weixin.Message{
		FromUserID: "user@im.wechat",
		ItemList: []weixin.MessageItem{
			{
				Type:     weixin.ItemTypeText,
				TextItem: &weixin.TextItem{Text: "/ask 总结一下"},
			},
		},
	})
	if !containsAll(reply.Text, "文献问答", "这是 Ask 的文字答案。") {
		t.Fatalf("handleIncomingMessageReply() = %+v, want ask answer text", reply)
	}

	selectedPath, cleanup, err := bridge.resolveVoiceReply(context.Background(), weixin.Message{
		FromUserID: "user@im.wechat",
	}, reply)
	if err != nil {
		t.Fatalf("resolveVoiceReply() error = %v", err)
	}
	cleanup()
	if synthCalled {
		t.Fatal("resolveVoiceReply() synthesized voice even though weixin voice output is disabled")
	}
	if selectedPath != "" {
		t.Fatalf("resolveVoiceReply() path = %q, want empty path when weixin voice output is disabled", selectedPath)
	}
}

func TestWeixinIMBridgeTestVoiceReturnsHintWhenVoiceOutputDisabled(t *testing.T) {
	svc, _, cfg := newTestService(t)
	if _, err := svc.UpdateTTSSettings(model.TTSSettings{
		AppID:                       "app-id",
		AccessKey:                   "access-key",
		Speaker:                     "speaker-id",
		WeixinVoiceOutputEnabled:    false,
		WeixinVoiceOutputEnabledSet: true,
	}); err != nil {
		t.Fatalf("UpdateTTSSettings() error = %v", err)
	}
	bridge := newTestWeixinBridge(t, svc, &fakeWeixinAIReader{answer: "ok"}, cfg.StorageDir)

	reply := bridge.handleIncomingText(context.Background(), "/testvoice")
	if !containsAll(reply, "微信 TTS 语音输出当前已关闭", "/voiceon", "/testvoice") {
		t.Fatalf("/testvoice reply = %q, want disabled hint", reply)
	}

	replyEnvelope := bridge.handleIncomingTextReply(context.Background(), "/testvoice")
	selectedPath, cleanup, err := bridge.resolveVoiceReply(context.Background(), weixin.Message{
		FromUserID: "user@im.wechat",
	}, replyEnvelope)
	if err != nil {
		t.Fatalf("resolveVoiceReply() error = %v, want nil when /testvoice is blocked by disabled voice output", err)
	}
	cleanup()
	if selectedPath != "" {
		t.Fatalf("resolveVoiceReply() path = %q, want empty path when /testvoice is blocked by disabled voice output", selectedPath)
	}
}

func TestWeixinIMBridgeVoiceRewriteFallbackSanitizesMarkdown(t *testing.T) {
	svc, _, cfg := newTestService(t)
	if _, err := svc.UpdateTTSSettings(model.TTSSettings{
		AppID:     "app-id",
		AccessKey: "access-key",
		Speaker:   "speaker-id",
	}); err != nil {
		t.Fatalf("UpdateTTSSettings() error = %v", err)
	}

	aiReader := &fakeWeixinAIReader{
		answer:        "ok",
		ttsRewriteErr: context.DeadlineExceeded,
	}
	bridge := newTestWeixinBridge(t, svc, aiReader, cfg.StorageDir)

	voiceDir := t.TempDir()
	voicePath := filepath.Join(voiceDir, "ask-reply.mp3")
	if err := os.WriteFile(voicePath, []byte("tts-audio"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	synthCalled := false
	bridge.synthesizeTTS = func(_ context.Context, text, uid string, settings model.TTSSettings) (string, func(), error) {
		synthCalled = true
		want := "见图形摘要 第 1 页图 1\n\n一句话概括：这篇论文构建了首个统一图谱。"
		if text != want {
			t.Fatalf("synthesizeTTS() text = %q, want %q", text, want)
		}
		return voicePath, func() {}, nil
	}

	reply := weixinReplyEnvelope{
		TTSText:         "见图形摘要 ![第 1 页图 1](figure://309)\n\n一句话概括：**这篇论文**构建了首个统一图谱。",
		OptimizeTTSText: true,
	}
	selectedPath, cleanup, err := bridge.resolveVoiceReply(context.Background(), weixin.Message{
		FromUserID: "user@im.wechat",
	}, reply)
	if err != nil {
		t.Fatalf("resolveVoiceReply() error = %v", err)
	}
	cleanup()
	if !synthCalled {
		t.Fatal("resolveVoiceReply() did not trigger synthesizeTTS")
	}
	if selectedPath != voicePath {
		t.Fatalf("resolveVoiceReply() path = %q, want %q", selectedPath, voicePath)
	}
}

func TestSplitWeixinReplyTextSplitsLongReply(t *testing.T) {
	longText := strings.Repeat("测", 5000)

	chunks := splitWeixinReplyText(longText)
	if len(chunks) != 2 {
		t.Fatalf("len(chunks) = %d, want %d for oversized uninterrupted text", len(chunks), 2)
	}
	for index, chunk := range chunks {
		if got := len([]rune(chunk)); got > weixinReplyChunkMaxRunes {
			t.Fatalf("chunk %d rune count = %d, want <= %d", index, got, weixinReplyChunkMaxRunes)
		}
	}
	if strings.Join(chunks, "") != longText {
		t.Fatal("splitWeixinReplyText() chunks do not reconstruct original text")
	}
}

func TestSplitWeixinReplyUnitsPrefersNaturalBreaks(t *testing.T) {
	text := "第一段先把背景交代清楚，也把主要结论说清楚。\n\n第二段先解释方法。第二段再解释结果。"

	chunks := splitWeixinReplyUnits(text)
	if len(chunks) != 3 {
		t.Fatalf("len(chunks) = %d, want %d", len(chunks), 3)
	}
	if chunks[0] != "第一段先把背景交代清楚，也把主要结论说清楚。\n\n" {
		t.Fatalf("chunks[0] = %q, want paragraph-aligned split", chunks[0])
	}
	if chunks[1] != "第二段先解释方法。" {
		t.Fatalf("chunks[1] = %q, want sentence-aligned split", chunks[1])
	}
	if chunks[2] != "第二段再解释结果。" {
		t.Fatalf("chunks[2] = %q, want remaining sentence preserved", chunks[2])
	}
	if strings.Join(chunks, "") != text {
		t.Fatal("splitWeixinReplyText() chunks do not reconstruct original text")
	}
}

func TestSplitWeixinReplyTextPacksNaturalUnitsUpToChunkLimit(t *testing.T) {
	sentence := strings.Repeat("甲", 900) + "。"
	text := sentence + sentence + sentence + sentence

	chunks := splitWeixinReplyText(text)
	if len(chunks) != 2 {
		t.Fatalf("len(chunks) = %d, want %d packed chunks", len(chunks), 2)
	}
	if got := len([]rune(chunks[0])); got > weixinReplyChunkMaxRunes {
		t.Fatalf("first chunk rune count = %d, want <= %d", got, weixinReplyChunkMaxRunes)
	}
	if got := len([]rune(chunks[1])); got > weixinReplyChunkMaxRunes {
		t.Fatalf("second chunk rune count = %d, want <= %d", got, weixinReplyChunkMaxRunes)
	}
	if strings.Join(chunks, "") != text {
		t.Fatal("splitWeixinReplyText() packed chunks do not reconstruct original text")
	}
}

func TestTrimWeixinReplyPreservesLongReply(t *testing.T) {
	longText := strings.Repeat("长", 5000)
	if got := trimWeixinReply(longText); got != longText {
		t.Fatalf("trimWeixinReply() = %q, want full text preserved", got)
	}
}

func TestSplitWeixinReplyTextReturnsNilForBlankText(t *testing.T) {
	chunks := splitWeixinReplyText(" \n\t ")
	if len(chunks) != 0 {
		t.Fatalf("len(chunks) = %d, want 0", len(chunks))
	}
}

func TestWeixinIMBridgeImportsPDFFileAndSelectsPaper(t *testing.T) {
	svc, _, cfg := newTestService(t)
	bridge := newTestWeixinBridge(t, svc, &fakeWeixinAIReader{answer: "ok"}, cfg.StorageDir)
	bridge.downloadFile = func(context.Context, weixin.MessageItem) (*weixin.DownloadedFile, error) {
		return &weixin.DownloadedFile{
			Filename:    "wechat-upload.bin",
			ContentType: "application/octet-stream",
			Data:        []byte("%PDF-1.4 wechat upload"),
		}, nil
	}

	reply := bridge.handleIncomingMessage(context.Background(), weixin.Message{
		ItemList: []weixin.MessageItem{
			{
				Type: weixin.ItemTypeFile,
				FileItem: &weixin.FileItem{
					FileName: "wechat-upload.bin",
					Len:      "22",
					Media: &weixin.CDNMedia{
						EncryptQueryParam: "encrypted",
						AESKey:            "aeskey",
					},
				},
			},
		},
	})

	if !containsAll(reply, "已从微信导入 PDF", "已选中文献") {
		t.Fatalf("import reply = %q, want import success message", reply)
	}

	result, err := svc.ListPapers(model.PaperFilter{})
	if err != nil {
		t.Fatalf("ListPapers() error = %v", err)
	}
	if result.Total != 1 || len(result.Papers) != 1 {
		t.Fatalf("paper total = %d papers=%d, want 1", result.Total, len(result.Papers))
	}
	if bridge.getContext().CurrentPaperID != result.Papers[0].ID {
		t.Fatalf("current paper = %d, want %d", bridge.getContext().CurrentPaperID, result.Papers[0].ID)
	}
	if got := result.Papers[0].OriginalFilename; got != "wechat-upload.pdf" {
		t.Fatalf("original filename = %q, want sniffed PDF filename with normalized .pdf suffix", got)
	}
}

func TestWeixinIMBridgeImportsPaperFromDOIText(t *testing.T) {
	svc, _, cfg := newTestService(t)
	svc.config.OAContactEmail = "ops@example.com"
	bridge := newTestWeixinBridge(t, svc, &fakeWeixinAIReader{
		commandPlanErr: errors.New("doi import should bypass command planning"),
	}, cfg.StorageDir)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/unpaywall/v2/"):
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"doi":"10.5555/wechat-doi","title":"WeChat DOI Import","best_oa_location":{"url_for_pdf":%q}}`, server.URL+"/files/wechat-doi.pdf")
		case r.URL.Path == "/files/wechat-doi.pdf":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write(testPDFBytes())
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	originalUnpaywall := unpaywallAPIBaseURL
	originalEuropePMC := europePMCSearchURL
	originalPMCID := pmcIDConvURL
	unpaywallAPIBaseURL = server.URL + "/unpaywall/v2/"
	europePMCSearchURL = server.URL + "/europe-pmc/search"
	pmcIDConvURL = server.URL + "/pmc/idconv"
	defer func() {
		unpaywallAPIBaseURL = originalUnpaywall
		europePMCSearchURL = originalEuropePMC
		pmcIDConvURL = originalPMCID
	}()

	reply := bridge.handleIncomingText(context.Background(), "https://doi.org/10.5555/WECHAT-DOI")
	if !containsAll(reply, "已通过 DOI 导入文献", "已选中文献") {
		t.Fatalf("doi import reply = %q, want DOI import success message", reply)
	}

	result, err := svc.ListPapers(model.PaperFilter{})
	if err != nil {
		t.Fatalf("ListPapers() error = %v", err)
	}
	if result.Total != 1 || len(result.Papers) != 1 {
		t.Fatalf("paper total = %d papers=%d, want 1", result.Total, len(result.Papers))
	}
	if got := result.Papers[0].DOI; got != "10.5555/wechat-doi" {
		t.Fatalf("paper doi = %q, want %q", got, "10.5555/wechat-doi")
	}
	if bridge.getContext().CurrentPaperID != result.Papers[0].ID {
		t.Fatalf("current paper = %d, want %d", bridge.getContext().CurrentPaperID, result.Papers[0].ID)
	}
}

func TestWeixinIMBridgeImportsPDFFileBackfillsFullText(t *testing.T) {
	svc, _, cfg := newTestService(t)
	svc.startBackground = true
	svc.pdfTextExtractor = func(path string) (string, error) {
		return "wechat imported full text", nil
	}

	bridge := newTestWeixinBridge(t, svc, &fakeWeixinAIReader{answer: "ok"}, cfg.StorageDir)
	bridge.downloadFile = func(context.Context, weixin.MessageItem) (*weixin.DownloadedFile, error) {
		return &weixin.DownloadedFile{
			Filename:    "wechat-text.pdf",
			ContentType: "application/pdf",
			Data:        []byte("%PDF-1.4 wechat full text"),
		}, nil
	}

	reply := bridge.handleIncomingMessage(context.Background(), weixin.Message{
		ItemList: []weixin.MessageItem{
			{
				Type: weixin.ItemTypeFile,
				FileItem: &weixin.FileItem{
					FileName: "wechat-text.pdf",
					Len:      "27",
					Media: &weixin.CDNMedia{
						EncryptQueryParam: "encrypted",
						AESKey:            "aeskey",
					},
				},
			},
		},
	})

	if !containsAll(reply, "已从微信导入 PDF", "已选中文献") {
		t.Fatalf("import reply = %q, want import success message", reply)
	}

	result, err := svc.ListPapers(model.PaperFilter{})
	if err != nil {
		t.Fatalf("ListPapers() error = %v", err)
	}
	if result.Total != 1 || len(result.Papers) != 1 {
		t.Fatalf("paper total = %d papers=%d, want 1", result.Total, len(result.Papers))
	}
	if got := waitForPaperPDFText(t, svc, result.Papers[0].ID); got != "wechat imported full text" {
		t.Fatalf("waitForPaperPDFText() = %q, want %q", got, "wechat imported full text")
	}
}

func TestWeixinIMBridgeReusesExistingPaperForDuplicatePDF(t *testing.T) {
	svc, _, cfg := newTestService(t)
	content := []byte("%PDF-1.4 duplicate upload")
	header := &multipart.FileHeader{
		Filename: "existing.pdf",
		Size:     int64(len(content)),
		Header: textproto.MIMEHeader{
			"Content-Type": []string{"application/pdf"},
		},
	}

	existing, err := svc.UploadPaper(&testMultipartFile{Reader: bytes.NewReader(content)}, header, UploadPaperParams{})
	if err != nil {
		t.Fatalf("UploadPaper() error = %v", err)
	}

	bridge := newTestWeixinBridge(t, svc, &fakeWeixinAIReader{answer: "ok"}, cfg.StorageDir)
	bridge.downloadFile = func(context.Context, weixin.MessageItem) (*weixin.DownloadedFile, error) {
		return &weixin.DownloadedFile{
			Filename:    "wechat-duplicate.pdf",
			ContentType: "application/pdf",
			Data:        append([]byte(nil), content...),
		}, nil
	}

	reply := bridge.handleIncomingMessage(context.Background(), weixin.Message{
		ItemList: []weixin.MessageItem{
			{
				Type: weixin.ItemTypeFile,
				FileItem: &weixin.FileItem{
					FileName: "wechat-duplicate.pdf",
					Len:      "25",
					Media: &weixin.CDNMedia{
						EncryptQueryParam: "encrypted",
						AESKey:            "aeskey",
					},
				},
			},
		},
	})

	if !containsAll(reply, "已在文献库中", "已选中文献") {
		t.Fatalf("duplicate reply = %q, want duplicate guidance", reply)
	}
	if bridge.getContext().CurrentPaperID != existing.ID {
		t.Fatalf("current paper = %d, want existing %d", bridge.getContext().CurrentPaperID, existing.ID)
	}

	result, err := svc.ListPapers(model.PaperFilter{})
	if err != nil {
		t.Fatalf("ListPapers() error = %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("paper total = %d, want 1 after duplicate import", result.Total)
	}
}

func TestWeixinIMBridgeRejectsNonPDFFiles(t *testing.T) {
	svc, _, cfg := newTestService(t)
	bridge := newTestWeixinBridge(t, svc, &fakeWeixinAIReader{answer: "ok"}, cfg.StorageDir)
	bridge.downloadFile = func(context.Context, weixin.MessageItem) (*weixin.DownloadedFile, error) {
		return &weixin.DownloadedFile{
			Filename:    "notes.txt",
			ContentType: "text/plain",
			Data:        []byte("plain text"),
		}, nil
	}

	reply := bridge.handleIncomingMessage(context.Background(), weixin.Message{
		ItemList: []weixin.MessageItem{
			{
				Type: weixin.ItemTypeFile,
				FileItem: &weixin.FileItem{
					FileName: "notes.txt",
					Len:      "10",
					Media: &weixin.CDNMedia{
						EncryptQueryParam: "encrypted",
						AESKey:            "aeskey",
					},
				},
			},
		},
	})

	if !strings.Contains(reply, "目前只支持 PDF") {
		t.Fatalf("reject reply = %q, want PDF-only guidance", reply)
	}

	result, err := svc.ListPapers(model.PaperFilter{})
	if err != nil {
		t.Fatalf("ListPapers() error = %v", err)
	}
	if result.Total != 0 {
		t.Fatalf("paper total = %d, want 0 after rejected import", result.Total)
	}
}

func TestShouldHandleWeixinMessageAllowsBoundUserWithNonLegacyMessageType(t *testing.T) {
	ok, reason := shouldHandleWeixinMessage(
		weixinBindingRecord{
			UserID:    "user@im.wechat",
			AccountID: "bot@im.bot",
		},
		weixin.Message{
			FromUserID:  "user@im.wechat",
			ToUserID:    "bot@im.bot",
			MessageType: weixin.MessageTypeBot,
		},
	)

	if !ok {
		t.Fatalf("shouldHandleWeixinMessage() ok = false, reason = %q, want true for bound user message", reason)
	}
}

func TestShouldHandleWeixinMessageRejectsBotEcho(t *testing.T) {
	ok, reason := shouldHandleWeixinMessage(
		weixinBindingRecord{
			UserID:    "user@im.wechat",
			AccountID: "bot@im.bot",
		},
		weixin.Message{
			FromUserID:  "bot@im.bot",
			ToUserID:    "user@im.wechat",
			MessageType: weixin.MessageTypeBot,
		},
	)

	if ok {
		t.Fatal("shouldHandleWeixinMessage() ok = true, want false for bot echo")
	}
	if reason != "bot_echo" {
		t.Fatalf("shouldHandleWeixinMessage() reason = %q, want %q", reason, "bot_echo")
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}

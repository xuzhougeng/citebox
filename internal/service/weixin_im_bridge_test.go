package service

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

type fakeWeixinAIReader struct {
	mu     sync.Mutex
	inputs []model.AIReadRequest
	answer string
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

func newTestWeixinBridge(t *testing.T, svc *LibraryService, aiReader weixinAIReader, storageDir string) *WeixinIMBridge {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewWeixinIMBridge(svc, aiReader, logger, storageDir)
}

func TestWeixinIMBridgeSearchAndSelectPaperByResultNumber(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	createBridgePaper(t, repo, "Atlas Alpha", "atlas-alpha.pdf")
	createBridgePaper(t, repo, "Atlas Beta", "atlas-beta.pdf")
	bridge := newTestWeixinBridge(t, svc, &fakeWeixinAIReader{answer: "ok"}, cfg.StorageDir)

	reply := bridge.handleIncomingText(context.Background(), "搜 Atlas")
	if reply == "" {
		t.Fatal("search reply is empty")
	}

	state := bridge.getContext()
	if len(state.SearchPaperIDs) != 2 {
		t.Fatalf("search result ids = %v, want 2 ids", state.SearchPaperIDs)
	}

	reply = bridge.handleIncomingText(context.Background(), "1")
	if state.SearchPaperIDs[0] != bridge.getContext().CurrentPaperID {
		t.Fatalf("current paper = %d, want %d from first search result", bridge.getContext().CurrentPaperID, state.SearchPaperIDs[0])
	}
	if reply == "" {
		t.Fatal("select reply is empty")
	}
}

func TestWeixinIMBridgeAppendsPaperNote(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createBridgePaper(t, repo, "Atlas Unique", "atlas-unique.pdf")
	bridge := newTestWeixinBridge(t, svc, &fakeWeixinAIReader{answer: "ok"}, cfg.StorageDir)

	_ = bridge.handleIncomingText(context.Background(), "搜 Atlas Unique")
	reply := bridge.handleIncomingText(context.Background(), "笔记 这是从微信追加的笔记")
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

	_ = bridge.handleIncomingText(context.Background(), "搜 Figure Atlas")
	_ = bridge.handleIncomingText(context.Background(), "图片 1")
	reply := bridge.handleIncomingText(context.Background(), "解读 解释这张图")

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

func TestWeixinIMBridgeQuestionCarriesHistory(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	createBridgePaper(t, repo, "History Atlas", "history-atlas.pdf")
	aiReader := &fakeWeixinAIReader{answer: "ok"}
	bridge := newTestWeixinBridge(t, svc, aiReader, cfg.StorageDir)

	_ = bridge.handleIncomingText(context.Background(), "搜 History Atlas")
	_ = bridge.handleIncomingText(context.Background(), "第一问")
	_ = bridge.handleIncomingText(context.Background(), "第二问")

	last := aiReader.lastInput()
	if last.Action != model.AIActionPaperQA {
		t.Fatalf("AI action = %q, want %q", last.Action, model.AIActionPaperQA)
	}
	if len(last.History) != 1 || last.History[0].Question != "第一问" {
		t.Fatalf("AI history = %+v, want previous QA turn", last.History)
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

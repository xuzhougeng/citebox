package service

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"mime/multipart"
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

func TestWeixinIMBridgeSelectFigureResolvesPreviewPath(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createBridgePaper(t, repo, "Preview Atlas", "preview-atlas.pdf")
	bridge := newTestWeixinBridge(t, svc, &fakeWeixinAIReader{answer: "ok"}, cfg.StorageDir)

	figurePath := filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename)
	if err := os.WriteFile(figurePath, []byte("preview-bytes"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_ = bridge.handleIncomingText(context.Background(), "搜 Preview Atlas")
	reply := bridge.handleIncomingText(context.Background(), "图片 1")
	previewPath, err := bridge.selectedFigurePreviewPath(weixin.Message{
		ItemList: []weixin.MessageItem{
			{
				Type:     weixin.ItemTypeText,
				TextItem: &weixin.TextItem{Text: "图片 1"},
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

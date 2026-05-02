package service

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
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

// fakeWeixinAIReader implements the narrow weixinAIReader interface. After the
// agent_session cutover the bridge only calls aiService for TTS rewrites; the
// other tests just need a non-nil value to construct the bridge.
type fakeWeixinAIReader struct {
	mu            sync.Mutex
	ttsRewrite    string
	ttsRewriteErr error
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
	return NewWeixinIMBridge(svc, aiReader, logger, storageDir, nil, nil)
}

func TestWeixinIMBridgeRunReportsDisabledState(t *testing.T) {
	svc, _, cfg := newTestService(t)
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	bridge := NewWeixinIMBridge(svc, &fakeWeixinAIReader{}, logger, cfg.StorageDir, nil, nil)

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
	bridge := NewWeixinIMBridge(svc, &fakeWeixinAIReader{}, logger, cfg.StorageDir, nil, nil)

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

func TestWeixinIMBridgeRunDisablesBridgeWhenSessionExpires(t *testing.T) {
	svc, _, cfg := newTestService(t)
	if _, err := svc.UpdateWeixinBridgeSettings(model.WeixinBridgeSettings{
		Enabled: true,
		DailyRecommendation: model.WeixinDailyRecommendationSettings{
			Enabled:  true,
			SendTime: "08:30",
		},
	}); err != nil {
		t.Fatalf("UpdateWeixinBridgeSettings() error = %v", err)
	}
	if err := svc.saveWeixinBinding(weixinBindingRecord{
		Token:     "bot-token-123",
		BaseURL:   "https://weixin.test",
		UserID:    "user@im.wechat",
		AccountID: "bot@im.bot",
	}); err != nil {
		t.Fatalf("saveWeixinBinding() error = %v", err)
	}

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	bridge := NewWeixinIMBridge(svc, &fakeWeixinAIReader{}, logger, cfg.StorageDir, nil, nil)

	var pollCalls int
	var pollCallsMu sync.Mutex
	bridge.newClient = func(binding weixinBindingRecord) *weixin.Client {
		return weixin.NewClient(binding.BaseURL, binding.Token, &http.Client{
			Transport: testRoundTripper(func(req *http.Request) (*http.Response, error) {
				pollCallsMu.Lock()
				pollCalls++
				pollCallsMu.Unlock()

				if req.URL.Path != "/ilink/bot/getupdates" {
					t.Fatalf("request path = %q, want %q", req.URL.Path, "/ilink/bot/getupdates")
				}
				return jsonHTTPResponse(http.StatusOK, `{"ret":0,"errcode":-14}`), nil
			}),
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- bridge.Run(ctx)
	}()

	deadline := time.Now().Add(2 * time.Second)
	disabled := false
	for time.Now().Before(deadline) {
		settings, err := svc.GetWeixinBridgeSettings()
		if err != nil {
			t.Fatalf("GetWeixinBridgeSettings() error = %v", err)
		}
		if !settings.Enabled {
			disabled = true
			if !settings.DailyRecommendation.Enabled || settings.DailyRecommendation.SendTime != "08:30" {
				t.Fatalf("GetWeixinBridgeSettings() after expiry = %+v, want disabled bridge with preserved daily recommendation", settings)
			}
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !disabled {
		t.Fatalf("weixin bridge was not auto-disabled after session expiry; logs=%q", logs.String())
	}

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil && err != context.Canceled {
			t.Fatalf("Run() error = %v, want nil or context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not stop after context cancellation")
	}

	pollCallsMu.Lock()
	gotPollCalls := pollCalls
	pollCallsMu.Unlock()
	if gotPollCalls != 1 {
		t.Fatalf("GetUpdates() calls = %d, want 1 before bridge auto-disables", gotPollCalls)
	}

	logOutput := logs.String()
	if !strings.Contains(logOutput, "weixin session expired; disabled bridge in settings") {
		t.Fatalf("Run() logs = %q, want session expiry auto-disable warning", logOutput)
	}
	if !strings.Contains(logOutput, "weixin IM bridge polling stopped") {
		t.Fatalf("Run() logs = %q, want polling stopped warning", logOutput)
	}
}

// TestWeixinIMBridgeUnknownSlashCommandReturnsHelpFallback exercises the
// agentSession==nil fallback path: when the bridge has no agent_session wired
// (early-boot or unit-test mode) any non-bridge-local input — including an
// unknown slash command — should return weixinHelpText() rather than crash or
// echo. Once T22 wires agent_session into bridge integration tests this can
// be expanded to assert /unknown routes to the registry's "unknown command"
// envelope.
func TestWeixinIMBridgeUnknownSlashCommandReturnsHelpFallback(t *testing.T) {
	svc, _, cfg := newTestService(t)
	bridge := newTestWeixinBridge(t, svc, &fakeWeixinAIReader{}, cfg.StorageDir)

	reply := bridge.handleIncomingText(context.Background(), "/unknown something")
	if !containsAll(reply, "微信 IM 优先响应 slash 命令", "`/help`") {
		t.Fatalf("unknown slash reply = %q, want help text", reply)
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
	bridge := newTestWeixinBridge(t, svc, &fakeWeixinAIReader{}, cfg.StorageDir)

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
	bridge := newTestWeixinBridge(t, svc, &fakeWeixinAIReader{}, cfg.StorageDir)

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
	bridge := newTestWeixinBridge(t, svc, &fakeWeixinAIReader{}, cfg.StorageDir)

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
	bridge := newTestWeixinBridge(t, svc, &fakeWeixinAIReader{}, cfg.StorageDir)
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

func TestWeixinIMBridgeImportsPDFFileBackfillsFullText(t *testing.T) {
	svc, _, cfg := newTestService(t)
	svc.startBackground = true
	svc.pdfTextExtractor = func(path string) (string, error) {
		return "wechat imported full text", nil
	}

	bridge := newTestWeixinBridge(t, svc, &fakeWeixinAIReader{}, cfg.StorageDir)
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

	bridge := newTestWeixinBridge(t, svc, &fakeWeixinAIReader{}, cfg.StorageDir)
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
	bridge := newTestWeixinBridge(t, svc, &fakeWeixinAIReader{}, cfg.StorageDir)
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

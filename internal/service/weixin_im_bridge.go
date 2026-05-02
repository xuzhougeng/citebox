package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/service/agent_session"
	"github.com/xuzhougeng/citebox/internal/weixin"
)

const (
	weixinBridgeStateDirName          = "weixin-bridge"
	weixinSyncBufFileName             = "sync_buf"
	weixinContextFileName             = "im_context.json"
	weixinDailyRecommendationInterval = 30 * time.Second
	weixinReplyChunkMaxRunes          = 3200
)

var errWeixinSessionExpired = errors.New("weixin session expired")

// weixinAIReader is the narrow slice of AIService the bridge still needs after
// the agent_session cutover. Text-dispatch (search/ask/interpret/...) now goes
// through agent_session; the bridge only needs the TTS rewrite step for the
// voice-output side of /ask replies.
type weixinAIReader interface {
	RewriteTextForTTS(ctx context.Context, text string) (string, error)
}

// weixinIMContext is the bridge-local on-disk state. Most fields here are now
// dormant — agent_session owns the live current_paper / current_figure /
// recent-search state via SurfaceStateStore. The struct is kept (and still
// (un)marshalled) so legacy `im_context.json` files continue to parse during
// the migration window. QAHistory in particular is consumed by
// agent_session.MigrateLegacyWeixinContext and not by the bridge.
type weixinIMContext struct {
	CurrentPaperID  int64                      `json:"current_paper_id"`
	CurrentFigureID int64                      `json:"current_figure_id"`
	SearchPaperIDs  []int64                    `json:"search_paper_ids,omitempty"`
	SearchFigureIDs []int64                    `json:"search_figure_ids,omitempty"`
	QAHistory       []model.AIConversationTurn `json:"qa_history,omitempty"`
	UpdatedAt       string                     `json:"updated_at,omitempty"`
}

type weixinReplyEnvelope struct {
	Text                      string
	PreviewCurrentFigure      bool
	TTSText                   string
	OptimizeTTSText           bool
	RequireTTS                bool
	VoicePendingNotice        string
	VoiceResolveFailureNotice string
	VoiceSendFailureNotice    string
}

type WeixinIMBridge struct {
	libraryService *LibraryService
	aiService      weixinAIReader
	agentSession   *agent_session.Service
	surfaceState   *agent_session.SurfaceStateStore
	logger         *slog.Logger
	downloadFile   func(context.Context, weixin.MessageItem) (*weixin.DownloadedFile, error)
	newClient      func(weixinBindingRecord) *weixin.Client
	synthesizeTTS  func(context.Context, string, string, model.TTSSettings) (string, func(), error)
	stateDir       string
	syncBufPath    string
	contextPath    string

	mu      sync.Mutex
	context weixinIMContext
}

// NewWeixinIMBridge wires the bridge with its agent_session collaborators.
//
// `agentSession` and `surfaceState` may be nil during early-boot wiring or in
// unit tests that don't exercise the dispatch path; in that mode the bridge
// falls back to returning the help text for any non-bridge-local input.
func NewWeixinIMBridge(libraryService *LibraryService, aiService weixinAIReader, logger *slog.Logger,
	storageDir string, agentSession *agent_session.Service, surfaceState *agent_session.SurfaceStateStore) *WeixinIMBridge {
	if logger == nil {
		logger = slog.Default()
	}

	bridge := &WeixinIMBridge{
		libraryService: libraryService,
		aiService:      aiService,
		agentSession:   agentSession,
		surfaceState:   surfaceState,
		logger:         logger.With("component", "weixin_im_bridge"),
		downloadFile: func(ctx context.Context, item weixin.MessageItem) (*weixin.DownloadedFile, error) {
			return weixin.DownloadFileItem(ctx, item, nil, "")
		},
		newClient: func(binding weixinBindingRecord) *weixin.Client {
			return weixin.NewClient(binding.BaseURL, binding.Token, nil)
		},
		stateDir: filepath.Join(storageDir, weixinBridgeStateDirName),
	}
	bridge.syncBufPath = filepath.Join(bridge.stateDir, weixinSyncBufFileName)
	bridge.contextPath = filepath.Join(bridge.stateDir, weixinContextFileName)
	bridge.synthesizeTTS = bridge.synthesizeReplyVoice
	bridge.loadContext()
	return bridge
}

func (b *WeixinIMBridge) Run(ctx context.Context) error {
	b.logger.Info("weixin IM bridge loop started")
	waitingForEnable := false
	waitingForBinding := false

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		enabled, err := b.libraryService.isWeixinBridgeEnabled()
		if err != nil {
			b.logger.Warn("load weixin bridge settings failed", "error", err)
			if !sleepContext(ctx, 5*time.Second) {
				return ctx.Err()
			}
			continue
		}
		if !enabled {
			if !waitingForEnable {
				b.logger.Info("weixin IM bridge is disabled; enable it in Settings to start polling")
				waitingForEnable = true
			}
			waitingForBinding = false
			if !sleepContext(ctx, 5*time.Second) {
				return ctx.Err()
			}
			continue
		}
		if waitingForEnable {
			b.logger.Info("weixin IM bridge enabled, checking binding state")
			waitingForEnable = false
		}

		binding, err := b.libraryService.loadWeixinBinding()
		if err != nil {
			b.logger.Warn("load weixin binding failed", "error", err)
			if !sleepContext(ctx, 5*time.Second) {
				return ctx.Err()
			}
			continue
		}
		if strings.TrimSpace(binding.Token) == "" {
			if !waitingForBinding {
				b.logger.Warn("weixin IM bridge enabled but no active binding found; complete Weixin binding in Settings before expecting message replies")
				waitingForBinding = true
			}
			if !sleepContext(ctx, 5*time.Second) {
				return ctx.Err()
			}
			continue
		}
		if waitingForBinding {
			b.logger.Info("weixin binding detected, starting IM polling")
			waitingForBinding = false
		}

		client := b.newClient(binding)
		if client == nil {
			client = weixin.NewClient(binding.BaseURL, binding.Token, nil)
		}
		b.logger.Info("weixin IM bridge polling", "user_id", binding.UserID)

		pollCtx, cancelPolling := context.WithCancel(ctx)
		dailyDone := make(chan struct{})
		go func() {
			defer close(dailyDone)
			b.runDailyRecommendationLoop(pollCtx)
		}()

		skipBackoff := false
		if err := b.runPolling(pollCtx, client, binding); err != nil && !errors.Is(err, context.Canceled) {
			skipBackoff = b.handlePollingStop(err)
		}
		cancelPolling()
		<-dailyDone

		if skipBackoff {
			continue
		}
		if !sleepContext(ctx, 3*time.Second) {
			return ctx.Err()
		}
	}
}

func (b *WeixinIMBridge) handlePollingStop(err error) bool {
	if errors.Is(err, errWeixinSessionExpired) {
		settings, disableErr := b.libraryService.disableWeixinBridge()
		if disableErr != nil {
			b.logger.Warn("auto-disable weixin bridge after session expiry failed", "error", disableErr)
		} else {
			b.logger.Warn(
				"weixin session expired; disabled bridge in settings",
				"daily_recommendation_enabled", settings.DailyRecommendation.Enabled,
				"daily_recommendation_send_time", settings.DailyRecommendation.SendTime,
			)
			b.logger.Warn("weixin IM bridge polling stopped", "error", err)
			return true
		}
	}

	b.logger.Warn("weixin IM bridge polling stopped", "error", err)
	return false
}

func (b *WeixinIMBridge) runDailyRecommendationLoop(ctx context.Context) {
	if result, err := b.libraryService.MaybeSendWeixinDailyRecommendation(ctx, time.Now()); err != nil {
		if !apperr.IsCode(err, apperr.CodeFailedPrecondition) {
			b.logger.Warn("send weixin daily recommendation failed", "error", err)
		}
	} else if result != nil {
		b.logger.Info("weixin daily recommendation sent",
			"figure_id", result.FigureID,
			"paper_title", result.PaperTitle,
			"display_label", result.DisplayLabel,
			"test", result.Test,
		)
	}

	ticker := time.NewTicker(weixinDailyRecommendationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case tickAt := <-ticker.C:
			result, err := b.libraryService.MaybeSendWeixinDailyRecommendation(ctx, tickAt)
			if err != nil {
				if !apperr.IsCode(err, apperr.CodeFailedPrecondition) {
					b.logger.Warn("send weixin daily recommendation failed", "error", err)
				}
				continue
			}
			if result == nil {
				continue
			}

			b.logger.Info("weixin daily recommendation sent",
				"figure_id", result.FigureID,
				"paper_title", result.PaperTitle,
				"display_label", result.DisplayLabel,
				"test", result.Test,
			)
		}
	}
}

func (b *WeixinIMBridge) runPolling(ctx context.Context, client *weixin.Client, binding weixinBindingRecord) error {
	updatesBuf := b.loadSyncBuf()

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		enabled, err := b.libraryService.isWeixinBridgeEnabled()
		if err != nil {
			return err
		}
		if !enabled {
			b.logger.Info("weixin IM bridge disabled, stopping poller")
			return nil
		}

		latestBinding, err := b.libraryService.loadWeixinBinding()
		if err == nil && !sameWeixinBinding(binding, latestBinding) {
			b.logger.Info("weixin binding changed, restarting poller")
			return nil
		}

		resp, err := client.GetUpdates(ctx, updatesBuf)
		if err != nil {
			return err
		}
		if resp.ErrCode == -14 {
			return errWeixinSessionExpired
		}

		if nextBuf := strings.TrimSpace(resp.GetUpdatesBuf); nextBuf != "" {
			updatesBuf = nextBuf
			if err := writeAtomicFile(b.syncBufPath, []byte(nextBuf)); err != nil {
				b.logger.Warn("save weixin sync buffer failed", "error", err)
			}
		}
		if len(resp.Msgs) > 0 {
			b.logger.Info("received weixin updates", "message_count", len(resp.Msgs))
		}

		for _, message := range resp.Msgs {
			if ok, reason := shouldHandleWeixinMessage(binding, message); !ok {
				switch reason {
				case "unexpected_sender":
					b.logger.Warn(
						"ignore message from unexpected weixin user",
						"from_user_id", strings.TrimSpace(message.FromUserID),
						"to_user_id", strings.TrimSpace(message.ToUserID),
						"message_type", message.MessageType,
					)
				case "unexpected_recipient":
					b.logger.Warn(
						"ignore weixin message to unexpected recipient",
						"from_user_id", strings.TrimSpace(message.FromUserID),
						"to_user_id", strings.TrimSpace(message.ToUserID),
						"message_type", message.MessageType,
					)
				}
				continue
			}

			b.logger.Info(
				"handle weixin message",
				"from_user_id", strings.TrimSpace(message.FromUserID),
				"to_user_id", strings.TrimSpace(message.ToUserID),
				"message_type", message.MessageType,
				"message_state", message.MessageState,
			)
			reply := b.handleIncomingMessageReply(ctx, message)
			reply.Text = trimWeixinReply(reply.Text)
			ttsSettings, ttsErr := b.resolveVoiceReplySettings(reply)
			if ttsErr != nil {
				if reply.VoiceResolveFailureNotice != "" {
					reply.Text = trimWeixinReply(appendWeixinReplyNotice(reply.Text, reply.VoiceResolveFailureNotice))
				}
				b.logger.Warn("resolve weixin voice reply settings failed", "error", ttsErr)
			}
			previewPath, previewErr := b.selectedFigurePreviewPath(message, reply)
			if previewErr != nil {
				reply.Text = trimWeixinReply(appendWeixinReplyNotice(reply.Text, "图片已选中，但原图预览不可用。"))
				b.logger.Warn("resolve weixin figure preview failed", "error", previewErr)
			}
			if reply.Text == "" && previewPath == "" && ttsSettings == nil {
				b.logger.Info("skip empty weixin reply", "from_user_id", strings.TrimSpace(message.FromUserID))
				continue
			}
			if reply.Text != "" {
				if err := sendWeixinTextReply(ctx, client, message.FromUserID, reply.Text, message.ContextToken); err != nil {
					b.logger.Warn("send weixin reply failed", "error", err)
				}
			}
			if ttsSettings != nil && reply.VoicePendingNotice != "" {
				if err := sendWeixinTextReply(ctx, client, message.FromUserID, reply.VoicePendingNotice, message.ContextToken); err != nil {
					b.logger.Warn("send weixin voice pending notice failed", "error", err)
				}
			}
			voicePath := ""
			cleanupVoice := func() {}
			if ttsSettings != nil {
				var voiceErr error
				voicePath, cleanupVoice, voiceErr = b.resolveVoiceReplyWithSettings(ctx, message, reply, *ttsSettings)
				if cleanupVoice == nil {
					cleanupVoice = func() {}
				}
				if voiceErr != nil {
					if reply.VoiceResolveFailureNotice != "" {
						if err := sendWeixinTextReply(ctx, client, message.FromUserID, reply.VoiceResolveFailureNotice, message.ContextToken); err != nil {
							b.logger.Warn("send weixin voice resolve failure notice failed", "error", err)
						}
					}
					b.logger.Warn("resolve weixin voice reply failed", "error", voiceErr)
				}
			}
			if voicePath != "" {
				if err := client.SendFileAttachment(ctx, message.FromUserID, voicePath, message.ContextToken); err != nil {
					b.logger.Warn("send weixin voice file failed", "error", err, "path", voicePath)
					if reply.VoiceSendFailureNotice != "" {
						if err := sendWeixinTextReply(ctx, client, message.FromUserID, reply.VoiceSendFailureNotice, message.ContextToken); err != nil {
							b.logger.Warn("send weixin voice file failure notice failed", "error", err)
						}
					}
				}
			}
			if previewPath != "" {
				if err := client.SendImageFile(ctx, message.FromUserID, previewPath, message.ContextToken); err != nil {
					b.logger.Warn("send weixin preview image failed", "error", err, "path", previewPath)
					if err := sendWeixinTextReply(ctx, client, message.FromUserID, "图片已选中，但预览发送失败。", message.ContextToken); err != nil {
						b.logger.Warn("send weixin preview failure notice failed", "error", err)
					}
				}
			}
			cleanupVoice()
		}
	}
}

func (b *WeixinIMBridge) handleIncomingMessage(ctx context.Context, message weixin.Message) string {
	return b.handleIncomingMessageReply(ctx, message).Text
}

func (b *WeixinIMBridge) handleIncomingMessageReply(ctx context.Context, message weixin.Message) weixinReplyEnvelope {
	if reply, handled := b.handleIncomingFile(ctx, message); handled {
		return weixinReplyEnvelope{Text: reply}
	}

	text := extractWeixinText(message)
	if text == "" {
		return weixinReplyEnvelope{}
	}
	return b.handleIncomingTextReply(ctx, text)
}

func (b *WeixinIMBridge) handleIncomingText(ctx context.Context, text string) string {
	return b.handleIncomingTextReply(ctx, text).Text
}

func (b *WeixinIMBridge) handleIncomingTextReply(ctx context.Context, text string) weixinReplyEnvelope {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "／") {
		text = "/" + strings.TrimPrefix(text, "／")
	}
	if text == "" {
		return weixinReplyEnvelope{}
	}

	// Bridge-local commands stay here so we don't pay an LLM round-trip for a
	// pure settings toggle and so /testvoice can attach a TTS envelope.
	switch strings.ToLower(strings.SplitN(text, " ", 2)[0]) {
	case "/voiceon":
		return b.setWeixinVoiceOutputEnabledReply(true)
	case "/voiceoff":
		return b.setWeixinVoiceOutputEnabledReply(false)
	case "/testvoice":
		return b.buildWeixinTestVoiceReply()
	}

	if b.agentSession == nil {
		return weixinReplyEnvelope{Text: weixinHelpText()}
	}

	// Map historical bridge aliases to the names the agent_session registry
	// knows. Slash commands the registry doesn't recognize fall through to its
	// "unknown command" error which the envelope renders verbatim.
	dispatchText := rewriteBridgeAliases(text)

	var sc agent_session.SurfaceContext
	if b.surfaceState != nil {
		raw, _ := b.surfaceState.Get("wechat")
		sc = agent_session.SurfaceContext{
			CurrentPaperID:        raw.CurrentPaperID,
			CurrentPaperTitle:     raw.CurrentPaperTitle,
			CurrentFigureID:       raw.CurrentFigureID,
			RecentSearchPaperIDs:  raw.RecentSearchPaperIDs,
			RecentSearchFigureIDs: raw.RecentSearchFigureIDs,
		}
	}

	req := agent_session.AgentRequest{
		UserID:         "default",
		Surface:        agent_session.SurfaceWeChat,
		Conversation:   agent_session.ConversationRef{Kind: agent_session.KindMainWeChat},
		SurfaceContext: sc,
		Input:          agent_session.Input{Text: dispatchText},
	}
	resp, err := b.agentSession.Handle(ctx, req)
	if err != nil {
		return weixinReplyEnvelope{Text: fmt.Sprintf("处理失败：%v", err)}
	}

	return chunksToWeixinEnvelope(resp.Chunks)
}

// rewriteBridgeAliases maps the WeChat-only command shortcuts onto the
// canonical agent_session command names. /h → /help, /qa → /ask, and the
// search target variants collapse onto /search (the unified /search command no
// longer distinguishes paper-only vs figure-only — the underlying library
// search returns papers, and figure-search shortcuts route via /random or
// /figures from a selected paper).
func rewriteBridgeAliases(text string) string {
	if !strings.HasPrefix(text, "/") {
		return text
	}
	parts := strings.SplitN(text, " ", 2)
	cmd := strings.ToLower(parts[0])
	switch cmd {
	case "/h":
		cmd = "/help"
	case "/qa":
		cmd = "/ask"
	case "/search-papers", "/search-figures":
		cmd = "/search"
	default:
		return text
	}
	if len(parts) == 2 {
		return cmd + " " + parts[1]
	}
	return cmd
}

// chunksToWeixinEnvelope folds the agent_session response chunks into the
// envelope shape the bridge's outbound send loop already understands. Text
// chunks are joined with blank-line separators; any image chunk flips the
// PreviewCurrentFigure flag so the existing send loop loads the preview from
// the surface state's currently-selected figure id. Placeholder text chunks
// are dropped because SendForSurface is currently synchronous — there's no
// streaming break between placeholder and final text on this surface.
func chunksToWeixinEnvelope(chunks []agent_session.OutboundChunk) weixinReplyEnvelope {
	env := weixinReplyEnvelope{}
	var lines []string
	for _, c := range chunks {
		switch c.Kind {
		case agent_session.ChunkText:
			if c.IsPlaceholder {
				continue
			}
			if strings.TrimSpace(c.Text) != "" {
				lines = append(lines, c.Text)
			}
		case agent_session.ChunkImage:
			env.PreviewCurrentFigure = true
		}
	}
	if len(lines) > 0 {
		env.Text = strings.Join(lines, "\n\n")
	}
	return env
}

func (b *WeixinIMBridge) handleIncomingFile(ctx context.Context, message weixin.Message) (string, bool) {
	for _, item := range message.ItemList {
		if item.Type != weixin.ItemTypeFile || item.FileItem == nil {
			continue
		}
		return b.importPDFFile(ctx, item), true
	}
	return "", false
}

func (b *WeixinIMBridge) importPDFFile(ctx context.Context, item weixin.MessageItem) string {
	fileItem := item.FileItem
	filename := strings.TrimSpace(fileItem.FileName)
	if filename == "" {
		filename = "wechat-upload.bin"
	}

	if size, ok := parseWeixinFileSize(fileItem.Len); ok && size > b.libraryService.config.MaxUploadSize {
		return fmt.Sprintf("PDF 大小超过限制 %s。", humanFileSize(b.libraryService.config.MaxUploadSize))
	}

	downloaded, err := b.downloadFile(ctx, item)
	if err != nil {
		return fmt.Sprintf("下载微信文件失败：%v", err)
	}
	if downloaded == nil || len(downloaded.Data) == 0 {
		return "微信文件为空，无法导入。"
	}

	filename = firstNonEmpty(strings.TrimSpace(downloaded.Filename), filename)
	contentType := detectWeixinFileContentType(filename, downloaded.ContentType, downloaded.Data)
	if !isPDF(filename, contentType) {
		return fmt.Sprintf("暂不支持导入 `%s`，目前只支持 PDF。", clipRunes(filename, 72))
	}
	if !strings.EqualFold(filepath.Ext(filename), ".pdf") {
		ext := filepath.Ext(filename)
		if ext != "" {
			filename = strings.TrimSuffix(filename, ext)
		}
		filename += ".pdf"
	}

	file := &weixinMultipartFile{Reader: bytes.NewReader(downloaded.Data)}
	header := &multipart.FileHeader{
		Filename: filename,
		Size:     int64(len(downloaded.Data)),
		Header: textproto.MIMEHeader{
			"Content-Type": []string{contentType},
		},
	}

	paper, err := b.libraryService.UploadPaper(file, header, UploadPaperParams{})
	if err != nil {
		var duplicateErr *DuplicatePaperError
		if errors.As(err, &duplicateErr) && duplicateErr.Paper != nil {
			b.activatePaperContext(duplicateErr.Paper.ID, true)
			return "该 PDF 已在文献库中，已切换到现有文献。\n\n" + b.formatPaperSelection(duplicateErr.Paper, false)
		}
		return fmt.Sprintf("导入微信 PDF 失败：%v", err)
	}

	b.activatePaperContext(paper.ID, true)

	prefix := "已从微信导入 PDF。"
	if paper.ExtractionStatus == "queued" {
		prefix = "已从微信导入 PDF，正在后台解析。"
	}

	return prefix + "\n\n" + b.formatPaperSelection(paper, false)
}

func (b *WeixinIMBridge) formatPaperSelection(paper *model.Paper, autoSelected bool) string {
	if paper == nil {
		return "文献不存在。"
	}

	prefix := "已选中文献"
	if autoSelected {
		prefix = "已自动选中文献"
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("%s [ID %d] %s", prefix, paper.ID, clipRunes(strings.TrimSpace(paper.Title), 72)))
	lines = append(lines, fmt.Sprintf("状态：%s | 图片：%d 张", paper.ExtractionStatus, len(topLevelFigures(paper.Figures))))

	summary := firstNonEmpty(strings.TrimSpace(paper.AbstractText), strings.TrimSpace(paper.PaperNotesText), strings.TrimSpace(paper.NotesText))
	if summary != "" {
		lines = append(lines, clipRunes(summary, 180))
	}

	lines = append(lines, "", "现在可以发送 `/ask 问题` 提问，发送 `/figures` 查看图片列表，或 `/note 你的内容` 追加文献笔记。")
	return strings.Join(lines, "\n")
}

func (b *WeixinIMBridge) activatePaperContext(paperID int64, clearSearch bool) {
	b.updateContext(func(state *weixinIMContext) {
		if clearSearch {
			state.SearchPaperIDs = nil
			state.SearchFigureIDs = nil
		}
		state.CurrentPaperID = paperID
		state.CurrentFigureID = 0
		state.QAHistory = nil
	})
	// Mirror into the agent_session surface state so subsequent /ask, /note,
	// /figures, /interpret commands (which read from SurfaceStateStore, not
	// the legacy weixinIMContext) see the just-imported paper. Resolve the
	// title best-effort; the empty fallback is fine.
	if b.surfaceState != nil {
		title := ""
		if paper, err := b.libraryService.GetPaper(paperID); err == nil && paper != nil {
			title = paper.Title
		}
		_ = b.surfaceState.SetCurrentPaper("wechat", paperID, title)
		_ = b.surfaceState.SetCurrentFigure("wechat", 0)
		if clearSearch {
			_ = b.surfaceState.SetSearchResults("wechat", nil, nil)
		}
	}
}

func (b *WeixinIMBridge) requireCurrentPaper() (*model.Paper, string) {
	state := b.getContext()
	if state.CurrentPaperID == 0 {
		return nil, "请先发送 `/search 自然语言检索内容`、`/search-papers ...` 或 `/recent` 选择文献。"
	}

	paper, err := b.libraryService.GetPaper(state.CurrentPaperID)
	if err != nil {
		b.setContext(weixinIMContext{})
		return nil, "当前文献已失效，请重新发送 `/search 自然语言检索内容`。"
	}
	return paper, ""
}

func (b *WeixinIMBridge) requireCurrentFigure() (*model.Paper, *model.Figure, string) {
	paper, errText := b.requireCurrentPaper()
	if errText != "" {
		return nil, nil, errText
	}

	figureID := b.getContext().CurrentFigureID
	if figureID == 0 {
		return nil, nil, "请先发送 `/figures` 查看列表，再用 `/figure 序号` 选中目标图片。"
	}

	figure := findFigureByID(paper.Figures, figureID)
	if figure == nil {
		b.updateContext(func(state *weixinIMContext) {
			state.CurrentFigureID = 0
		})
		return nil, nil, "当前图片已失效，请重新发送 `/figures` 查看列表。"
	}
	return paper, figure, ""
}

func (b *WeixinIMBridge) getContext() weixinIMContext {
	b.mu.Lock()
	defer b.mu.Unlock()

	context := b.context
	context.SearchPaperIDs = append([]int64(nil), context.SearchPaperIDs...)
	context.SearchFigureIDs = append([]int64(nil), context.SearchFigureIDs...)
	context.QAHistory = append([]model.AIConversationTurn(nil), context.QAHistory...)
	return context
}

func (b *WeixinIMBridge) setContext(next weixinIMContext) {
	next.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	b.mu.Lock()
	b.context = next
	b.mu.Unlock()

	if err := b.persistContext(next); err != nil {
		b.logger.Warn("save weixin IM context failed", "error", err)
	}
}

func (b *WeixinIMBridge) updateContext(mutator func(*weixinIMContext)) {
	b.mu.Lock()
	next := b.context
	mutator(&next)
	next.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	b.context = next
	b.mu.Unlock()

	if err := b.persistContext(next); err != nil {
		b.logger.Warn("save weixin IM context failed", "error", err)
	}
}

func (b *WeixinIMBridge) loadContext() {
	data, err := os.ReadFile(b.contextPath)
	if err != nil {
		return
	}

	var state weixinIMContext
	if err := json.Unmarshal(data, &state); err != nil {
		b.logger.Warn("load weixin IM context failed", "error", err)
		return
	}

	b.mu.Lock()
	b.context = state
	b.mu.Unlock()
}

func (b *WeixinIMBridge) persistContext(state weixinIMContext) error {
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomicFile(b.contextPath, payload)
}

func (b *WeixinIMBridge) loadSyncBuf() string {
	data, err := os.ReadFile(b.syncBufPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (b *WeixinIMBridge) selectedFigurePreviewPath(message weixin.Message, reply weixinReplyEnvelope) (string, error) {
	// chunksToWeixinEnvelope flips PreviewCurrentFigure whenever the
	// agent_session response carries a figure chunk; that's the canonical
	// signal that we should attach a preview. We deliberately ignore the
	// legacy text-prefix sniffing path — agent_session command wording has
	// diverged from the old "已选中图片 [ID ..." strings.
	if !reply.PreviewCurrentFigure {
		return "", nil
	}

	figureID := int64(0)
	if b.surfaceState != nil {
		if sc, err := b.surfaceState.Get("wechat"); err == nil {
			figureID = sc.CurrentFigureID
		}
	}
	if figureID == 0 {
		return "", nil
	}

	figure, err := b.libraryService.repo.GetFigure(figureID)
	if err != nil || figure == nil {
		return "", nil
	}
	return b.figureListItemPreviewPath(figure)
}

func (b *WeixinIMBridge) figureListItemPreviewPath(figure *model.FigureListItem) (string, error) {
	if figure == nil {
		return "", errors.New("figure is nil")
	}
	filename := filepath.Base(strings.TrimSpace(figure.Filename))
	if filename == "" {
		return "", errors.New("figure filename is empty")
	}
	targetPath := filepath.Join(b.libraryService.config.FiguresDir(), filename)
	if _, err := os.Stat(targetPath); err != nil {
		return "", err
	}
	return targetPath, nil
}

func (b *WeixinIMBridge) resolveVoiceReply(ctx context.Context, message weixin.Message, reply weixinReplyEnvelope) (string, func(), error) {
	settings, err := b.resolveVoiceReplySettings(reply)
	if err != nil || settings == nil {
		return "", func() {}, err
	}
	return b.resolveVoiceReplyWithSettings(ctx, message, reply, *settings)
}

func (b *WeixinIMBridge) resolveVoiceReplySettings(reply weixinReplyEnvelope) (*model.TTSSettings, error) {
	if strings.TrimSpace(reply.TTSText) == "" {
		return nil, nil
	}

	settings, err := b.libraryService.GetTTSSettings()
	if err != nil {
		return nil, err
	}
	if settings == nil {
		if reply.RequireTTS {
			return nil, errors.New("weixin tts settings are missing")
		}
		return nil, nil
	}
	if err := validateTTSSettings(*settings); err != nil {
		if reply.RequireTTS {
			return nil, err
		}
		return nil, nil
	}
	if !settings.WeixinVoiceOutputEnabled {
		return nil, nil
	}
	if b.synthesizeTTS == nil {
		return nil, errors.New("weixin tts synthesizer is nil")
	}
	return settings, nil
}

func (b *WeixinIMBridge) buildWeixinTestVoiceReply() weixinReplyEnvelope {
	settings, err := b.libraryService.GetTTSSettings()
	if err != nil {
		return weixinReplyEnvelope{Text: fmt.Sprintf("读取 TTS 配置失败：%v", err)}
	}
	if settings != nil && !settings.WeixinVoiceOutputEnabled {
		return weixinReplyEnvelope{Text: "微信 TTS 语音输出当前已关闭。发送 `/voiceon` 重新开启后，再试 `/testvoice`。"}
	}
	return weixinReplyEnvelope{
		Text:                      fmt.Sprintf("测试语音：%s", ttsTestDemoText),
		TTSText:                   ttsTestDemoText,
		RequireTTS:                true,
		VoiceResolveFailureNotice: "测试语音生成失败，请先保存可用的 TTS 配置。",
		VoiceSendFailureNotice:    "测试语音发送失败。",
	}
}

func (b *WeixinIMBridge) setWeixinVoiceOutputEnabledReply(enabled bool) weixinReplyEnvelope {
	settings, err := b.libraryService.SetWeixinVoiceOutputEnabled(enabled)
	if err != nil {
		return weixinReplyEnvelope{Text: fmt.Sprintf("更新微信 TTS 语音输出失败：%v", err)}
	}

	if enabled {
		if err := validateTTSSettings(*settings); err != nil {
			return weixinReplyEnvelope{Text: "已开启微信 TTS 语音输出。当前还没有可用的 TTS 配置，请先在设置页保存。"}
		}
		return weixinReplyEnvelope{Text: "已开启微信 TTS 语音输出。`/ask`、`/qa` 和 `/testvoice` 会附带语音。"}
	}

	return weixinReplyEnvelope{Text: "已关闭微信 TTS 语音输出。`/ask`、`/qa` 和 `/testvoice` 将只返回文字。"}
}

func (b *WeixinIMBridge) resolveVoiceReplyWithSettings(ctx context.Context, message weixin.Message, reply weixinReplyEnvelope, settings model.TTSSettings) (string, func(), error) {
	ttsText := strings.TrimSpace(reply.TTSText)
	if ttsText == "" {
		return "", func() {}, nil
	}

	fallbackText := sanitizeMarkdownForTTS(ttsText)
	if fallbackText == "" {
		fallbackText = ttsText
	}
	if reply.OptimizeTTSText && b.aiService != nil {
		rewritten, err := b.aiService.RewriteTextForTTS(ctx, ttsText)
		if err != nil {
			b.logger.Warn("rewrite weixin tts text failed, fallback to sanitized original", "error", err)
			ttsText = fallbackText
		} else if normalized := normalizeTTSReadbackText(rewritten); normalized != "" {
			ttsText = normalized
		} else {
			ttsText = fallbackText
		}
	} else {
		ttsText = fallbackText
	}
	return b.synthesizeTTS(ctx, ttsText, strings.TrimSpace(message.FromUserID), settings)
}

func (b *WeixinIMBridge) synthesizeReplyVoice(ctx context.Context, text, uid string, settings model.TTSSettings) (string, func(), error) {
	return synthesizeDoubaoTTSFile(ctx, b.stateDir, text, uid, newDoubaoTTSSettings(settings))
}

func (b *WeixinIMBridge) figurePreviewPath(figure *model.Figure) (string, error) {
	if figure == nil {
		return "", errors.New("figure is nil")
	}

	filename := filepath.Base(strings.TrimSpace(figure.Filename))
	if filename == "" {
		return "", errors.New("figure filename is empty")
	}

	targetPath := filepath.Join(b.libraryService.config.FiguresDir(), filename)
	if _, err := os.Stat(targetPath); err != nil {
		return "", err
	}
	return targetPath, nil
}

func shouldHandleWeixinMessage(binding weixinBindingRecord, message weixin.Message) (bool, string) {
	fromUserID := strings.TrimSpace(message.FromUserID)
	toUserID := strings.TrimSpace(message.ToUserID)
	boundUserID := strings.TrimSpace(binding.UserID)
	accountID := strings.TrimSpace(binding.AccountID)

	switch {
	case strings.TrimSpace(message.GroupID) != "":
		return false, "group_message"
	case fromUserID == "":
		return false, "missing_sender"
	case accountID != "" && fromUserID == accountID:
		return false, "bot_echo"
	case boundUserID != "" && fromUserID != boundUserID:
		return false, "unexpected_sender"
	case accountID != "" && toUserID != "" && toUserID != accountID:
		return false, "unexpected_recipient"
	case boundUserID == "" && accountID == "" && message.MessageType != weixin.MessageTypeUser:
		return false, "unknown_sender_type"
	default:
		return true, ""
	}
}

func extractWeixinText(message weixin.Message) string {
	for _, item := range message.ItemList {
		if item.Type == weixin.ItemTypeText && item.TextItem != nil {
			return strings.TrimSpace(item.TextItem.Text)
		}
		if item.Type == weixin.ItemTypeVoice && item.VoiceItem != nil && strings.TrimSpace(item.VoiceItem.Text) != "" {
			return strings.TrimSpace(item.VoiceItem.Text)
		}
	}
	return ""
}

func detectWeixinFileContentType(filename, reportedContentType string, data []byte) string {
	reportedContentType = strings.TrimSpace(reportedContentType)
	if isPDF(filename, reportedContentType) {
		if strings.Contains(strings.ToLower(reportedContentType), "pdf") {
			return reportedContentType
		}
		return "application/pdf"
	}
	if len(data) > 0 {
		sample := data
		if len(sample) > 512 {
			sample = sample[:512]
		}
		sniffed := http.DetectContentType(sample)
		if isPDF(filename, sniffed) {
			return "application/pdf"
		}
	}
	if reportedContentType != "" {
		return reportedContentType
	}
	return "application/octet-stream"
}

func parseWeixinFileSize(value string) (int64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	size, err := strconv.ParseInt(value, 10, 64)
	if err != nil || size < 0 {
		return 0, false
	}
	return size, true
}

func weixinHelpText() string {
	return strings.Join([]string{
		"微信 IM 优先响应 slash 命令；普通文字会先通过 LLM 识别成最合适的 slash 操作。可用命令：",
		"`/search 自然语言`：自动理解意图，拆成约 5 个关键词后搜索文献或图片，并返回最可能的 1-3 条",
		"`/search-papers 自然语言`：强制只搜文献",
		"`/search-figures 自然语言`：强制只搜图片",
		"`/recent`：查看最近几篇文献",
		"`/paper 1`：选择检索结果中的文献；普通文字如“看看第三篇文献”也会优先路由到这里",
		"`/figures`：查看当前文献的图片列表",
		"`/figure 1`：选择检索结果中的图片或当前文献中的图片；普通文字如“看看第二张图”也会优先路由到这里，并回发原图预览",
		"`/random`：随机选中一张图片，并回发原图预览",
		"直接发送 PDF：自动导入文献并切换上下文",
		"直接发送 DOI 或 DOI 链接：自动尝试导入可下载的 Open Access PDF",
		"`/ask 问题` 或 `/qa 问题`：对当前文献提问",
		"`/note 内容`：追加文献/图片笔记",
		"`/interpret 问题`：解读当前图片",
		"`/testvoice`：回一条测试文本，并追加一段基于当前 TTS 配置的 Hello World 语音",
		"`/voiceoff`：关闭微信 TTS 语音输出，后续 `/ask`、`/qa`、`/testvoice` 只返回文字",
		"`/voiceon`：重新开启微信 TTS 语音输出",
		"`/status`：查看当前上下文",
		"`/reset`：清空当前上下文",
		"`/help`：查看帮助",
		"如果普通文字也无法可靠识别，才会返回这份帮助，避免误触发 AI 问答。",
	}, "\n")
}

func parseWeixinSlashCommand(text string) (string, string, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || !strings.HasPrefix(trimmed, "/") {
		return "", "", false
	}

	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return "", "", false
	}

	command := strings.ToLower(fields[0])
	if len(fields) == 1 {
		return command, "", true
	}
	return command, strings.TrimSpace(strings.TrimPrefix(trimmed, fields[0])), true
}

func trimWeixinReply(text string) string {
	return strings.TrimSpace(text)
}

func appendWeixinReplyNotice(text, notice string) string {
	text = strings.TrimSpace(text)
	notice = strings.TrimSpace(notice)
	switch {
	case text == "":
		return notice
	case notice == "":
		return text
	default:
		return text + "\n\n" + notice
	}
}

func splitWeixinReplyText(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	units := splitWeixinReplyUnits(text)
	return packWeixinReplyUnits(units, weixinReplyChunkMaxRunes)
}

func splitWeixinReplyUnits(text string) []string {
	runes := []rune(text)

	chunks := make([]string, 0, strings.Count(text, "\n")+1)
	start := 0
	for index := 0; index < len(runes); index++ {
		split := 0
		switch {
		case matchWeixinReplyParagraphBreak(runes, index) > 0:
			split = matchWeixinReplyParagraphBreak(runes, index)
		case matchWeixinReplyLineBreak(runes, index) > 0:
			split = matchWeixinReplyLineBreak(runes, index)
		case matchWeixinReplySentenceBreak(runes, index) > 0:
			split = matchWeixinReplySentenceBreak(runes, index)
		}
		if split > start {
			chunks = append(chunks, string(runes[start:split]))
			start = split
		}
	}
	if start < len(runes) {
		chunks = append(chunks, string(runes[start:]))
	}
	return chunks
}

func packWeixinReplyUnits(units []string, maxRunes int) []string {
	if len(units) == 0 {
		return nil
	}
	if maxRunes <= 0 {
		return append([]string(nil), units...)
	}

	chunks := make([]string, 0, len(units))
	var current strings.Builder
	currentRunes := 0
	flush := func() {
		if currentRunes == 0 {
			return
		}
		chunks = append(chunks, current.String())
		current.Reset()
		currentRunes = 0
	}

	for _, unit := range units {
		for _, part := range splitWeixinReplyOversizedUnit(unit, maxRunes) {
			partRunes := len([]rune(part))
			if currentRunes > 0 && currentRunes+partRunes > maxRunes {
				flush()
			}
			current.WriteString(part)
			currentRunes += partRunes
		}
	}

	flush()
	return chunks
}

func splitWeixinReplyOversizedUnit(text string, maxRunes int) []string {
	if maxRunes <= 0 {
		return []string{text}
	}

	runes := []rune(text)
	if len(runes) <= maxRunes {
		return []string{text}
	}

	matchers := []func([]rune, int) int{
		matchWeixinReplyParagraphBreak,
		matchWeixinReplyLineBreak,
		matchWeixinReplySentenceBreak,
		matchWeixinReplyClauseBreak,
		matchWeixinReplyWhitespaceBreak,
	}

	chunks := make([]string, 0, (len(runes)+maxRunes-1)/maxRunes)
	for start := 0; start < len(runes); {
		end := findWeixinReplySplitIndex(runes, start, maxRunes, matchers)
		if end <= start {
			end = minInt(start+maxRunes, len(runes))
		}
		chunks = append(chunks, string(runes[start:end]))
		start = end
	}
	return chunks
}

func findWeixinReplySplitIndex(runes []rune, start, maxRunes int, matchers []func([]rune, int) int) int {
	if start >= len(runes) {
		return len(runes)
	}
	endLimit := minInt(start+maxRunes, len(runes))
	if endLimit >= len(runes) {
		return len(runes)
	}

	searchStart := maxInt(start, endLimit-weixinReplySplitLookback(maxRunes))
	for _, matcher := range matchers {
		for index := endLimit - 1; index >= searchStart; index-- {
			if split := matcher(runes, index); split > start {
				return minInt(split, endLimit)
			}
		}
	}
	return 0
}

func weixinReplySplitLookback(maxRunes int) int {
	if maxRunes <= 1 {
		return 1
	}
	lookback := (maxRunes * 2) / 3
	if lookback <= 0 {
		return 1
	}
	return lookback
}

func matchWeixinReplyParagraphBreak(runes []rune, index int) int {
	if index <= 0 || index >= len(runes) {
		return 0
	}
	if runes[index-1] == '\n' && runes[index] == '\n' {
		return index + 1
	}
	return 0
}

func matchWeixinReplyLineBreak(runes []rune, index int) int {
	if index < 0 || index >= len(runes) || runes[index] != '\n' {
		return 0
	}
	return index + 1
}

func matchWeixinReplySentenceBreak(runes []rune, index int) int {
	if index < 0 || index >= len(runes) {
		return 0
	}
	if isWeixinReplySentenceBoundaryRune(runes[index]) {
		return extendWeixinReplySplitIndex(runes, index+1)
	}
	if isWeixinReplyClosingRune(runes[index]) && index > 0 && isWeixinReplySentenceBoundaryRune(runes[index-1]) {
		return extendWeixinReplySplitIndex(runes, index+1)
	}
	return 0
}

func matchWeixinReplyClauseBreak(runes []rune, index int) int {
	if index < 0 || index >= len(runes) {
		return 0
	}
	if isWeixinReplyClauseBoundaryRune(runes[index]) {
		return extendWeixinReplySplitIndex(runes, index+1)
	}
	if isWeixinReplyClosingRune(runes[index]) && index > 0 && isWeixinReplyClauseBoundaryRune(runes[index-1]) {
		return extendWeixinReplySplitIndex(runes, index+1)
	}
	return 0
}

func matchWeixinReplyWhitespaceBreak(runes []rune, index int) int {
	if index < 0 || index >= len(runes) || !isWeixinReplyWhitespaceRune(runes[index]) {
		return 0
	}
	return extendWeixinReplySplitIndex(runes, index+1)
}

func extendWeixinReplySplitIndex(runes []rune, index int) int {
	for index < len(runes) {
		if isWeixinReplyClosingRune(runes[index]) || isWeixinReplyWhitespaceRune(runes[index]) {
			index++
			continue
		}
		break
	}
	return index
}

func isWeixinReplySentenceBoundaryRune(r rune) bool {
	switch r {
	case '。', '！', '？', '!', '?', ';', '；':
		return true
	default:
		return false
	}
}

func isWeixinReplyClauseBoundaryRune(r rune) bool {
	switch r {
	case '，', '、', ',', '：', ':':
		return true
	default:
		return false
	}
}

func isWeixinReplyClosingRune(r rune) bool {
	switch r {
	case '"', '\'', ')', ']', '}', '》', '」', '』', '】', '）':
		return true
	default:
		return false
	}
}

func isWeixinReplyWhitespaceRune(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r':
		return true
	default:
		return false
	}
}

func sendWeixinTextReply(ctx context.Context, client *weixin.Client, toUserID, text, contextToken string) error {
	chunks := splitWeixinReplyText(text)
	for _, chunk := range chunks {
		if err := client.SendTextMessage(ctx, toUserID, chunk, contextToken); err != nil {
			return err
		}
	}
	return nil
}

func clipRunes(text string, max int) string {
	text = strings.TrimSpace(text)
	if max <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

func tagNamesFromPaper(tags []model.Tag) []string {
	if len(tags) == 0 {
		return nil
	}
	names := make([]string, 0, len(tags))
	for _, tag := range tags {
		if trimmed := strings.TrimSpace(tag.Name); trimmed != "" {
			names = append(names, trimmed)
		}
	}
	return names
}

func findFigureByID(figures []model.Figure, figureID int64) *model.Figure {
	for index := range figures {
		if figures[index].ID == figureID {
			return &figures[index]
		}
	}
	return nil
}

func sameWeixinBinding(left, right weixinBindingRecord) bool {
	return strings.TrimSpace(left.Token) == strings.TrimSpace(right.Token) &&
		strings.TrimSpace(left.BaseURL) == strings.TrimSpace(right.BaseURL) &&
		strings.TrimSpace(left.UserID) == strings.TrimSpace(right.UserID) &&
		strings.TrimSpace(left.AccountID) == strings.TrimSpace(right.AccountID)
}

func sleepContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

type weixinMultipartFile struct {
	*bytes.Reader
}

func (f *weixinMultipartFile) Close() error {
	return nil
}

func writeAtomicFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

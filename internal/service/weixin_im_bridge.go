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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/weixin"
)

const (
	weixinBridgeStateDirName            = "weixin-bridge"
	weixinSyncBufFileName               = "sync_buf"
	weixinContextFileName               = "im_context.json"
	weixinReplyMaxRunes                 = 3200
	weixinReplyChunkRunes               = 1000
	weixinHistoryLimit                  = 6
	weixinSearchKeywordLimit            = 6
	weixinSearchKeywordPerLanguageLimit = 3
	weixinSearchResultLimit             = 3
	weixinSearchProbeLimit              = 3
	weixinSearchReviewLimit             = 6
)

const (
	weixinSearchTargetPaper  = "paper"
	weixinSearchTargetFigure = "figure"
)

type weixinAIReader interface {
	ReadPaper(ctx context.Context, input model.AIReadRequest) (*model.AIReadResponse, error)
	PlanWeixinCommand(ctx context.Context, text string, context weixinIntentContext) (*weixinCommandPlan, error)
	PlanWeixinSearch(ctx context.Context, query, forcedTarget string) (*weixinSearchPlan, error)
	ReviewWeixinPaperSearch(ctx context.Context, query string, keywords []string, candidates []model.Paper) (*weixinSearchReview, error)
	ReviewWeixinFigureSearch(ctx context.Context, query string, keywords []string, candidates []model.FigureListItem) (*weixinSearchReview, error)
	RewriteTextForTTS(ctx context.Context, text string) (string, error)
}

type weixinIMContext struct {
	CurrentPaperID  int64                      `json:"current_paper_id"`
	CurrentFigureID int64                      `json:"current_figure_id"`
	SearchPaperIDs  []int64                    `json:"search_paper_ids,omitempty"`
	SearchFigureIDs []int64                    `json:"search_figure_ids,omitempty"`
	QAHistory       []model.AIConversationTurn `json:"qa_history,omitempty"`
	UpdatedAt       string                     `json:"updated_at,omitempty"`
}

type weixinSearchPlan struct {
	Target     string   `json:"target"`
	Keywords   []string `json:"keywords"`
	KeywordsZH []string `json:"keywords_zh,omitempty"`
	KeywordsEN []string `json:"keywords_en,omitempty"`
}

type weixinIntentContext struct {
	CurrentPaperID    int64  `json:"current_paper_id,omitempty"`
	CurrentPaperTitle string `json:"current_paper_title,omitempty"`
	CurrentFigureID   int64  `json:"current_figure_id,omitempty"`
	SearchPaperCount  int    `json:"search_paper_count,omitempty"`
	SearchFigureCount int    `json:"search_figure_count,omitempty"`
}

type weixinCommandPlan struct {
	Command string `json:"command"`
	Arg     string `json:"arg,omitempty"`
}

type weixinReplyEnvelope struct {
	Text                      string
	TTSText                   string
	OptimizeTTSText           bool
	RequireTTS                bool
	VoicePendingNotice        string
	VoiceResolveFailureNotice string
	VoiceSendFailureNotice    string
}

type weixinPaperSearchCandidate struct {
	Paper model.Paper
	Score int
}

type weixinFigureSearchCandidate struct {
	Figure model.FigureListItem
	Score  int
}

type weixinSearchReview struct {
	Summary     string  `json:"summary"`
	SelectedIDs []int64 `json:"selected_ids"`
}

type WeixinIMBridge struct {
	libraryService *LibraryService
	aiService      weixinAIReader
	logger         *slog.Logger
	downloadFile   func(context.Context, weixin.MessageItem) (*weixin.DownloadedFile, error)
	synthesizeTTS  func(context.Context, string, string, model.TTSSettings) (string, func(), error)
	stateDir       string
	syncBufPath    string
	contextPath    string

	mu      sync.Mutex
	context weixinIMContext
}

func NewWeixinIMBridge(libraryService *LibraryService, aiService weixinAIReader, logger *slog.Logger, storageDir string) *WeixinIMBridge {
	if logger == nil {
		logger = slog.Default()
	}

	bridge := &WeixinIMBridge{
		libraryService: libraryService,
		aiService:      aiService,
		logger:         logger.With("component", "weixin_im_bridge"),
		downloadFile: func(ctx context.Context, item weixin.MessageItem) (*weixin.DownloadedFile, error) {
			return weixin.DownloadFileItem(ctx, item, nil, "")
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

		client := weixin.NewClient(binding.BaseURL, binding.Token, nil)
		b.logger.Info("weixin IM bridge polling", "user_id", binding.UserID)

		if err := b.runPolling(ctx, client, binding); err != nil && !errors.Is(err, context.Canceled) {
			b.logger.Warn("weixin IM bridge polling stopped", "error", err)
		}

		if !sleepContext(ctx, 3*time.Second) {
			return ctx.Err()
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
			return fmt.Errorf("weixin session expired")
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
			previewPath, previewErr := b.selectedFigurePreviewPath(message, reply.Text)
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

	command, arg, ok := parseWeixinSlashCommand(text)
	if !ok {
		planned := b.planWeixinPlainTextCommand(ctx, text)
		if planned == nil {
			return weixinReplyEnvelope{Text: weixinHelpText()}
		}
		return b.executeWeixinCommandReply(ctx, planned.Command, planned.Arg)
	}

	return b.executeWeixinCommandReply(ctx, command, arg)
}

func (b *WeixinIMBridge) planWeixinPlainTextCommand(ctx context.Context, text string) *weixinCommandPlan {
	intentContext := b.buildWeixinIntentContext()
	if b.aiService == nil {
		return nil
	}

	plan, err := b.aiService.PlanWeixinCommand(ctx, text, intentContext)
	if err != nil {
		b.logger.Warn("plan weixin plain text command failed", "text", text, "error", err)
		return nil
	}
	if plan == nil {
		return nil
	}

	normalized := normalizeWeixinPlainTextCommand(plan.Command)
	if normalized == "" {
		return nil
	}
	return &weixinCommandPlan{
		Command: normalized,
		Arg:     strings.TrimSpace(plan.Arg),
	}
}

func (b *WeixinIMBridge) buildWeixinIntentContext() weixinIntentContext {
	state := b.getContext()
	intentContext := weixinIntentContext{
		CurrentPaperID:    state.CurrentPaperID,
		CurrentFigureID:   state.CurrentFigureID,
		SearchPaperCount:  len(state.SearchPaperIDs),
		SearchFigureCount: len(state.SearchFigureIDs),
	}
	if state.CurrentPaperID > 0 {
		paper, err := b.libraryService.GetPaper(state.CurrentPaperID)
		if err == nil && paper != nil {
			intentContext.CurrentPaperTitle = clipRunes(paper.Title, 120)
		}
	}
	return intentContext
}

func (b *WeixinIMBridge) executeWeixinCommand(ctx context.Context, command, arg string) string {
	return b.executeWeixinCommandReply(ctx, command, arg).Text
}

func (b *WeixinIMBridge) executeWeixinCommandReply(ctx context.Context, command, arg string) weixinReplyEnvelope {
	switch command {
	case "/help", "/h":
		return weixinReplyEnvelope{Text: weixinHelpText()}
	case "/status":
		return weixinReplyEnvelope{Text: b.statusText()}
	case "/reset":
		b.setContext(weixinIMContext{})
		return weixinReplyEnvelope{Text: "已清空微信上下文。发送 `/search 自然语言检索内容` 或 `/recent` 开始。"}
	case "/figures":
		return weixinReplyEnvelope{Text: b.listFigures()}
	case "/recent":
		return weixinReplyEnvelope{Text: b.listRecentPapers()}
	case "/search":
		return weixinReplyEnvelope{Text: b.search(ctx, arg, "")}
	case "/search-papers":
		return weixinReplyEnvelope{Text: b.search(ctx, arg, weixinSearchTargetPaper)}
	case "/search-figures":
		return weixinReplyEnvelope{Text: b.search(ctx, arg, weixinSearchTargetFigure)}
	case "/paper":
		return weixinReplyEnvelope{Text: b.selectPaper(arg)}
	case "/figure":
		return weixinReplyEnvelope{Text: b.selectFigure(arg)}
	case "/note":
		return weixinReplyEnvelope{Text: b.appendNote(arg)}
	case "/interpret":
		return weixinReplyEnvelope{Text: b.interpretCurrentFigure(ctx, arg)}
	case "/ask", "/qa":
		return b.answerCurrentPaperReply(ctx, arg)
	case "/testvoice":
		return weixinReplyEnvelope{
			Text:                      fmt.Sprintf("测试语音：%s", ttsTestDemoText),
			TTSText:                   ttsTestDemoText,
			RequireTTS:                true,
			VoiceResolveFailureNotice: "测试语音生成失败，请先保存可用的 TTS 配置。",
			VoiceSendFailureNotice:    "测试语音发送失败。",
		}
	default:
		return weixinReplyEnvelope{Text: weixinHelpText()}
	}
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

func (b *WeixinIMBridge) search(ctx context.Context, query, forcedTarget string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		switch normalizeWeixinSearchTarget(forcedTarget) {
		case weixinSearchTargetPaper:
			return "用法：`/search-papers 自然语言检索内容`"
		case weixinSearchTargetFigure:
			return "用法：`/search-figures 自然语言检索内容`"
		default:
			return "用法：`/search 自然语言检索内容`"
		}
	}

	plan := b.planWeixinSearch(ctx, query, forcedTarget)
	switch plan.Target {
	case weixinSearchTargetFigure:
		return b.searchFigures(ctx, query, plan)
	default:
		return b.searchPapers(ctx, query, plan)
	}
}

func (b *WeixinIMBridge) planWeixinSearch(ctx context.Context, query, forcedTarget string) *weixinSearchPlan {
	normalizedForcedTarget := normalizeWeixinSearchTarget(forcedTarget)
	if b.aiService != nil {
		plan, err := b.aiService.PlanWeixinSearch(ctx, query, normalizedForcedTarget)
		if err == nil && plan != nil && normalizeWeixinSearchTarget(plan.Target) != "" && len(plan.Keywords) > 0 {
			return &weixinSearchPlan{
				Target:     normalizeWeixinSearchTarget(firstNonEmpty(normalizedForcedTarget, plan.Target)),
				KeywordsZH: normalizeWeixinSearchKeywordsForLanguage(plan.KeywordsZH, "zh"),
				KeywordsEN: normalizeWeixinSearchKeywordsForLanguage(plan.KeywordsEN, "en"),
				Keywords:   mergeWeixinSearchKeywords(plan.KeywordsZH, plan.KeywordsEN, plan.Keywords),
			}
		}
		if err != nil {
			b.logger.Warn("plan weixin search failed, fallback to heuristic search", "query", query, "forced_target", normalizedForcedTarget, "error", err)
		}
	}

	return heuristicWeixinSearchPlan(query, normalizedForcedTarget)
}

func heuristicWeixinSearchPlan(query, forcedTarget string) *weixinSearchPlan {
	cleaned := cleanupWeixinSearchQuery(query)
	target := normalizeWeixinSearchTarget(forcedTarget)
	if target == "" {
		target = inferWeixinSearchTarget(query, cleaned)
	}

	keywordsZH, keywordsEN := expandWeixinSearchKeywords(query, cleaned, target)
	keywordsZH = normalizeWeixinSearchKeywordsForLanguage(keywordsZH, "zh")
	keywordsEN = normalizeWeixinSearchKeywordsForLanguage(keywordsEN, "en")

	return &weixinSearchPlan{
		Target:     target,
		KeywordsZH: keywordsZH,
		KeywordsEN: keywordsEN,
		Keywords:   mergeWeixinSearchKeywords(keywordsZH, keywordsEN),
	}
}

func (b *WeixinIMBridge) searchPapers(ctx context.Context, query string, plan *weixinSearchPlan) string {
	keywordsZH, keywordsEN, keywords := resolveWeixinSearchPlanKeywords(query, plan)

	candidates, err := b.collectPaperSearchCandidates(keywordsZH, keywordsEN)
	if err != nil {
		return fmt.Sprintf("搜索失败：%v", err)
	}
	if len(candidates) == 0 {
		b.updateContext(func(state *weixinIMContext) {
			state.SearchPaperIDs = nil
			state.SearchFigureIDs = nil
		})
		return fmt.Sprintf("没有找到和 `%s` 相关的文献。已尝试关键词：%s", query, strings.Join(keywords, " / "))
	}

	displayCandidates, summary := b.reviewPaperSearchCandidates(ctx, query, keywords, candidates)
	ids := make([]int64, 0, len(displayCandidates))
	for _, candidate := range displayCandidates {
		ids = append(ids, candidate.Paper.ID)
	}

	if len(candidates) == 1 && len(displayCandidates) == 1 {
		paper := displayCandidates[0].Paper
		b.updateContext(func(state *weixinIMContext) {
			state.SearchPaperIDs = ids
			state.SearchFigureIDs = nil
			state.CurrentPaperID = paper.ID
			state.CurrentFigureID = 0
			state.QAHistory = nil
		})
		reply := b.formatPaperSelection(&paper, true)
		if summary != "" {
			lines := append(formatWeixinSearchKeywordLines(keywordsZH, keywordsEN), fmt.Sprintf("评估：%s", summary), "", reply)
			reply = strings.Join(lines, "\n")
		}
		return reply
	}

	b.updateContext(func(state *weixinIMContext) {
		state.SearchPaperIDs = ids
		state.SearchFigureIDs = nil
	})

	var lines []string
	lines = append(lines, formatWeixinSearchKeywordLines(keywordsZH, keywordsEN)...)
	if summary != "" {
		lines = append(lines, fmt.Sprintf("评估：%s", summary))
	}
	lines = append(lines, fmt.Sprintf("汇总后最可能的文献 %d 篇：", len(displayCandidates)))
	for index, candidate := range displayCandidates {
		paper := candidate.Paper
		lines = append(lines, fmt.Sprintf("%d. [%d] %s", index+1, paper.ID, clipRunes(strings.TrimSpace(paper.Title), 56)))
		lines = append(lines, fmt.Sprintf("   状态：%s | 图片：%d 张", paper.ExtractionStatus, paper.FigureCount))
		summaryText := firstNonEmpty(strings.TrimSpace(paper.AbstractText), strings.TrimSpace(paper.PaperNotesText), strings.TrimSpace(paper.NotesText))
		if summaryText != "" {
			lines = append(lines, fmt.Sprintf("   %s", clipRunes(summaryText, 88)))
		}
	}
	lines = append(lines, "", "发送 `/paper 1` 选中目标文献。")
	return strings.Join(lines, "\n")
}

func (b *WeixinIMBridge) searchFigures(ctx context.Context, query string, plan *weixinSearchPlan) string {
	keywordsZH, keywordsEN, keywords := resolveWeixinSearchPlanKeywords(query, plan)

	candidates, err := b.collectFigureSearchCandidates(keywordsZH, keywordsEN)
	if err != nil {
		return fmt.Sprintf("搜索失败：%v", err)
	}
	if len(candidates) == 0 {
		b.updateContext(func(state *weixinIMContext) {
			state.SearchFigureIDs = nil
		})
		return fmt.Sprintf("没有找到和 `%s` 相关的图片。已尝试关键词：%s", query, strings.Join(keywords, " / "))
	}

	displayCandidates, summary := b.reviewFigureSearchCandidates(ctx, query, keywords, candidates)
	ids := make([]int64, 0, len(displayCandidates))
	for _, candidate := range displayCandidates {
		ids = append(ids, candidate.Figure.ID)
	}
	b.updateContext(func(state *weixinIMContext) {
		state.SearchFigureIDs = ids
		state.SearchPaperIDs = nil
	})

	var lines []string
	lines = append(lines, formatWeixinSearchKeywordLines(keywordsZH, keywordsEN)...)
	if summary != "" {
		lines = append(lines, fmt.Sprintf("评估：%s", summary))
	}
	lines = append(lines, fmt.Sprintf("汇总后最可能的图片 %d 张：", len(displayCandidates)))
	for index, candidate := range displayCandidates {
		figure := candidate.Figure
		label := firstNonEmpty(figure.DisplayLabel, fmt.Sprintf("图 %d", figure.FigureIndex))
		lines = append(lines, fmt.Sprintf("%d. [ID %d] %s · %s", index+1, figure.ID, clipRunes(figure.PaperTitle, 40), label))
		lines = append(lines, fmt.Sprintf("   第 %d 页 | %s", figure.PageNumber, clipRunes(firstNonEmpty(figure.Caption, "无图注"), 88)))
	}
	lines = append(lines, "", "发送 `/figure 1` 选中图片并回发预览。")
	return strings.Join(lines, "\n")
}

func (b *WeixinIMBridge) collectPaperSearchCandidates(keywordsZH, keywordsEN []string) ([]weixinPaperSearchCandidate, error) {
	byID := map[int64]*weixinPaperSearchCandidate{}

	for keywordIndex, keyword := range keywordsZH {
		result, err := b.libraryService.ListPapers(model.PaperFilter{
			Keyword:  keyword,
			Page:     1,
			PageSize: weixinSearchProbeLimit,
		})
		if err != nil {
			return nil, err
		}

		for rank, paper := range result.Papers {
			score := scoreWeixinPaperCandidate(paper, keyword, keywordIndex, rank)
			existing, ok := byID[paper.ID]
			if !ok {
				candidate := &weixinPaperSearchCandidate{Paper: paper}
				byID[paper.ID] = candidate
				existing = candidate
			}
			existing.Score += score
		}
	}
	for keywordIndex, keyword := range keywordsEN {
		result, err := b.libraryService.ListPapers(model.PaperFilter{
			Keyword:  keyword,
			Page:     1,
			PageSize: weixinSearchProbeLimit,
		})
		if err != nil {
			return nil, err
		}

		for rank, paper := range result.Papers {
			score := scoreWeixinPaperCandidate(paper, keyword, keywordIndex, rank)
			existing, ok := byID[paper.ID]
			if !ok {
				candidate := &weixinPaperSearchCandidate{Paper: paper}
				byID[paper.ID] = candidate
				existing = candidate
			}
			existing.Score += score
		}
	}

	candidates := make([]weixinPaperSearchCandidate, 0, len(byID))
	for _, candidate := range byID {
		candidates = append(candidates, *candidate)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			if candidates[i].Paper.UpdatedAt.Equal(candidates[j].Paper.UpdatedAt) {
				return candidates[i].Paper.ID > candidates[j].Paper.ID
			}
			return candidates[i].Paper.UpdatedAt.After(candidates[j].Paper.UpdatedAt)
		}
		return candidates[i].Score > candidates[j].Score
	})
	return candidates, nil
}

func (b *WeixinIMBridge) collectFigureSearchCandidates(keywordsZH, keywordsEN []string) ([]weixinFigureSearchCandidate, error) {
	byID := map[int64]*weixinFigureSearchCandidate{}

	for keywordIndex, keyword := range keywordsZH {
		result, err := b.libraryService.ListFigures(model.FigureFilter{
			Keyword:  keyword,
			Page:     1,
			PageSize: weixinSearchProbeLimit,
		})
		if err != nil {
			return nil, err
		}

		for rank, figure := range result.Figures {
			score := scoreWeixinFigureCandidate(figure, keyword, keywordIndex, rank)
			existing, ok := byID[figure.ID]
			if !ok {
				candidate := &weixinFigureSearchCandidate{Figure: figure}
				byID[figure.ID] = candidate
				existing = candidate
			}
			existing.Score += score
		}
	}
	for keywordIndex, keyword := range keywordsEN {
		result, err := b.libraryService.ListFigures(model.FigureFilter{
			Keyword:  keyword,
			Page:     1,
			PageSize: weixinSearchProbeLimit,
		})
		if err != nil {
			return nil, err
		}

		for rank, figure := range result.Figures {
			score := scoreWeixinFigureCandidate(figure, keyword, keywordIndex, rank)
			existing, ok := byID[figure.ID]
			if !ok {
				candidate := &weixinFigureSearchCandidate{Figure: figure}
				byID[figure.ID] = candidate
				existing = candidate
			}
			existing.Score += score
		}
	}

	candidates := make([]weixinFigureSearchCandidate, 0, len(byID))
	for _, candidate := range byID {
		candidates = append(candidates, *candidate)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			if candidates[i].Figure.UpdatedAt.Equal(candidates[j].Figure.UpdatedAt) {
				return candidates[i].Figure.ID > candidates[j].Figure.ID
			}
			return candidates[i].Figure.UpdatedAt.After(candidates[j].Figure.UpdatedAt)
		}
		return candidates[i].Score > candidates[j].Score
	})
	return candidates, nil
}

func (b *WeixinIMBridge) reviewPaperSearchCandidates(ctx context.Context, query string, keywords []string, candidates []weixinPaperSearchCandidate) ([]weixinPaperSearchCandidate, string) {
	if len(candidates) == 0 {
		return nil, ""
	}

	reviewPool := candidates
	if len(reviewPool) > weixinSearchReviewLimit {
		reviewPool = append([]weixinPaperSearchCandidate(nil), reviewPool[:weixinSearchReviewLimit]...)
	}

	summary := localWeixinSearchSummary(keywords, "标题/摘要/标签")
	if b.aiService == nil {
		return limitPaperCandidates(reviewPool, weixinSearchResultLimit), summary
	}

	papers := make([]model.Paper, 0, len(reviewPool))
	for _, candidate := range reviewPool {
		papers = append(papers, candidate.Paper)
	}

	review, err := b.aiService.ReviewWeixinPaperSearch(ctx, query, keywords, papers)
	if err != nil {
		b.logger.Warn("review weixin paper search failed, fallback to score-based ranking", "query", query, "error", err)
		return limitPaperCandidates(reviewPool, weixinSearchResultLimit), summary
	}

	selected := selectPaperCandidatesByID(reviewPool, review.SelectedIDs)
	if len(selected) == 0 {
		return limitPaperCandidates(reviewPool, weixinSearchResultLimit), summary
	}

	return limitPaperCandidates(selected, weixinSearchResultLimit), firstNonEmpty(review.Summary, summary)
}

func (b *WeixinIMBridge) reviewFigureSearchCandidates(ctx context.Context, query string, keywords []string, candidates []weixinFigureSearchCandidate) ([]weixinFigureSearchCandidate, string) {
	if len(candidates) == 0 {
		return nil, ""
	}

	reviewPool := candidates
	if len(reviewPool) > weixinSearchReviewLimit {
		reviewPool = append([]weixinFigureSearchCandidate(nil), reviewPool[:weixinSearchReviewLimit]...)
	}

	summary := localWeixinSearchSummary(keywords, "图注/标签/所属文献")
	if b.aiService == nil {
		return limitFigureCandidates(reviewPool, weixinSearchResultLimit), summary
	}

	figures := make([]model.FigureListItem, 0, len(reviewPool))
	for _, candidate := range reviewPool {
		figures = append(figures, candidate.Figure)
	}

	review, err := b.aiService.ReviewWeixinFigureSearch(ctx, query, keywords, figures)
	if err != nil {
		b.logger.Warn("review weixin figure search failed, fallback to score-based ranking", "query", query, "error", err)
		return limitFigureCandidates(reviewPool, weixinSearchResultLimit), summary
	}

	selected := selectFigureCandidatesByID(reviewPool, review.SelectedIDs)
	if len(selected) == 0 {
		return limitFigureCandidates(reviewPool, weixinSearchResultLimit), summary
	}

	return limitFigureCandidates(selected, weixinSearchResultLimit), firstNonEmpty(review.Summary, summary)
}

func scoreWeixinPaperCandidate(paper model.Paper, keyword string, keywordIndex, rank int) int {
	score := 100 - keywordIndex*12 - rank*6
	text := strings.ToLower(strings.Join([]string{
		paper.Title,
		paper.AbstractText,
		paper.PaperNotesText,
		paper.NotesText,
	}, "\n"))
	lowerKeyword := strings.ToLower(strings.TrimSpace(keyword))
	if lowerKeyword == "" {
		return score
	}
	if strings.Contains(strings.ToLower(paper.Title), lowerKeyword) {
		score += 30
	}
	if strings.Contains(strings.ToLower(paper.AbstractText), lowerKeyword) {
		score += 16
	}
	if strings.Contains(strings.ToLower(paper.PaperNotesText), lowerKeyword) {
		score += 8
	}
	for _, tag := range paper.Tags {
		if strings.Contains(strings.ToLower(tag.Name), lowerKeyword) {
			score += 14
			break
		}
	}
	if strings.Contains(text, lowerKeyword) {
		score += 6
	}
	return score
}

func scoreWeixinFigureCandidate(figure model.FigureListItem, keyword string, keywordIndex, rank int) int {
	score := 100 - keywordIndex*12 - rank*6
	lowerKeyword := strings.ToLower(strings.TrimSpace(keyword))
	if lowerKeyword == "" {
		return score
	}
	if strings.Contains(strings.ToLower(figure.Caption), lowerKeyword) {
		score += 26
	}
	if strings.Contains(strings.ToLower(figure.PaperTitle), lowerKeyword) {
		score += 18
	}
	if strings.Contains(strings.ToLower(figure.NotesText), lowerKeyword) {
		score += 8
	}
	for _, tag := range figure.Tags {
		if strings.Contains(strings.ToLower(tag.Name), lowerKeyword) {
			score += 14
			break
		}
	}
	if strings.Contains(strings.ToLower(figure.DisplayLabel), lowerKeyword) {
		score += 6
	}
	return score
}

func limitPaperCandidates(candidates []weixinPaperSearchCandidate, limit int) []weixinPaperSearchCandidate {
	if len(candidates) <= limit {
		return candidates
	}
	return append([]weixinPaperSearchCandidate(nil), candidates[:limit]...)
}

func limitFigureCandidates(candidates []weixinFigureSearchCandidate, limit int) []weixinFigureSearchCandidate {
	if len(candidates) <= limit {
		return candidates
	}
	return append([]weixinFigureSearchCandidate(nil), candidates[:limit]...)
}

func selectPaperCandidatesByID(candidates []weixinPaperSearchCandidate, ids []int64) []weixinPaperSearchCandidate {
	byID := make(map[int64]weixinPaperSearchCandidate, len(candidates))
	for _, candidate := range candidates {
		byID[candidate.Paper.ID] = candidate
	}

	selected := make([]weixinPaperSearchCandidate, 0, len(ids))
	for _, id := range ids {
		candidate, ok := byID[id]
		if !ok {
			continue
		}
		selected = append(selected, candidate)
	}
	return selected
}

func selectFigureCandidatesByID(candidates []weixinFigureSearchCandidate, ids []int64) []weixinFigureSearchCandidate {
	byID := make(map[int64]weixinFigureSearchCandidate, len(candidates))
	for _, candidate := range candidates {
		byID[candidate.Figure.ID] = candidate
	}

	selected := make([]weixinFigureSearchCandidate, 0, len(ids))
	for _, id := range ids {
		candidate, ok := byID[id]
		if !ok {
			continue
		}
		selected = append(selected, candidate)
	}
	return selected
}

func localWeixinSearchSummary(keywords []string, dimensions string) string {
	return fmt.Sprintf("已按中英文共 %d 个关键词分别检索、去重汇总，并结合%s做了一次匹配度排序。", len(keywords), dimensions)
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

func (b *WeixinIMBridge) listRecentPapers() string {
	result, err := b.libraryService.ListPapers(model.PaperFilter{
		Page:     1,
		PageSize: 5,
	})
	if err != nil {
		return fmt.Sprintf("读取最近文献失败：%v", err)
	}
	if result.Total == 0 {
		return "当前还没有文献。"
	}

	ids := make([]int64, 0, len(result.Papers))
	for _, paper := range result.Papers {
		ids = append(ids, paper.ID)
	}
	b.updateContext(func(state *weixinIMContext) {
		state.SearchPaperIDs = ids
		state.SearchFigureIDs = nil
	})

	var lines []string
	lines = append(lines, "最近文献：")
	for index, paper := range result.Papers {
		lines = append(lines, fmt.Sprintf("%d. [%d] %s", index+1, paper.ID, clipRunes(strings.TrimSpace(paper.Title), 56)))
		lines = append(lines, fmt.Sprintf("   更新时间：%s | 图片：%d 张", paper.UpdatedAt.Format("2006-01-02 15:04"), paper.FigureCount))
	}
	lines = append(lines, "", "发送 `/paper 1` 选中文献。")
	return strings.Join(lines, "\n")
}

func (b *WeixinIMBridge) selectPaper(arg string) string {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return "用法：`/paper 序号` 或 `/paper 文献ID`"
	}

	value, err := strconv.Atoi(arg)
	if err != nil || value <= 0 {
		return "请输入有效的文献序号或文献 ID。"
	}

	paperID := int64(value)
	state := b.getContext()
	if value <= len(state.SearchPaperIDs) {
		paperID = state.SearchPaperIDs[value-1]
	}

	paper, err := b.libraryService.GetPaper(paperID)
	if err != nil {
		return fmt.Sprintf("读取文献失败：%v", err)
	}

	b.activatePaperContext(paper.ID, false)
	return b.formatPaperSelection(paper, false)
}

func (b *WeixinIMBridge) listFigures() string {
	paper, errText := b.requireCurrentPaper()
	if errText != "" {
		return errText
	}
	figures := topLevelFigures(paper.Figures)
	if len(figures) == 0 {
		return "当前文献没有图片。"
	}
	b.updateContext(func(state *weixinIMContext) {
		state.SearchFigureIDs = nil
	})

	var lines []string
	lines = append(lines, fmt.Sprintf("文献 [%d] %s 的图片：", paper.ID, clipRunes(paper.Title, 48)))
	for index, figure := range figures {
		caption := firstNonEmpty(strings.TrimSpace(figure.Caption), strings.TrimSpace(figure.OriginalName), "未命名图片")
		lines = append(lines, fmt.Sprintf("%d. [ID %d] 第 %d 页 · 图 %d", index+1, figure.ID, figure.PageNumber, figure.FigureIndex))
		lines = append(lines, fmt.Sprintf("   %s", clipRunes(caption, 56)))
	}
	lines = append(lines, "", "发送 `/figure 1` 选中图片；如果原图可用，会自动回发图片预览，然后可继续发送 `/interpret 问题`。")
	return strings.Join(lines, "\n")
}

func (b *WeixinIMBridge) selectFigure(arg string) string {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return "用法：`/figure 序号` 或 `/figure 图片ID`"
	}

	value, err := strconv.Atoi(arg)
	if err != nil || value <= 0 {
		return "请输入有效的图片序号。"
	}

	state := b.getContext()
	if value <= len(state.SearchFigureIDs) && len(state.SearchFigureIDs) > 0 {
		return b.selectFigureFromSearchResult(state.SearchFigureIDs[value-1])
	}

	paper, errText := b.requireCurrentPaper()
	if errText != "" {
		return errText
	}
	figures := topLevelFigures(paper.Figures)

	var figure *model.Figure
	if value <= len(figures) {
		figure = &figures[value-1]
	} else {
		for index := range figures {
			if figures[index].ID == int64(value) {
				figure = &figures[index]
				break
			}
		}
	}
	if figure == nil {
		return fmt.Sprintf("未找到序号为 %d 的图片。先发送 `/figures` 查看当前文献图片，或重新 `/search`。", value)
	}

	b.updateContext(func(state *weixinIMContext) {
		state.CurrentFigureID = figure.ID
		state.SearchFigureIDs = nil
		state.QAHistory = nil
	})

	caption := firstNonEmpty(strings.TrimSpace(figure.Caption), strings.TrimSpace(figure.OriginalName), "未命名图片")
	return fmt.Sprintf(
		"已选中图片 [ID %d]\n第 %d 页 · 图 %d\n%s\n\n如果原图可用，下一条消息会回发图片预览。发送 `/interpret 问题` 获取图片解读，或 `/note 你的内容` 追加图片笔记。",
		figure.ID,
		figure.PageNumber,
		figure.FigureIndex,
		clipRunes(caption, 180),
	)
}

func (b *WeixinIMBridge) selectFigureFromSearchResult(figureID int64) string {
	figureRef, err := b.libraryService.repo.GetFigure(figureID)
	if err != nil {
		return fmt.Sprintf("读取图片失败：%v", err)
	}
	if figureRef == nil {
		return "目标图片不存在，请重新搜索。"
	}

	paper, err := b.libraryService.GetPaper(figureRef.PaperID)
	if err != nil {
		return fmt.Sprintf("读取图片所属文献失败：%v", err)
	}

	figure := findFigureByID(paper.Figures, figureID)
	if figure == nil {
		return "目标图片已失效，请重新搜索。"
	}

	b.updateContext(func(state *weixinIMContext) {
		state.CurrentPaperID = paper.ID
		state.CurrentFigureID = figure.ID
		state.SearchFigureIDs = nil
		state.QAHistory = nil
	})

	caption := firstNonEmpty(strings.TrimSpace(figure.Caption), strings.TrimSpace(figure.OriginalName), "未命名图片")
	return fmt.Sprintf(
		"已选中图片 [ID %d]\n所属文献：[ID %d] %s\n第 %d 页 · 图 %d\n%s\n\n如果原图可用，下一条消息会回发图片预览。发送 `/interpret 问题` 获取图片解读，或 `/note 你的内容` 追加图片笔记。",
		figure.ID,
		paper.ID,
		clipRunes(paper.Title, 72),
		figure.PageNumber,
		figure.FigureIndex,
		clipRunes(caption, 180),
	)
}

func (b *WeixinIMBridge) appendNote(note string) string {
	note = strings.TrimSpace(note)
	if note == "" {
		return "用法：`/note 你的内容`"
	}

	state := b.getContext()
	if state.CurrentFigureID > 0 {
		paper, errText := b.requireCurrentPaper()
		if errText != "" {
			return errText
		}

		figure := findFigureByID(paper.Figures, state.CurrentFigureID)
		if figure == nil {
			b.updateContext(func(context *weixinIMContext) {
				context.CurrentFigureID = 0
			})
			return "当前图片已不存在，请重新发送 `/figures` 查看列表。"
		}

		nextNotes := appendWeixinNote(figure.NotesText, note)
		if _, err := b.libraryService.UpdateFigure(figure.ID, UpdateFigureParams{NotesText: &nextNotes}); err != nil {
			return fmt.Sprintf("追加图片笔记失败：%v", err)
		}
		return fmt.Sprintf("已追加到图片笔记 [ID %d]。", figure.ID)
	}

	paper, errText := b.requireCurrentPaper()
	if errText != "" {
		return errText
	}

	nextNotes := appendWeixinNote(paper.PaperNotesText, note)
	if _, err := b.libraryService.UpdatePaper(paper.ID, UpdatePaperParams{
		Title:          paper.Title,
		AbstractText:   paper.AbstractText,
		NotesText:      paper.NotesText,
		PaperNotesText: nextNotes,
		GroupID:        paper.GroupID,
		Tags:           tagNamesFromPaper(paper.Tags),
	}); err != nil {
		return fmt.Sprintf("追加文献笔记失败：%v", err)
	}

	return fmt.Sprintf("已追加到文献笔记 [%d] %s。", paper.ID, clipRunes(paper.Title, 36))
}

func (b *WeixinIMBridge) interpretCurrentFigure(ctx context.Context, question string) string {
	paper, figure, errText := b.requireCurrentFigure()
	if errText != "" {
		return errText
	}

	response, err := b.aiService.ReadPaper(ctx, model.AIReadRequest{
		PaperID:  paper.ID,
		FigureID: figure.ID,
		Action:   model.AIActionFigureInterpretation,
		Question: strings.TrimSpace(question),
	})
	if err != nil {
		return fmt.Sprintf("图片解读失败：%v", err)
	}

	caption := firstNonEmpty(strings.TrimSpace(figure.Caption), strings.TrimSpace(figure.OriginalName), "未命名图片")
	return fmt.Sprintf("图片解读 · %s\n\n%s", clipRunes(caption, 72), strings.TrimSpace(response.Answer))
}

func (b *WeixinIMBridge) answerCurrentPaper(ctx context.Context, question string) string {
	return b.answerCurrentPaperReply(ctx, question).Text
}

func (b *WeixinIMBridge) answerCurrentPaperReply(ctx context.Context, question string) weixinReplyEnvelope {
	question = strings.TrimSpace(question)
	if question == "" {
		return weixinReplyEnvelope{Text: "用法：`/ask 你的问题`"}
	}

	paper, errText := b.requireCurrentPaper()
	if errText != "" {
		return weixinReplyEnvelope{Text: errText}
	}

	state := b.getContext()
	response, err := b.aiService.ReadPaper(ctx, model.AIReadRequest{
		PaperID:  paper.ID,
		Action:   model.AIActionPaperQA,
		Question: question,
		History:  append([]model.AIConversationTurn(nil), state.QAHistory...),
	})
	if err != nil {
		return weixinReplyEnvelope{Text: fmt.Sprintf("问答失败：%v", err)}
	}

	turn := model.AIConversationTurn{
		Question: question,
		Answer:   strings.TrimSpace(response.Answer),
	}
	b.updateContext(func(state *weixinIMContext) {
		state.QAHistory = append(state.QAHistory, turn)
		if len(state.QAHistory) > weixinHistoryLimit {
			state.QAHistory = append([]model.AIConversationTurn(nil), state.QAHistory[len(state.QAHistory)-weixinHistoryLimit:]...)
		}
	})

	replyText := fmt.Sprintf("文献问答 · %s\n\n%s", clipRunes(paper.Title, 72), turn.Answer)
	reply := weixinReplyEnvelope{Text: replyText}
	if turn.Answer != "" {
		reply.TTSText = turn.Answer
		reply.OptimizeTTSText = true
		reply.VoicePendingNotice = "语音内容生成中，请稍后。"
	}
	return reply
}

func (b *WeixinIMBridge) statusText() string {
	state := b.getContext()
	if state.CurrentPaperID == 0 {
		switch {
		case len(state.SearchFigureIDs) > 0:
			return fmt.Sprintf("当前未选中文献，但最近一次图片检索已有 %d 条候选。发送 `/figure 1` 继续，或重新 `/search`。", len(state.SearchFigureIDs))
		case len(state.SearchPaperIDs) > 0:
			return fmt.Sprintf("当前未选中文献，但最近一次文献检索已有 %d 条候选。发送 `/paper 1` 继续，或重新 `/search`。", len(state.SearchPaperIDs))
		default:
			return "当前未选中文献。发送 `/search 自然语言检索内容` 或 `/recent` 开始。"
		}
	}

	paper, err := b.libraryService.GetPaper(state.CurrentPaperID)
	if err != nil {
		b.setContext(weixinIMContext{})
		return "当前文献上下文已失效，请重新发送 `/search 自然语言检索内容`。"
	}

	lines := []string{
		fmt.Sprintf("当前文献：[ID %d] %s", paper.ID, clipRunes(paper.Title, 72)),
		fmt.Sprintf("图片数量：%d", len(topLevelFigures(paper.Figures))),
		fmt.Sprintf("历史问答：%d 轮", len(state.QAHistory)),
	}

	if state.CurrentFigureID > 0 {
		if figure := findFigureByID(paper.Figures, state.CurrentFigureID); figure != nil {
			lines = append(lines, fmt.Sprintf("当前图片：[ID %d] 第 %d 页 · 图 %d", figure.ID, figure.PageNumber, figure.FigureIndex))
		}
	}

	if len(state.SearchPaperIDs) > 0 {
		lines = append(lines, fmt.Sprintf("最近检索结果：%d 条", len(state.SearchPaperIDs)))
	}
	if len(state.SearchFigureIDs) > 0 {
		lines = append(lines, fmt.Sprintf("最近图片候选：%d 条", len(state.SearchFigureIDs)))
	}
	lines = append(lines, "发送 `/ask 问题` 提问，或使用 `/figures`、`/interpret 问题`、`/note 内容`。")
	return strings.Join(lines, "\n")
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

func (b *WeixinIMBridge) selectedFigurePreviewPath(message weixin.Message, replyText string) (string, error) {
	command, _, ok := parseWeixinSlashCommand(extractWeixinText(message))
	if !ok || command != "/figure" {
		return "", nil
	}
	if !strings.HasPrefix(strings.TrimSpace(replyText), "已选中图片 [ID ") {
		return "", nil
	}

	_, figure, errText := b.requireCurrentFigure()
	if errText != "" {
		return "", nil
	}
	return b.figurePreviewPath(figure)
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
	if b.synthesizeTTS == nil {
		return nil, errors.New("weixin tts synthesizer is nil")
	}
	return settings, nil
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
		"直接发送 PDF：自动导入文献并切换上下文",
		"`/ask 问题` 或 `/qa 问题`：对当前文献提问",
		"`/note 内容`：追加文献/图片笔记",
		"`/interpret 问题`：解读当前图片",
		"`/testvoice`：回一条测试文本，并追加一段基于当前 TTS 配置的 Hello World 语音",
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

func normalizeWeixinPlainTextCommand(command string) string {
	switch strings.ToLower(strings.TrimSpace(command)) {
	case "/help", "help", "帮助":
		return "/help"
	case "/status", "status", "上下文", "状态":
		return "/status"
	case "/reset", "reset", "清空", "重置":
		return "/reset"
	case "/recent", "recent", "最近":
		return "/recent"
	case "/figures", "figures", "图片列表", "看图":
		return "/figures"
	case "/search", "search", "搜索":
		return "/search"
	case "/search-papers", "search-papers", "search_papers":
		return "/search-papers"
	case "/search-figures", "search-figures", "search_figures":
		return "/search-figures"
	case "/paper", "paper":
		return "/paper"
	case "/figure", "figure":
		return "/figure"
	case "/ask", "ask", "/qa", "qa", "问答":
		return "/ask"
	case "/note", "note", "笔记":
		return "/note"
	case "/interpret", "interpret", "解图", "解读":
		return "/interpret"
	default:
		return ""
	}
}

func resolveWeixinSearchPlanKeywords(query string, plan *weixinSearchPlan) ([]string, []string, []string) {
	if plan == nil {
		plan = heuristicWeixinSearchPlan(query, "")
	}

	keywordsZH := normalizeWeixinSearchKeywordsForLanguage(plan.KeywordsZH, "zh")
	keywordsEN := normalizeWeixinSearchKeywordsForLanguage(plan.KeywordsEN, "en")
	if len(keywordsZH) == 0 || len(keywordsEN) == 0 {
		fallbackPlan := heuristicWeixinSearchPlan(query, plan.Target)
		keywordsZH = mergeWeixinSearchKeywords(keywordsZH, fallbackPlan.KeywordsZH)
		keywordsEN = mergeWeixinSearchKeywords(keywordsEN, fallbackPlan.KeywordsEN)
		keywordsZH = normalizeWeixinSearchKeywordsForLanguage(keywordsZH, "zh")
		keywordsEN = normalizeWeixinSearchKeywordsForLanguage(keywordsEN, "en")
	}

	keywords := mergeWeixinSearchKeywords(keywordsZH, keywordsEN, plan.Keywords)
	return keywordsZH, keywordsEN, keywords
}

func formatWeixinSearchKeywordLines(keywordsZH, keywordsEN []string) []string {
	lines := []string{}
	if len(keywordsZH) > 0 {
		lines = append(lines, fmt.Sprintf("中文关键词：%s", strings.Join(keywordsZH, " / ")))
	}
	if len(keywordsEN) > 0 {
		lines = append(lines, fmt.Sprintf("英文关键词：%s", strings.Join(keywordsEN, " / ")))
	}
	if len(lines) == 0 {
		return []string{"检索关键词：无"}
	}
	return lines
}

func cleanupWeixinSearchQuery(query string) string {
	cleaned := strings.TrimSpace(query)
	replacements := []string{
		"我想要一张", "我想找一张", "帮我找一张", "给我一张", "来一张",
		"我想要", "我想找", "帮我找", "帮我搜", "给我找", "有没有", "请帮我找", "请搜索", "搜索一下",
		"相关文献", "相关文章", "相关论文", "相关图片", "相关配图",
	}
	for _, replacement := range replacements {
		cleaned = strings.ReplaceAll(cleaned, replacement, " ")
	}

	cleaned = strings.Trim(cleaned, " \t\r\n,，。！？!?:：;；")
	return strings.Join(strings.Fields(cleaned), " ")
}

func inferWeixinSearchTarget(query, cleaned string) string {
	normalized := strings.ToLower(firstNonEmpty(cleaned, query))

	figureSignals := []string{
		"火山图", "热图", "umap", "t-sne", "tsne", "pca", "dotplot", "dot plot", "violin plot", "小提琴图",
		"box plot", "箱线图", "scatter", "散点图", "volcano", "heatmap", "kaplan", "survival curve", "生存曲线",
		"western blot", "免疫荧光", "图片", "配图", "figure", "plot", "想要一张", "来一张",
	}
	for _, signal := range figureSignals {
		if strings.Contains(normalized, signal) {
			return weixinSearchTargetFigure
		}
	}

	return weixinSearchTargetPaper
}

func expandWeixinSearchKeywords(query, cleaned, target string) ([]string, []string) {
	type keywordExpansion struct {
		Needles    []string
		KeywordsZH []string
		KeywordsEN []string
	}

	expansions := []keywordExpansion{
		{Needles: []string{"火山图", "volcano"}, KeywordsZH: []string{"火山图", "差异表达"}, KeywordsEN: []string{"volcano plot", "differential expression"}},
		{Needles: []string{"热图", "heatmap"}, KeywordsZH: []string{"热图", "表达热图"}, KeywordsEN: []string{"heatmap", "expression heatmap"}},
		{Needles: []string{"umap"}, KeywordsZH: []string{"UMAP", "降维嵌入"}, KeywordsEN: []string{"UMAP", "embedding"}},
		{Needles: []string{"tsne", "t-sne"}, KeywordsZH: []string{"t-SNE", "降维嵌入"}, KeywordsEN: []string{"t-SNE", "embedding"}},
		{Needles: []string{"pca"}, KeywordsZH: []string{"PCA", "主成分分析"}, KeywordsEN: []string{"PCA", "principal component analysis"}},
		{Needles: []string{"小提琴图", "violin"}, KeywordsZH: []string{"小提琴图"}, KeywordsEN: []string{"violin plot"}},
		{Needles: []string{"点图", "dot plot", "dotplot"}, KeywordsZH: []string{"点图"}, KeywordsEN: []string{"dot plot"}},
		{Needles: []string{"箱线图", "box plot"}, KeywordsZH: []string{"箱线图"}, KeywordsEN: []string{"box plot"}},
		{Needles: []string{"散点图", "scatter"}, KeywordsZH: []string{"散点图"}, KeywordsEN: []string{"scatter plot"}},
		{Needles: []string{"生存曲线", "kaplan", "km曲线", "survival"}, KeywordsZH: []string{"生存曲线", "Kaplan-Meier"}, KeywordsEN: []string{"Kaplan-Meier", "survival curve"}},
		{Needles: []string{"western blot", "wb"}, KeywordsZH: []string{"蛋白印迹"}, KeywordsEN: []string{"western blot", "immunoblot"}},
		{Needles: []string{"免疫荧光", "immunofluorescence"}, KeywordsZH: []string{"免疫荧光"}, KeywordsEN: []string{"immunofluorescence"}},
		{Needles: []string{"单细胞", "single cell", "single-cell"}, KeywordsZH: []string{"单细胞"}, KeywordsEN: []string{"single cell", "single-cell"}},
		{Needles: []string{"图谱", "atlas"}, KeywordsZH: []string{"图谱"}, KeywordsEN: []string{"atlas"}},
		{Needles: []string{"综述", "review"}, KeywordsZH: []string{"综述"}, KeywordsEN: []string{"review"}},
		{Needles: []string{"空间转录组", "spatial transcriptomics"}, KeywordsZH: []string{"空间转录组"}, KeywordsEN: []string{"spatial transcriptomics"}},
		{Needles: []string{"差异表达", "differential expression"}, KeywordsZH: []string{"差异表达"}, KeywordsEN: []string{"differential expression"}},
		{Needles: []string{"轨迹", "拟时", "trajectory", "pseudotime"}, KeywordsZH: []string{"轨迹", "拟时"}, KeywordsEN: []string{"trajectory", "pseudotime"}},
	}

	normalized := strings.ToLower(strings.Join([]string{query, cleaned}, " "))
	keywordsZH := []string{}
	keywordsEN := []string{}
	for _, expansion := range expansions {
		matched := false
		for _, needle := range expansion.Needles {
			if strings.Contains(normalized, strings.ToLower(needle)) {
				matched = true
				break
			}
		}
		if matched {
			keywordsZH = append(keywordsZH, expansion.KeywordsZH...)
			keywordsEN = append(keywordsEN, expansion.KeywordsEN...)
		}
	}

	queryLanguage := detectTranslationLanguageKey(firstNonEmpty(cleaned, query))
	switch queryLanguage {
	case "han":
		keywordsZH = append([]string{firstNonEmpty(cleaned, query)}, keywordsZH...)
	case "latin":
		keywordsEN = append([]string{firstNonEmpty(cleaned, query)}, keywordsEN...)
	default:
		keywordsZH = append([]string{cleaned}, keywordsZH...)
		keywordsEN = append([]string{cleaned}, keywordsEN...)
	}

	if target == weixinSearchTargetPaper {
		paperHint := strings.TrimSpace(strings.ReplaceAll(cleaned, "文献", ""))
		if detectTranslationLanguageKey(paperHint) == "han" {
			keywordsZH = append(keywordsZH, paperHint)
		}
		if detectTranslationLanguageKey(paperHint) == "latin" {
			keywordsEN = append(keywordsEN, paperHint)
		}
	}

	return keywordsZH, keywordsEN
}

func appendWeixinNote(existing, incoming string) string {
	entry := fmt.Sprintf("[微信 %s]\n%s", time.Now().Format("2006-01-02 15:04"), strings.TrimSpace(incoming))
	existing = strings.TrimSpace(existing)
	if existing == "" {
		return entry
	}
	return existing + "\n\n" + entry
}

func trimWeixinReply(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= weixinReplyMaxRunes {
		return text
	}
	return string(runes[:weixinReplyMaxRunes]) + "\n\n[内容已截断]"
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

func splitWeixinReplyText(text string, maxRunes int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if maxRunes <= 0 {
		return []string{text}
	}

	runes := []rune(text)
	if len(runes) <= maxRunes {
		return []string{text}
	}

	chunks := make([]string, 0, (len(runes)+maxRunes-1)/maxRunes)
	for start := 0; start < len(runes); start += maxRunes {
		end := start + maxRunes
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[start:end]))
	}
	return chunks
}

func sendWeixinTextReply(ctx context.Context, client *weixin.Client, toUserID, text, contextToken string) error {
	chunks := splitWeixinReplyText(text, weixinReplyChunkRunes)
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

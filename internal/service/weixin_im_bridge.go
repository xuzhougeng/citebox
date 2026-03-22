package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/weixin"
)

const (
	weixinBridgeStateDirName = "weixin-bridge"
	weixinSyncBufFileName    = "sync_buf"
	weixinContextFileName    = "im_context.json"
	weixinReplyMaxRunes      = 3200
	weixinHistoryLimit       = 6
)

type weixinAIReader interface {
	ReadPaper(ctx context.Context, input model.AIReadRequest) (*model.AIReadResponse, error)
}

type weixinIMContext struct {
	CurrentPaperID  int64                      `json:"current_paper_id"`
	CurrentFigureID int64                      `json:"current_figure_id"`
	SearchPaperIDs  []int64                    `json:"search_paper_ids,omitempty"`
	QAHistory       []model.AIConversationTurn `json:"qa_history,omitempty"`
	UpdatedAt       string                     `json:"updated_at,omitempty"`
}

type WeixinIMBridge struct {
	libraryService *LibraryService
	aiService      weixinAIReader
	logger         *slog.Logger
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
		stateDir:       filepath.Join(storageDir, weixinBridgeStateDirName),
	}
	bridge.syncBufPath = filepath.Join(bridge.stateDir, weixinSyncBufFileName)
	bridge.contextPath = filepath.Join(bridge.stateDir, weixinContextFileName)
	bridge.loadContext()
	return bridge
}

func (b *WeixinIMBridge) Run(ctx context.Context) error {
	b.logger.Info("weixin IM bridge loop started")

	for {
		if err := ctx.Err(); err != nil {
			return err
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
			if !sleepContext(ctx, 5*time.Second) {
				return ctx.Err()
			}
			continue
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

		for _, message := range resp.Msgs {
			if message.MessageType != weixin.MessageTypeUser {
				continue
			}
			if strings.TrimSpace(binding.UserID) != "" && strings.TrimSpace(message.FromUserID) != strings.TrimSpace(binding.UserID) {
				b.logger.Warn("ignore message from unexpected weixin user", "from_user_id", message.FromUserID)
				continue
			}

			text := extractWeixinText(message)
			if text == "" {
				continue
			}

			reply := trimWeixinReply(b.handleIncomingText(ctx, text))
			if reply == "" {
				continue
			}
			if err := client.SendTextMessage(ctx, message.FromUserID, reply, message.ContextToken); err != nil {
				b.logger.Warn("send weixin reply failed", "error", err)
			}
		}
	}
}

func (b *WeixinIMBridge) handleIncomingText(ctx context.Context, text string) string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "／") {
		text = "/" + strings.TrimPrefix(text, "／")
	}
	if text == "" {
		return ""
	}

	switch {
	case isWeixinHelpCommand(text):
		return weixinHelpText()
	case text == "/status" || text == "状态":
		return b.statusText()
	case text == "/reset" || text == "重置":
		b.setContext(weixinIMContext{})
		return "已清空微信上下文。发送 `搜 关键词` 或 `最近文献` 开始。"
	case text == "/figures" || text == "图片":
		return b.listFigures()
	case text == "/recent" || text == "最近文献":
		return b.listRecentPapers()
	}

	if arg, ok := matchWeixinCommand(text, "/search", "搜"); ok {
		return b.searchPapers(arg)
	}
	if arg, ok := matchWeixinCommand(text, "/paper", "文献"); ok {
		return b.selectPaper(arg)
	}
	if arg, ok := matchWeixinCommand(text, "/figure", "图片"); ok {
		return b.selectFigure(arg)
	}
	if arg, ok := matchWeixinCommand(text, "/note", "笔记"); ok {
		return b.appendNote(arg)
	}
	if arg, ok := matchWeixinCommand(text, "/interpret", "解读"); ok {
		return b.interpretCurrentFigure(ctx, arg)
	}

	if _, err := strconv.Atoi(text); err == nil && len(b.getContext().SearchPaperIDs) > 0 {
		return b.selectPaper(text)
	}

	return b.answerCurrentPaper(ctx, text)
}

func (b *WeixinIMBridge) searchPapers(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return "用法：`搜 关键词`"
	}

	result, err := b.libraryService.ListPapers(model.PaperFilter{
		Keyword:  query,
		Page:     1,
		PageSize: 5,
	})
	if err != nil {
		return fmt.Sprintf("搜索失败：%v", err)
	}
	if result.Total == 0 {
		b.updateContext(func(state *weixinIMContext) {
			state.SearchPaperIDs = nil
		})
		return fmt.Sprintf("没有找到和 `%s` 相关的文献。", query)
	}

	ids := make([]int64, 0, len(result.Papers))
	for _, paper := range result.Papers {
		ids = append(ids, paper.ID)
	}

	if result.Total == 1 && len(result.Papers) == 1 {
		paper := result.Papers[0]
		b.updateContext(func(state *weixinIMContext) {
			state.SearchPaperIDs = ids
			state.CurrentPaperID = paper.ID
			state.CurrentFigureID = 0
			state.QAHistory = nil
		})
		return b.formatPaperSelection(&paper, true)
	}

	b.updateContext(func(state *weixinIMContext) {
		state.SearchPaperIDs = ids
	})

	var lines []string
	lines = append(lines, fmt.Sprintf("找到 %d 篇文献，当前展示前 %d 条：", result.Total, len(result.Papers)))
	for index, paper := range result.Papers {
		lines = append(lines, fmt.Sprintf("%d. [%d] %s", index+1, paper.ID, clipRunes(strings.TrimSpace(paper.Title), 56)))
		lines = append(lines, fmt.Sprintf("   状态：%s | 图片：%d 张", paper.ExtractionStatus, paper.FigureCount))
	}
	lines = append(lines, "", "发送 `文献 1` 选中目标文献，或直接回复编号。")
	return strings.Join(lines, "\n")
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
	})

	var lines []string
	lines = append(lines, "最近文献：")
	for index, paper := range result.Papers {
		lines = append(lines, fmt.Sprintf("%d. [%d] %s", index+1, paper.ID, clipRunes(strings.TrimSpace(paper.Title), 56)))
		lines = append(lines, fmt.Sprintf("   更新时间：%s | 图片：%d 张", paper.UpdatedAt.Format("2006-01-02 15:04"), paper.FigureCount))
	}
	lines = append(lines, "", "发送 `文献 1` 选中文献。")
	return strings.Join(lines, "\n")
}

func (b *WeixinIMBridge) selectPaper(arg string) string {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return "用法：`文献 序号` 或 `文献 文献ID`"
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

	b.updateContext(func(state *weixinIMContext) {
		state.CurrentPaperID = paper.ID
		state.CurrentFigureID = 0
		state.QAHistory = nil
	})
	return b.formatPaperSelection(paper, false)
}

func (b *WeixinIMBridge) listFigures() string {
	paper, errText := b.requireCurrentPaper()
	if errText != "" {
		return errText
	}
	if len(paper.Figures) == 0 {
		return "当前文献没有图片。"
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("文献 [%d] %s 的图片：", paper.ID, clipRunes(paper.Title, 48)))
	for index, figure := range paper.Figures {
		caption := firstNonEmpty(strings.TrimSpace(figure.Caption), strings.TrimSpace(figure.OriginalName), "未命名图片")
		lines = append(lines, fmt.Sprintf("%d. [ID %d] 第 %d 页 · 图 %d", index+1, figure.ID, figure.PageNumber, figure.FigureIndex))
		lines = append(lines, fmt.Sprintf("   %s", clipRunes(caption, 56)))
	}
	lines = append(lines, "", "发送 `图片 1` 选中图片，然后可发送 `解读`。")
	return strings.Join(lines, "\n")
}

func (b *WeixinIMBridge) selectFigure(arg string) string {
	paper, errText := b.requireCurrentPaper()
	if errText != "" {
		return errText
	}

	arg = strings.TrimSpace(arg)
	if arg == "" {
		return "用法：`图片 序号`"
	}

	value, err := strconv.Atoi(arg)
	if err != nil || value <= 0 {
		return "请输入有效的图片序号。"
	}

	var figure *model.Figure
	if value <= len(paper.Figures) {
		figure = &paper.Figures[value-1]
	} else {
		for index := range paper.Figures {
			if paper.Figures[index].ID == int64(value) {
				figure = &paper.Figures[index]
				break
			}
		}
	}
	if figure == nil {
		return fmt.Sprintf("未找到序号为 %d 的图片。先发送 `图片` 查看列表。", value)
	}

	b.updateContext(func(state *weixinIMContext) {
		state.CurrentFigureID = figure.ID
	})

	caption := firstNonEmpty(strings.TrimSpace(figure.Caption), strings.TrimSpace(figure.OriginalName), "未命名图片")
	return fmt.Sprintf(
		"已选中图片 [ID %d]\n第 %d 页 · 图 %d\n%s\n\n发送 `解读` 获取图片解读，或 `笔记 你的内容` 追加图片笔记。",
		figure.ID,
		figure.PageNumber,
		figure.FigureIndex,
		clipRunes(caption, 180),
	)
}

func (b *WeixinIMBridge) appendNote(note string) string {
	note = strings.TrimSpace(note)
	if note == "" {
		return "用法：`笔记 你的内容`"
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
			return "当前图片已不存在，请重新发送 `图片` 查看列表。"
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
	question = strings.TrimSpace(question)
	if question == "" {
		return ""
	}

	paper, errText := b.requireCurrentPaper()
	if errText != "" {
		return errText
	}

	state := b.getContext()
	response, err := b.aiService.ReadPaper(ctx, model.AIReadRequest{
		PaperID:  paper.ID,
		Action:   model.AIActionPaperQA,
		Question: question,
		History:  append([]model.AIConversationTurn(nil), state.QAHistory...),
	})
	if err != nil {
		return fmt.Sprintf("问答失败：%v", err)
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

	return fmt.Sprintf("文献问答 · %s\n\n%s", clipRunes(paper.Title, 72), turn.Answer)
}

func (b *WeixinIMBridge) statusText() string {
	state := b.getContext()
	if state.CurrentPaperID == 0 {
		return "当前未选中文献。发送 `搜 关键词` 或 `最近文献` 开始。"
	}

	paper, err := b.libraryService.GetPaper(state.CurrentPaperID)
	if err != nil {
		b.setContext(weixinIMContext{})
		return "当前文献上下文已失效，请重新发送 `搜 关键词`。"
	}

	lines := []string{
		fmt.Sprintf("当前文献：[ID %d] %s", paper.ID, clipRunes(paper.Title, 72)),
		fmt.Sprintf("图片数量：%d", len(paper.Figures)),
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
	lines = append(lines, "可直接提问，或发送 `图片`、`解读`、`笔记 内容`。")
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
	lines = append(lines, fmt.Sprintf("状态：%s | 图片：%d 张", paper.ExtractionStatus, len(paper.Figures)))

	summary := firstNonEmpty(strings.TrimSpace(paper.AbstractText), strings.TrimSpace(paper.PaperNotesText), strings.TrimSpace(paper.NotesText))
	if summary != "" {
		lines = append(lines, clipRunes(summary, 180))
	}

	lines = append(lines, "", "现在可以直接提问，发送 `图片` 查看图片列表，或 `笔记 你的内容` 追加文献笔记。")
	return strings.Join(lines, "\n")
}

func (b *WeixinIMBridge) requireCurrentPaper() (*model.Paper, string) {
	state := b.getContext()
	if state.CurrentPaperID == 0 {
		return nil, "请先发送 `搜 关键词` 或 `最近文献` 选择文献。"
	}

	paper, err := b.libraryService.GetPaper(state.CurrentPaperID)
	if err != nil {
		b.setContext(weixinIMContext{})
		return nil, "当前文献已失效，请重新发送 `搜 关键词`。"
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
		return nil, nil, "请先发送 `图片` 查看列表，再用 `图片 序号` 选中目标图片。"
	}

	figure := findFigureByID(paper.Figures, figureID)
	if figure == nil {
		b.updateContext(func(state *weixinIMContext) {
			state.CurrentFigureID = 0
		})
		return nil, nil, "当前图片已失效，请重新发送 `图片` 查看列表。"
	}
	return paper, figure, ""
}

func (b *WeixinIMBridge) getContext() weixinIMContext {
	b.mu.Lock()
	defer b.mu.Unlock()

	context := b.context
	context.SearchPaperIDs = append([]int64(nil), context.SearchPaperIDs...)
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

func isWeixinHelpCommand(text string) bool {
	switch strings.TrimSpace(strings.ToLower(text)) {
	case "/help", "/h", "帮助":
		return true
	default:
		return false
	}
}

func weixinHelpText() string {
	return strings.Join([]string{
		"微信 IM 可用命令：",
		"`搜 关键词`：搜索文献",
		"`最近文献`：查看最近几篇文献",
		"`文献 1`：选择检索结果中的文献",
		"`图片`：查看当前文献的图片列表",
		"`图片 1`：选择当前文献中的图片",
		"`笔记 内容`：追加文献/图片笔记",
		"`解读`：解读当前图片",
		"`状态`：查看当前上下文",
		"`重置`：清空当前上下文",
		"选中文献后，直接发送文字或语音即可进入文献问答。",
	}, "\n")
}

func matchWeixinCommand(text, slashCommand, plainPrefix string) (string, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == slashCommand || trimmed == plainPrefix {
		return "", true
	}
	if strings.HasPrefix(trimmed, slashCommand+" ") {
		return strings.TrimSpace(strings.TrimPrefix(trimmed, slashCommand)), true
	}
	if strings.HasPrefix(trimmed, plainPrefix+" ") {
		return strings.TrimSpace(strings.TrimPrefix(trimmed, plainPrefix)), true
	}
	return "", false
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

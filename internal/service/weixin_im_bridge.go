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
	downloadFile   func(context.Context, weixin.MessageItem) (*weixin.DownloadedFile, error)
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
			reply := trimWeixinReply(b.handleIncomingMessage(ctx, message))
			previewPath, previewErr := b.selectedFigurePreviewPath(message, reply)
			if previewErr != nil {
				reply = trimWeixinReply(appendWeixinReplyNotice(reply, "图片已选中，但原图预览不可用。"))
				b.logger.Warn("resolve weixin figure preview failed", "error", previewErr)
			}
			if reply == "" && previewPath == "" {
				b.logger.Info("skip empty weixin reply", "from_user_id", strings.TrimSpace(message.FromUserID))
				continue
			}
			if reply != "" {
				if err := client.SendTextMessage(ctx, message.FromUserID, reply, message.ContextToken); err != nil {
					b.logger.Warn("send weixin reply failed", "error", err)
				}
			}
			if previewPath != "" {
				if err := client.SendImageFile(ctx, message.FromUserID, previewPath, message.ContextToken); err != nil {
					b.logger.Warn("send weixin preview image failed", "error", err, "path", previewPath)
					if err := client.SendTextMessage(ctx, message.FromUserID, "图片已选中，但预览发送失败。", message.ContextToken); err != nil {
						b.logger.Warn("send weixin preview failure notice failed", "error", err)
					}
				}
			}
		}
	}
}

func (b *WeixinIMBridge) handleIncomingMessage(ctx context.Context, message weixin.Message) string {
	if reply, handled := b.handleIncomingFile(ctx, message); handled {
		return reply
	}

	text := extractWeixinText(message)
	if text == "" {
		return ""
	}
	return b.handleIncomingText(ctx, text)
}

func (b *WeixinIMBridge) handleIncomingText(ctx context.Context, text string) string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "／") {
		text = "/" + strings.TrimPrefix(text, "／")
	}
	if text == "" {
		return ""
	}

	command, arg, ok := parseWeixinSlashCommand(text)
	if !ok {
		return weixinHelpText()
	}

	return b.executeWeixinCommand(ctx, command, arg)
}

func (b *WeixinIMBridge) executeWeixinCommand(ctx context.Context, command, arg string) string {
	switch command {
	case "/help", "/h":
		return weixinHelpText()
	case "/status":
		return b.statusText()
	case "/reset":
		b.setContext(weixinIMContext{})
		return "已清空微信上下文。发送 `/search 关键词` 或 `/recent` 开始。"
	case "/figures":
		return b.listFigures()
	case "/recent":
		return b.listRecentPapers()
	case "/search":
		return b.searchPapers(arg)
	case "/paper":
		return b.selectPaper(arg)
	case "/figure":
		return b.selectFigure(arg)
	case "/note":
		return b.appendNote(arg)
	case "/interpret":
		return b.interpretCurrentFigure(ctx, arg)
	case "/ask", "/qa":
		return b.answerCurrentPaper(ctx, arg)
	default:
		return weixinHelpText()
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

func (b *WeixinIMBridge) searchPapers(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return "用法：`/search 关键词`"
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
	lines = append(lines, "", "发送 `/paper 1` 选中目标文献。")
	return strings.Join(lines, "\n")
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
	paper, errText := b.requireCurrentPaper()
	if errText != "" {
		return errText
	}
	figures := topLevelFigures(paper.Figures)

	arg = strings.TrimSpace(arg)
	if arg == "" {
		return "用法：`/figure 序号`"
	}

	value, err := strconv.Atoi(arg)
	if err != nil || value <= 0 {
		return "请输入有效的图片序号。"
	}

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
		return fmt.Sprintf("未找到序号为 %d 的图片。先发送 `图片` 查看列表。", value)
	}

	b.updateContext(func(state *weixinIMContext) {
		state.CurrentFigureID = figure.ID
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
	question = strings.TrimSpace(question)
	if question == "" {
		return "用法：`/ask 你的问题`"
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
		return "当前未选中文献。发送 `/search 关键词` 或 `/recent` 开始。"
	}

	paper, err := b.libraryService.GetPaper(state.CurrentPaperID)
	if err != nil {
		b.setContext(weixinIMContext{})
		return "当前文献上下文已失效，请重新发送 `/search 关键词`。"
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
		}
		state.CurrentPaperID = paperID
		state.CurrentFigureID = 0
		state.QAHistory = nil
	})
}

func (b *WeixinIMBridge) requireCurrentPaper() (*model.Paper, string) {
	state := b.getContext()
	if state.CurrentPaperID == 0 {
		return nil, "请先发送 `/search 关键词` 或 `/recent` 选择文献。"
	}

	paper, err := b.libraryService.GetPaper(state.CurrentPaperID)
	if err != nil {
		b.setContext(weixinIMContext{})
		return nil, "当前文献已失效，请重新发送 `/search 关键词`。"
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
		"微信 IM 仅响应 slash 命令。可用命令：",
		"`/search 关键词`：搜索文献",
		"`/recent`：查看最近几篇文献",
		"`/paper 1`：选择检索结果中的文献",
		"`/figures`：查看当前文献的图片列表",
		"`/figure 1`：选择当前文献中的图片，并回发原图预览",
		"直接发送 PDF：自动导入文献并切换上下文",
		"`/ask 问题` 或 `/qa 问题`：对当前文献提问",
		"`/note 内容`：追加文献/图片笔记",
		"`/interpret 问题`：解读当前图片",
		"`/status`：查看当前上下文",
		"`/reset`：清空当前上下文",
		"`/help`：查看帮助",
		"未识别到 slash 命令时，会直接返回这份帮助，避免误触发 AI 问答。",
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

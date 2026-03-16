package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"paper_image_db/internal/apperr"
	"paper_image_db/internal/config"
	"paper_image_db/internal/model"
	"paper_image_db/internal/repository"
)

const aiSettingsKey = "ai_settings"

type AIService struct {
	repo       *repository.LibraryRepository
	config     *config.Config
	httpClient *http.Client
	logger     *slog.Logger
}

type aiImageInput struct {
	MIMEType string
	Data     string
}

type aiStructuredResult struct {
	Answer         string
	SuggestedTags  []string
	SuggestedGroup string
}

type aiReadPrepared struct {
	settings        model.AISettings
	action          model.AIAction
	question        string
	paper           *model.Paper
	systemPrompt    string
	userPrompt      string
	includedFigures int
	images          []aiImageInput
}

func NewAIService(repo *repository.LibraryRepository, cfg *config.Config, logger *slog.Logger) *AIService {
	if logger == nil {
		logger = slog.Default().With("component", "ai_service")
	}

	return &AIService{
		repo:   repo,
		config: cfg,
		logger: logger,
		httpClient: &http.Client{
			Timeout: 180 * time.Second,
		},
	}
}

func (s *AIService) GetSettings() (*model.AISettings, error) {
	raw, err := s.repo.GetAppSetting(aiSettingsKey)
	if err != nil {
		return nil, err
	}

	settings := model.DefaultAISettings()
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &settings); err != nil {
			return nil, apperr.Wrap(apperr.CodeInternal, "解析 AI 设置失败", err)
		}
	}

	normalized, err := normalizeAISettings(settings)
	if err != nil {
		return nil, err
	}

	return &normalized, nil
}

func (s *AIService) UpdateSettings(input model.AISettings) (*model.AISettings, error) {
	settings, err := normalizeAISettings(input)
	if err != nil {
		return nil, err
	}

	payload, err := json.Marshal(settings)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "序列化 AI 设置失败", err)
	}

	if err := s.repo.UpsertAppSetting(aiSettingsKey, string(payload)); err != nil {
		return nil, err
	}

	return &settings, nil
}

func (s *AIService) ReadPaper(ctx context.Context, input model.AIReadRequest) (*model.AIReadResponse, error) {
	prepared, err := s.prepareRead(input, true)
	if err != nil {
		return nil, err
	}

	rawText, mode, err := s.callProvider(ctx, prepared)
	if err != nil {
		return nil, err
	}

	return buildAIReadResponse(prepared, mode, rawText), nil
}

func (s *AIService) ReadPaperStream(ctx context.Context, input model.AIReadRequest, onEvent func(model.AIReadStreamEvent) error) error {
	prepared, err := s.prepareRead(input, false)
	if err != nil {
		return err
	}
	if prepared.action != model.AIActionFigureInterpretation {
		return apperr.New(apperr.CodeInvalidArgument, "当前只有图片解读支持流式输出")
	}

	mode := aiProviderMode(prepared.settings)
	if err := onEvent(model.AIReadStreamEvent{
		Type: "meta",
		Result: &model.AIReadResponse{
			Success:         true,
			Provider:        prepared.settings.Provider,
			Model:           prepared.settings.Model,
			Mode:            mode,
			Action:          prepared.action,
			PaperID:         prepared.paper.ID,
			Question:        prepared.question,
			IncludedFigures: prepared.includedFigures,
		},
	}); err != nil {
		return err
	}

	rawText, err := s.callProviderStream(ctx, prepared, func(delta string) error {
		if delta == "" {
			return nil
		}
		return onEvent(model.AIReadStreamEvent{
			Type:  "delta",
			Delta: delta,
		})
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		return err
	}

	result := buildAIReadResponse(prepared, mode, rawText)
	result.SuggestedTags = []string{}
	result.SuggestedGroup = ""
	if err := onEvent(model.AIReadStreamEvent{
		Type:   "final",
		Result: result,
	}); err != nil {
		return err
	}
	return onEvent(model.AIReadStreamEvent{Type: "done"})
}

func (s *AIService) prepareRead(input model.AIReadRequest, structuredOutput bool) (*aiReadPrepared, error) {
	if input.PaperID <= 0 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "paper_id 无效")
	}

	settings, err := s.GetSettings()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(settings.APIKey) == "" {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "请先在 AI 页面配置 API Key")
	}

	action := normalizeAIAction(input.Action)
	question := strings.TrimSpace(input.Question)
	if question == "" {
		question = defaultAIQuestion(action)
	}
	history, err := normalizeConversationHistory(action, input.History)
	if err != nil {
		return nil, err
	}

	paper, err := s.repo.GetPaperDetail(input.PaperID)
	if err != nil {
		return nil, err
	}
	if paper == nil {
		return nil, apperr.New(apperr.CodeNotFound, "文献不存在")
	}
	if strings.TrimSpace(paper.PDFText) == "" && len(paper.Figures) == 0 {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "当前文献缺少可供 AI 阅读的正文或图片，请先完成解析")
	}

	groups, err := s.repo.ListGroups()
	if err != nil {
		return nil, err
	}
	tags, err := s.repo.ListTags()
	if err != nil {
		return nil, err
	}

	images, figureSummaries, err := s.loadFigureInputs(paper, settings.MaxFigures)
	if err != nil {
		return nil, err
	}

	systemPrompt, userPrompt := buildAIPrompts(*settings, paper, groups, tags, action, question, history, figureSummaries, len(images), structuredOutput)

	s.logger.Info("ai paper read started",
		"provider", settings.Provider,
		"model", settings.Model,
		"paper_id", paper.ID,
		"action", action,
		"figures", len(images),
	)

	return &aiReadPrepared{
		settings:        *settings,
		action:          action,
		question:        question,
		paper:           paper,
		systemPrompt:    systemPrompt,
		userPrompt:      userPrompt,
		includedFigures: len(images),
		images:          images,
	}, nil
}

func buildAIReadResponse(prepared *aiReadPrepared, mode, rawText string) *model.AIReadResponse {
	parsed := extractStructuredAIResult(rawText)
	answer := parsed.Answer
	if strings.TrimSpace(answer) == "" {
		answer = strings.TrimSpace(rawText)
	}

	return &model.AIReadResponse{
		Success:         true,
		Provider:        prepared.settings.Provider,
		Model:           prepared.settings.Model,
		Mode:            mode,
		Action:          prepared.action,
		PaperID:         prepared.paper.ID,
		Question:        prepared.question,
		Answer:          answer,
		SuggestedTags:   parsed.SuggestedTags,
		SuggestedGroup:  parsed.SuggestedGroup,
		IncludedFigures: prepared.includedFigures,
	}
}

func normalizeAISettings(input model.AISettings) (model.AISettings, error) {
	defaults := model.DefaultAISettings()
	settings := input

	if strings.TrimSpace(string(settings.Provider)) == "" {
		settings.Provider = defaults.Provider
	}
	if !isSupportedAIProvider(settings.Provider) {
		return model.AISettings{}, apperr.New(apperr.CodeInvalidArgument, "暂不支持该 AI 提供商")
	}

	if strings.TrimSpace(settings.BaseURL) == "" {
		settings.BaseURL = defaultAIBaseURL(settings.Provider)
	}
	if strings.TrimSpace(settings.Model) == "" {
		settings.Model = defaultAIModel(settings.Provider)
	}
	if settings.Temperature < 0 || settings.Temperature > 2 {
		return model.AISettings{}, apperr.New(apperr.CodeInvalidArgument, "temperature 必须在 0 到 2 之间")
	}
	if settings.MaxOutputTokens <= 0 {
		settings.MaxOutputTokens = defaults.MaxOutputTokens
	}
	if settings.MaxOutputTokens > 16384 {
		return model.AISettings{}, apperr.New(apperr.CodeInvalidArgument, "max_output_tokens 过大")
	}
	if settings.MaxFigures < 0 {
		return model.AISettings{}, apperr.New(apperr.CodeInvalidArgument, "max_figures 不能为负数")
	}

	if strings.TrimSpace(settings.SystemPrompt) == "" {
		settings.SystemPrompt = defaults.SystemPrompt
	}
	if strings.TrimSpace(settings.QAPrompt) == "" {
		settings.QAPrompt = defaults.QAPrompt
	}
	if strings.TrimSpace(settings.FigurePrompt) == "" {
		settings.FigurePrompt = defaults.FigurePrompt
	}
	if strings.TrimSpace(settings.TagPrompt) == "" {
		settings.TagPrompt = defaults.TagPrompt
	}
	if strings.TrimSpace(settings.GroupPrompt) == "" {
		settings.GroupPrompt = defaults.GroupPrompt
	}
	if settings.Provider != model.AIProviderOpenAI {
		settings.OpenAILegacyMode = false
	}

	settings.BaseURL = strings.TrimRight(strings.TrimSpace(settings.BaseURL), "/")
	settings.Model = strings.TrimSpace(settings.Model)
	settings.APIKey = strings.TrimSpace(settings.APIKey)
	settings.SystemPrompt = strings.TrimSpace(settings.SystemPrompt)
	settings.QAPrompt = strings.TrimSpace(settings.QAPrompt)
	settings.FigurePrompt = strings.TrimSpace(settings.FigurePrompt)
	settings.TagPrompt = strings.TrimSpace(settings.TagPrompt)
	settings.GroupPrompt = strings.TrimSpace(settings.GroupPrompt)

	return settings, nil
}

func buildAIPrompts(
	settings model.AISettings,
	paper *model.Paper,
	groups []model.Group,
	tags []model.Tag,
	action model.AIAction,
	question string,
	history []model.AIConversationTurn,
	figureSummaries []string,
	includedFigures int,
	structuredOutput bool,
) (string, string) {
	groupName := "未分组"
	if strings.TrimSpace(paper.GroupName) != "" {
		groupName = paper.GroupName
	}

	tagNames := make([]string, 0, len(paper.Tags))
	for _, tag := range paper.Tags {
		tagNames = append(tagNames, tag.Name)
	}

	fullText := strings.TrimSpace(paper.PDFText)
	if fullText == "" {
		fullText = "未提取到正文内容。"
	}

	abstractText := strings.TrimSpace(paper.AbstractText)
	if abstractText == "" {
		abstractText = "无"
	}
	notesText := strings.TrimSpace(paper.NotesText)
	if notesText == "" {
		notesText = "无"
	}

	figureSection := "未提取到图片。"
	if len(figureSummaries) > 0 {
		figureSection = strings.Join(figureSummaries, "\n")
	}

	existingTagNames := "当前库中还没有标签。"
	if len(tags) > 0 {
		names := make([]string, 0, len(tags))
		for _, tag := range tags {
			names = append(names, tag.Name)
		}
		existingTagNames = strings.Join(names, "，")
	}

	existingGroupNames := "当前库中还没有分组。"
	if len(groups) > 0 {
		names := make([]string, 0, len(groups))
		for _, group := range groups {
			if description := strings.TrimSpace(group.Description); description != "" {
				names = append(names, fmt.Sprintf("%s（%s）", group.Name, description))
			} else {
				names = append(names, group.Name)
			}
		}
		existingGroupNames = strings.Join(names, "；")
	}

	conversationSection := ""
	if action == model.AIActionPaperQA {
		conversationSection = buildConversationSection(history)
	}

	outputRequirements := strings.TrimSpace(aiOutputRequirements(action, structuredOutput))

	userPrompt := fmt.Sprintf(`任务类型: %s

论文信息:
- 标题: %s
- 原始文件名: %s
- 当前分组: %s
- 当前标签: %s
- 摘要: %s
- 备注: %s
- 本次附带图片数: %d

图片列表:
%s

现有标签库:
%s

现有分组库:
%s

用户问题:
%s

历史对话:
%s

动作提示:
%s

全文:
%s

输出要求:
%s`,
		action,
		paper.Title,
		paper.OriginalFilename,
		groupName,
		joinOrFallback(tagNames, "无"),
		abstractText,
		notesText,
		includedFigures,
		figureSection,
		existingTagNames,
		existingGroupNames,
		question,
		conversationSection,
		actionPromptFor(settings, action),
		fullText,
		outputRequirements,
	)

	return settings.SystemPrompt, userPrompt
}

func aiOutputRequirements(action model.AIAction, structuredOutput bool) string {
	if structuredOutput {
		return `1. 只返回 JSON 对象，不要使用 Markdown 代码块。
2. JSON 必须包含 answer、suggested_tags、suggested_group 三个字段。
3. 如果当前任务不需要标签建议或分组建议，请分别返回空数组 [] 和空字符串 ""。
4. answer 中请直接给出结论、依据和必要的限制说明。`
	}

	if action == model.AIActionFigureInterpretation {
		return `1. 直接输出自然语言正文，不要返回 JSON、代码块或额外元数据。
2. 优先围绕当前图片说明图像内容、支撑结论、与全文主线的关系以及局限。
3. 尽量分成短段落，保证可以逐段流式阅读。`
	}

	return `1. 直接输出自然语言正文，不要返回 JSON、代码块或额外元数据。`
}

func actionPromptFor(settings model.AISettings, action model.AIAction) string {
	switch action {
	case model.AIActionFigureInterpretation:
		return settings.FigurePrompt
	case model.AIActionTagSuggestion:
		return settings.TagPrompt
	case model.AIActionGroupSuggestion:
		return settings.GroupPrompt
	default:
		return settings.QAPrompt
	}
}

func normalizeAIAction(action model.AIAction) model.AIAction {
	switch action {
	case model.AIActionFigureInterpretation, model.AIActionTagSuggestion, model.AIActionGroupSuggestion, model.AIActionPaperQA:
		return action
	default:
		return model.AIActionPaperQA
	}
}

func defaultAIQuestion(action model.AIAction) string {
	switch action {
	case model.AIActionFigureInterpretation:
		return "请围绕这篇文章的关键图片做一次重点解读。"
	case model.AIActionTagSuggestion:
		return "请为这篇文献生成一组适合检索和归档的标签。"
	case model.AIActionGroupSuggestion:
		return "请判断这篇文献最适合放到哪个分组。"
	default:
		return "请概括这篇文献的核心问题、方法、主要结论和证据。"
	}
}

func normalizeConversationHistory(action model.AIAction, input []model.AIConversationTurn) ([]model.AIConversationTurn, error) {
	if action != model.AIActionPaperQA {
		return nil, nil
	}

	history := make([]model.AIConversationTurn, 0, len(input))
	for _, turn := range input {
		question := strings.TrimSpace(turn.Question)
		answer := strings.TrimSpace(turn.Answer)
		if question == "" || answer == "" {
			continue
		}
		history = append(history, model.AIConversationTurn{
			Question: question,
			Answer:   answer,
		})
	}
	if len(history) > 4 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "自由提问最多支持 5 轮对话")
	}
	return history, nil
}

func buildConversationSection(history []model.AIConversationTurn) string {
	if len(history) == 0 {
		return "这是当前会话的第一轮提问。"
	}

	lines := make([]string, 0, len(history)*2)
	for index, turn := range history {
		lines = append(lines, fmt.Sprintf("第 %d 轮用户: %s", index+1, turn.Question))
		lines = append(lines, fmt.Sprintf("第 %d 轮助手: %s", index+1, turn.Answer))
	}
	return strings.Join(lines, "\n")
}

func (s *AIService) loadFigureInputs(paper *model.Paper, maxFigures int) ([]aiImageInput, []string, error) {
	figures := paper.Figures
	if maxFigures > 0 && len(figures) > maxFigures {
		figures = figures[:maxFigures]
	}

	images := make([]aiImageInput, 0, len(figures))
	summaries := make([]string, 0, len(figures))
	for _, figure := range figures {
		summary := fmt.Sprintf("- 第 %d 页图 %d：caption=%s", figure.PageNumber, figure.FigureIndex, fallbackText(strings.TrimSpace(figure.Caption), "无"))
		summaries = append(summaries, summary)

		if strings.TrimSpace(figure.Filename) == "" {
			continue
		}

		imagePath := filepath.Join(s.config.FiguresDir(), figure.Filename)
		data, err := os.ReadFile(imagePath)
		if err != nil {
			if os.IsNotExist(err) {
				s.logger.Warn("ai figure file missing", "paper_id", paper.ID, "filename", figure.Filename)
				continue
			}
			return nil, nil, apperr.Wrap(apperr.CodeInternal, "读取图片文件失败", err)
		}

		mimeType := strings.TrimSpace(figure.ContentType)
		if mimeType == "" {
			mimeType = mime.TypeByExtension(strings.ToLower(filepath.Ext(figure.Filename)))
		}
		if mimeType == "" {
			mimeType = "image/png"
		}

		images = append(images, aiImageInput{
			MIMEType: mimeType,
			Data:     base64.StdEncoding.EncodeToString(data),
		})
	}

	return images, summaries, nil
}

func (s *AIService) callProvider(ctx context.Context, prepared *aiReadPrepared) (string, string, error) {
	mode := aiProviderMode(prepared.settings)
	switch prepared.settings.Provider {
	case model.AIProviderOpenAI:
		if prepared.settings.OpenAILegacyMode {
			text, err := s.callOpenAIChatCompletions(ctx, prepared.settings, prepared.systemPrompt, prepared.userPrompt, prepared.images)
			return text, mode, err
		}
		text, err := s.callOpenAIResponses(ctx, prepared.settings, prepared.systemPrompt, prepared.userPrompt, prepared.images)
		return text, mode, err
	case model.AIProviderAnthropic:
		text, err := s.callAnthropicMessages(ctx, prepared.settings, prepared.systemPrompt, prepared.userPrompt, prepared.images)
		return text, mode, err
	case model.AIProviderGemini:
		text, err := s.callGeminiGenerateContent(ctx, prepared.settings, prepared.systemPrompt, prepared.userPrompt, prepared.images)
		return text, mode, err
	default:
		return "", "", apperr.New(apperr.CodeInvalidArgument, "暂不支持该 AI 提供商")
	}
}

func (s *AIService) callProviderStream(ctx context.Context, prepared *aiReadPrepared, onDelta func(string) error) (string, error) {
	switch prepared.settings.Provider {
	case model.AIProviderOpenAI:
		if prepared.settings.OpenAILegacyMode {
			return s.callOpenAIChatCompletionsStream(ctx, prepared.settings, prepared.systemPrompt, prepared.userPrompt, prepared.images, onDelta)
		}
		return s.callOpenAIResponsesStream(ctx, prepared.settings, prepared.systemPrompt, prepared.userPrompt, prepared.images, onDelta)
	case model.AIProviderAnthropic:
		return s.callAnthropicMessagesStream(ctx, prepared.settings, prepared.systemPrompt, prepared.userPrompt, prepared.images, onDelta)
	case model.AIProviderGemini:
		return s.callGeminiGenerateContentStream(ctx, prepared.settings, prepared.systemPrompt, prepared.userPrompt, prepared.images, onDelta)
	default:
		return "", apperr.New(apperr.CodeInvalidArgument, "暂不支持该 AI 提供商")
	}
}

func aiProviderMode(settings model.AISettings) string {
	switch settings.Provider {
	case model.AIProviderOpenAI:
		if settings.OpenAILegacyMode {
			return "chat_completions"
		}
		return "responses"
	case model.AIProviderAnthropic:
		return "messages"
	case model.AIProviderGemini:
		return "generate_content"
	default:
		return ""
	}
}

func (s *AIService) callOpenAIResponses(ctx context.Context, settings model.AISettings, systemPrompt, userPrompt string, images []aiImageInput) (string, error) {
	content := []map[string]interface{}{
		{"type": "input_text", "text": userPrompt},
	}
	for _, image := range images {
		content = append(content, map[string]interface{}{
			"type":      "input_image",
			"image_url": "data:" + image.MIMEType + ";base64," + image.Data,
		})
	}

	payload := map[string]interface{}{
		"model":             settings.Model,
		"instructions":      systemPrompt,
		"input":             []map[string]interface{}{{"role": "user", "content": content}},
		"temperature":       settings.Temperature,
		"max_output_tokens": settings.MaxOutputTokens,
	}

	body, err := s.postJSON(
		ctx,
		joinProviderURL(settings.BaseURL, defaultAIBaseURL(model.AIProviderOpenAI), "/v1/responses"),
		map[string]string{
			"Authorization": "Bearer " + settings.APIKey,
		},
		payload,
	)
	if err != nil {
		return "", err
	}

	var response struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", apperr.Wrap(apperr.CodeUnavailable, "解析 OpenAI Responses 响应失败", err)
	}
	if strings.TrimSpace(response.OutputText) != "" {
		return response.OutputText, nil
	}
	for _, item := range response.Output {
		for _, content := range item.Content {
			if strings.TrimSpace(content.Text) != "" {
				return content.Text, nil
			}
		}
	}
	return "", apperr.New(apperr.CodeUnavailable, "OpenAI Responses 未返回文本内容")
}

func (s *AIService) callOpenAIChatCompletions(ctx context.Context, settings model.AISettings, systemPrompt, userPrompt string, images []aiImageInput) (string, error) {
	userContent := []map[string]interface{}{
		{"type": "text", "text": userPrompt},
	}
	for _, image := range images {
		userContent = append(userContent, map[string]interface{}{
			"type": "image_url",
			"image_url": map[string]interface{}{
				"url": "data:" + image.MIMEType + ";base64," + image.Data,
			},
		})
	}

	payload := map[string]interface{}{
		"model": settings.Model,
		"messages": []map[string]interface{}{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userContent},
		},
		"temperature": settings.Temperature,
		"max_tokens":  settings.MaxOutputTokens,
	}

	body, err := s.postJSON(
		ctx,
		joinProviderURL(settings.BaseURL, defaultAIBaseURL(model.AIProviderOpenAI), "/v1/chat/completions"),
		map[string]string{
			"Authorization": "Bearer " + settings.APIKey,
		},
		payload,
	)
	if err != nil {
		return "", err
	}

	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", apperr.Wrap(apperr.CodeUnavailable, "解析 OpenAI Chat Completions 响应失败", err)
	}
	if len(response.Choices) == 0 || strings.TrimSpace(response.Choices[0].Message.Content) == "" {
		return "", apperr.New(apperr.CodeUnavailable, "OpenAI Chat Completions 未返回文本内容")
	}
	return response.Choices[0].Message.Content, nil
}

func (s *AIService) callAnthropicMessages(ctx context.Context, settings model.AISettings, systemPrompt, userPrompt string, images []aiImageInput) (string, error) {
	content := []map[string]interface{}{
		{"type": "text", "text": userPrompt},
	}
	for _, image := range images {
		content = append(content, map[string]interface{}{
			"type": "image",
			"source": map[string]interface{}{
				"type":       "base64",
				"media_type": image.MIMEType,
				"data":       image.Data,
			},
		})
	}

	payload := map[string]interface{}{
		"model":       settings.Model,
		"max_tokens":  settings.MaxOutputTokens,
		"temperature": settings.Temperature,
		"system":      systemPrompt,
		"messages": []map[string]interface{}{
			{"role": "user", "content": content},
		},
	}

	body, err := s.postJSON(
		ctx,
		joinProviderURL(settings.BaseURL, defaultAIBaseURL(model.AIProviderAnthropic), "/v1/messages"),
		map[string]string{
			"x-api-key":         settings.APIKey,
			"anthropic-version": "2023-06-01",
		},
		payload,
	)
	if err != nil {
		return "", err
	}

	var response struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", apperr.Wrap(apperr.CodeUnavailable, "解析 Anthropic 响应失败", err)
	}
	for _, item := range response.Content {
		if strings.TrimSpace(item.Text) != "" {
			return item.Text, nil
		}
	}
	return "", apperr.New(apperr.CodeUnavailable, "Anthropic 未返回文本内容")
}

func (s *AIService) callGeminiGenerateContent(ctx context.Context, settings model.AISettings, systemPrompt, userPrompt string, images []aiImageInput) (string, error) {
	parts := []map[string]interface{}{
		{"text": userPrompt},
	}
	for _, image := range images {
		parts = append(parts, map[string]interface{}{
			"inline_data": map[string]interface{}{
				"mime_type": image.MIMEType,
				"data":      image.Data,
			},
		})
	}

	payload := map[string]interface{}{
		"system_instruction": map[string]interface{}{
			"parts": []map[string]interface{}{
				{"text": systemPrompt},
			},
		},
		"contents": []map[string]interface{}{
			{"parts": parts},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     settings.Temperature,
			"maxOutputTokens": settings.MaxOutputTokens,
		},
	}

	endpoint := joinProviderURL(settings.BaseURL, defaultAIBaseURL(model.AIProviderGemini), "/v1beta/models/"+url.PathEscape(settings.Model)+":generateContent")
	endpointWithKey, err := addQuery(endpoint, "key", settings.APIKey)
	if err != nil {
		return "", apperr.Wrap(apperr.CodeInvalidArgument, "Gemini Base URL 无效", err)
	}

	body, err := s.postJSON(ctx, endpointWithKey, nil, payload)
	if err != nil {
		return "", err
	}

	var response struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", apperr.Wrap(apperr.CodeUnavailable, "解析 Gemini 响应失败", err)
	}
	for _, candidate := range response.Candidates {
		for _, part := range candidate.Content.Parts {
			if strings.TrimSpace(part.Text) != "" {
				return part.Text, nil
			}
		}
	}
	return "", apperr.New(apperr.CodeUnavailable, "Gemini 未返回文本内容")
}

func (s *AIService) callOpenAIResponsesStream(ctx context.Context, settings model.AISettings, systemPrompt, userPrompt string, images []aiImageInput, onDelta func(string) error) (string, error) {
	content := []map[string]interface{}{
		{"type": "input_text", "text": userPrompt},
	}
	for _, image := range images {
		content = append(content, map[string]interface{}{
			"type":      "input_image",
			"image_url": "data:" + image.MIMEType + ";base64," + image.Data,
		})
	}

	payload := map[string]interface{}{
		"model":             settings.Model,
		"instructions":      systemPrompt,
		"input":             []map[string]interface{}{{"role": "user", "content": content}},
		"temperature":       settings.Temperature,
		"max_output_tokens": settings.MaxOutputTokens,
		"stream":            true,
	}

	var raw strings.Builder
	err := s.postJSONStream(
		ctx,
		joinProviderURL(settings.BaseURL, defaultAIBaseURL(model.AIProviderOpenAI), "/v1/responses"),
		map[string]string{
			"Authorization": "Bearer " + settings.APIKey,
		},
		payload,
		func(eventType, data string) error {
			delta, err := extractOpenAIResponsesStreamDelta(eventType, data)
			if err != nil {
				return err
			}
			if delta == "" {
				return nil
			}
			raw.WriteString(delta)
			return onDelta(delta)
		},
	)
	if err != nil {
		return "", err
	}
	return raw.String(), nil
}

func (s *AIService) callOpenAIChatCompletionsStream(ctx context.Context, settings model.AISettings, systemPrompt, userPrompt string, images []aiImageInput, onDelta func(string) error) (string, error) {
	userContent := []map[string]interface{}{
		{"type": "text", "text": userPrompt},
	}
	for _, image := range images {
		userContent = append(userContent, map[string]interface{}{
			"type": "image_url",
			"image_url": map[string]interface{}{
				"url": "data:" + image.MIMEType + ";base64," + image.Data,
			},
		})
	}

	payload := map[string]interface{}{
		"model": settings.Model,
		"messages": []map[string]interface{}{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userContent},
		},
		"temperature": settings.Temperature,
		"max_tokens":  settings.MaxOutputTokens,
		"stream":      true,
	}

	var raw strings.Builder
	err := s.postJSONStream(
		ctx,
		joinProviderURL(settings.BaseURL, defaultAIBaseURL(model.AIProviderOpenAI), "/v1/chat/completions"),
		map[string]string{
			"Authorization": "Bearer " + settings.APIKey,
		},
		payload,
		func(_ string, data string) error {
			delta, err := extractOpenAIChatCompletionsStreamDelta(data)
			if err != nil {
				return err
			}
			if delta == "" {
				return nil
			}
			raw.WriteString(delta)
			return onDelta(delta)
		},
	)
	if err != nil {
		return "", err
	}
	return raw.String(), nil
}

func (s *AIService) callAnthropicMessagesStream(ctx context.Context, settings model.AISettings, systemPrompt, userPrompt string, images []aiImageInput, onDelta func(string) error) (string, error) {
	content := []map[string]interface{}{
		{"type": "text", "text": userPrompt},
	}
	for _, image := range images {
		content = append(content, map[string]interface{}{
			"type": "image",
			"source": map[string]interface{}{
				"type":       "base64",
				"media_type": image.MIMEType,
				"data":       image.Data,
			},
		})
	}

	payload := map[string]interface{}{
		"model":       settings.Model,
		"max_tokens":  settings.MaxOutputTokens,
		"temperature": settings.Temperature,
		"system":      systemPrompt,
		"stream":      true,
		"messages": []map[string]interface{}{
			{"role": "user", "content": content},
		},
	}

	var raw strings.Builder
	err := s.postJSONStream(
		ctx,
		joinProviderURL(settings.BaseURL, defaultAIBaseURL(model.AIProviderAnthropic), "/v1/messages"),
		map[string]string{
			"x-api-key":         settings.APIKey,
			"anthropic-version": "2023-06-01",
		},
		payload,
		func(eventType, data string) error {
			delta, err := extractAnthropicMessagesStreamDelta(eventType, data)
			if err != nil {
				return err
			}
			if delta == "" {
				return nil
			}
			raw.WriteString(delta)
			return onDelta(delta)
		},
	)
	if err != nil {
		return "", err
	}
	return raw.String(), nil
}

func (s *AIService) callGeminiGenerateContentStream(ctx context.Context, settings model.AISettings, systemPrompt, userPrompt string, images []aiImageInput, onDelta func(string) error) (string, error) {
	parts := []map[string]interface{}{
		{"text": userPrompt},
	}
	for _, image := range images {
		parts = append(parts, map[string]interface{}{
			"inline_data": map[string]interface{}{
				"mime_type": image.MIMEType,
				"data":      image.Data,
			},
		})
	}

	payload := map[string]interface{}{
		"system_instruction": map[string]interface{}{
			"parts": []map[string]interface{}{
				{"text": systemPrompt},
			},
		},
		"contents": []map[string]interface{}{
			{"parts": parts},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     settings.Temperature,
			"maxOutputTokens": settings.MaxOutputTokens,
		},
	}

	endpoint := joinProviderURL(settings.BaseURL, defaultAIBaseURL(model.AIProviderGemini), "/v1beta/models/"+url.PathEscape(settings.Model)+":streamGenerateContent")
	endpointWithKey, err := addQuery(endpoint, "key", settings.APIKey)
	if err != nil {
		return "", apperr.Wrap(apperr.CodeInvalidArgument, "Gemini Base URL 无效", err)
	}
	endpointWithAlt, err := addQuery(endpointWithKey, "alt", "sse")
	if err != nil {
		return "", apperr.Wrap(apperr.CodeInvalidArgument, "Gemini 流式 URL 无效", err)
	}

	var raw strings.Builder
	err = s.postJSONStream(ctx, endpointWithAlt, nil, payload, func(_ string, data string) error {
		chunk, err := extractGeminiStreamChunk(data)
		if err != nil {
			return err
		}
		delta := diffAccumulatedChunk(raw.String(), chunk)
		if delta == "" {
			return nil
		}
		raw.WriteString(delta)
		return onDelta(delta)
	})
	if err != nil {
		return "", err
	}
	return raw.String(), nil
}

func (s *AIService) postJSON(ctx context.Context, endpoint string, headers map[string]string, payload interface{}) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "序列化 AI 请求失败", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "创建 AI 请求失败", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeUnavailable, "调用 AI 接口失败", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeUnavailable, "读取 AI 接口响应失败", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, apperr.New(apperr.CodeUnavailable, fmt.Sprintf("AI 接口返回 %d: %s", resp.StatusCode, extractProviderError(respBody)))
	}

	return respBody, nil
}

func (s *AIService) postJSONStream(ctx context.Context, endpoint string, headers map[string]string, payload interface{}, onEvent func(eventType, data string) error) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return apperr.Wrap(apperr.CodeInternal, "序列化 AI 流式请求失败", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return apperr.Wrap(apperr.CodeInternal, "创建 AI 流式请求失败", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return context.Canceled
		}
		return apperr.Wrap(apperr.CodeUnavailable, "调用 AI 流式接口失败", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return apperr.Wrap(apperr.CodeUnavailable, "读取 AI 流式接口错误响应失败", readErr)
		}
		return apperr.New(apperr.CodeUnavailable, fmt.Sprintf("AI 接口返回 %d: %s", resp.StatusCode, extractProviderError(respBody)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	eventType := ""
	dataLines := make([]string, 0, 4)
	flushEvent := func() error {
		if eventType == "" && len(dataLines) == 0 {
			return nil
		}
		data := strings.Join(dataLines, "\n")
		eventType = strings.TrimSpace(eventType)
		dataLines = dataLines[:0]
		currentEvent := eventType
		eventType = ""
		if strings.TrimSpace(data) == "" {
			return nil
		}
		return onEvent(currentEvent, data)
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := flushEvent(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return context.Canceled
		}
		return apperr.Wrap(apperr.CodeUnavailable, "读取 AI 流式响应失败", err)
	}
	return flushEvent()
}

func extractOpenAIResponsesStreamDelta(eventType, data string) (string, error) {
	if strings.TrimSpace(data) == "" || strings.TrimSpace(data) == "[DONE]" {
		return "", nil
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return "", apperr.Wrap(apperr.CodeUnavailable, "解析 OpenAI Responses 流式事件失败", err)
	}

	typeName := firstString(payload["type"])
	if eventType == "" {
		eventType = typeName
	}
	if strings.HasSuffix(eventType, "output_text.delta") || strings.HasSuffix(typeName, "output_text.delta") {
		if delta, ok := payload["delta"].(string); ok {
			return delta, nil
		}
		if text, ok := payload["text"].(string); ok {
			return text, nil
		}
	}
	return "", nil
}

func extractOpenAIChatCompletionsStreamDelta(data string) (string, error) {
	if strings.TrimSpace(data) == "" || strings.TrimSpace(data) == "[DONE]" {
		return "", nil
	}

	var payload struct {
		Choices []struct {
			Delta struct {
				Content interface{} `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return "", apperr.Wrap(apperr.CodeUnavailable, "解析 OpenAI Chat Completions 流式事件失败", err)
	}
	if len(payload.Choices) == 0 {
		return "", nil
	}
	return stringifyContentDelta(payload.Choices[0].Delta.Content), nil
}

func extractAnthropicMessagesStreamDelta(eventType, data string) (string, error) {
	if strings.TrimSpace(data) == "" || strings.TrimSpace(data) == "[DONE]" {
		return "", nil
	}
	if eventType != "content_block_delta" {
		return "", nil
	}

	var payload struct {
		Delta struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta"`
	}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return "", apperr.Wrap(apperr.CodeUnavailable, "解析 Anthropic 流式事件失败", err)
	}
	if payload.Delta.Type != "text_delta" {
		return "", nil
	}
	return payload.Delta.Text, nil
}

func extractGeminiStreamChunk(data string) (string, error) {
	if strings.TrimSpace(data) == "" || strings.TrimSpace(data) == "[DONE]" {
		return "", nil
	}

	var payload struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return "", apperr.Wrap(apperr.CodeUnavailable, "解析 Gemini 流式事件失败", err)
	}

	parts := make([]string, 0, 4)
	for _, candidate := range payload.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				parts = append(parts, part.Text)
			}
		}
	}
	return strings.Join(parts, ""), nil
}

func diffAccumulatedChunk(accumulated, chunk string) string {
	if chunk == "" {
		return ""
	}
	if accumulated != "" && strings.HasPrefix(chunk, accumulated) {
		return chunk[len(accumulated):]
	}
	return chunk
}

func stringifyContentDelta(content interface{}) string {
	switch typed := content.(type) {
	case string:
		return typed
	case []interface{}:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if segment, ok := item.(map[string]interface{}); ok {
				if firstString(segment["type"]) == "text" {
					if text, ok := segment["text"].(string); ok && text != "" {
						parts = append(parts, text)
					}
				}
			}
		}
		return strings.Join(parts, "")
	default:
		return ""
	}
}

func extractStructuredAIResult(text string) aiStructuredResult {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return aiStructuredResult{}
	}

	for _, candidate := range []string{
		trimmed,
		trimCodeFence(trimmed),
		trimJSONObject(trimmed),
	} {
		result, ok := parseStructuredAIJSON(candidate)
		if ok {
			return result
		}
	}

	return aiStructuredResult{Answer: trimmed}
}

func parseStructuredAIJSON(candidate string) (aiStructuredResult, bool) {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return aiStructuredResult{}, false
	}

	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(candidate), &raw); err != nil {
		return aiStructuredResult{}, false
	}

	result := aiStructuredResult{
		Answer:         firstString(raw["answer"], raw["response"], raw["analysis"]),
		SuggestedTags:  toStringSlice(raw["suggested_tags"], raw["tags"]),
		SuggestedGroup: firstString(raw["suggested_group"], raw["group"]),
	}
	return result, true
}

func trimCodeFence(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "```") {
		return ""
	}
	lines := strings.Split(text, "\n")
	if len(lines) < 3 {
		return ""
	}
	return strings.Join(lines[1:len(lines)-1], "\n")
}

func trimJSONObject(text string) string {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return ""
	}
	return text[start : end+1]
}

func extractProviderError(body []byte) string {
	message := strings.TrimSpace(string(body))

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return fallbackText(message, "未知错误")
	}

	if value := firstString(
		payload["message"],
		payload["error"],
		payload["detail"],
	); value != "" {
		return value
	}

	if nested, ok := payload["error"].(map[string]interface{}); ok {
		if value := firstString(nested["message"], nested["type"]); value != "" {
			return value
		}
	}

	return fallbackText(message, "未知错误")
}

func joinProviderURL(baseURL, defaultBase, endpoint string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = defaultBase
	}

	switch {
	case strings.HasSuffix(base, "/v1") && strings.HasPrefix(endpoint, "/v1/"):
		return base + strings.TrimPrefix(endpoint, "/v1")
	case strings.HasSuffix(base, "/v1beta") && strings.HasPrefix(endpoint, "/v1beta/"):
		return base + strings.TrimPrefix(endpoint, "/v1beta")
	default:
		return base + endpoint
	}
}

func addQuery(rawURL, key, value string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set(key, value)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func isSupportedAIProvider(provider model.AIProvider) bool {
	switch provider {
	case model.AIProviderOpenAI, model.AIProviderAnthropic, model.AIProviderGemini:
		return true
	default:
		return false
	}
}

func defaultAIBaseURL(provider model.AIProvider) string {
	switch provider {
	case model.AIProviderAnthropic:
		return "https://api.anthropic.com"
	case model.AIProviderGemini:
		return "https://generativelanguage.googleapis.com"
	default:
		return "https://api.openai.com"
	}
}

func defaultAIModel(provider model.AIProvider) string {
	switch provider {
	case model.AIProviderAnthropic:
		return "claude-3-7-sonnet-latest"
	case model.AIProviderGemini:
		return "gemini-2.5-flash"
	default:
		return "gpt-4.1-mini"
	}
}

func firstString(values ...interface{}) string {
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return strings.TrimSpace(typed)
			}
		case map[string]interface{}:
			if name := firstString(typed["name"], typed["title"]); name != "" {
				return name
			}
		}
	}
	return ""
}

func toStringSlice(values ...interface{}) []string {
	for _, value := range values {
		switch typed := value.(type) {
		case []interface{}:
			result := make([]string, 0, len(typed))
			for _, item := range typed {
				if text := firstString(item); text != "" {
					result = append(result, text)
				}
			}
			if len(result) > 0 {
				return result
			}
		case []string:
			result := make([]string, 0, len(typed))
			for _, item := range typed {
				if trimmed := strings.TrimSpace(item); trimmed != "" {
					result = append(result, trimmed)
				}
			}
			if len(result) > 0 {
				return result
			}
		case string:
			parts := strings.Split(typed, ",")
			result := make([]string, 0, len(parts))
			for _, part := range parts {
				if trimmed := strings.TrimSpace(part); trimmed != "" {
					result = append(result, trimmed)
				}
			}
			if len(result) > 0 {
				return result
			}
		}
	}
	return []string{}
}

func joinOrFallback(values []string, fallback string) string {
	if len(values) == 0 {
		return fallback
	}
	return strings.Join(values, "，")
}

func fallbackText(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

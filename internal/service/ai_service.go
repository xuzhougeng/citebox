package service

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/config"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"

	_ "image/gif"
	_ "image/png"
)

const (
	aiSettingsKey                = "ai_settings"
	aiRolePromptsKey             = "ai_prompt_presets"
	aiFigureImageMaxBytes        = 3 * 1024 * 1024
	aiFigureImageTotalBudget     = 12 * 1024 * 1024
	aiFigureImageMaxDimension    = 2200
	aiFigureImageMinDimension    = 960
	aiFigureImageJPEGQuality     = 82
	aiFigureImageMinJPEGQuality  = 58
	aiFigureImageCompressionRuns = 6
)

var aiMarkdownFigureReferencePattern = regexp.MustCompile(`!\[([^\]]*)\]\(figure://([0-9]+)\)`)

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

type legacyAIPromptPreset struct {
	Name            string `json:"name"`
	SystemPrompt    string `json:"system_prompt"`
	QAPrompt        string `json:"qa_prompt"`
	FigurePrompt    string `json:"figure_prompt"`
	TagPrompt       string `json:"tag_prompt"`
	GroupPrompt     string `json:"group_prompt"`
	TranslatePrompt string `json:"translate_prompt"`
}

type aiReadPrepared struct {
	settings          model.AISettings
	action            model.AIAction
	question          string
	promptQuestion    string
	activeRolePrompts []model.AIRolePrompt
	paper             *model.Paper
	systemPrompt      string
	userPrompt        string
	includedFigures   int
	images            []aiImageInput
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
	rolePrompts, err := s.GetRolePrompts()
	if err != nil {
		return nil, err
	}
	normalized.RolePrompts = rolePrompts

	return &normalized, nil
}

func (s *AIService) UpdateSettings(input model.AISettings) (*model.AISettings, error) {
	saveRolePrompts := input.RolePrompts != nil
	var rolePrompts []model.AIRolePrompt
	var err error
	if saveRolePrompts {
		rolePrompts, err = normalizeAIRolePrompts(input.RolePrompts)
		if err != nil {
			return nil, err
		}
	}

	input.RolePrompts = nil
	settings, err := normalizeAISettings(input)
	if err != nil {
		return nil, err
	}

	storedSettings := settings
	storedSettings.RolePrompts = nil

	payload, err := json.Marshal(storedSettings)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "序列化 AI 设置失败", err)
	}

	if err := s.repo.UpsertAppSetting(aiSettingsKey, string(payload)); err != nil {
		return nil, err
	}

	if saveRolePrompts {
		if _, err := s.UpdateRolePrompts(rolePrompts); err != nil {
			return nil, err
		}
		settings.RolePrompts = rolePrompts
		return &settings, nil
	}

	existingRolePrompts, err := s.GetRolePrompts()
	if err != nil {
		return nil, err
	}
	settings.RolePrompts = existingRolePrompts

	return &settings, nil
}

func (s *AIService) UpdateModelSettings(input model.AIModelSettingsUpdate) (*model.AISettings, error) {
	current, err := s.GetSettings()
	if err != nil {
		return nil, err
	}

	next := *current
	next.Models = input.Models
	next.SceneModels = input.SceneModels
	next.Temperature = input.Temperature
	next.MaxFigures = input.MaxFigures
	next.Translation = input.Translation
	next.RolePrompts = nil

	return s.UpdateSettings(next)
}

func (s *AIService) UpdatePromptSettings(input model.AIPromptSettingsUpdate) (*model.AISettings, error) {
	current, err := s.GetSettings()
	if err != nil {
		return nil, err
	}

	next := *current
	next.SystemPrompt = input.SystemPrompt
	next.QAPrompt = input.QAPrompt
	next.FigurePrompt = input.FigurePrompt
	next.TagPrompt = input.TagPrompt
	next.GroupPrompt = input.GroupPrompt
	next.TranslatePrompt = input.TranslatePrompt
	next.RolePrompts = nil

	return s.UpdateSettings(next)
}

func (s *AIService) GetRolePrompts() ([]model.AIRolePrompt, error) {
	raw, err := s.repo.GetAppSetting(aiRolePromptsKey)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw) == "" {
		return []model.AIRolePrompt{}, nil
	}

	rolePrompts, err := parseStoredAIRolePrompts(raw)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "解析角色 Prompt 失败", err)
	}
	return rolePrompts, nil
}

func (s *AIService) UpdateRolePrompts(input []model.AIRolePrompt) ([]model.AIRolePrompt, error) {
	rolePrompts, err := normalizeAIRolePrompts(input)
	if err != nil {
		return nil, err
	}
	if len(rolePrompts) == 0 {
		if err := s.repo.DeleteAppSetting(aiRolePromptsKey); err != nil {
			return nil, err
		}
		return []model.AIRolePrompt{}, nil
	}

	payload, err := json.Marshal(rolePrompts)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "序列化角色 Prompt 失败", err)
	}
	if err := s.repo.UpsertAppSetting(aiRolePromptsKey, string(payload)); err != nil {
		return nil, err
	}
	return rolePrompts, nil
}

func (s *AIService) CheckModel(ctx context.Context, input model.AIModelConfig) (*model.AIModelCheckResponse, error) {
	normalized, err := normalizeAIModelConfig(input, model.DefaultAISettings().Models[0], 1)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(normalized.APIKey) == "" {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "请先填写 API Key 再检查模型")
	}

	runtimeSettings := model.DefaultAISettings()
	runtimeSettings.Provider = normalized.Provider
	runtimeSettings.APIKey = normalized.APIKey
	runtimeSettings.BaseURL = normalized.BaseURL
	runtimeSettings.Model = normalized.Model
	runtimeSettings.MaxOutputTokens = normalized.MaxOutputTokens
	runtimeSettings.OpenAILegacyMode = normalized.OpenAILegacyMode
	mode := aiProviderMode(runtimeSettings)

	rawText, providerMode, err := s.callProvider(ctx, &aiReadPrepared{
		settings:     runtimeSettings,
		systemPrompt: "你是模型联通性检查助手。请只回复 OK。",
		userPrompt:   "请只回复 OK",
	})
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(rawText) == "" {
		return nil, apperr.New(apperr.CodeUnavailable, "模型检查未返回文本内容")
	}

	return &model.AIModelCheckResponse{
		Success:  true,
		Provider: normalized.Provider,
		Model:    normalized.Model,
		Mode:     firstNonEmpty(providerMode, mode),
		Message:  "模型检查通过",
	}, nil
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

func (s *AIService) Translate(ctx context.Context, input model.AITranslateRequest) (*model.AITranslateResponse, error) {
	text := strings.TrimSpace(input.Text)
	if text == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "缺少需要翻译的文本")
	}

	settings, err := s.GetSettings()
	if err != nil {
		return nil, err
	}

	modelConfig, err := resolveModelForAction(*settings, model.AIActionTranslate)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(modelConfig.APIKey) == "" {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "请先在 AI 页面为翻译场景配置可用模型和 API Key")
	}

	sourceLanguage, targetLanguage := resolveTranslationDirection(settings.Translation, text)
	systemPrompt, userPrompt := buildAITranslatePrompts(*settings, sourceLanguage, targetLanguage, text)

	runtimeSettings := *settings
	runtimeSettings.Provider = modelConfig.Provider
	runtimeSettings.APIKey = modelConfig.APIKey
	runtimeSettings.BaseURL = modelConfig.BaseURL
	runtimeSettings.Model = modelConfig.Model
	runtimeSettings.MaxOutputTokens = modelConfig.MaxOutputTokens
	runtimeSettings.OpenAILegacyMode = modelConfig.OpenAILegacyMode

	rawText, mode, err := s.callProvider(ctx, &aiReadPrepared{
		settings:     runtimeSettings,
		action:       model.AIActionTranslate,
		systemPrompt: systemPrompt,
		userPrompt:   userPrompt,
	})
	if err != nil {
		return nil, err
	}

	translation := normalizeTranslationOutput(rawText)
	if translation == "" {
		return nil, apperr.New(apperr.CodeUnavailable, "翻译结果为空")
	}

	return &model.AITranslateResponse{
		Success:        true,
		Provider:       runtimeSettings.Provider,
		Model:          runtimeSettings.Model,
		Mode:           mode,
		SourceLanguage: sourceLanguage,
		TargetLanguage: targetLanguage,
		Translation:    translation,
	}, nil
}

func (s *AIService) ExportReadMarkdown(ctx context.Context, input model.AIReadExportRequest) (string, []byte, error) {
	if input.PaperID <= 0 {
		return "", nil, apperr.New(apperr.CodeInvalidArgument, "paper_id 无效")
	}
	content := strings.TrimSpace(input.Content)
	if content == "" {
		content = strings.TrimSpace(input.Answer)
	}
	if content == "" {
		return "", nil, apperr.New(apperr.CodeInvalidArgument, "缺少可导出的 Markdown 内容")
	}
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	scope := normalizeAIReadExportScope(input.Scope)

	paper, err := s.repo.GetPaperDetail(input.PaperID)
	if err != nil {
		return "", nil, err
	}
	if paper == nil {
		return "", nil, apperr.New(apperr.CodeNotFound, "文献不存在")
	}

	figureByID := make(map[int64]model.Figure, len(paper.Figures))
	for _, figure := range paper.Figures {
		figureByID[figure.ID] = figure
	}

	type markdownAsset struct {
		Path string
		Data []byte
	}

	assetPaths := map[int64]string{}
	assets := make([]markdownAsset, 0, 4)
	var rewriteErr error
	rewritten := aiMarkdownFigureReferencePattern.ReplaceAllStringFunc(content, func(match string) string {
		if rewriteErr != nil {
			return match
		}

		parts := aiMarkdownFigureReferencePattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}

		figureID, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			rewriteErr = apperr.New(apperr.CodeInvalidArgument, "回答里的图片引用格式无效")
			return match
		}

		assetPath, ok := assetPaths[figureID]
		if !ok {
			figure, exists := figureByID[figureID]
			if !exists {
				rewriteErr = apperr.New(apperr.CodeInvalidArgument, fmt.Sprintf("回答里引用了当前文献不存在的图片 #%d", figureID))
				return match
			}

			var assetData []byte
			assetPath, assetData, err = s.loadMarkdownExportAsset(paper, figure)
			if err != nil {
				rewriteErr = err
				return match
			}

			assetPaths[figureID] = assetPath
			assets = append(assets, markdownAsset{
				Path: assetPath,
				Data: assetData,
			})
		}

		return fmt.Sprintf("![%s](%s)", parts[1], assetPath)
	})
	if rewriteErr != nil {
		return "", nil, rewriteErr
	}

	var archive bytes.Buffer
	zipWriter := zip.NewWriter(&archive)

	answerWriter, err := zipWriter.Create(aiReadExportMarkdownFilename(scope))
	if err != nil {
		return "", nil, apperr.Wrap(apperr.CodeInternal, "创建 Markdown 导出文件失败", err)
	}
	if _, err := io.WriteString(answerWriter, rewritten); err != nil {
		return "", nil, apperr.Wrap(apperr.CodeInternal, "写入 Markdown 导出内容失败", err)
	}

	for _, asset := range assets {
		fileWriter, err := zipWriter.Create(asset.Path)
		if err != nil {
			return "", nil, apperr.Wrap(apperr.CodeInternal, "创建导出图片文件失败", err)
		}
		if _, err := fileWriter.Write(asset.Data); err != nil {
			return "", nil, apperr.Wrap(apperr.CodeInternal, "写入导出图片文件失败", err)
		}
	}

	if err := zipWriter.Close(); err != nil {
		return "", nil, apperr.Wrap(apperr.CodeInternal, "生成导出压缩包失败", err)
	}

	return aiReadExportFilename(paper, scope, input.TurnIndex), archive.Bytes(), nil
}

func (s *AIService) ReadPaperStream(ctx context.Context, input model.AIReadRequest, onEvent func(model.AIReadStreamEvent) error) error {
	prepared, err := s.prepareRead(input, false)
	if err != nil {
		return err
	}
	if prepared.action != model.AIActionFigureInterpretation && prepared.action != model.AIActionPaperQA {
		return apperr.New(apperr.CodeInvalidArgument, "当前只有自由提问和图片解读支持流式输出")
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
	action := normalizeAIAction(input.Action)
	modelConfig, err := resolveModelForAction(*settings, action)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(modelConfig.APIKey) == "" {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "请先在 AI 页面为当前场景配置可用模型和 API Key")
	}
	question := strings.TrimSpace(input.Question)
	if question == "" {
		question = defaultAIQuestion(action)
	}
	promptQuestion := question
	var activeRolePrompts []model.AIRolePrompt
	if action == model.AIActionPaperQA {
		promptQuestion, activeRolePrompts = resolveAIRolePrompts(question, settings.RolePrompts)
	}
	if promptQuestion == "" {
		promptQuestion = defaultAIQuestion(action)
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
		return nil, apperr.New(apperr.CodeFailedPrecondition, "当前文献缺少可供 AI伴读的正文或图片，请先完成解析")
	}

	groups, err := s.repo.ListGroups()
	if err != nil {
		return nil, err
	}
	tags, err := s.repo.ListTags(tagScopeForAIAction(action))
	if err != nil {
		return nil, err
	}

	selectedFigures, err := selectFiguresForAI(paper, action, input.FigureID, settings.MaxFigures)
	if err != nil {
		return nil, err
	}

	images, figureSummaries, err := s.loadFigureInputs(paper, selectedFigures, action)
	if err != nil {
		return nil, err
	}

	systemPrompt, userPrompt := buildAIPrompts(*settings, paper, groups, tags, action, question, promptQuestion, history, figureSummaries, len(images), activeRolePrompts, structuredOutput)

	runtimeSettings := *settings
	runtimeSettings.Provider = modelConfig.Provider
	runtimeSettings.APIKey = modelConfig.APIKey
	runtimeSettings.BaseURL = modelConfig.BaseURL
	runtimeSettings.Model = modelConfig.Model
	runtimeSettings.MaxOutputTokens = modelConfig.MaxOutputTokens
	runtimeSettings.OpenAILegacyMode = modelConfig.OpenAILegacyMode

	s.logger.Info("ai paper read started",
		"provider", runtimeSettings.Provider,
		"model", runtimeSettings.Model,
		"paper_id", paper.ID,
		"action", action,
		"figures", len(images),
	)

	return &aiReadPrepared{
		settings:          runtimeSettings,
		action:            action,
		question:          question,
		promptQuestion:    promptQuestion,
		activeRolePrompts: activeRolePrompts,
		paper:             paper,
		systemPrompt:      systemPrompt,
		userPrompt:        userPrompt,
		includedFigures:   len(images),
		images:            images,
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

func (s *AIService) loadMarkdownExportAsset(paper *model.Paper, figure model.Figure) (string, []byte, error) {
	if paper == nil {
		return "", nil, apperr.New(apperr.CodeNotFound, "paper not found")
	}

	paperFigure := findFigureByID(paper.Figures, figure.ID)
	if paperFigure == nil {
		return "", nil, apperr.New(apperr.CodeNotFound, fmt.Sprintf("导出失败：图片不存在（figure #%d）", figure.ID))
	}

	data, _, err := loadFigureImageData(s.config.FiguresDir(), paper.Figures, *paperFigure)
	if err != nil {
		return "", nil, err
	}

	return "assets/" + aiReadExportAssetName(figure), data, nil
}

func normalizeAIReadExportScope(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "conversation") {
		return "conversation"
	}
	return "turn"
}

func aiReadExportFilename(paper *model.Paper, scope string, turnIndex int) string {
	base := fmt.Sprintf("paper_%d_ai_reader", paper.ID)
	if scope == "conversation" {
		return base + "_conversation.zip"
	}
	if turnIndex > 0 {
		return fmt.Sprintf("%s_turn_%02d.zip", base, turnIndex)
	}
	return base + ".zip"
}

func aiReadExportMarkdownFilename(scope string) string {
	if scope == "conversation" {
		return "conversation.md"
	}
	return "answer.md"
}

func aiReadExportAssetName(figure model.Figure) string {
	ext := extensionForFigure(figure.ContentType, figure.Filename)
	return fmt.Sprintf("figure-p%d-n%d-%d%s", figure.PageNumber, figure.FigureIndex, figure.ID, ext)
}

func normalizeAISettings(input model.AISettings) (model.AISettings, error) {
	defaults := model.DefaultAISettings()
	settings := input

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
	if strings.TrimSpace(settings.TranslatePrompt) == "" {
		settings.TranslatePrompt = defaults.TranslatePrompt
	}
	settings.SystemPrompt = strings.TrimSpace(settings.SystemPrompt)
	settings.QAPrompt = strings.TrimSpace(settings.QAPrompt)
	settings.FigurePrompt = strings.TrimSpace(settings.FigurePrompt)
	settings.TagPrompt = strings.TrimSpace(settings.TagPrompt)
	settings.GroupPrompt = strings.TrimSpace(settings.GroupPrompt)
	settings.TranslatePrompt = strings.TrimSpace(settings.TranslatePrompt)
	if strings.TrimSpace(settings.Translation.PrimaryLanguage) == "" {
		settings.Translation.PrimaryLanguage = defaults.Translation.PrimaryLanguage
	}
	if strings.TrimSpace(settings.Translation.TargetLanguage) == "" {
		settings.Translation.TargetLanguage = defaults.Translation.TargetLanguage
	}
	settings.Translation.PrimaryLanguage = strings.TrimSpace(settings.Translation.PrimaryLanguage)
	settings.Translation.TargetLanguage = strings.TrimSpace(settings.Translation.TargetLanguage)

	models, err := normalizeAIModels(settings, defaults)
	if err != nil {
		return model.AISettings{}, err
	}
	settings.Models = models
	settings.SceneModels = normalizeAISceneModelSelection(settings.SceneModels, settings.Models)

	defaultModel, err := resolveModelByID(settings.Models, settings.SceneModels.DefaultModelID)
	if err != nil {
		return model.AISettings{}, err
	}
	settings.Provider = defaultModel.Provider
	settings.APIKey = defaultModel.APIKey
	settings.BaseURL = defaultModel.BaseURL
	settings.Model = defaultModel.Model
	settings.MaxOutputTokens = defaultModel.MaxOutputTokens
	settings.OpenAILegacyMode = defaultModel.OpenAILegacyMode

	return settings, nil
}

func parseStoredAIRolePrompts(raw string) ([]model.AIRolePrompt, error) {
	var rolePrompts []model.AIRolePrompt
	if err := json.Unmarshal([]byte(raw), &rolePrompts); err == nil {
		normalized, normalizeErr := normalizeAIRolePrompts(rolePrompts)
		if normalizeErr == nil || strings.Contains(raw, "\"prompt\"") || len(rolePrompts) == 0 {
			return normalized, normalizeErr
		}
	}

	var legacyPresets []legacyAIPromptPreset
	if err := json.Unmarshal([]byte(raw), &legacyPresets); err != nil {
		return nil, err
	}

	rolePrompts = make([]model.AIRolePrompt, 0, len(legacyPresets))
	for _, item := range legacyPresets {
		rolePrompts = append(rolePrompts, convertLegacyPromptPresetToRolePrompt(item))
	}

	return normalizeAIRolePrompts(rolePrompts)
}

func convertLegacyPromptPresetToRolePrompt(input legacyAIPromptPreset) model.AIRolePrompt {
	sections := make([]string, 0, 6)
	appendSection := func(label, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		sections = append(sections, fmt.Sprintf("%s:\n%s", label, value))
	}

	appendSection("System Prompt", input.SystemPrompt)
	appendSection("通用问答 Prompt", input.QAPrompt)
	appendSection("图片解读 Prompt", input.FigurePrompt)
	appendSection("Tag 建议 Prompt", input.TagPrompt)
	appendSection("分组建议 Prompt", input.GroupPrompt)
	appendSection("翻译 Prompt", input.TranslatePrompt)

	return model.AIRolePrompt{
		Name:   strings.TrimSpace(input.Name),
		Prompt: strings.Join(sections, "\n\n"),
	}
}

func normalizeAIRolePrompts(input []model.AIRolePrompt) ([]model.AIRolePrompt, error) {
	if len(input) == 0 {
		return []model.AIRolePrompt{}, nil
	}
	if len(input) > 50 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "角色 Prompt 数量不能超过 50 个")
	}

	rolePrompts := make([]model.AIRolePrompt, 0, len(input))
	seenNames := make(map[string]struct{}, len(input))
	for _, item := range input {
		rolePrompt := model.AIRolePrompt{
			Name:   strings.TrimSpace(item.Name),
			Prompt: strings.TrimSpace(item.Prompt),
		}
		if rolePrompt.Name == "" {
			return nil, apperr.New(apperr.CodeInvalidArgument, "角色名称不能为空")
		}
		if rolePrompt.Prompt == "" {
			return nil, apperr.New(apperr.CodeInvalidArgument, "角色 Prompt 不能为空")
		}

		lookupKey := strings.ToLower(rolePrompt.Name)
		if _, exists := seenNames[lookupKey]; exists {
			return nil, apperr.New(apperr.CodeInvalidArgument, "角色名称不能重复")
		}
		seenNames[lookupKey] = struct{}{}
		rolePrompts = append(rolePrompts, rolePrompt)
	}

	return rolePrompts, nil
}

func normalizeAIModels(settings model.AISettings, defaults model.AISettings) ([]model.AIModelConfig, error) {
	fallbackModel := defaults.Models[0]
	if settings.MaxOutputTokens > 0 {
		fallbackModel.MaxOutputTokens = settings.MaxOutputTokens
	}

	inputModels := settings.Models
	if len(inputModels) == 0 {
		inputModels = []model.AIModelConfig{{
			ID:               fallbackModel.ID,
			Name:             fallbackModel.Name,
			Provider:         settings.Provider,
			APIKey:           settings.APIKey,
			BaseURL:          settings.BaseURL,
			Model:            settings.Model,
			MaxOutputTokens:  settings.MaxOutputTokens,
			OpenAILegacyMode: settings.OpenAILegacyMode,
		}}
	}

	models := make([]model.AIModelConfig, 0, len(inputModels))
	seenIDs := map[string]struct{}{}
	for index, item := range inputModels {
		normalized, err := normalizeAIModelConfig(item, fallbackModel, index+1)
		if err != nil {
			return nil, err
		}
		if _, exists := seenIDs[normalized.ID]; exists {
			return nil, apperr.New(apperr.CodeInvalidArgument, "AI 模型 ID 不能重复")
		}
		seenIDs[normalized.ID] = struct{}{}
		models = append(models, normalized)
	}
	if len(models) == 0 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "至少需要保留一个 AI 模型")
	}
	return models, nil
}

func normalizeAIModelConfig(input model.AIModelConfig, fallback model.AIModelConfig, index int) (model.AIModelConfig, error) {
	config := input

	if strings.TrimSpace(config.ID) == "" {
		config.ID = fmt.Sprintf("model_%d", index)
	}
	config.ID = strings.TrimSpace(config.ID)

	if strings.TrimSpace(string(config.Provider)) == "" {
		config.Provider = fallback.Provider
	}
	if !isSupportedAIProvider(config.Provider) {
		return model.AIModelConfig{}, apperr.New(apperr.CodeInvalidArgument, "暂不支持该 AI 提供商")
	}

	config.APIKey = strings.TrimSpace(config.APIKey)
	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	config.Model = strings.TrimSpace(config.Model)
	config.Name = strings.TrimSpace(config.Name)

	if config.BaseURL == "" {
		config.BaseURL = defaultAIBaseURL(config.Provider)
	}
	if config.Model == "" {
		config.Model = defaultAIModel(config.Provider)
	}
	if config.MaxOutputTokens <= 0 {
		config.MaxOutputTokens = fallback.MaxOutputTokens
	}
	if config.MaxOutputTokens > 16384 {
		return model.AIModelConfig{}, apperr.New(apperr.CodeInvalidArgument, "max_output_tokens 过大")
	}
	if config.Name == "" {
		config.Name = fmt.Sprintf("%s / %s", strings.ToUpper(string(config.Provider)), config.Model)
	}
	if config.Provider != model.AIProviderOpenAI {
		config.OpenAILegacyMode = false
	}

	return config, nil
}

func normalizeAISceneModelSelection(input model.AISceneModelSelection, models []model.AIModelConfig) model.AISceneModelSelection {
	selection := input
	if len(models) == 0 {
		return model.AISceneModelSelection{}
	}

	selection.DefaultModelID = normalizeSceneModelID(selection.DefaultModelID, models, models[0].ID)
	selection.QAModelID = normalizeSceneModelID(selection.QAModelID, models, selection.DefaultModelID)
	selection.IMIntentModelID = normalizeSceneModelID(selection.IMIntentModelID, models, selection.DefaultModelID)
	selection.FigureModelID = normalizeSceneModelID(selection.FigureModelID, models, selection.DefaultModelID)
	selection.TagModelID = normalizeSceneModelID(selection.TagModelID, models, selection.DefaultModelID)
	selection.GroupModelID = normalizeSceneModelID(selection.GroupModelID, models, selection.DefaultModelID)
	selection.TranslateModelID = normalizeSceneModelID(selection.TranslateModelID, models, selection.DefaultModelID)
	return selection
}

func normalizeSceneModelID(modelID string, models []model.AIModelConfig, fallback string) string {
	modelID = strings.TrimSpace(modelID)
	if modelID != "" {
		for _, item := range models {
			if item.ID == modelID {
				return modelID
			}
		}
	}
	for _, item := range models {
		if item.ID == fallback {
			return fallback
		}
	}
	if len(models) == 0 {
		return ""
	}
	return models[0].ID
}

func resolveModelForAction(settings model.AISettings, action model.AIAction) (model.AIModelConfig, error) {
	modelID := settings.SceneModels.DefaultModelID
	switch action {
	case model.AIActionFigureInterpretation:
		modelID = firstNonEmpty(settings.SceneModels.FigureModelID, settings.SceneModels.DefaultModelID)
	case model.AIActionTagSuggestion:
		modelID = firstNonEmpty(settings.SceneModels.TagModelID, settings.SceneModels.DefaultModelID)
	case model.AIActionGroupSuggestion:
		modelID = firstNonEmpty(settings.SceneModels.GroupModelID, settings.SceneModels.DefaultModelID)
	case model.AIActionTranslate:
		modelID = firstNonEmpty(settings.SceneModels.TranslateModelID, settings.SceneModels.DefaultModelID)
	default:
		modelID = firstNonEmpty(settings.SceneModels.QAModelID, settings.SceneModels.DefaultModelID)
	}
	return resolveModelByID(settings.Models, modelID)
}

func resolveModelByID(models []model.AIModelConfig, modelID string) (model.AIModelConfig, error) {
	modelID = strings.TrimSpace(modelID)
	for _, item := range models {
		if item.ID == modelID {
			return item, nil
		}
	}
	if len(models) == 0 {
		return model.AIModelConfig{}, apperr.New(apperr.CodeFailedPrecondition, "请先在 AI 页面配置至少一个模型")
	}
	if modelID == "" {
		return models[0], nil
	}
	return model.AIModelConfig{}, apperr.New(apperr.CodeFailedPrecondition, "当前场景绑定的 AI 模型不存在，请到配置页重新选择")
}

func buildAIPrompts(
	settings model.AISettings,
	paper *model.Paper,
	groups []model.Group,
	tags []model.Tag,
	action model.AIAction,
	displayQuestion string,
	promptQuestion string,
	history []model.AIConversationTurn,
	figureSummaries []string,
	includedFigures int,
	activeRolePrompts []model.AIRolePrompt,
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
	notesText := buildPaperNotesContext(paper)

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

	rolePromptNames := buildAIRolePromptNames(activeRolePrompts)
	outputRequirements := strings.TrimSpace(aiOutputRequirements(action, structuredOutput))
	scopeDescription := actionScopeDescription(action)
	if strings.TrimSpace(displayQuestion) == "" {
		displayQuestion = promptQuestion
	}

	userPrompt := fmt.Sprintf(`任务类型: %s

场景范围:
%s

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

原始输入:
%s

角色调用:
%s

历史对话:
%s

场景指令:
%s

全文:
%s

输出要求:
%s`,
		action,
		scopeDescription,
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
		promptQuestion,
		displayQuestion,
		rolePromptNames,
		conversationSection,
		actionPromptFor(settings, action),
		fullText,
		outputRequirements,
	)

	systemPrompt := settings.SystemPrompt
	roleSystemPrompt := buildAIRolePromptSystemSection(activeRolePrompts)
	if roleSystemPrompt != "" {
		systemPrompt = strings.TrimSpace(systemPrompt + "\n\n" + roleSystemPrompt)
	}

	return systemPrompt, userPrompt
}

func buildPaperNotesContext(paper *model.Paper) string {
	managementNotes := strings.TrimSpace(paper.NotesText)
	paperNotes := strings.TrimSpace(paper.PaperNotesText)

	switch {
	case managementNotes == "" && paperNotes == "":
		return "无"
	case managementNotes == "":
		return "文献笔记:\n" + paperNotes
	case paperNotes == "":
		return "管理笔记:\n" + managementNotes
	default:
		return "管理笔记:\n" + managementNotes + "\n\n文献笔记:\n" + paperNotes
	}
}

func resolveAIRolePrompts(question string, available []model.AIRolePrompt) (string, []model.AIRolePrompt) {
	if strings.TrimSpace(question) == "" || len(available) == 0 {
		return strings.TrimSpace(question), nil
	}

	type candidate struct {
		rolePrompt model.AIRolePrompt
		token      string
	}

	candidates := make([]candidate, 0, len(available))
	for _, item := range available {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		candidates = append(candidates, candidate{
			rolePrompt: item,
			token:      "@" + name,
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return len([]rune(candidates[i].rolePrompt.Name)) > len([]rune(candidates[j].rolePrompt.Name))
	})

	cleanedQuestion := question
	activeRolePrompts := make([]model.AIRolePrompt, 0, len(candidates))
	seenNames := make(map[string]struct{}, len(candidates))
	for _, item := range candidates {
		if !strings.Contains(cleanedQuestion, item.token) {
			continue
		}
		cleanedQuestion = strings.ReplaceAll(cleanedQuestion, item.token, " ")
		lookupKey := strings.ToLower(strings.TrimSpace(item.rolePrompt.Name))
		if _, exists := seenNames[lookupKey]; exists {
			continue
		}
		seenNames[lookupKey] = struct{}{}
		activeRolePrompts = append(activeRolePrompts, item.rolePrompt)
	}

	return strings.Join(strings.Fields(cleanedQuestion), " "), activeRolePrompts
}

func buildAIRolePromptNames(rolePrompts []model.AIRolePrompt) string {
	if len(rolePrompts) == 0 {
		return "未调用角色 Prompt。"
	}

	names := make([]string, 0, len(rolePrompts))
	for _, item := range rolePrompts {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		names = append(names, "@"+name)
	}
	if len(names) == 0 {
		return "未调用角色 Prompt。"
	}
	return strings.Join(names, "，")
}

func buildAIRolePromptSystemSection(rolePrompts []model.AIRolePrompt) string {
	if len(rolePrompts) == 0 {
		return ""
	}

	sections := make([]string, 0, len(rolePrompts))
	for _, item := range rolePrompts {
		name := strings.TrimSpace(item.Name)
		prompt := strings.TrimSpace(item.Prompt)
		if name == "" || prompt == "" {
			continue
		}
		sections = append(sections, fmt.Sprintf("角色：%s\n%s", name, prompt))
	}
	if len(sections) == 0 {
		return ""
	}

	return "以下是当前用户通过 @ 调用的角色 Prompt，请在本次回答中一并遵守：\n\n" + strings.Join(sections, "\n\n")
}

func buildAITranslatePrompts(settings model.AISettings, sourceLanguage, targetLanguage, text string) (string, string) {
	userPrompt := fmt.Sprintf(
		`任务类型: translate

翻译方向:
- 原文语言: %s
- 目标语言: %s

场景指令:
%s

输出要求:
1. 只返回译文正文，不要附加解释、注释、标题、前缀或代码块。
2. 保留原文中的换行、列表层级、数字、单位、缩写和专有名词。
3. 如果原文已经是目标语言，也请只做必要润色后输出正文。

待翻译文本:
%s`,
		sourceLanguage,
		targetLanguage,
		settings.TranslatePrompt,
		text,
	)
	return settings.SystemPrompt, userPrompt
}

func resolveTranslationDirection(config model.AITranslationConfig, text string) (string, string) {
	primaryLanguage := strings.TrimSpace(config.PrimaryLanguage)
	targetLanguage := strings.TrimSpace(config.TargetLanguage)
	if primaryLanguage == "" {
		primaryLanguage = model.DefaultAISettings().Translation.PrimaryLanguage
	}
	if targetLanguage == "" {
		targetLanguage = model.DefaultAISettings().Translation.TargetLanguage
	}
	if translationTextMatchesPrimaryLanguage(primaryLanguage, text) {
		return primaryLanguage, targetLanguage
	}
	return "其他语言", primaryLanguage
}

func translationTextMatchesPrimaryLanguage(primaryLanguage, text string) bool {
	normalizedPrimary := normalizeTranslationLanguageKey(primaryLanguage)
	detectedLanguage := detectTranslationLanguageKey(text)
	if normalizedPrimary == "" || detectedLanguage == "" {
		return false
	}
	return normalizedPrimary == detectedLanguage
}

func normalizeTranslationLanguageKey(language string) string {
	normalized := strings.ToLower(strings.TrimSpace(language))
	switch normalized {
	case "zh", "zh-cn", "zh-hans", "zh-hant", "chinese", "mandarin", "中文", "汉语", "简体中文", "繁體中文", "繁体中文":
		return "han"
	case "ja", "jp", "japanese", "日语", "日文", "日本語", "日本语":
		return "japanese"
	case "ko", "korean", "韩语", "韓語", "한국어":
		return "hangul"
	case "en", "english", "英文", "英语":
		return "latin"
	case "fr", "french", "法语", "法文":
		return "latin"
	case "de", "german", "德语", "德文":
		return "latin"
	case "es", "spanish", "西班牙语", "西班牙文":
		return "latin"
	case "pt", "portuguese", "葡萄牙语", "葡萄牙文":
		return "latin"
	case "it", "italian", "意大利语", "意大利文":
		return "latin"
	case "ru", "russian", "俄语", "俄文":
		return "cyrillic"
	case "ar", "arabic", "阿拉伯语", "阿拉伯文":
		return "arabic"
	default:
		return ""
	}
}

func detectTranslationLanguageKey(text string) string {
	type scriptCounts struct {
		japanese int
		hangul   int
		han      int
		latin    int
		cyrillic int
		arabic   int
	}

	var counts scriptCounts
	for _, r := range text {
		switch {
		case unicode.In(r, unicode.Hiragana, unicode.Katakana):
			counts.japanese += 2
		case unicode.In(r, unicode.Hangul):
			counts.hangul += 2
		case unicode.In(r, unicode.Han):
			counts.han++
		case unicode.In(r, unicode.Cyrillic):
			counts.cyrillic++
		case unicode.In(r, unicode.Arabic):
			counts.arabic++
		case unicode.In(r, unicode.Latin):
			if unicode.IsLetter(r) {
				counts.latin++
			}
		}
	}

	switch {
	case counts.japanese > 0:
		return "japanese"
	case counts.hangul > 0:
		return "hangul"
	case counts.han > 0:
		return "han"
	case counts.cyrillic > 0:
		return "cyrillic"
	case counts.arabic > 0:
		return "arabic"
	case counts.latin > 0:
		return "latin"
	default:
		return ""
	}
}

func normalizeTranslationOutput(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "```") {
		trimmed = strings.TrimPrefix(trimmed, "```")
		if newline := strings.Index(trimmed, "\n"); newline >= 0 {
			trimmed = trimmed[newline+1:]
		}
		trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, "```"))
	}
	return strings.TrimSpace(trimmed)
}

func aiOutputRequirements(action model.AIAction, structuredOutput bool) string {
	if structuredOutput {
		if action == model.AIActionPaperQA {
			return `1. 只返回 JSON 对象，不要使用 Markdown 代码块。
2. JSON 必须包含 answer、suggested_tags、suggested_group 三个字段。
3. 如果当前任务不需要标签建议或分组建议，请分别返回空数组 [] 和空字符串 ""。
4. answer 中请直接给出结论、依据和必要的限制说明。
5. answer 支持使用 Markdown；如果需要插入论文图片，只能使用系统提供的图片引用，格式为 ![图片说明](figure://<figure_id>)。
6. 不要伪造本地文件路径、文件名或外部图片 URL。`
		}
		if action == model.AIActionTagSuggestion {
			return `1. 只返回 JSON 对象，不要使用 Markdown 代码块。
2. JSON 必须包含 suggested_tags 字段（字符串数组）。
3. answer 字段只填一句话概括，不需要展开分析。
4. 不要长篇解释，只给出标签列表。`
		}
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
	case model.AIActionTranslate:
		return settings.TranslatePrompt
	default:
		return settings.QAPrompt
	}
}

func actionScopeDescription(action model.AIAction) string {
	switch action {
	case model.AIActionFigureInterpretation:
		return "只针对当前选中的这张图片进行解读；论文全文仅作为补充上下文。"
	case model.AIActionTagSuggestion:
		return "只针对当前选中的这张图片生成图片标签；不是整篇文献的文献标签。"
	case model.AIActionGroupSuggestion:
		return "针对当前整篇文献进行分组判断。"
	default:
		return "针对当前整篇文献回答用户问题。"
	}
}

func normalizeAIAction(action model.AIAction) model.AIAction {
	switch action {
	case model.AIActionFigureInterpretation, model.AIActionTagSuggestion, model.AIActionGroupSuggestion, model.AIActionPaperQA, model.AIActionTranslate:
		return action
	default:
		return model.AIActionPaperQA
	}
}

func tagScopeForAIAction(action model.AIAction) model.TagScope {
	switch action {
	case model.AIActionFigureInterpretation, model.AIActionTagSuggestion:
		return model.TagScopeFigure
	default:
		return model.TagScopePaper
	}
}

func defaultAIQuestion(action model.AIAction) string {
	switch action {
	case model.AIActionFigureInterpretation:
		return "当前任务只针对当前选中的这张图片。"
	case model.AIActionTagSuggestion:
		return "当前任务只针对当前选中的这张图片。"
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

func selectFiguresForAI(paper *model.Paper, action model.AIAction, figureID int64, maxFigures int) ([]model.Figure, error) {
	if (action == model.AIActionFigureInterpretation || action == model.AIActionTagSuggestion) && figureID > 0 {
		for _, figure := range paper.Figures {
			if figure.ID == figureID {
				return []model.Figure{figure}, nil
			}
		}
		return nil, apperr.New(apperr.CodeInvalidArgument, "指定图片不存在于当前文献")
	}

	figures := topLevelFigures(paper.Figures)
	if maxFigures > 0 && len(figures) > maxFigures {
		figures = figures[:maxFigures]
	}
	return figures, nil
}

func (s *AIService) loadFigureInputs(paper *model.Paper, figures []model.Figure, action model.AIAction) ([]aiImageInput, []string, error) {
	images := make([]aiImageInput, 0, len(figures))
	summaries := make([]string, 0, len(figures))
	totalBytes := 0
	budgetReached := false
	for _, figure := range figures {
		summary := buildAIFigureSummary(figure, action)
		summaries = append(summaries, summary)

		if budgetReached {
			continue
		}
		data, mimeType, err := loadFigureImageData(s.config.FiguresDir(), paper.Figures, figure)
		if err != nil {
			if apperr.IsCode(err, apperr.CodeNotFound) {
				s.logger.Warn("ai figure image missing",
					"paper_id", paper.ID,
					"figure_id", figure.ID,
					"filename", figure.Filename,
				)
				continue
			}
			return nil, nil, err
		}

		compressedData, compressedMIMEType, err := compressAIImage(data, mimeType)
		if err != nil {
			s.logger.Warn("ai figure compression failed",
				"paper_id", paper.ID,
				"figure_id", figure.ID,
				"filename", figure.Filename,
				"error", err,
			)
			continue
		}
		if totalBytes > 0 && totalBytes+len(compressedData) > aiFigureImageTotalBudget {
			s.logger.Warn("ai figure image budget reached",
				"paper_id", paper.ID,
				"figure_id", figure.ID,
				"filename", figure.Filename,
				"included", len(images),
				"budget_bytes", aiFigureImageTotalBudget,
			)
			budgetReached = true
			continue
		}

		images = append(images, aiImageInput{
			MIMEType: compressedMIMEType,
			Data:     base64.StdEncoding.EncodeToString(compressedData),
		})
		totalBytes += len(compressedData)
	}

	return images, summaries, nil
}

func buildAIFigureSummary(figure model.Figure, action model.AIAction) string {
	label := fmt.Sprintf("第 %d 页图 %d", figure.PageNumber, figure.FigureIndex)
	caption := fallbackText(strings.TrimSpace(figure.Caption), "无")
	if action == model.AIActionPaperQA {
		return fmt.Sprintf("- figure_id=%d；标签=%s；caption=%s；如需插图请使用 ![%s](figure://%d)", figure.ID, label, caption, label, figure.ID)
	}
	return fmt.Sprintf("- %s：caption=%s", label, caption)
}

func compressAIImage(data []byte, mimeType string) ([]byte, string, error) {
	mimeType = normalizeAIImageMIMEType(mimeType, data)

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		if len(data) <= aiFigureImageMaxBytes {
			return data, mimeType, nil
		}
		return nil, "", err
	}

	bounds := img.Bounds()
	if len(data) <= aiFigureImageMaxBytes && maxInt(bounds.Dx(), bounds.Dy()) <= aiFigureImageMaxDimension {
		return data, mimeType, nil
	}

	maxDimension := aiFigureImageMaxDimension
	quality := aiFigureImageJPEGQuality
	var best []byte
	for attempt := 0; attempt < aiFigureImageCompressionRuns; attempt++ {
		candidate := resizeImageForAI(img, maxDimension)
		encoded, err := encodeAIJPEG(candidate, quality)
		if err != nil {
			return nil, "", err
		}
		if len(best) == 0 || len(encoded) < len(best) {
			best = encoded
		}
		if len(encoded) <= aiFigureImageMaxBytes {
			return encoded, "image/jpeg", nil
		}

		nextDimension := int(float64(maxDimension) * 0.82)
		if nextDimension < aiFigureImageMinDimension {
			nextDimension = aiFigureImageMinDimension
		}
		maxDimension = nextDimension

		quality -= 6
		if quality < aiFigureImageMinJPEGQuality {
			quality = aiFigureImageMinJPEGQuality
		}
	}

	if len(best) > 0 {
		return best, "image/jpeg", nil
	}
	return nil, "", errors.New("无法压缩图片")
}

func normalizeAIImageMIMEType(mimeType string, data []byte) string {
	mimeType = strings.TrimSpace(strings.SplitN(strings.TrimSpace(mimeType), ";", 2)[0])
	if mimeType == "" && len(data) > 0 {
		mimeType = strings.TrimSpace(strings.SplitN(http.DetectContentType(data), ";", 2)[0])
	}
	if !strings.HasPrefix(mimeType, "image/") {
		return "image/png"
	}
	return mimeType
}

func resizeImageForAI(src image.Image, maxDimension int) image.Image {
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 || maxDimension <= 0 {
		return src
	}

	largest := maxInt(width, height)
	if largest <= maxDimension {
		return src
	}

	scale := float64(maxDimension) / float64(largest)
	dstWidth := maxInt(1, int(float64(width)*scale))
	dstHeight := maxInt(1, int(float64(height)*scale))
	dst := image.NewRGBA(image.Rect(0, 0, dstWidth, dstHeight))

	for y := 0; y < dstHeight; y++ {
		srcY := bounds.Min.Y + int(float64(y)*float64(height)/float64(dstHeight))
		if srcY >= bounds.Max.Y {
			srcY = bounds.Max.Y - 1
		}
		for x := 0; x < dstWidth; x++ {
			srcX := bounds.Min.X + int(float64(x)*float64(width)/float64(dstWidth))
			if srcX >= bounds.Max.X {
				srcX = bounds.Max.X - 1
			}
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}

	return dst
}

func encodeAIJPEG(src image.Image, quality int) ([]byte, error) {
	bounds := src.Bounds()
	canvas := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(canvas, canvas.Bounds(), src, bounds.Min, draw.Over)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, canvas, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
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

	for _, candidate := range []string{
		trimmed,
		trimCodeFence(trimmed),
	} {
		result, ok := parseStructuredAIJSONLoose(candidate)
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

func parseStructuredAIJSONLoose(candidate string) (aiStructuredResult, bool) {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return aiStructuredResult{}, false
	}
	if !strings.Contains(candidate, "\"answer\"") &&
		!strings.Contains(candidate, "\"response\"") &&
		!strings.Contains(candidate, "\"analysis\"") &&
		!strings.Contains(candidate, "\"suggested_tags\"") &&
		!strings.Contains(candidate, "\"suggested_group\"") {
		return aiStructuredResult{}, false
	}

	result := aiStructuredResult{}
	found := false

	if answer, ok := extractPartialJSONStringField(candidate, "answer", "response", "analysis"); ok {
		result.Answer = answer
		found = true
	}
	if tags, ok := extractPartialJSONStringArrayField(candidate, "suggested_tags", "tags"); ok {
		result.SuggestedTags = tags
		found = true
	}
	if group, ok := extractPartialJSONStringField(candidate, "suggested_group", "group"); ok {
		result.SuggestedGroup = group
		found = true
	}

	return result, found
}

func extractPartialJSONStringField(text string, keys ...string) (string, bool) {
	for _, key := range keys {
		start, ok := findJSONFieldValueStart(text, key, '"')
		if !ok {
			continue
		}
		value, _, _ := decodePartialJSONString(text, start)
		return value, true
	}
	return "", false
}

func extractPartialJSONStringArrayField(text string, keys ...string) ([]string, bool) {
	for _, key := range keys {
		start, ok := findJSONFieldValueStart(text, key, '[')
		if !ok {
			continue
		}
		values := make([]string, 0, 4)
		i := start
		for i < len(text) {
			i = skipJSONWhitespace(text, i)
			if i >= len(text) || text[i] == ']' {
				break
			}
			if text[i] == ',' {
				i++
				continue
			}
			if text[i] != '"' {
				break
			}

			value, next, _ := decodePartialJSONString(text, i+1)
			if strings.TrimSpace(value) != "" {
				values = append(values, value)
			}
			i = next
		}

		if len(values) > 0 {
			return values, true
		}
	}
	return nil, false
}

func findJSONFieldValueStart(text, key string, opening byte) (int, bool) {
	pattern := `"` + key + `"`
	searchFrom := 0
	for searchFrom < len(text) {
		relative := strings.Index(text[searchFrom:], pattern)
		if relative < 0 {
			return 0, false
		}
		index := searchFrom + relative + len(pattern)
		index = skipJSONWhitespace(text, index)
		if index >= len(text) || text[index] != ':' {
			searchFrom += relative + len(pattern)
			continue
		}
		index++
		index = skipJSONWhitespace(text, index)
		if index >= len(text) || text[index] != opening {
			searchFrom += relative + len(pattern)
			continue
		}
		return index + 1, true
	}
	return 0, false
}

func skipJSONWhitespace(text string, index int) int {
	for index < len(text) {
		switch text[index] {
		case ' ', '\n', '\r', '\t':
			index++
		default:
			return index
		}
	}
	return index
}

func decodePartialJSONString(text string, start int) (string, int, bool) {
	var builder strings.Builder
	for i := start; i < len(text); i++ {
		ch := text[i]
		if ch == '"' {
			return builder.String(), i + 1, true
		}
		if ch != '\\' {
			builder.WriteByte(ch)
			continue
		}

		i++
		if i >= len(text) {
			return builder.String(), len(text), false
		}

		switch text[i] {
		case '"', '\\', '/':
			builder.WriteByte(text[i])
		case 'b':
			builder.WriteByte('\b')
		case 'f':
			builder.WriteByte('\f')
		case 'n':
			builder.WriteByte('\n')
		case 'r':
			builder.WriteByte('\r')
		case 't':
			builder.WriteByte('\t')
		case 'u':
			if i+4 >= len(text) {
				return builder.String(), len(text), false
			}
			if value, ok := parseJSONUnicodeEscape(text[i+1 : i+5]); ok {
				builder.WriteRune(value)
				i += 4
				continue
			}
			builder.WriteString(`\u`)
		default:
			builder.WriteByte(text[i])
		}
	}

	return builder.String(), len(text), false
}

func parseJSONUnicodeEscape(text string) (rune, bool) {
	if len(text) != 4 {
		return 0, false
	}

	var value rune
	for _, ch := range text {
		value <<= 4
		switch {
		case ch >= '0' && ch <= '9':
			value += ch - '0'
		case ch >= 'a' && ch <= 'f':
			value += ch - 'a' + 10
		case ch >= 'A' && ch <= 'F':
			value += ch - 'A' + 10
		default:
			return 0, false
		}
	}
	return value, true
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

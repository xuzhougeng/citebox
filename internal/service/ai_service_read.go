package service

import (
	"context"
	"errors"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

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
	if !aiModelIsConfigured(modelConfig) {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "请先在 AI 页面为翻译场景配置可用模型和凭据")
	}

	sourceLanguage, targetLanguage := resolveTranslationDirection(settings.Translation, text)
	systemPrompt, userPrompt := buildAITranslatePrompts(*settings, sourceLanguage, targetLanguage, text)

	runtimeSettings := *settings
	applyAIModelConfig(&runtimeSettings, modelConfig)

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

func (s *AIService) ReadPaperStream(ctx context.Context, input model.AIReadRequest, onEvent func(model.AIReadStreamEvent) error) error {
	prepared, err := s.prepareRead(input, false)
	if err != nil {
		return err
	}
	if prepared.action != model.AIActionFigureInterpretation && prepared.action != model.AIActionPaperQA {
		return apperr.New(apperr.CodeInvalidArgument, "当前只有自由提问和图片解读支持流式输出")
	}
	mode := aiProviderMode(prepared.settings)
	metaEvent := model.AIReadStreamEvent{
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
	}
	metaSent := false
	sendMeta := func() error {
		if metaSent {
			return nil
		}
		metaSent = true
		return onEvent(metaEvent)
	}

	rawText, err := s.callProviderStream(ctx, prepared, func(delta string) error {
		if delta == "" {
			return nil
		}
		if err := sendMeta(); err != nil {
			return err
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
	if err := sendMeta(); err != nil {
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
	if requiresImageInput(action) && !aiModelSupportsImages(modelConfig) {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "当前场景选择的模型未启用图片输入，请在 AI 设置中更换支持图片的模型，或打开该模型的图片输入能力")
	}
	if !aiModelIsConfigured(modelConfig) {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "请先在 AI 页面为当前场景配置可用模型和凭据")
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
		return nil, apperr.New(apperr.CodeFailedPrecondition, "当前文献缺少可供 AI 助手的正文或图片，请先完成解析")
	}

	groups, err := s.repo.ListGroups()
	if err != nil {
		return nil, err
	}
	tags, err := s.repo.ListTags(tagScopeForAIAction(action))
	if err != nil {
		return nil, err
	}

	includeFigures := includeFiguresForRead(input, action)
	includeImagePayloads := includeFigures && aiModelSupportsImages(modelConfig)
	var selectedFigures []model.Figure
	if includeFigures {
		selectedFigures, err = selectFiguresForAI(paper, action, input.FigureID, settings.MaxFigures)
		if err != nil {
			return nil, err
		}
	}

	images, figureSummaries, err := s.loadFigureInputsWithOptions(paper, selectedFigures, action, includeImagePayloads)
	if err != nil {
		return nil, err
	}

	systemPrompt, userPrompt := buildAIPrompts(*settings, paper, groups, tags, action, question, promptQuestion, history, figureSummaries, len(images), activeRolePrompts, structuredOutput)

	runtimeSettings := *settings
	applyAIModelConfig(&runtimeSettings, modelConfig)

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

func includeFiguresForRead(input model.AIReadRequest, action model.AIAction) bool {
	if action == model.AIActionPaperQA && input.IncludeFigures != nil {
		return *input.IncludeFigures
	}
	return true
}

func requiresImageInput(action model.AIAction) bool {
	return action == model.AIActionFigureInterpretation || action == model.AIActionTagSuggestion
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

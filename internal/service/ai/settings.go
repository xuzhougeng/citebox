package ai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

// SettingsKey AI 设置存储键
const SettingsKey = "ai_settings"

// DefaultBaseURL 返回默认的基础 URL
func DefaultBaseURL(provider model.AIProvider) string {
	switch provider {
	case model.AIProviderOpenAI:
		return "https://api.openai.com"
	case model.AIProviderAnthropic:
		return "https://api.anthropic.com"
	case model.AIProviderGemini:
		return "https://generativelanguage.googleapis.com"
	default:
		return ""
	}
}

// DefaultModel 返回默认模型
func DefaultModel(provider model.AIProvider) string {
	switch provider {
	case model.AIProviderOpenAI:
		return "gpt-4o-mini"
	case model.AIProviderAnthropic:
		return "claude-3-haiku-20240307"
	case model.AIProviderGemini:
		return "gemini-1.5-flash"
	default:
		return ""
	}
}

// IsSupportedProvider 检查提供商是否受支持
func IsSupportedProvider(provider model.AIProvider) bool {
	switch provider {
	case model.AIProviderOpenAI, model.AIProviderAnthropic, model.AIProviderGemini:
		return true
	default:
		return false
	}
}

// NormalizeAction 标准化 AI 动作
func NormalizeAction(action model.AIAction) model.AIAction {
	switch action {
	case model.AIActionFigureInterpretation, model.AIActionTagSuggestion, model.AIActionGroupSuggestion, model.AIActionPaperQA, model.AIActionTranslate:
		return action
	default:
		return model.AIActionPaperQA
	}
}

// TagScopeForAction 返回动作对应的标签作用域
func TagScopeForAction(action model.AIAction) model.TagScope {
	switch action {
	case model.AIActionFigureInterpretation, model.AIActionTagSuggestion:
		return model.TagScopeFigure
	default:
		return model.TagScopePaper
	}
}

// ResolveModelForAction 解析动作对应的模型配置
func ResolveModelForAction(settings model.AISettings, action model.AIAction) (model.AIModelConfig, error) {
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
	return ResolveModelByID(settings.Models, modelID)
}

// ResolveModelByID 根据 ID 解析模型配置
func ResolveModelByID(models []model.AIModelConfig, modelID string) (model.AIModelConfig, error) {
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

// NormalizeSettings 标准化 AI 设置
func NormalizeSettings(input model.AISettings) (model.AISettings, error) {
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

	models, err := normalizeModels(settings, defaults)
	if err != nil {
		return model.AISettings{}, err
	}
	settings.Models = models
	settings.SceneModels = normalizeSceneModelSelection(settings.SceneModels, settings.Models)

	defaultModel, err := ResolveModelByID(settings.Models, settings.SceneModels.DefaultModelID)
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

func normalizeModels(settings model.AISettings, defaults model.AISettings) ([]model.AIModelConfig, error) {
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
		normalized, err := normalizeModelConfig(item, fallbackModel, index+1)
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

func normalizeModelConfig(input model.AIModelConfig, fallback model.AIModelConfig, index int) (model.AIModelConfig, error) {
	config := input

	if strings.TrimSpace(config.ID) == "" {
		config.ID = fmt.Sprintf("model_%d", index)
	}
	config.ID = strings.TrimSpace(config.ID)

	if strings.TrimSpace(string(config.Provider)) == "" {
		config.Provider = fallback.Provider
	}
	if !IsSupportedProvider(config.Provider) {
		return model.AIModelConfig{}, apperr.New(apperr.CodeInvalidArgument, "暂不支持该 AI 提供商")
	}

	config.APIKey = strings.TrimSpace(config.APIKey)
	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	config.Model = strings.TrimSpace(config.Model)
	config.Name = strings.TrimSpace(config.Name)

	if config.BaseURL == "" {
		config.BaseURL = DefaultBaseURL(config.Provider)
	}
	if config.Model == "" {
		config.Model = DefaultModel(config.Provider)
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

func normalizeSceneModelSelection(input model.AISceneModelSelection, models []model.AIModelConfig) model.AISceneModelSelection {
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

// MarshalSettings 将设置序列化为 JSON
func MarshalSettings(settings model.AISettings) (string, error) {
	data, err := json.Marshal(settings)
	if err != nil {
		return "", apperr.Wrap(apperr.CodeInternal, "序列化 AI 设置失败", err)
	}
	return string(data), nil
}

// UnmarshalSettings 从 JSON 反序列化设置
func UnmarshalSettings(data string) (model.AISettings, error) {
	settings := model.DefaultAISettings()
	if strings.TrimSpace(data) != "" {
		if err := json.Unmarshal([]byte(data), &settings); err != nil {
			return model.AISettings{}, apperr.Wrap(apperr.CodeInternal, "解析 AI 设置失败", err)
		}
	}
	return NormalizeSettings(settings)
}

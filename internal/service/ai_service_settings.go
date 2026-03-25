package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

type legacyAIPromptPreset struct {
	Name            string `json:"name"`
	SystemPrompt    string `json:"system_prompt"`
	QAPrompt        string `json:"qa_prompt"`
	FigurePrompt    string `json:"figure_prompt"`
	TagPrompt       string `json:"tag_prompt"`
	GroupPrompt     string `json:"group_prompt"`
	TranslatePrompt string `json:"translate_prompt"`
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

package ai_conversation

import (
	"strings"

	"github.com/xuzhougeng/citebox/internal/model"
)

func assistantMasterSettings(settings model.AISettings) model.AISettings {
	return settingsWithSceneModel(settings, firstNonEmpty(
		settings.SceneModels.AssistantMasterModelID,
		settings.SceneModels.QAModelID,
		settings.SceneModels.DefaultModelID,
	))
}

func assistantSubagentSettings(settings model.AISettings) model.AISettings {
	return settingsWithSceneModel(settings, firstNonEmpty(
		settings.SceneModels.AssistantSubagentModelID,
		settings.SceneModels.IMIntentModelID,
		settings.SceneModels.AssistantMasterModelID,
		settings.SceneModels.DefaultModelID,
	))
}

func settingsWithSceneModel(settings model.AISettings, modelID string) model.AISettings {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return settings
	}
	for _, item := range settings.Models {
		if item.ID == modelID {
			applySceneModelConfig(&settings, item)
			return settings
		}
	}
	return settings
}

func applySceneModelConfig(settings *model.AISettings, config model.AIModelConfig) {
	settings.Provider = config.Provider
	settings.APIKey = config.APIKey
	settings.BaseURL = config.BaseURL
	settings.Model = config.Model
	settings.MaxOutputTokens = config.MaxOutputTokens
	settings.OpenAILegacyMode = config.OpenAILegacyMode
	settings.OmitTemperature = config.OmitTemperature
	settings.ThinkingEnabled = config.ThinkingEnabled
	settings.ReasoningEffort = config.ReasoningEffort
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

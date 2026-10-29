package service

import (
	"github.com/xuzhougeng/citebox/internal/magpie"
	"github.com/xuzhougeng/citebox/internal/model"
)

// RuntimeAISettings supplies request settings without exposing an overlay to
// the settings editor, which saves the user's ordinary model configuration.
type RuntimeAISettings struct{ service *AIService }

func (s *AIService) RuntimeSettings() RuntimeAISettings { return RuntimeAISettings{service: s} }
func (r RuntimeAISettings) GetSettings() (*model.AISettings, error) {
	return r.service.GetRuntimeSettings()
}

// RuntimeManaged reports whether a valid Magpie connection currently supplies
// the text models, so the UI can skip "no API key" warnings for personal settings.
func (s *AIService) RuntimeManaged() bool {
	if s.config == nil {
		return false
	}
	_, ok := magpie.Load(s.config.MagpieConfigPath)
	return ok
}

// GetRuntimeSettings is read for each new operation. In-flight requests keep
// their existing snapshot; a change or disconnect affects the next operation.
func (s *AIService) GetRuntimeSettings() (*model.AISettings, error) {
	settings, err := s.GetSettings()
	if err != nil {
		return nil, err
	}
	if s.config == nil {
		return settings, nil
	}
	connection, ok := magpie.Load(s.config.MagpieConfigPath)
	if !ok {
		return settings, nil
	}
	// Keep IDs and scene bindings so every existing text workflow can resolve
	// its model without persisting the gateway key to the application database.
	for i := range settings.Models {
		p := &settings.Models[i]
		p.Name = "Magpie"
		p.Provider = model.AIProviderOpenAI
		p.BaseURL = connection.BaseURL
		p.APIKey = connection.APIKey
		p.Model = connection.Model
		p.OpenAILegacyMode = true
		p.OmitTemperature = true
		p.ThinkingEnabled = false
		p.ReasoningEffort = ""
		p.MaxOutputTokens = 8192
	}
	if len(settings.Models) > 0 {
		applyAIModelConfig(settings, settings.Models[0])
	}
	return settings, nil
}

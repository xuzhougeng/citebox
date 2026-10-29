package service

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/magpie"
	"github.com/xuzhougeng/citebox/internal/model"
)

// magpieSettingsKey holds only the user's in-app choices for the Magpie
// connection, never the gateway address or key.
const magpieSettingsKey = "magpie_settings"

// magpieDefaultMaxOutputTokens applies when Magpie does not know the model's
// output limit.
const magpieDefaultMaxOutputTokens = 8192

// magpieHelperTimeout replaces the 20-second deadlines of retrieval planning
// and evidence grading. Magpie may route to subscription backends with a high
// fixed latency: through codex/gpt-6.1-sol a plan took 13-16 s and grading 12
// snippets 22 s even at the lightest level.
const magpieHelperTimeout = 45 * time.Second

type magpieSettings struct {
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

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
	_, ok := s.loadMagpie()
	return ok
}

func (s *AIService) loadMagpie() (magpie.Connection, bool) {
	if s.config == nil {
		return magpie.Connection{}, false
	}
	return magpie.Load(s.config.MagpieConfigPath)
}

// GetRuntimeSettings is read for each new operation. In-flight requests keep
// their existing snapshot; a change or disconnect affects the next operation.
func (s *AIService) GetRuntimeSettings() (*model.AISettings, error) {
	settings, err := s.GetSettings()
	if err != nil {
		return nil, err
	}
	connection, ok := s.loadMagpie()
	if !ok {
		return settings, nil
	}
	runtime, err := s.magpieRuntime(connection)
	if err != nil {
		return nil, err
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
		// Magpie takes reasoning_effort; the vendor-specific "thinking" switch
		// is not part of its Chat Completions surface. Without known levels no
		// effort is sent, which leaves the provider default in place.
		p.ThinkingEnabled = false
		p.ReasoningEffort = runtime.ReasoningEffort
		p.MaxOutputTokens = runtime.MaxOutputTokens
	}
	if len(settings.Models) > 0 {
		applyAIModelConfig(settings, settings.Models[0])
	}
	settings.LightReasoningEffort = magpieLightEffort(runtime.ReasoningEfforts, runtime.ReasoningEffort)
	settings.HelperTimeout = magpieHelperTimeout
	return settings, nil
}

// MagpieRuntime reports the connected model and the effort the next request
// uses, so the settings page can offer only the levels that model supports.
func (s *AIService) MagpieRuntime() (*model.AIMagpieRuntime, error) {
	connection, ok := s.loadMagpie()
	if !ok {
		return &model.AIMagpieRuntime{ReasoningEfforts: []string{}}, nil
	}
	return s.magpieRuntime(connection)
}

// UpdateMagpieReasoningEffort saves the in-app choice separately from the
// personal model settings. An empty value returns to the model default.
func (s *AIService) UpdateMagpieReasoningEffort(effort string) (*model.AIMagpieRuntime, error) {
	connection, ok := s.loadMagpie()
	if !ok {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "当前没有有效的 Magpie 连接")
	}
	effort = strings.TrimSpace(effort)
	if effort != "" && !connection.Info.SupportsEffort(effort) {
		return nil, apperr.New(apperr.CodeInvalidArgument, "当前 Magpie 模型不支持该思考强度")
	}
	if effort == "" {
		if err := s.repo.DeleteAppSetting(magpieSettingsKey); err != nil {
			return nil, apperr.Wrap(apperr.CodeInternal, "保存 Magpie 设置失败", err)
		}
	} else {
		payload, err := json.Marshal(magpieSettings{ReasoningEffort: effort})
		if err != nil {
			return nil, apperr.Wrap(apperr.CodeInternal, "序列化 Magpie 设置失败", err)
		}
		if err := s.repo.UpsertAppSetting(magpieSettingsKey, string(payload)); err != nil {
			return nil, apperr.Wrap(apperr.CodeInternal, "保存 Magpie 设置失败", err)
		}
	}
	return s.magpieRuntime(connection)
}

func (s *AIService) magpieRuntime(connection magpie.Connection) (*model.AIMagpieRuntime, error) {
	saved, err := s.magpieSettings()
	if err != nil {
		return nil, err
	}
	runtime := &model.AIMagpieRuntime{
		Connected:        true,
		Model:            connection.Model,
		MaxOutputTokens:  magpieDefaultMaxOutputTokens,
		ReasoningEfforts: []string{},
	}
	if info := connection.Info; info != nil {
		runtime.ContextTokens = info.Context
		if info.Output > 0 {
			runtime.MaxOutputTokens = info.Output
		}
		runtime.ReasoningEfforts = append(runtime.ReasoningEfforts, info.Efforts...)
	}
	runtime.DefaultReasoningEffort = magpieDefaultEffort(runtime.ReasoningEfforts)
	// A saved level survives model switches but is sent only while the
	// current model supports it.
	runtime.ReasoningEffort = runtime.DefaultReasoningEffort
	if connection.Info.SupportsEffort(saved.ReasoningEffort) {
		runtime.ReasoningEffort = saved.ReasoningEffort
	}
	return runtime, nil
}

func (s *AIService) magpieSettings() (magpieSettings, error) {
	var settings magpieSettings
	raw, err := s.repo.GetAppSetting(magpieSettingsKey)
	if err != nil {
		return settings, apperr.Wrap(apperr.CodeInternal, "读取 Magpie 设置失败", err)
	}
	// A damaged preference only loses the choice; the connection stays usable.
	if strings.TrimSpace(raw) != "" && json.Unmarshal([]byte(raw), &settings) != nil {
		return magpieSettings{}, nil
	}
	return settings, nil
}

func magpieDefaultEffort(efforts []string) string {
	for _, preferred := range []string{"medium", "high"} {
		for _, effort := range efforts {
			if effort == preferred {
				return effort
			}
		}
	}
	if len(efforts) > 0 {
		return efforts[0]
	}
	return ""
}

// magpieLightEffort picks the level for retrieval planning and evidence
// grading. These calls have hard deadlines and answer with a short JSON
// object, so a lower level than the chat answer keeps them from timing out.
func magpieLightEffort(efforts []string, effort string) string {
	switch effort {
	case "", "none", "minimal", "low":
		return effort
	}
	for _, light := range []string{"low", "minimal"} {
		for _, item := range efforts {
			if item == light {
				return light
			}
		}
	}
	return effort
}

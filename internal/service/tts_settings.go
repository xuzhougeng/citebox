package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

const (
	ttsSettingsKey     = "tts_settings"
	ttsTestFilename    = "tts-test.mp3"
	ttsTestContentType = "audio/mpeg"
	ttsTestDemoText    = "Hello World from CiteBox test voice"
)

type legacyWeixinBridgeTTSSettings struct {
	TTSAppID      string `json:"tts_app_id"`
	TTSAccessKey  string `json:"tts_access_key"`
	TTSResourceID string `json:"tts_resource_id"`
	TTSSpeaker    string `json:"tts_speaker"`
}

type persistedTTSSettings struct {
	AppID                    string `json:"app_id"`
	AccessKey                string `json:"access_key"`
	ResourceID               string `json:"resource_id"`
	Speaker                  string `json:"speaker"`
	WeixinVoiceOutputEnabled *bool  `json:"weixin_voice_output_enabled"`
}

func (s *LibraryService) GetTTSSettings() (*model.TTSSettings, error) {
	raw, err := s.repo.GetAppSetting(ttsSettingsKey)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "读取 TTS 配置失败", err)
	}

	settings := normalizeTTSSettings(model.TTSSettings{})
	if strings.TrimSpace(raw) != "" {
		settings, err := decodePersistedTTSSettings(raw)
		if err != nil {
			return nil, apperr.Wrap(apperr.CodeInternal, "解析 TTS 配置失败", err)
		}
		return &settings, nil
	}

	legacyRaw, err := s.repo.GetAppSetting(weixinBridgeSettingsKey)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "读取旧版 TTS 配置失败", err)
	}
	if strings.TrimSpace(legacyRaw) == "" {
		return &settings, nil
	}

	var legacy legacyWeixinBridgeTTSSettings
	if err := json.Unmarshal([]byte(legacyRaw), &legacy); err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "解析旧版 TTS 配置失败", err)
	}

	settings = normalizeTTSSettings(model.TTSSettings{
		AppID:      legacy.TTSAppID,
		AccessKey:  legacy.TTSAccessKey,
		ResourceID: legacy.TTSResourceID,
		Speaker:    legacy.TTSSpeaker,
	})
	return &settings, nil
}

func (s *LibraryService) UpdateTTSSettings(input model.TTSSettings) (*model.TTSSettings, error) {
	settings := normalizeTTSSettings(input)

	payload, err := json.Marshal(settings)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "序列化 TTS 配置失败", err)
	}
	if err := s.repo.UpsertAppSetting(ttsSettingsKey, string(payload)); err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "保存 TTS 配置失败", err)
	}

	return &settings, nil
}

func (s *LibraryService) SetWeixinVoiceOutputEnabled(enabled bool) (*model.TTSSettings, error) {
	settings, err := s.GetTTSSettings()
	if err != nil {
		return nil, err
	}
	if settings == nil {
		settings = &model.TTSSettings{}
	}
	settings.WeixinVoiceOutputEnabled = enabled
	settings.WeixinVoiceOutputEnabledSet = true
	return s.UpdateTTSSettings(*settings)
}

func (s *LibraryService) getTTSSettingsSummary() model.TTSSettings {
	settings, err := s.GetTTSSettings()
	if err != nil {
		s.logger.Warn("load tts settings failed", "error", err)
		return normalizeTTSSettings(model.TTSSettings{})
	}
	return *settings
}

func (s *LibraryService) TestTTS(ctx context.Context, input model.TTSSettings) ([]byte, string, string, error) {
	settings := normalizeTTSSettings(input)
	if err := validateTTSSettings(settings); err != nil {
		return nil, "", "", apperr.New(apperr.CodeInvalidArgument, "请先填写完整的 TTS 配置")
	}
	if s.ttsAudioSynthesizer == nil {
		return nil, "", "", apperr.New(apperr.CodeInternal, "TTS 测试器未初始化")
	}

	audio, extension, err := s.ttsAudioSynthesizer(ctx, ttsTestDemoText, settings)
	if err != nil {
		return nil, "", "", apperr.New(apperr.CodeUnavailable, fmt.Sprintf("TTS 测试失败：%v", err))
	}
	if len(audio) == 0 {
		return nil, "", "", apperr.New(apperr.CodeUnavailable, "TTS 测试失败：返回了空音频")
	}

	filename := ttsTestFilename
	if trimmed := strings.TrimSpace(extension); trimmed != "" && trimmed != ".mp3" {
		filename = "tts-test" + trimmed
	}
	contentType := ttsTestContentType
	if extension != "" && extension != ".mp3" {
		contentType = detectTTSAudioContentType(extension)
	}

	return audio, filename, contentType, nil
}

func synthesizeTTSTestAudio(ctx context.Context, text string, settings model.TTSSettings) ([]byte, string, error) {
	return synthesizeDoubaoTTSAudio(ctx, doubaoTTSHTTPClient, newDoubaoTTSSettings(settings), text, "citebox-settings-test")
}

func decodePersistedTTSSettings(raw string) (model.TTSSettings, error) {
	var persisted persistedTTSSettings
	if err := json.Unmarshal([]byte(raw), &persisted); err != nil {
		return model.TTSSettings{}, err
	}

	settings := model.TTSSettings{
		AppID:                    persisted.AppID,
		AccessKey:                persisted.AccessKey,
		ResourceID:               persisted.ResourceID,
		Speaker:                  persisted.Speaker,
		WeixinVoiceOutputEnabled: model.DefaultWeixinVoiceOutputEnabled,
	}
	if persisted.WeixinVoiceOutputEnabled != nil {
		settings.WeixinVoiceOutputEnabled = *persisted.WeixinVoiceOutputEnabled
		settings.WeixinVoiceOutputEnabledSet = true
	}
	return normalizeTTSSettings(settings), nil
}

func normalizeTTSSettings(input model.TTSSettings) model.TTSSettings {
	weixinVoiceOutputEnabled := model.DefaultWeixinVoiceOutputEnabled
	if input.WeixinVoiceOutputEnabledSet {
		weixinVoiceOutputEnabled = input.WeixinVoiceOutputEnabled
	}

	settings := model.TTSSettings{
		AppID:                       strings.TrimSpace(input.AppID),
		AccessKey:                   strings.TrimSpace(input.AccessKey),
		ResourceID:                  strings.TrimSpace(input.ResourceID),
		Speaker:                     strings.TrimSpace(input.Speaker),
		WeixinVoiceOutputEnabled:    weixinVoiceOutputEnabled,
		WeixinVoiceOutputEnabledSet: true,
	}
	if settings.ResourceID == "" {
		settings.ResourceID = doubaoTTSDefaultResourceID
	}
	return settings
}

func validateTTSSettings(settings model.TTSSettings) error {
	if !isTTSConfigured(settings) {
		return fmt.Errorf("tts is not configured")
	}
	return nil
}

func isTTSConfigured(settings model.TTSSettings) bool {
	return strings.TrimSpace(settings.AppID) != "" &&
		strings.TrimSpace(settings.AccessKey) != "" &&
		strings.TrimSpace(settings.Speaker) != ""
}

func detectTTSAudioContentType(extension string) string {
	switch strings.ToLower(strings.TrimSpace(extension)) {
	case ".wav":
		return "audio/wav"
	case ".ogg":
		return "audio/ogg"
	case ".m4a":
		return "audio/mp4"
	case ".aac":
		return "audio/aac"
	case ".flac":
		return "audio/flac"
	case ".mp3":
		return "audio/mpeg"
	default:
		return "application/octet-stream"
	}
}

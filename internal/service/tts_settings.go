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

func (s *LibraryService) GetTTSSettings() (*model.TTSSettings, error) {
	raw, err := s.repo.GetAppSetting(ttsSettingsKey)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "读取 TTS 配置失败", err)
	}

	settings := normalizeTTSSettings(model.TTSSettings{})
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &settings); err != nil {
			return nil, apperr.Wrap(apperr.CodeInternal, "解析 TTS 配置失败", err)
		}
		settings = normalizeTTSSettings(settings)
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
		return nil, "", "", apperr.Wrap(apperr.CodeUnavailable, "TTS 测试失败", err)
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

func normalizeTTSSettings(input model.TTSSettings) model.TTSSettings {
	settings := model.TTSSettings{
		AppID:      strings.TrimSpace(input.AppID),
		AccessKey:  strings.TrimSpace(input.AccessKey),
		ResourceID: strings.TrimSpace(input.ResourceID),
		Speaker:    strings.TrimSpace(input.Speaker),
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

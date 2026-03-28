package service

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

const weixinBridgeSettingsKey = "weixin_bridge_settings"

func (s *LibraryService) GetWeixinBridgeSettings() (*model.WeixinBridgeSettings, error) {
	raw, err := s.repo.GetAppSetting(weixinBridgeSettingsKey)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "读取微信桥接配置失败", err)
	}

	settings := defaultWeixinBridgeSettings(s.config.WeixinBridgeEnabled)
	if strings.TrimSpace(raw) == "" {
		return &settings, nil
	}

	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "解析微信桥接配置失败", err)
	}
	settings = normalizeWeixinBridgeSettings(settings)
	return &settings, nil
}

func (s *LibraryService) UpdateWeixinBridgeSettings(input model.WeixinBridgeSettings) (*model.WeixinBridgeSettings, error) {
	settings, err := validateWeixinBridgeSettings(input)
	if err != nil {
		return nil, err
	}

	payload, err := json.Marshal(settings)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "序列化微信桥接配置失败", err)
	}
	if err := s.repo.UpsertAppSetting(weixinBridgeSettingsKey, string(payload)); err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "保存微信桥接配置失败", err)
	}

	return &settings, nil
}

func (s *LibraryService) getWeixinBridgeSettingsSummary() model.WeixinBridgeSettings {
	settings, err := s.GetWeixinBridgeSettings()
	if err != nil {
		s.logger.Warn("load weixin bridge settings failed", "error", err)
		return defaultWeixinBridgeSettings(s.config.WeixinBridgeEnabled)
	}
	return *settings
}

func (s *LibraryService) isWeixinBridgeEnabled() (bool, error) {
	settings, err := s.GetWeixinBridgeSettings()
	if err != nil {
		return false, err
	}
	return settings.Enabled, nil
}

func (s *LibraryService) disableWeixinBridge() (*model.WeixinBridgeSettings, error) {
	settings, err := s.GetWeixinBridgeSettings()
	if err != nil {
		return nil, err
	}
	if !settings.Enabled {
		return settings, nil
	}

	settings.Enabled = false
	return s.UpdateWeixinBridgeSettings(*settings)
}

func validateWeixinBridgeSettings(input model.WeixinBridgeSettings) (model.WeixinBridgeSettings, error) {
	settings := defaultWeixinBridgeSettings(input.Enabled)
	settings.Enabled = input.Enabled
	settings.DailyRecommendation.Enabled = input.DailyRecommendation.Enabled

	sendTime, err := normalizeWeixinDailyRecommendationSendTime(input.DailyRecommendation.SendTime)
	if err != nil {
		return model.WeixinBridgeSettings{}, err
	}
	settings.DailyRecommendation.SendTime = sendTime
	return settings, nil
}

func defaultWeixinBridgeSettings(enabled bool) model.WeixinBridgeSettings {
	return model.WeixinBridgeSettings{
		Enabled: enabled,
		DailyRecommendation: model.WeixinDailyRecommendationSettings{
			Enabled:  false,
			SendTime: model.DefaultWeixinDailyRecommendationSendTime,
		},
	}
}

func normalizeWeixinBridgeSettings(input model.WeixinBridgeSettings) model.WeixinBridgeSettings {
	settings := defaultWeixinBridgeSettings(input.Enabled)
	settings.Enabled = input.Enabled
	settings.DailyRecommendation.Enabled = input.DailyRecommendation.Enabled

	sendTime, err := normalizeWeixinDailyRecommendationSendTime(input.DailyRecommendation.SendTime)
	if err == nil {
		settings.DailyRecommendation.SendTime = sendTime
	}
	return settings
}

func normalizeWeixinDailyRecommendationSendTime(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return model.DefaultWeixinDailyRecommendationSendTime, nil
	}

	parsed, err := time.Parse("15:04", trimmed)
	if err != nil {
		return "", apperr.New(apperr.CodeInvalidArgument, "今日推荐时间格式必须是 HH:MM")
	}
	return parsed.Format("15:04"), nil
}

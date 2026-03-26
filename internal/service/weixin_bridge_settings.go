package service

import (
	"encoding/json"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

const weixinBridgeSettingsKey = "weixin_bridge_settings"

func (s *LibraryService) GetWeixinBridgeSettings() (*model.WeixinBridgeSettings, error) {
	raw, err := s.repo.GetAppSetting(weixinBridgeSettingsKey)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "读取微信桥接配置失败", err)
	}

	settings := normalizeWeixinBridgeSettings(model.WeixinBridgeSettings{
		Enabled: s.config.WeixinBridgeEnabled,
	})
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
	settings := normalizeWeixinBridgeSettings(input)

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
		return normalizeWeixinBridgeSettings(model.WeixinBridgeSettings{
			Enabled: s.config.WeixinBridgeEnabled,
		})
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

func normalizeWeixinBridgeSettings(input model.WeixinBridgeSettings) model.WeixinBridgeSettings {
	return model.WeixinBridgeSettings{
		Enabled: input.Enabled,
	}
}

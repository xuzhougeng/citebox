package service

import (
	"encoding/json"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

const desktopCloseSettingsKey = "desktop_close_settings"

func (s *LibraryService) GetDesktopCloseSettings() (*model.DesktopCloseSettings, error) {
	raw, err := s.repo.GetAppSetting(desktopCloseSettingsKey)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "读取桌面端关闭配置失败", err)
	}

	settings := model.DesktopCloseSettings{
		Action: model.DesktopCloseActionAsk,
	}
	if strings.TrimSpace(raw) == "" {
		return &settings, nil
	}

	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "解析桌面端关闭配置失败", err)
	}

	normalized := normalizeDesktopCloseSettings(settings)
	return &normalized, nil
}

func (s *LibraryService) UpdateDesktopCloseSettings(input model.DesktopCloseSettings) (*model.DesktopCloseSettings, error) {
	settings := normalizeDesktopCloseSettings(input)

	if settings.Action == model.DesktopCloseActionAsk {
		if err := s.repo.DeleteAppSetting(desktopCloseSettingsKey); err != nil {
			return nil, apperr.Wrap(apperr.CodeInternal, "保存桌面端关闭配置失败", err)
		}
		return &settings, nil
	}

	payload, err := json.Marshal(settings)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "序列化桌面端关闭配置失败", err)
	}
	if err := s.repo.UpsertAppSetting(desktopCloseSettingsKey, string(payload)); err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "保存桌面端关闭配置失败", err)
	}

	return &settings, nil
}

func normalizeDesktopCloseSettings(input model.DesktopCloseSettings) model.DesktopCloseSettings {
	return model.DesktopCloseSettings{
		Action: model.NormalizeDesktopCloseAction(strings.TrimSpace(input.Action)),
	}
}

package service

import (
	"encoding/json"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/zotero"
)

const zoteroSettingsKey = "zotero_settings"

func defaultZoteroSettings() model.ZoteroSettings {
	return model.ZoteroSettings{
		BaseURL:         zotero.DefaultBaseURL,
		IncludeChildren: true,
	}
}

func (s *LibraryService) GetZoteroSettings() (*model.ZoteroSettings, error) {
	raw, err := s.repo.GetAppSetting(zoteroSettingsKey)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "读取 Zotero 配置失败", err)
	}
	settings := defaultZoteroSettings()
	if strings.TrimSpace(raw) == "" {
		return &settings, nil
	}
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "解析 Zotero 配置失败", err)
	}
	normalized, err := normalizeZoteroSettings(settings)
	if err != nil {
		return nil, err
	}
	return &normalized, nil
}

func (s *LibraryService) UpdateZoteroSettings(input model.ZoteroSettings) (*model.ZoteroSettings, error) {
	settings, err := normalizeZoteroSettings(input)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(settings)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "序列化 Zotero 配置失败", err)
	}
	if err := s.repo.UpsertAppSetting(zoteroSettingsKey, string(payload)); err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "保存 Zotero 配置失败", err)
	}
	return &settings, nil
}

func normalizeZoteroSettings(input model.ZoteroSettings) (model.ZoteroSettings, error) {
	baseURL, err := zotero.NormalizeBaseURL(input.BaseURL)
	if err != nil {
		return model.ZoteroSettings{}, apperr.New(apperr.CodeInvalidArgument, err.Error())
	}
	keys := make([]string, 0, len(input.LastCollectionKeys))
	seen := map[string]struct{}{}
	for _, key := range input.LastCollectionKeys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	return model.ZoteroSettings{
		BaseURL:            baseURL,
		IncludeChildren:    input.IncludeChildren,
		LastCollectionKeys: keys,
		LastRunID:          strings.TrimSpace(input.LastRunID),
	}, nil
}

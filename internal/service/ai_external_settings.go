package service

import (
	"encoding/json"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

const aiExternalSearchSettingsKey = "ai_external_search_settings"

func (s *LibraryService) GetAIExternalSearchSettings() (*model.AIExternalSearchSettings, error) {
	raw, err := s.repo.GetAppSetting(aiExternalSearchSettingsKey)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "读取 AI 外部搜索配置失败", err)
	}
	settings := model.NormalizeAIExternalSearchSettings(model.AIExternalSearchSettings{})
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &settings); err != nil {
			return nil, apperr.Wrap(apperr.CodeInternal, "解析 AI 外部搜索配置失败", err)
		}
	}
	normalized := model.NormalizeAIExternalSearchSettings(settings)
	return &normalized, nil
}

func (s *LibraryService) UpdateAIExternalSearchSettings(input model.AIExternalSearchSettings) (*model.AIExternalSearchSettings, error) {
	settings := model.NormalizeAIExternalSearchSettings(input)
	payload, err := json.Marshal(settings)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "序列化 AI 外部搜索配置失败", err)
	}
	if err := s.repo.UpsertAppSetting(aiExternalSearchSettingsKey, string(payload)); err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "保存 AI 外部搜索配置失败", err)
	}
	return &settings, nil
}

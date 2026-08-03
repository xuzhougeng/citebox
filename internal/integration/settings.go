package integration

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/repository"
)

const (
	// settingsKey 是集成设置在 app_settings 表中的键
	settingsKey = "integration_settings"
	// DefaultPort 是内置 MCP 服务的默认端口
	DefaultPort = 19831
)

// IntegrationSettings 外部集成（内置 MCP 服务）设置
type IntegrationSettings struct {
	Enabled bool `json:"enabled"`
	Port    int  `json:"port"`
}

// DefaultSettings 返回默认设置：关闭、默认端口
func DefaultSettings() IntegrationSettings {
	return IntegrationSettings{Enabled: false, Port: DefaultPort}
}

// Service 管理集成设置的读写，存储模式与 RemoteMCPService 一致（JSON 存入 app_settings）
type Service struct {
	repo *repository.SettingRepository
}

// NewService 创建集成设置服务
func NewService(repo *repository.SettingRepository) *Service {
	return &Service{repo: repo}
}

// Get 读取集成设置；未保存或内容损坏时返回默认设置
func (s *Service) Get() IntegrationSettings {
	raw, err := s.repo.GetAppSetting(settingsKey)
	if err != nil || strings.TrimSpace(raw) == "" {
		return DefaultSettings()
	}
	settings := DefaultSettings()
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return DefaultSettings()
	}
	if settings.Port < 1 || settings.Port > 65535 {
		settings.Port = DefaultPort
	}
	return settings
}

// Update 保存集成设置
func (s *Service) Update(enabled bool, port int) error {
	if port < 1 || port > 65535 {
		return apperr.New(apperr.CodeInvalidArgument, "集成服务端口无效")
	}
	raw, err := json.Marshal(IntegrationSettings{Enabled: enabled, Port: port})
	if err != nil {
		return apperr.Wrap(apperr.CodeInternal, "编码集成设置失败", err)
	}
	return s.repo.UpsertAppSetting(settingsKey, string(raw))
}

// BaseURL 返回集成服务的基础地址（仅绑定回环地址）
func (s *Service) BaseURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", s.Get().Port)
}

package service

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/codexapp"
	"github.com/xuzhougeng/citebox/internal/config"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

const (
	aiSettingsKey                = "ai_settings"
	aiRolePromptsKey             = "ai_prompt_presets"
	aiFigureImageMaxBytes        = 3 * 1024 * 1024
	aiFigureImageTotalBudget     = 12 * 1024 * 1024
	aiFigureImageMaxDimension    = 2200
	aiFigureImageMinDimension    = 960
	aiFigureImageJPEGQuality     = 82
	aiFigureImageMinJPEGQuality  = 58
	aiFigureImageCompressionRuns = 6
)

type AIService struct {
	repo       *repository.LibraryRepository
	config     *config.Config
	httpClient *http.Client
	logger     *slog.Logger
	codex      *codexapp.Client
}

func (s *AIService) SetCodexClient(client *codexapp.Client) {
	s.codex = client
}

func (s *AIService) Close() error {
	if s == nil || s.codex == nil {
		return nil
	}
	return s.codex.Close()
}

func (s *AIService) CodexStatus(ctx context.Context) codexapp.Status {
	if s == nil || s.codex == nil {
		return codexapp.Status{Message: "Codex 订阅仅在 CiteBox 桌面端可用"}
	}
	return s.codex.Status(ctx)
}

func (s *AIService) CodexModels(ctx context.Context) ([]codexapp.Model, error) {
	if s == nil || s.codex == nil {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "Codex 订阅仅在 CiteBox 桌面端可用")
	}
	models, err := s.codex.Models(ctx)
	if err != nil {
		return nil, mapCodexError(err)
	}
	return models, nil
}

type aiImageInput struct {
	MIMEType string
	Data     string
}

type aiReadPrepared struct {
	settings          model.AISettings
	action            model.AIAction
	question          string
	promptQuestion    string
	activeRolePrompts []model.AIRolePrompt
	paper             *model.Paper
	systemPrompt      string
	userPrompt        string
	includedFigures   int
	images            []aiImageInput
}

func NewAIService(repo *repository.LibraryRepository, cfg *config.Config, logger *slog.Logger) *AIService {
	if logger == nil {
		logger = slog.Default().With("component", "ai_service")
	}

	return &AIService{
		repo:   repo,
		config: cfg,
		logger: logger,
		httpClient: &http.Client{
			Timeout: 180 * time.Second,
		},
	}
}

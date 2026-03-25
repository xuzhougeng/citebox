package service

import (
	"log/slog"
	"net/http"
	"time"

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

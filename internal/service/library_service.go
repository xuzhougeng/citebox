package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/config"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

type LibraryService struct {
	repo                *repository.LibraryRepository
	repoMu              sync.RWMutex // protects repo during ImportDatabase
	config              *config.Config
	httpClient          *http.Client
	logger              *slog.Logger
	startBackground     bool
	weixinRecommendMu   sync.Mutex
	pdfTextExtractor    func(string) (string, error)
	ttsAudioSynthesizer func(context.Context, string, model.TTSSettings) ([]byte, string, error)
	weixinClientFactory func(token string) weixinBindingClient
	wolaiClientFactory  func(settings model.WolaiSettings) (wolaiClient, error)
}

const (
	extractorSettingsKey             = "extractor_settings"
	runtimePasswordKey               = "runtime_admin_password"
	manualPendingStatus              = "manual_pending"
	extractionModeAuto               = "auto"
	extractionModeManual             = "manual"
	extractorProfileManual           = "manual"
	extractorProfilePDFFigXV1        = "pdffigx_v1"
	extractorProfileOpenSourceVision = "open_source_vision"
	pdfTextSourceExtractor           = "extractor"
	pdfTextSourcePDFJS               = "pdfjs"
	figureSourceAuto                 = "auto"
	manualFigureSourceManual         = "manual"
	manualFigureSourceLLM            = "llm"
)

type LibraryServiceOption func(*LibraryService)

type DuplicatePaperError struct {
	Paper *model.Paper
	Err   error
}

func (e *DuplicatePaperError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "PDF 已存在"
}

func (e *DuplicatePaperError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type UploadPaperParams struct {
	Title          string
	GroupID        *int64
	Tags           []string
	ExtractionMode string
}

type UpdatePaperParams struct {
	Title          string
	PDFText        *string
	AbstractText   string
	NotesText      string
	PaperNotesText string
	GroupID        *int64
	Tags           []string
}

type UpdateFigureParams struct {
	Tags      []string
	Caption   *string
	NotesText *string
}

type CreatePaletteParams struct {
	Name   string
	Colors []string
}

type CreateSubfiguresParams struct {
	Regions []model.SubfigureExtractionRegion
}

type ManualExtractParams struct {
	Regions []model.ManualExtractionRegion
}

type extractionResult struct {
	PDFText string
	Boxes   json.RawMessage
	Figures []extractedFigure
}

type extractedFigure struct {
	Filename    string
	ContentType string
	PageNumber  int
	FigureIndex int
	Caption     string
	BBox        json.RawMessage
	Data        string
	Source      string
}

type extractorResponse struct {
	Success  *bool             `json:"success"`
	Status   string            `json:"status"`
	Message  string            `json:"message"`
	PDFText  string            `json:"pdf_text"`
	Text     string            `json:"text"`
	FullText string            `json:"full_text"`
	Boxes    json.RawMessage   `json:"boxes"`
	Figures  []extractorFigure `json:"figures"`
	Images   []extractorFigure `json:"images"`
}

type extractorFigure struct {
	Filename        string          `json:"filename"`
	Name            string          `json:"name"`
	ContentType     string          `json:"content_type"`
	MIMEType        string          `json:"mime_type"`
	PageNumber      int             `json:"page_number"`
	Page            int             `json:"page"`
	FigureIndex     int             `json:"figure_index"`
	Index           int             `json:"index"`
	Caption         string          `json:"caption"`
	BBox            json.RawMessage `json:"bbox"`
	Box             json.RawMessage `json:"box"`
	Data            string          `json:"data"`
	Base64          string          `json:"base64"`
	ImageBase64     string          `json:"image_base64"`
	ThumbnailBase64 string          `json:"thumbnail_base64"`
	ImageURL        string          `json:"image_url"`
	ThumbnailURL    string          `json:"thumbnail_url"`
}

type extractorJobStatusResponse struct {
	JobID     string `json:"job_id"`
	Status    string `json:"status"`
	StatusURL string `json:"status_url"`
	ResultURL string `json:"result_url"`
	Error     string `json:"error"`
}

func WithLogger(logger *slog.Logger) LibraryServiceOption {
	return func(service *LibraryService) {
		if logger != nil {
			service.logger = logger
		}
	}
}

func WithoutBackgroundJobs() LibraryServiceOption {
	return func(service *LibraryService) {
		service.startBackground = false
	}
}

func NewLibraryService(repo *repository.LibraryRepository, cfg *config.Config, opts ...LibraryServiceOption) (*LibraryService, error) {
	for _, dir := range []string{cfg.StorageDir, cfg.PapersDir(), cfg.FiguresDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, apperr.Wrap(apperr.CodeInternal, fmt.Sprintf("创建存储目录失败: %s", dir), err)
		}
	}

	service := &LibraryService{
		repo:                repo,
		config:              cfg,
		logger:              slog.Default().With("component", "library_service"),
		startBackground:     true,
		httpClient:          &http.Client{},
		ttsAudioSynthesizer: synthesizeTTSTestAudio,
		weixinClientFactory: defaultWeixinBindingClientFactory,
		wolaiClientFactory:  defaultWolaiClientFactory,
	}
	service.pdfTextExtractor = service.extractServerPDFTextFallback

	for _, opt := range opts {
		opt(service)
	}

	if err := service.backfillPaperChecksums(); err != nil {
		return nil, err
	}
	if err := service.migrateLegacyManualPendingPapers(); err != nil {
		return nil, err
	}

	if service.startBackground {
		go service.resumePendingExtractions()
	}

	return service, nil
}

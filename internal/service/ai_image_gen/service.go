package ai_image_gen

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

// imageRepoWriter is the subset of AIGeneratedImageRepository used by Service.
type imageRepoWriter interface {
	InsertImage(repository.AIGeneratedImage) (int64, error)
	AddResultCard(repository.AIResultCard) (int64, error)
}

// LoadInputsFn loads paper + figure context and the base64 vision images for
// the requested IDs.
type LoadInputsFn func(ctx context.Context, paperIDs, figureIDs []int64) (VisionInputContext, []model.AIImageInput, error)

// SettingsFn returns the image-gen-specific settings + the broader AI settings.
type SettingsFn func() (model.AIImageGenSettings, model.AISettings, error)

type ServiceDeps struct {
	Repo        imageRepoWriter
	Storage     *Storage
	Vision      VisionCaller
	API         APIClient
	LoadInputs  LoadInputsFn
	GetSettings SettingsFn
	// VisionTimeout / ImageTimeout cap each stage. Defaults: vision 60s, image 120s.
	VisionTimeout time.Duration
	ImageTimeout  time.Duration
}

type Service struct {
	deps ServiceDeps
}

func NewService(deps ServiceDeps) *Service {
	if deps.VisionTimeout == 0 {
		deps.VisionTimeout = 60 * time.Second
	}
	if deps.ImageTimeout == 0 {
		deps.ImageTimeout = 120 * time.Second
	}
	return &Service{deps: deps}
}

func (s *Service) Generate(ctx context.Context, in GenerateInput) error {
	emit := func(t string, data any) {
		if in.OnEvent == nil {
			return
		}
		_ = in.OnEvent(StreamEvent{Type: t, Data: data})
	}

	imgSettings, aiSettings, err := s.deps.GetSettings()
	if err != nil {
		return err
	}
	if !imgSettings.Enabled {
		return apperr.New(apperr.CodeFailedPrecondition, "请先在 AI 设置中启用图像生成")
	}
	if len(in.PaperIDs) == 0 && len(in.FigureIDs) == 0 {
		return apperr.New(apperr.CodeInvalidArgument, "请引用至少一篇文献或一张图")
	}

	emit("image_prompt_drafting", PromptDraftedEvent{TurnRunID: in.TurnRunID})

	visionCtx, cancelVision := context.WithTimeout(ctx, s.deps.VisionTimeout)
	defer cancelVision()

	visionInput, images, err := s.deps.LoadInputs(visionCtx, in.PaperIDs, in.FigureIDs)
	if err != nil {
		emit("image_failed", FailedEvent{TurnRunID: in.TurnRunID, Stage: "vision", Reason: err.Error()})
		return err
	}
	visionInput.UserText = in.UserText

	imagePrompt, err := s.deps.Vision.CallVision(visionCtx, aiSettings,
		VisionSystemPrompt(), BuildVisionUserPrompt(visionInput), images)
	if err != nil {
		emit("image_failed", FailedEvent{TurnRunID: in.TurnRunID, Stage: "vision", Reason: err.Error()})
		return err
	}
	emit("image_prompt_drafted", PromptDraftedEvent{TurnRunID: in.TurnRunID, Prompt: imagePrompt})

	cost := costEstimate(imgSettings.Model, imgSettings.Size, imgSettings.Quality)
	emit("image_generating", GeneratingEvent{
		TurnRunID: in.TurnRunID, Model: imgSettings.Model,
		Size: imgSettings.Size, Quality: imgSettings.Quality, CostEstimateUSD: cost,
	})

	// Detach from request context so a client disconnect can't abort a paid
	// API call mid-flight.
	imageCtx, cancelImage := context.WithTimeout(context.Background(), s.deps.ImageTimeout)
	defer cancelImage()

	pngBytes, err := s.deps.API.Generate(imageCtx, imgSettings, imagePrompt)
	if err != nil {
		emit("image_failed", FailedEvent{TurnRunID: in.TurnRunID, Stage: "image_api", Reason: err.Error()})
		return err
	}

	relPath, err := s.deps.Storage.Save(in.ConversationID, pngBytes)
	if err != nil {
		emit("image_failed", FailedEvent{TurnRunID: in.TurnRunID, Stage: "save", Reason: err.Error()})
		return err
	}

	row := repository.AIGeneratedImage{
		TurnRunID: in.TurnRunID, ConversationID: in.ConversationID,
		FilePath: relPath, Prompt: imagePrompt,
		Model: imgSettings.Model, Size: imgSettings.Size, Quality: imgSettings.Quality,
		SourcePaperIDs: in.PaperIDs, SourceFigureIDs: in.FigureIDs,
		CostEstimateUSD: nullFloat(cost),
	}
	imageID, err := s.deps.Repo.InsertImage(row)
	if err != nil {
		emit("image_failed", FailedEvent{TurnRunID: in.TurnRunID, Stage: "save", Reason: err.Error()})
		return err
	}

	cardPayload := map[string]interface{}{
		"image_id":          imageID,
		"file_url":          fmt.Sprintf("/api/ai-generated-images/%d/file", imageID),
		"prompt":            imagePrompt,
		"model":             imgSettings.Model,
		"size":              imgSettings.Size,
		"quality":           imgSettings.Quality,
		"source_paper_ids":  in.PaperIDs,
		"source_figure_ids": in.FigureIDs,
		"cost_estimate_usd": cost,
	}
	cardJSON, err := jsonMarshal(cardPayload)
	if err != nil {
		return err
	}
	if _, err := s.deps.Repo.AddResultCard(repository.AIResultCard{
		TurnRunID: in.TurnRunID, CardType: "generated_image", SortOrder: 0, PayloadJSON: cardJSON,
	}); err != nil {
		emit("image_failed", FailedEvent{TurnRunID: in.TurnRunID, Stage: "save", Reason: err.Error()})
		return err
	}
	emit("image_generated", GeneratedEvent{TurnRunID: in.TurnRunID, Card: cardPayload})
	return nil
}

func costEstimate(modelName, size, quality string) float64 {
	switch quality {
	case "low":
		return 0.04
	case "medium":
		return 0.07
	case "high":
		return 0.19
	}
	return 0.0
}

func nullFloat(v float64) sql.NullFloat64 {
	return sql.NullFloat64{Float64: v, Valid: true}
}

func jsonMarshal(v interface{}) (string, error) {
	out, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

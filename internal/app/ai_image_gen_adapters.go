package app

import (
	"context"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service"
)

// aiGeneratedImageRepoAdapter bridges *repository.LibraryRepository into the
// unexported imageRepoWriter interface required by ai_image_gen.Service.
type aiGeneratedImageRepoAdapter struct {
	libRepo *repository.LibraryRepository
}

func (a aiGeneratedImageRepoAdapter) InsertImage(img repository.AIGeneratedImage) (int64, error) {
	return a.libRepo.AIGeneratedImage.Insert(img)
}

func (a aiGeneratedImageRepoAdapter) AddResultCard(card repository.AIResultCard) (int64, error) {
	return a.libRepo.AIConversation.AddResultCard(card)
}

func (a aiGeneratedImageRepoAdapter) DeleteImage(id int64) error {
	return a.libRepo.AIGeneratedImage.DeleteImage(id)
}

// visionAdapter bridges *service.AIService into the ai_image_gen.VisionCaller
// interface by delegating to CallVisionForImageGen.
type visionAdapter struct {
	svc *service.AIService
}

func (a visionAdapter) CallVision(ctx context.Context, settings model.AISettings,
	systemPrompt, userPrompt string, images []model.AIImageInput) (string, error) {
	return a.svc.CallVisionForImageGen(ctx, settings, systemPrompt, userPrompt, images)
}

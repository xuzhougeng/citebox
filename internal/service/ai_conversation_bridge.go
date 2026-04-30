package service

import (
	"context"

	"github.com/xuzhougeng/citebox/internal/model"
)

// CallProviderStreamGeneric is the public LLM streaming primitive used by
// the ai_conversation package. It bypasses the read-paper specific prepare
// step and accepts arbitrary system/user prompts plus optional images.
//
// Returns the full assembled assistant text, the provider mode label
// (e.g. "responses", "chat"), and an error.
func (s *AIService) CallProviderStreamGeneric(ctx context.Context,
	settings model.AISettings,
	systemPrompt, userPrompt string,
	images []model.AIImageInput,
	onDelta func(string) error,
) (string, string, error) {
	internalImages := make([]aiImageInput, 0, len(images))
	for _, im := range images {
		internalImages = append(internalImages, aiImageInput{MIMEType: im.MIMEType, Data: im.Data})
	}
	prepared := &aiReadPrepared{
		settings:     settings,
		action:       model.AIActionPaperQA,
		systemPrompt: systemPrompt,
		userPrompt:   userPrompt,
		images:       internalImages,
	}
	rawText, err := s.callProviderStream(ctx, prepared, onDelta)
	if err != nil {
		return "", "", err
	}
	return rawText, aiProviderMode(settings), nil
}

// CallProviderGeneric is the non-streaming counterpart, used by the title
// generator and the summarizer.
func (s *AIService) CallProviderGeneric(ctx context.Context,
	settings model.AISettings,
	systemPrompt, userPrompt string,
) (string, string, error) {
	prepared := &aiReadPrepared{
		settings:     settings,
		action:       model.AIActionPaperQA,
		systemPrompt: systemPrompt,
		userPrompt:   userPrompt,
	}
	rawText, mode, err := s.callProvider(ctx, prepared)
	if err != nil {
		return "", "", err
	}
	return rawText, mode, nil
}

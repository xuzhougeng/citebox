package ai_image_gen

import (
	"context"

	"github.com/xuzhougeng/citebox/internal/model"
)

type GenerateInput struct {
	ConversationID int64
	TurnRunID      int64
	PaperIDs       []int64
	FigureIDs      []int64
	UserText       string
	OnEvent        func(StreamEvent) error
}

type StreamEvent struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// PromptDraftedEvent is emitted after the vision stage produces an image prompt.
type PromptDraftedEvent struct {
	TurnRunID int64  `json:"turn_run_id"`
	Prompt    string `json:"prompt"`
}

// GeneratingEvent is emitted just before the image API call.
type GeneratingEvent struct {
	TurnRunID       int64   `json:"turn_run_id"`
	Model           string  `json:"model"`
	Size            string  `json:"size"`
	Quality         string  `json:"quality"`
	CostEstimateUSD float64 `json:"cost_estimate_usd"`
}

// GeneratedEvent is emitted on success with the persisted card payload.
type GeneratedEvent struct {
	TurnRunID int64                  `json:"turn_run_id"`
	Card      map[string]interface{} `json:"card"`
}

// FailedEvent is emitted on failure at any stage.
type FailedEvent struct {
	TurnRunID int64  `json:"turn_run_id"`
	Stage     string `json:"stage"` // "vision" | "image_api" | "save"
	Reason    string `json:"reason"`
}

// VisionCaller wraps the existing service.AIService.CallProviderVision-style
// helper. It returns the model's text output when given a system prompt + user
// prompt + base64 images.
type VisionCaller interface {
	CallVision(ctx context.Context, settings model.AISettings, systemPrompt, userPrompt string,
		images []model.AIImageInput) (string, error)
}

type Generator interface {
	Generate(ctx context.Context, in GenerateInput) error
}

package service

import (
	"encoding/json"
	"github.com/xuzhougeng/citebox/internal/model"
)

type extractionPageCheckpoint struct {
	Version int                      `json:"version"`
	Boxes   []map[string]interface{} `json:"boxes"`
	Figures []extractedFigure        `json:"figures"`
}

func (s *LibraryService) extractionCheckpointHash(aiSvc *AIService, paperID int64, pdfPath, filename string) (string, error) {
	checksum, err := fileSHA256(pdfPath)
	if err != nil {
		return "", err
	}
	settings, err := aiSvc.GetSettings()
	if err != nil {
		return "", err
	}
	config, err := resolveModelForAction(*settings, model.AIActionFigureInterpretation)
	if err != nil {
		return "", err
	}
	paper, err := s.repo.GetPaperDetail(paperID)
	if err != nil {
		return "", err
	}
	title := ""
	if paper != nil {
		title = paper.Title
	}
	// Only non-secret configuration enters the fingerprint. Checkpoints never
	// store credentials or model reasoning, only completed extraction artifacts.
	data, err := json.Marshal(struct {
		PDF, Filename, Title, Provider, Model, BaseURL, Prompt, Reasoning string
		MaxTokens                                                         int
		Legacy, Thinking                                                  bool
	}{checksum, filename, title, string(config.Provider), config.Model, config.BaseURL, aiFigureRegionDetectionSystemPrompt + "checkpoint-v1-dpi180", config.ReasoningEffort, config.MaxOutputTokens, config.OpenAILegacyMode, config.ThinkingEnabled})
	if err != nil {
		return "", err
	}
	return sha256Hex(data), nil
}

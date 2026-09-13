package service

import (
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/service/ai_context"
	"strings"
	"testing"
)

func TestLegacyReadingAndBatchFigurePromptsRespectTextBudget(t *testing.T) {
	for _, action := range []model.AIAction{model.AIActionPaperQA, model.AIActionFigureInterpretation} {
		settings := model.DefaultAISettings()
		settings.ContextBudgetTokens = 4000
		paper := &model.Paper{ID: 1, Title: "Study", PDFText: strings.Repeat("Results and background. ", 10000) + "\nSTAR METHODS\nTAIL_METHOD_MARKER", PaperNotesText: strings.Repeat("Derived notes. ", 10000)}
		system, user := buildAIPrompts(settings, paper, nil, nil, action, "Explain the results", "Explain the results", nil, []string{"Caption: Results"}, 1, nil, false)
		if got := ai_context.EstimateTokens(system) + ai_context.EstimateTokens(user); got > 4000 {
			t.Fatalf("%s exceeded budget: %d", action, got)
		}
		if strings.Contains(user, paper.PDFText) || !strings.Contains(user, "TAIL_METHOD_MARKER") || !strings.Contains(user, "非完整全文") {
			t.Fatalf("%s did not use bounded section context", action)
		}
	}
}

package ai_assistant

import (
	"github.com/xuzhougeng/citebox/internal/model"
	"testing"
)

func TestRelevantFiguresPreferExplicitLaterFigureAndCaption(t *testing.T) {
	figures := []model.Figure{{ID: 1, FigureIndex: 1, Caption: "Overview"}, {ID: 2, FigureIndex: 5, Caption: "Treatment response"}, {ID: 3, FigureIndex: 6, Caption: "Survival analysis"}}
	if got := RelevantFigures(figures, "Explain Fig. 5"); got[0].ID != 2 {
		t.Fatalf("figure number: %+v", got)
	}
	if got := RelevantFigures(figures, "survival"); got[0].ID != 3 {
		t.Fatalf("caption relevance: %+v", got)
	}
	if got := RelevantFigures(figures, ""); got[0].ID != 1 {
		t.Fatal("empty query changed stable order")
	}
}

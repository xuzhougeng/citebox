package ai_conversation

import (
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
	"testing"
)

func TestStrictEvidenceDoesNotReuseAINotesAsPaperFindings(t *testing.T) {
	papers := stubPaperGetter{1: &model.Paper{ID: 1, PaperNotesText: "ALPHAMARKER supports the claim."}}
	citations := localEvidence(papers, "ALPHAMARKER", []repository.AIPinnedPaper{{PaperID: 1}}, 12)
	if len(citations) != 0 {
		t.Fatalf("notes presented as original evidence: %+v", citations)
	}
}

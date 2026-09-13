package ai_context

import (
	"github.com/xuzhougeng/citebox/internal/model"
	"strings"
	"testing"
)

func TestFigureContextFindsTargetEvidenceAndRetainsLateMethods(t *testing.T) {
	p := model.Paper{ID: 1, Title: "Study", PDFText: strings.Repeat("Front matter. ", 2000) + "\nTARGETMARKER explains the treatment effect.\n" + strings.Repeat("Other results. ", 2000) + "\nSTAR METHODS\nLATE_METHODS_MARKER\n" + strings.Repeat("protocol ", 1000)}
	block := FigureBody(p, "TARGETMARKER treatment effect", 2000)
	if EstimateTokens(block) > 2000 || !strings.Contains(block, "TARGETMARKER") || !strings.Contains(block, "LATE_METHODS_MARKER") || !strings.Contains(block, "非完整全文") {
		t.Fatalf("figure context: %s", block)
	}
}

func TestSharedPaperBlockKeepsShortBodyWhole(t *testing.T) {
	p := model.Paper{ID: 1, Title: "Study", PDFText: "Short methods and results."}
	block, usage := PaperBlock(p, 1000)
	if !strings.Contains(block, p.PDFText) || usage.IncludedBodyRunes != len([]rune(p.PDFText)) {
		t.Fatalf("context: %s %+v", block, usage)
	}
}

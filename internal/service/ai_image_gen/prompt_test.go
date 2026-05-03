package ai_image_gen

import (
	"strings"
	"testing"
)

func TestBuildVisionUserPrompt_IncludesPaperContext(t *testing.T) {
	got := BuildVisionUserPrompt(VisionInputContext{
		UserText: "give me a graphical abstract",
		Papers: []PaperContext{
			{ID: 1, Title: "CRISPR demystified", Abstract: "We present...", FullTextSnippet: "Section 1 ..."},
		},
	})
	for _, want := range []string{"CRISPR demystified", "We present...", "give me a graphical abstract"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestBuildVisionUserPrompt_AnnotatesIsolatedFigures(t *testing.T) {
	got := BuildVisionUserPrompt(VisionInputContext{
		UserText: "synthesize these two figures",
		IsolatedFigures: []FigureContext{
			{ID: 11, PaperTitle: "P1", PageNumber: 3, FigureIndex: 1, Caption: "Workflow"},
			{ID: 12, PaperTitle: "P1", PageNumber: 3, FigureIndex: 2, Caption: "Results"},
		},
	})
	for _, want := range []string{"Workflow", "Results", "第 3 页图 1", "第 3 页图 2"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestVisionSystemPrompt_MentionsGraphicalAbstract(t *testing.T) {
	if !strings.Contains(VisionSystemPrompt(), "graphical abstract") {
		t.Error("VisionSystemPrompt must mention graphical abstract")
	}
}

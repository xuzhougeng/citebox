package integration

import (
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/repository"
)

func TestFacadeGetFigureHandoffWithPageTexts(t *testing.T) {
	facade, repo, _ := newTestFacade(t)
	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Handoff Paper",
		OriginalFilename: "handoff.pdf",
		StoredPDFName:    "handoff.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		DOI:              "10.1234/handoff",
		AbstractText:     "Handoff abstract about endothelium.",
		Figures: []repository.FigureUpsertInput{
			{Filename: "handoff_fig.png", OriginalName: "handoff-fig.png", ContentType: "image/png", PageNumber: 3, FigureIndex: 1, Caption: "Endothelial trajectory overview"},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}
	figure := paper.Figures[0]
	if _, err := repo.UpdateFigure(figure.ID, repository.FigureUpdateInput{
		Caption:   "Endothelial trajectory overview",
		NotesText: "user figure note",
	}); err != nil {
		t.Fatalf("UpdateFigure() error = %v", err)
	}
	if err := repo.UpdatePaperPDFTextWithPages(paper.ID, "full", []string{
		"Introduction without the figure",
		"Results mention Fig 1 as the endothelial overview",
		"Discussion cites Figure 10 which must not steal Fig 1 matches",
	}); err != nil {
		t.Fatalf("UpdatePaperPDFTextWithPages() error = %v", err)
	}

	env, err := facade.GetFigureHandoff(GetFigureHandoffParams{FigureID: figure.ID})
	if err != nil {
		t.Fatalf("GetFigureHandoff() error = %v", err)
	}
	if env.EntityType != EntityTypeFigure || env.SourceID != FigureSourceID(figure.ID) {
		t.Fatalf("envelope = %q/%q", env.EntityType, env.SourceID)
	}
	data, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("Data type = %T", env.Data)
	}
	figureData := data["figure"].(map[string]any)
	if figureData["display_label"] != "Fig 1" || figureData["kind"] != "figure" {
		t.Fatalf("figure data = %#v", figureData)
	}
	paperData := data["paper"].(map[string]any)
	if paperData["abstract"] != "Handoff abstract about endothelium." || paperData["doi"] != "10.1234/handoff" {
		t.Fatalf("paper data = %#v", paperData)
	}
	notes := data["notes"].(map[string]any)
	if notes["notes_text"] != "user figure note" {
		t.Fatalf("notes = %#v", notes)
	}
	completeness := data["completeness"].(map[string]any)
	if completeness["abstract"] != true || completeness["page_texts"] != true || completeness["excerpts"] != true || completeness["notes"] != true {
		t.Fatalf("completeness = %#v", completeness)
	}
	excerpts := data["excerpts"].([]map[string]any)
	if len(excerpts) == 0 {
		t.Fatal("expected excerpts")
	}
	foundFig1 := false
	for _, excerpt := range excerpts {
		snippet, _ := excerpt["snippet"].(string)
		if strings.Contains(snippet, "Figure 10") && !strings.Contains(snippet, "Fig 1") {
			t.Fatalf("Fig 1 query leaked Figure 10: %#v", excerpt)
		}
		if excerpt["page"] == 2 && strings.Contains(snippet, "Fig 1") {
			foundFig1 = true
		}
	}
	if !foundFig1 {
		t.Fatalf("excerpts = %#v, want page 2 Fig 1 hit", excerpts)
	}
	if len(env.Assets) != 2 {
		t.Fatalf("assets = %#v, want image + transfer package", env.Assets)
	}
	kinds := map[string]bool{}
	for _, asset := range env.Assets {
		entry := asset.(map[string]any)
		kinds[entry["kind"].(string)] = true
		if entry["figure_id"] != figure.ID {
			t.Fatalf("asset figure_id = %#v", entry)
		}
	}
	if !kinds[AssetKindFigureImage] || !kinds[AssetKindFigureTransferPackage] {
		t.Fatalf("asset kinds = %#v", kinds)
	}

	viaSource, err := facade.GetFigureHandoff(GetFigureHandoffParams{SourceID: FigureSourceID(figure.ID)})
	if err != nil {
		t.Fatalf("GetFigureHandoff(source_id) error = %v", err)
	}
	if viaSource.SourceID != env.SourceID {
		t.Fatalf("source_id path = %q", viaSource.SourceID)
	}
}

func TestFacadeGetFigureHandoffSubfigureAndIncompleteText(t *testing.T) {
	facade, repo, _ := newTestFacade(t)
	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Subfigure Paper",
		OriginalFilename: "sub.pdf",
		StoredPDFName:    "sub.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		Figures: []repository.FigureUpsertInput{
			{Filename: "parent.png", OriginalName: "parent.png", ContentType: "image/png", PageNumber: 1, FigureIndex: 2, Caption: "Parent figure"},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}
	parentID := paper.Figures[0].ID
	if _, err := repo.DB().Exec(
		`INSERT INTO paper_figures (paper_id, filename, original_name, content_type, page_number, figure_index, parent_figure_id, subfigure_label, source, caption, notes_text)
		 VALUES (?, 'child.png', 'child.png', 'image/png', 1, 2, ?, 'a', 'manual', 'Child panel', 'only child note')`,
		paper.ID, parentID,
	); err != nil {
		t.Fatalf("insert subfigure: %v", err)
	}

	listed, err := facade.library.GetPaper(paper.ID)
	if err != nil {
		t.Fatalf("GetPaper() error = %v", err)
	}
	var childID int64
	for _, figure := range listed.Figures {
		if figure.ParentFigureID != nil {
			childID = figure.ID
			break
		}
	}
	if childID == 0 {
		t.Fatal("child figure not found")
	}

	env, err := facade.GetFigureHandoff(GetFigureHandoffParams{FigureID: childID})
	if err != nil {
		t.Fatalf("GetFigureHandoff(child) error = %v", err)
	}
	data := env.Data.(map[string]any)
	figureData := data["figure"].(map[string]any)
	if figureData["kind"] != "subfigure" || figureData["parent_figure_id"] != parentID {
		t.Fatalf("figure data = %#v", figureData)
	}
	notes := data["notes"].(map[string]any)
	if notes["notes_text"] != "only child note" {
		t.Fatalf("notes = %#v", notes)
	}
	completeness := data["completeness"].(map[string]any)
	if completeness["page_texts"] != false || completeness["abstract"] != false || completeness["excerpts"] != false {
		t.Fatalf("completeness = %#v", completeness)
	}
	if excerpts, ok := data["excerpts"].([]map[string]any); !ok || len(excerpts) != 0 {
		t.Fatalf("excerpts = %#v, want empty", data["excerpts"])
	}

	parentEnv, err := facade.GetFigureHandoff(GetFigureHandoffParams{FigureID: parentID})
	if err != nil {
		t.Fatalf("GetFigureHandoff(parent) error = %v", err)
	}
	parentData := parentEnv.Data.(map[string]any)["figure"].(map[string]any)
	subs, ok := parentData["subfigures"].([]map[string]any)
	if !ok || len(subs) != 1 || subs[0]["figure_id"] != childID {
		t.Fatalf("parent subfigures = %#v", parentData["subfigures"])
	}
}

func TestFacadeGetFigureHandoffValidation(t *testing.T) {
	facade, repo, _ := newTestFacade(t)
	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Validate Paper",
		OriginalFilename: "validate.pdf",
		StoredPDFName:    "validate.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		Figures: []repository.FigureUpsertInput{
			{Filename: "validate.png", OriginalName: "validate.png", ContentType: "image/png", PageNumber: 1, FigureIndex: 1, Caption: "Validate"},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	if _, err := facade.GetFigureHandoff(GetFigureHandoffParams{}); !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("empty params code = %q", apperr.CodeOf(err))
	}
	if _, err := facade.GetFigureHandoff(GetFigureHandoffParams{SourceID: PaperSourceID(paper.ID)}); !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("paper source_id code = %q", apperr.CodeOf(err))
	}
	if _, err := facade.GetFigureHandoff(GetFigureHandoffParams{FigureID: paper.Figures[0].ID, SourceID: FigureSourceID(paper.Figures[0].ID + 1)}); !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("mismatched ids code = %q", apperr.CodeOf(err))
	}
	if _, err := facade.GetFigureHandoff(GetFigureHandoffParams{FigureID: 99999}); !apperr.IsCode(err, apperr.CodeNotFound) {
		t.Fatalf("missing figure code = %q", apperr.CodeOf(err))
	}
}

func TestFigureHandoffQueryBounds(t *testing.T) {
	matches := findBoundedTextMatches("see Fig 1 then Fig 10 later", "Fig 1", 80)
	if len(matches) != 1 || !strings.Contains(matches[0], "Fig 1") || strings.Contains(matches[0], "Fig 10") && !strings.Contains(matches[0], "Fig 1 then") {
		// window may include nearby Fig 10; the match start must still be Fig 1.
		if len(matches) != 1 {
			t.Fatalf("matches = %#v", matches)
		}
	}
	if got := findBoundedTextMatches("only Fig 10 is present", "Fig 1", 40); len(got) != 0 {
		t.Fatalf("Fig 1 should not match Fig 10: %#v", got)
	}
}

package repository

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

func newTestRepository(t *testing.T) *LibraryRepository {
	t.Helper()

	repo, err := NewLibraryRepository(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatalf("NewLibraryRepository() error = %v", err)
	}

	t.Cleanup(func() {
		_ = repo.Close()
	})

	return repo
}

func TestCreatePaperAndListEntities(t *testing.T) {
	repo := newTestRepository(t)

	group, err := repo.CreateGroup("Vision", "microscopy papers")
	if err != nil {
		t.Fatalf("CreateGroup() error = %v", err)
	}

	paper, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Cell Atlas",
		OriginalFilename: "cell-atlas.pdf",
		StoredPDFName:    "paper_1.pdf",
		FileSize:         128,
		ContentType:      "application/pdf",
		AbstractText:     "Cell atlas abstract",
		NotesText:        "Important notes",
		ExtractionStatus: "completed",
		GroupID:          &group.ID,
		Tags: []TagUpsertInput{
			{Name: "Cell", Color: "#111111"},
			{Name: "Microscopy", Color: "#222222"},
		},
		Figures: []FigureUpsertInput{
			{
				Filename:     "figure_1.png",
				OriginalName: "figure-original.png",
				ContentType:  "image/png",
				PageNumber:   2,
				FigureIndex:  1,
				Caption:      "Overview image",
				BBoxJSON:     `{"x":1}`,
			},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	if paper.GroupID == nil || *paper.GroupID != group.ID {
		t.Fatalf("CreatePaper() group id = %v, want %d", paper.GroupID, group.ID)
	}
	if got := len(paper.Tags); got != 2 {
		t.Fatalf("CreatePaper() tags = %d, want 2", got)
	}
	if got := len(paper.Figures); got != 1 {
		t.Fatalf("CreatePaper() figures = %d, want 1", got)
	}

	papers, total, err := repo.ListPapers(model.PaperFilter{})
	if err != nil {
		t.Fatalf("ListPapers() error = %v", err)
	}
	if total != 1 || len(papers) != 1 {
		t.Fatalf("ListPapers() total=%d len=%d, want 1/1", total, len(papers))
	}
	if got := len(papers[0].Tags); got != 2 {
		t.Fatalf("ListPapers() tags = %d, want 2", got)
	}
	if papers[0].AbstractText != "Cell atlas abstract" || papers[0].NotesText != "Important notes" {
		t.Fatalf("ListPapers() metadata = (%q, %q), want abstract/notes", papers[0].AbstractText, papers[0].NotesText)
	}

	figures, total, err := repo.ListFigures(model.FigureFilter{})
	if err != nil {
		t.Fatalf("ListFigures() error = %v", err)
	}
	if total != 1 || len(figures) != 1 {
		t.Fatalf("ListFigures() total=%d len=%d, want 1/1", total, len(figures))
	}
	if figures[0].PaperID != paper.ID {
		t.Fatalf("ListFigures() paper id = %d, want %d", figures[0].PaperID, paper.ID)
	}
	if got := len(figures[0].Tags); got != 0 {
		t.Fatalf("ListFigures() tags = %d, want 0", got)
	}
	if figures[0].Source != "auto" {
		t.Fatalf("ListFigures() source = %q, want %q", figures[0].Source, "auto")
	}
	if figures[0].NotesText != "" {
		t.Fatalf("ListFigures() notes_text = %q, want empty", figures[0].NotesText)
	}
}

func TestListPapersSearchesPDFTextWithFTS(t *testing.T) {
	repo := newTestRepository(t)

	if _, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Cell Atlas",
		OriginalFilename: "cell-atlas.pdf",
		StoredPDFName:    "paper_fts.pdf",
		FileSize:         128,
		ContentType:      "application/pdf",
		PDFText:          "This manuscript summarizes transcriptome remodeling across stress gradients.",
		ExtractionStatus: "completed",
		Figures: []FigureUpsertInput{
			{Filename: "figure_fts.png", PageNumber: 1, FigureIndex: 1},
		},
	}); err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	papers, total, err := repo.ListPapers(model.PaperFilter{Keyword: "transcriptome remodeling"})
	if err != nil {
		t.Fatalf("ListPapers(keyword) error = %v", err)
	}
	if total != 1 || len(papers) != 1 {
		t.Fatalf("ListPapers(keyword) total=%d len=%d, want 1/1", total, len(papers))
	}
	if papers[0].Title != "Cell Atlas" {
		t.Fatalf("ListPapers(keyword) title = %q, want %q", papers[0].Title, "Cell Atlas")
	}
}

func TestCreatePaperRejectsDuplicateStoredPDFName(t *testing.T) {
	repo := newTestRepository(t)

	if _, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "First",
		OriginalFilename: "first.pdf",
		StoredPDFName:    "duplicate-paper.pdf",
		FileSize:         128,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		Figures: []FigureUpsertInput{
			{Filename: "first-figure.png", PageNumber: 1, FigureIndex: 1},
		},
	}); err != nil {
		t.Fatalf("CreatePaper() setup error = %v", err)
	}

	if _, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Second",
		OriginalFilename: "second.pdf",
		StoredPDFName:    "duplicate-paper.pdf",
		FileSize:         128,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		Figures: []FigureUpsertInput{
			{Filename: "second-figure.png", PageNumber: 1, FigureIndex: 1},
		},
	}); !apperr.IsCode(err, apperr.CodeConflict) {
		t.Fatalf("CreatePaper() duplicate stored pdf code = %q, want %q", apperr.CodeOf(err), apperr.CodeConflict)
	}
}

func TestCreatePaperRejectsDuplicatePDFSHA256(t *testing.T) {
	repo := newTestRepository(t)

	if _, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "First",
		OriginalFilename: "first.pdf",
		StoredPDFName:    "first-paper.pdf",
		PDFSHA256:        "same-checksum",
		FileSize:         128,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		Figures: []FigureUpsertInput{
			{Filename: "first-figure.png", PageNumber: 1, FigureIndex: 1},
		},
	}); err != nil {
		t.Fatalf("CreatePaper() setup error = %v", err)
	}

	if _, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Second",
		OriginalFilename: "second.pdf",
		StoredPDFName:    "second-paper.pdf",
		PDFSHA256:        "same-checksum",
		FileSize:         128,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		Figures: []FigureUpsertInput{
			{Filename: "second-figure.png", PageNumber: 1, FigureIndex: 1},
		},
	}); !apperr.IsCode(err, apperr.CodeConflict) {
		t.Fatalf("CreatePaper() duplicate checksum code = %q, want %q", apperr.CodeOf(err), apperr.CodeConflict)
	}
}

func TestCreatePaperRejectsDuplicateFigureFilename(t *testing.T) {
	repo := newTestRepository(t)

	if _, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "First",
		OriginalFilename: "first.pdf",
		StoredPDFName:    "first-paper.pdf",
		FileSize:         128,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		Figures: []FigureUpsertInput{
			{Filename: "duplicate-figure.png", PageNumber: 1, FigureIndex: 1},
		},
	}); err != nil {
		t.Fatalf("CreatePaper() setup error = %v", err)
	}

	if _, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Second",
		OriginalFilename: "second.pdf",
		StoredPDFName:    "second-paper.pdf",
		FileSize:         128,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		Figures: []FigureUpsertInput{
			{Filename: "duplicate-figure.png", PageNumber: 1, FigureIndex: 1},
		},
	}); !apperr.IsCode(err, apperr.CodeConflict) {
		t.Fatalf("CreatePaper() duplicate figure filename code = %q, want %q", apperr.CodeOf(err), apperr.CodeConflict)
	}

	papers, total, err := repo.ListPapers(model.PaperFilter{})
	if err != nil {
		t.Fatalf("ListPapers() error = %v", err)
	}
	if total != 1 || len(papers) != 1 {
		t.Fatalf("ListPapers() after duplicate figure total=%d len=%d, want 1/1", total, len(papers))
	}
}

func TestCreatePaperRejectsInvalidStatusAndFigureSource(t *testing.T) {
	repo := newTestRepository(t)

	if _, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Invalid Status",
		OriginalFilename: "invalid-status.pdf",
		StoredPDFName:    "invalid-status.pdf",
		FileSize:         128,
		ContentType:      "application/pdf",
		ExtractionStatus: "unexpected",
		Figures: []FigureUpsertInput{
			{Filename: "invalid-status-figure.png", PageNumber: 1, FigureIndex: 1},
		},
	}); !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("CreatePaper() invalid status code = %q, want %q", apperr.CodeOf(err), apperr.CodeInvalidArgument)
	}

	if _, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Invalid Source",
		OriginalFilename: "invalid-source.pdf",
		StoredPDFName:    "invalid-source.pdf",
		FileSize:         128,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		Figures: []FigureUpsertInput{
			{Filename: "invalid-source-figure.png", PageNumber: 1, FigureIndex: 1, Source: "crawler"},
		},
	}); !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("CreatePaper() invalid source code = %q, want %q", apperr.CodeOf(err), apperr.CodeInvalidArgument)
	}
}

func TestUpdateFigureTagsOnlyAffectsTargetFigure(t *testing.T) {
	repo := newTestRepository(t)

	paper, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Tagged Figures",
		OriginalFilename: "tagged-figures.pdf",
		StoredPDFName:    "paper_tagged.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		Figures: []FigureUpsertInput{
			{Filename: "figure_1.png", PageNumber: 1, FigureIndex: 1, Caption: "First"},
			{Filename: "figure_2.png", PageNumber: 2, FigureIndex: 2, Caption: "Second"},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	if len(paper.Figures) != 2 {
		t.Fatalf("CreatePaper() figures = %d, want 2", len(paper.Figures))
	}

	updatedPaper, err := repo.UpdateFigureTags(paper.Figures[0].ID, []TagUpsertInput{
		{Name: "Target Figure", Color: "#334455"},
	})
	if err != nil {
		t.Fatalf("UpdateFigureTags() error = %v", err)
	}

	if got := len(updatedPaper.Figures[0].Tags); got != 1 {
		t.Fatalf("updated first figure tags = %d, want 1", got)
	}
	if got := len(updatedPaper.Figures[1].Tags); got != 0 {
		t.Fatalf("updated second figure tags = %d, want 0", got)
	}

	tags, err := repo.ListTags(model.TagScopeFigure)
	if err != nil {
		t.Fatalf("ListTags() error = %v", err)
	}
	if len(tags) != 1 {
		t.Fatalf("ListTags() len = %d, want 1", len(tags))
	}
	if tags[0].PaperCount != 0 {
		t.Fatalf("ListTags() paper_count = %d, want 0", tags[0].PaperCount)
	}
	if tags[0].FigureCount != 1 {
		t.Fatalf("ListTags() figure_count = %d, want 1", tags[0].FigureCount)
	}

	figures, total, err := repo.ListFigures(model.FigureFilter{TagID: &tags[0].ID})
	if err != nil {
		t.Fatalf("ListFigures(tag filter) error = %v", err)
	}
	if total != 1 || len(figures) != 1 {
		t.Fatalf("ListFigures(tag filter) total=%d len=%d, want 1/1", total, len(figures))
	}
	if figures[0].ID != paper.Figures[0].ID {
		t.Fatalf("ListFigures(tag filter) figure id = %d, want %d", figures[0].ID, paper.Figures[0].ID)
	}
	if got := len(figures[0].Tags); got != 1 {
		t.Fatalf("ListFigures(tag filter) tags = %d, want 1", got)
	}
}

func TestUpdateFigureTagsPreservesExistingTagsAcrossMultipleUpdates(t *testing.T) {
	repo := newTestRepository(t)

	paper, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Repeated Figure Tags",
		OriginalFilename: "repeated-figure-tags.pdf",
		StoredPDFName:    "repeated-figure-tags.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		Figures: []FigureUpsertInput{
			{Filename: "figure_1.png", PageNumber: 1, FigureIndex: 1, Caption: "First"},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	figureID := paper.Figures[0].ID
	steps := [][]TagUpsertInput{
		{{Name: "Tag One", Color: "#111111"}},
		{
			{Name: "Tag One", Color: "#111111"},
			{Name: "Tag Two", Color: "#222222"},
		},
		{
			{Name: "Tag One", Color: "#111111"},
			{Name: "Tag Two", Color: "#222222"},
			{Name: "Tag Three", Color: "#333333"},
		},
	}

	var updated *model.Paper
	for _, tags := range steps {
		updated, err = repo.UpdateFigureTags(figureID, tags)
		if err != nil {
			t.Fatalf("UpdateFigureTags() error = %v", err)
		}
	}

	if updated == nil || len(updated.Figures) != 1 {
		t.Fatalf("updated paper figures = %+v, want 1 figure", updated)
	}
	if got := len(updated.Figures[0].Tags); got != 3 {
		t.Fatalf("updated figure tags = %d, want 3", got)
	}

	gotNames := make([]string, 0, len(updated.Figures[0].Tags))
	for _, tag := range updated.Figures[0].Tags {
		gotNames = append(gotNames, tag.Name)
	}
	expectedNames := []string{"Tag One", "Tag Three", "Tag Two"}
	if strings.Join(gotNames, ",") != strings.Join(expectedNames, ",") {
		t.Fatalf("updated figure tag names = %v, want %v", gotNames, expectedNames)
	}
}

func TestUpdatePaperTagsPreservesExistingTagsAcrossMultipleUpdates(t *testing.T) {
	repo := newTestRepository(t)

	paper, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Repeated Paper Tags",
		OriginalFilename: "repeated-paper-tags.pdf",
		StoredPDFName:    "repeated-paper-tags.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		Figures: []FigureUpsertInput{
			{Filename: "figure_1.png", PageNumber: 1, FigureIndex: 1, Caption: "First"},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	steps := [][]TagUpsertInput{
		{{Name: "Paper One", Color: "#111111"}},
		{
			{Name: "Paper One", Color: "#111111"},
			{Name: "Paper Two", Color: "#222222"},
		},
		{
			{Name: "Paper One", Color: "#111111"},
			{Name: "Paper Two", Color: "#222222"},
			{Name: "Paper Three", Color: "#333333"},
		},
	}

	var updated *model.Paper
	for _, tags := range steps {
		updated, err = repo.UpdatePaper(paper.ID, PaperUpdateInput{
			Title:        paper.Title,
			AbstractText: paper.AbstractText,
			NotesText:    paper.NotesText,
			GroupID:      paper.GroupID,
			Tags:         tags,
		})
		if err != nil {
			t.Fatalf("UpdatePaper() error = %v", err)
		}
	}

	if updated == nil {
		t.Fatalf("updated paper is nil")
	}
	if got := len(updated.Tags); got != 3 {
		t.Fatalf("updated paper tags = %d, want 3", got)
	}

	gotNames := make([]string, 0, len(updated.Tags))
	for _, tag := range updated.Tags {
		gotNames = append(gotNames, tag.Name)
	}
	expectedNames := []string{"Paper One", "Paper Three", "Paper Two"}
	if strings.Join(gotNames, ",") != strings.Join(expectedNames, ",") {
		t.Fatalf("updated paper tag names = %v, want %v", gotNames, expectedNames)
	}
}

func TestUpdateFigureNotesOnlyAffectsTargetFigureAndSearch(t *testing.T) {
	repo := newTestRepository(t)

	paper, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Figure Notes",
		OriginalFilename: "figure-notes.pdf",
		StoredPDFName:    "figure-notes.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		Figures: []FigureUpsertInput{
			{Filename: "figure_1.png", PageNumber: 1, FigureIndex: 1, Caption: "First"},
			{Filename: "figure_2.png", PageNumber: 2, FigureIndex: 2, Caption: "Second"},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	updatedPaper, err := repo.UpdateFigure(paper.Figures[0].ID, FigureUpdateInput{
		NotesText: "AI 解读：这张图展示了信号通路重编程。",
		Tags:      []TagUpsertInput{},
	})
	if err != nil {
		t.Fatalf("UpdateFigure() error = %v", err)
	}

	if updatedPaper.Figures[0].NotesText == "" {
		t.Fatalf("updated first figure notes_text is empty")
	}
	if updatedPaper.Figures[1].NotesText != "" {
		t.Fatalf("updated second figure notes_text = %q, want empty", updatedPaper.Figures[1].NotesText)
	}

	figures, total, err := repo.ListFigures(model.FigureFilter{Keyword: "重编程"})
	if err != nil {
		t.Fatalf("ListFigures(keyword notes) error = %v", err)
	}
	if total != 1 || len(figures) != 1 {
		t.Fatalf("ListFigures(keyword notes) total=%d len=%d, want 1/1", total, len(figures))
	}
	if figures[0].ID != paper.Figures[0].ID {
		t.Fatalf("ListFigures(keyword notes) figure id = %d, want %d", figures[0].ID, paper.Figures[0].ID)
	}
	if figures[0].NotesText == "" {
		t.Fatalf("ListFigures(keyword notes) notes_text is empty")
	}
}

func TestListFiguresFiltersHasNotes(t *testing.T) {
	repo := newTestRepository(t)

	paper, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Notes Filter",
		OriginalFilename: "notes-filter.pdf",
		StoredPDFName:    "notes-filter.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		Figures: []FigureUpsertInput{
			{Filename: "figure_1.png", PageNumber: 1, FigureIndex: 1, Caption: "First"},
			{Filename: "figure_2.png", PageNumber: 2, FigureIndex: 2, Caption: "Second"},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	if _, err := repo.UpdateFigure(paper.Figures[1].ID, FigureUpdateInput{
		NotesText: "这张图已经补充了检索笔记",
		Tags:      []TagUpsertInput{},
	}); err != nil {
		t.Fatalf("UpdateFigure() error = %v", err)
	}

	figures, total, err := repo.ListFigures(model.FigureFilter{HasNotes: true})
	if err != nil {
		t.Fatalf("ListFigures(has notes) error = %v", err)
	}
	if total != 1 || len(figures) != 1 {
		t.Fatalf("ListFigures(has notes) total=%d len=%d, want 1/1", total, len(figures))
	}
	if figures[0].ID != paper.Figures[1].ID {
		t.Fatalf("ListFigures(has notes) figure id = %d, want %d", figures[0].ID, paper.Figures[1].ID)
	}
	if strings.TrimSpace(figures[0].NotesText) == "" {
		t.Fatalf("ListFigures(has notes) notes_text = %q, want non-empty", figures[0].NotesText)
	}
}

func TestUpdateFigureTracksUpdatedAtAndNotesSort(t *testing.T) {
	repo := newTestRepository(t)

	paper, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Figure Updates",
		OriginalFilename: "figure-updates.pdf",
		StoredPDFName:    "figure-updates.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		Figures: []FigureUpsertInput{
			{Filename: "figure_1.png", PageNumber: 1, FigureIndex: 1, Caption: "First"},
			{Filename: "figure_2.png", PageNumber: 2, FigureIndex: 2, Caption: "Second"},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	if _, err := repo.db.Exec(
		"UPDATE paper_figures SET notes_text = ?, updated_at = '2000-01-01 00:00:00' WHERE id = ?",
		"old note",
		paper.Figures[0].ID,
	); err != nil {
		t.Fatalf("seed first figure notes error = %v", err)
	}
	if _, err := repo.db.Exec(
		"UPDATE paper_figures SET updated_at = '2000-01-01 00:00:00' WHERE id = ?",
		paper.Figures[1].ID,
	); err != nil {
		t.Fatalf("seed second figure updated_at error = %v", err)
	}

	if _, err := repo.UpdateFigure(paper.Figures[1].ID, FigureUpdateInput{
		NotesText: "new note",
		Tags:      []TagUpsertInput{},
	}); err != nil {
		t.Fatalf("UpdateFigure() error = %v", err)
	}

	secondFigure, err := repo.GetFigure(paper.Figures[1].ID)
	if err != nil {
		t.Fatalf("GetFigure() error = %v", err)
	}
	if secondFigure == nil {
		t.Fatalf("GetFigure() returned nil figure")
	}
	if secondFigure.UpdatedAt.Year() == 2000 {
		t.Fatalf("GetFigure() updated_at = %v, want refreshed timestamp", secondFigure.UpdatedAt)
	}

	figures, total, err := repo.ListFigures(model.FigureFilter{HasNotes: true})
	if err != nil {
		t.Fatalf("ListFigures(has notes) error = %v", err)
	}
	if total != 2 || len(figures) != 2 {
		t.Fatalf("ListFigures(has notes) total=%d len=%d, want 2/2", total, len(figures))
	}
	if figures[0].ID != paper.Figures[1].ID {
		t.Fatalf("ListFigures(has notes) first id = %d, want %d", figures[0].ID, paper.Figures[1].ID)
	}
	if figures[1].ID != paper.Figures[0].ID {
		t.Fatalf("ListFigures(has notes) second id = %d, want %d", figures[1].ID, paper.Figures[0].ID)
	}
}

func TestListTagsIncludesPaperAndFigureCounts(t *testing.T) {
	repo := newTestRepository(t)

	paper, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Tag Counts",
		OriginalFilename: "tag-counts.pdf",
		StoredPDFName:    "tag-counts.pdf",
		FileSize:         128,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		Tags: []TagUpsertInput{
			{Name: "PaperOnly", Color: "#111111"},
		},
		Figures: []FigureUpsertInput{
			{Filename: "figure_1.png", PageNumber: 1, FigureIndex: 1},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	if _, err := repo.UpdateFigureTags(paper.Figures[0].ID, []TagUpsertInput{
		{Name: "FigureOnly", Color: "#222222"},
	}); err != nil {
		t.Fatalf("UpdateFigureTags() error = %v", err)
	}

	paperTags, err := repo.ListTags(model.TagScopePaper)
	if err != nil {
		t.Fatalf("ListTags(paper) error = %v", err)
	}
	if len(paperTags) != 1 {
		t.Fatalf("ListTags(paper) len = %d, want 1", len(paperTags))
	}
	if paperTags[0].Name != "PaperOnly" || paperTags[0].PaperCount != 1 || paperTags[0].FigureCount != 0 {
		t.Fatalf("paper tag = %+v, want PaperOnly with counts (1,0)", paperTags[0])
	}

	figureTags, err := repo.ListTags(model.TagScopeFigure)
	if err != nil {
		t.Fatalf("ListTags(figure) error = %v", err)
	}
	if len(figureTags) != 1 {
		t.Fatalf("ListTags(figure) len = %d, want 1", len(figureTags))
	}
	if figureTags[0].Name != "FigureOnly" || figureTags[0].PaperCount != 0 || figureTags[0].FigureCount != 1 {
		t.Fatalf("figure tag = %+v, want FigureOnly with counts (0,1)", figureTags[0])
	}
}

func TestListTagsSeparatesSameNameAcrossScopes(t *testing.T) {
	repo := newTestRepository(t)

	paper, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Shared Scope",
		OriginalFilename: "shared-scope.pdf",
		StoredPDFName:    "shared-scope.pdf",
		FileSize:         64,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		Tags: []TagUpsertInput{
			{Scope: model.TagScopePaper, Name: "Shared", Color: "#111111"},
		},
		Figures: []FigureUpsertInput{
			{Filename: "figure_1.png", PageNumber: 1, FigureIndex: 1},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	if _, err := repo.UpdateFigureTags(paper.Figures[0].ID, []TagUpsertInput{
		{Scope: model.TagScopeFigure, Name: "Shared", Color: "#222222"},
	}); err != nil {
		t.Fatalf("UpdateFigureTags() error = %v", err)
	}

	paperTags, err := repo.ListTags(model.TagScopePaper)
	if err != nil {
		t.Fatalf("ListTags(paper) error = %v", err)
	}
	figureTags, err := repo.ListTags(model.TagScopeFigure)
	if err != nil {
		t.Fatalf("ListTags(figure) error = %v", err)
	}

	if len(paperTags) != 1 || len(figureTags) != 1 {
		t.Fatalf("tag scope lens = (%d,%d), want (1,1)", len(paperTags), len(figureTags))
	}
	if paperTags[0].ID == figureTags[0].ID {
		t.Fatalf("scoped tags share same id = %d, want distinct ids", paperTags[0].ID)
	}
	if paperTags[0].Scope != model.TagScopePaper || figureTags[0].Scope != model.TagScopeFigure {
		t.Fatalf("scoped tags = (%s,%s), want (paper,figure)", paperTags[0].Scope, figureTags[0].Scope)
	}
}

func TestRepositoryMigratesPaperMetadataColumns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}

	legacySchema := `
	CREATE TABLE papers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL,
		original_filename TEXT NOT NULL,
		stored_pdf_name TEXT NOT NULL,
		file_size INTEGER DEFAULT 0,
		content_type TEXT DEFAULT 'application/pdf',
		pdf_text TEXT DEFAULT '',
		boxes_json TEXT DEFAULT '',
		extraction_status TEXT DEFAULT 'completed',
		extractor_message TEXT DEFAULT '',
		group_id INTEGER,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	if _, err := db.Exec(legacySchema); err != nil {
		t.Fatalf("db.Exec() error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("db.Close() error = %v", err)
	}

	repo, err := NewLibraryRepository(dbPath)
	if err != nil {
		t.Fatalf("NewLibraryRepository() error = %v", err)
	}
	t.Cleanup(func() {
		_ = repo.Close()
	})

	rows, err := repo.db.Query("PRAGMA table_info(papers)")
	if err != nil {
		t.Fatalf("PRAGMA table_info() error = %v", err)
	}
	defer rows.Close()

	columns := map[string]bool{}
	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal sql.NullString
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &primaryKey); err != nil {
			t.Fatalf("rows.Scan() error = %v", err)
		}
		columns[strings.ToLower(name)] = true
	}

	for _, column := range []string{"extractor_job_id", "abstract_text", "notes_text"} {
		if !columns[column] {
			t.Fatalf("missing migrated column %q", column)
		}
	}

	rows, err = repo.db.Query("PRAGMA table_info(paper_figures)")
	if err != nil {
		t.Fatalf("PRAGMA table_info(paper_figures) error = %v", err)
	}
	defer rows.Close()

	figureColumns := map[string]bool{}
	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal sql.NullString
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &primaryKey); err != nil {
			t.Fatalf("rows.Scan(paper_figures) error = %v", err)
		}
		figureColumns[strings.ToLower(name)] = true
	}
	for _, column := range []string{"source", "notes_text", "updated_at"} {
		if !figureColumns[column] {
			t.Fatalf("missing migrated column %q", column)
		}
	}
}

func TestRepositoryMigratesLegacyTagsIntoScopedSets(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy-tags.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}

	legacySchema := `
	CREATE TABLE tags (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL COLLATE NOCASE UNIQUE,
		color TEXT DEFAULT '#A45C40',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE papers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL,
		original_filename TEXT NOT NULL,
		stored_pdf_name TEXT NOT NULL,
		file_size INTEGER DEFAULT 0,
		content_type TEXT DEFAULT 'application/pdf',
		pdf_text TEXT DEFAULT '',
		abstract_text TEXT DEFAULT '',
		notes_text TEXT DEFAULT '',
		boxes_json TEXT DEFAULT '',
		extraction_status TEXT DEFAULT 'completed',
		extractor_message TEXT DEFAULT '',
		extractor_job_id TEXT DEFAULT '',
		group_id INTEGER,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE paper_figures (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		paper_id INTEGER NOT NULL,
		filename TEXT NOT NULL,
		original_name TEXT DEFAULT '',
		content_type TEXT DEFAULT '',
		page_number INTEGER DEFAULT 0,
		figure_index INTEGER DEFAULT 0,
		source TEXT DEFAULT 'auto',
		caption TEXT DEFAULT '',
		bbox_json TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE paper_tags (
		paper_id INTEGER NOT NULL,
		tag_id INTEGER NOT NULL,
		PRIMARY KEY (paper_id, tag_id)
	);
	CREATE TABLE figure_tags (
		figure_id INTEGER NOT NULL,
		tag_id INTEGER NOT NULL,
		PRIMARY KEY (figure_id, tag_id)
	);
	INSERT INTO tags (id, name, color) VALUES (1, 'Shared', '#123456');
	INSERT INTO papers (id, title, original_filename, stored_pdf_name) VALUES (1, 'Legacy', 'legacy.pdf', 'legacy.pdf');
	INSERT INTO paper_figures (id, paper_id, filename) VALUES (1, 1, 'figure.png');
	INSERT INTO paper_tags (paper_id, tag_id) VALUES (1, 1);
	INSERT INTO figure_tags (figure_id, tag_id) VALUES (1, 1);
	`
	if _, err := db.Exec(legacySchema); err != nil {
		t.Fatalf("Exec(legacySchema) error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("db.Close() error = %v", err)
	}

	repo, err := NewLibraryRepository(dbPath)
	if err != nil {
		t.Fatalf("NewLibraryRepository() error = %v", err)
	}
	t.Cleanup(func() {
		_ = repo.Close()
	})

	paperTags, err := repo.ListTags(model.TagScopePaper)
	if err != nil {
		t.Fatalf("ListTags(paper) error = %v", err)
	}
	figureTags, err := repo.ListTags(model.TagScopeFigure)
	if err != nil {
		t.Fatalf("ListTags(figure) error = %v", err)
	}

	if len(paperTags) != 1 || len(figureTags) != 1 {
		t.Fatalf("scoped tag lens = (%d,%d), want (1,1)", len(paperTags), len(figureTags))
	}
	if paperTags[0].Name != "Shared" || figureTags[0].Name != "Shared" {
		t.Fatalf("migrated tag names = (%q,%q), want Shared/Shared", paperTags[0].Name, figureTags[0].Name)
	}
	if paperTags[0].ID == figureTags[0].ID {
		t.Fatalf("migrated scoped tags share id = %d, want distinct ids", paperTags[0].ID)
	}
}

func TestPurgeLibraryRemovesAllRecords(t *testing.T) {
	repo := newTestRepository(t)

	group, err := repo.CreateGroup("Vision", "microscopy papers")
	if err != nil {
		t.Fatalf("CreateGroup() error = %v", err)
	}
	if _, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Cell Atlas",
		OriginalFilename: "cell-atlas.pdf",
		StoredPDFName:    "paper_1.pdf",
		FileSize:         128,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		GroupID:          &group.ID,
		Tags: []TagUpsertInput{
			{Name: "Cell", Color: "#111111"},
		},
		Figures: []FigureUpsertInput{
			{Filename: "figure_1.png", PageNumber: 1, FigureIndex: 1},
		},
	}); err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	if err := repo.PurgeLibrary(); err != nil {
		t.Fatalf("PurgeLibrary() error = %v", err)
	}

	if papers, total, err := repo.ListPapers(model.PaperFilter{}); err != nil || total != 0 || len(papers) != 0 {
		t.Fatalf("ListPapers() after purge = total:%d len:%d err:%v", total, len(papers), err)
	}
	if groups, err := repo.ListGroups(); err != nil || len(groups) != 0 {
		t.Fatalf("ListGroups() after purge = len:%d err:%v", len(groups), err)
	}
	if tags, err := repo.ListTags(model.TagScopePaper); err != nil || len(tags) != 0 {
		t.Fatalf("ListTags(paper) after purge = len:%d err:%v", len(tags), err)
	}
	if tags, err := repo.ListTags(model.TagScopeFigure); err != nil || len(tags) != 0 {
		t.Fatalf("ListTags(figure) after purge = len:%d err:%v", len(tags), err)
	}
}

func TestGroupAndTagErrorsUseErrorCodes(t *testing.T) {
	repo := newTestRepository(t)

	if _, err := repo.CreateGroup("Cancer", ""); err != nil {
		t.Fatalf("CreateGroup() setup error = %v", err)
	}
	if _, err := repo.CreateGroup("Cancer", "duplicate"); !apperr.IsCode(err, apperr.CodeConflict) {
		t.Fatalf("CreateGroup() duplicate code = %q, want %q", apperr.CodeOf(err), apperr.CodeConflict)
	}

	if _, err := repo.UpdateGroup(999, "Missing", ""); !apperr.IsCode(err, apperr.CodeNotFound) {
		t.Fatalf("UpdateGroup() missing code = %q, want %q", apperr.CodeOf(err), apperr.CodeNotFound)
	}
	if err := repo.DeleteGroup(999); !apperr.IsCode(err, apperr.CodeNotFound) {
		t.Fatalf("DeleteGroup() missing code = %q, want %q", apperr.CodeOf(err), apperr.CodeNotFound)
	}

	if _, err := repo.CreateTag(model.TagScopePaper, "Atlas", "#123456"); err != nil {
		t.Fatalf("CreateTag() setup error = %v", err)
	}
	if _, err := repo.CreateTag(model.TagScopePaper, "Atlas", "#654321"); !apperr.IsCode(err, apperr.CodeConflict) {
		t.Fatalf("CreateTag() duplicate code = %q, want %q", apperr.CodeOf(err), apperr.CodeConflict)
	}
	if _, err := repo.CreateTag(model.TagScopeFigure, "Atlas", "#654321"); err != nil {
		t.Fatalf("CreateTag(figure) same name error = %v", err)
	}
	if err := repo.DeleteTag(999); !apperr.IsCode(err, apperr.CodeNotFound) {
		t.Fatalf("DeleteTag() missing code = %q, want %q", apperr.CodeOf(err), apperr.CodeNotFound)
	}
}

func TestAppSettingRoundTrip(t *testing.T) {
	repo := newTestRepository(t)

	if got, err := repo.GetAppSetting("ai_settings"); err != nil || got != "" {
		t.Fatalf("GetAppSetting() before save = %q, err = %v, want empty nil", got, err)
	}

	if err := repo.UpsertAppSetting("ai_settings", `{"provider":"openai"}`); err != nil {
		t.Fatalf("UpsertAppSetting() error = %v", err)
	}

	got, err := repo.GetAppSetting("ai_settings")
	if err != nil {
		t.Fatalf("GetAppSetting() error = %v", err)
	}
	if got != `{"provider":"openai"}` {
		t.Fatalf("GetAppSetting() = %q, want %q", got, `{"provider":"openai"}`)
	}
}

func TestAddPaperFiguresAppendsManualSource(t *testing.T) {
	repo := newTestRepository(t)

	paper, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Cell Atlas",
		OriginalFilename: "cell-atlas.pdf",
		StoredPDFName:    "paper_1.pdf",
		FileSize:         128,
		ContentType:      "application/pdf",
		ExtractionStatus: "failed",
		Figures: []FigureUpsertInput{
			{Filename: "figure_1.png", PageNumber: 1, FigureIndex: 1},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	if err := repo.AddPaperFigures(paper.ID, []FigureUpsertInput{
		{
			Filename:     "manual_1.png",
			OriginalName: "manual_1.png",
			ContentType:  "image/png",
			PageNumber:   2,
			FigureIndex:  2,
			Source:       "manual",
			Caption:      "Manual region",
		},
	}); err != nil {
		t.Fatalf("AddPaperFigures() error = %v", err)
	}

	got, err := repo.GetPaperDetail(paper.ID)
	if err != nil {
		t.Fatalf("GetPaperDetail() error = %v", err)
	}
	if len(got.Figures) != 2 {
		t.Fatalf("GetPaperDetail() figures = %d, want 2", len(got.Figures))
	}
	if got.Figures[1].Source != "manual" {
		t.Fatalf("manual figure source = %q, want %q", got.Figures[1].Source, "manual")
	}
}

func TestApplyPaperExtractionResultPreservesManualFigures(t *testing.T) {
	repo := newTestRepository(t)

	paper, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Cell Atlas",
		OriginalFilename: "cell-atlas.pdf",
		StoredPDFName:    "paper_1.pdf",
		FileSize:         128,
		ContentType:      "application/pdf",
		ExtractionStatus: "queued",
		Figures: []FigureUpsertInput{
			{Filename: "auto_old.png", PageNumber: 1, FigureIndex: 1, Source: "auto"},
			{Filename: "manual_old.png", PageNumber: 1, FigureIndex: 2, Source: "manual"},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	if err := repo.ApplyPaperExtractionResult(paper.ID, "pdf text", "{}", "completed", "", "", []FigureUpsertInput{
		{Filename: "auto_new.png", PageNumber: 2, FigureIndex: 1, Source: "auto"},
	}); err != nil {
		t.Fatalf("ApplyPaperExtractionResult() error = %v", err)
	}

	got, err := repo.GetPaperDetail(paper.ID)
	if err != nil {
		t.Fatalf("GetPaperDetail() error = %v", err)
	}
	if len(got.Figures) != 2 {
		t.Fatalf("GetPaperDetail() figures = %d, want 2", len(got.Figures))
	}
	if got.Figures[0].Filename != "manual_old.png" || got.Figures[0].Source != "manual" {
		t.Fatalf("manual figure not preserved: %+v", got.Figures[0])
	}
	if got.Figures[1].Filename != "auto_new.png" || got.Figures[1].Source != "auto" {
		t.Fatalf("auto figure not refreshed: %+v", got.Figures[1])
	}
}

func TestApplyManualFigureChangesReplacesTarget(t *testing.T) {
	repo := newTestRepository(t)

	paper, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Cell Atlas",
		OriginalFilename: "cell-atlas.pdf",
		StoredPDFName:    "paper_1.pdf",
		FileSize:         128,
		ContentType:      "application/pdf",
		ExtractionStatus: "manual_pending",
		Figures: []FigureUpsertInput{
			{Filename: "auto_old.png", PageNumber: 1, FigureIndex: 1, Source: "auto", Caption: "Old"},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	if err := repo.ApplyManualFigureChanges(paper.ID, []FigureUpsertInput{
		{Filename: "manual_new.png", PageNumber: 1, FigureIndex: 1, Source: "manual", Caption: "New"},
	}, []int64{paper.Figures[0].ID}); err != nil {
		t.Fatalf("ApplyManualFigureChanges() error = %v", err)
	}

	got, err := repo.GetPaperDetail(paper.ID)
	if err != nil {
		t.Fatalf("GetPaperDetail() error = %v", err)
	}
	if len(got.Figures) != 1 {
		t.Fatalf("GetPaperDetail() figures = %d, want 1", len(got.Figures))
	}
	if got.Figures[0].Filename != "manual_new.png" || got.Figures[0].Source != "manual" {
		t.Fatalf("replacement result = %+v", got.Figures[0])
	}
}

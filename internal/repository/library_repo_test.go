package repository

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"paper_image_db/internal/apperr"
	"paper_image_db/internal/model"
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
	if got := len(figures[0].Tags); got != 2 {
		t.Fatalf("ListFigures() tags = %d, want 2", got)
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
	if tags, err := repo.ListTags(); err != nil || len(tags) != 0 {
		t.Fatalf("ListTags() after purge = len:%d err:%v", len(tags), err)
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

	if _, err := repo.CreateTag("Atlas", "#123456"); err != nil {
		t.Fatalf("CreateTag() setup error = %v", err)
	}
	if _, err := repo.CreateTag("Atlas", "#654321"); !apperr.IsCode(err, apperr.CodeConflict) {
		t.Fatalf("CreateTag() duplicate code = %q, want %q", apperr.CodeOf(err), apperr.CodeConflict)
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

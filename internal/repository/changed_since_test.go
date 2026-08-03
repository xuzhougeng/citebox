package repository

import (
	"testing"
	"time"

	"github.com/xuzhougeng/citebox/internal/model"
)

func mustCreateChangedSincePaper(t *testing.T, repo *LibraryRepository, title string) *model.Paper {
	t.Helper()
	paper, err := repo.CreatePaper(PaperUpsertInput{
		Title:            title,
		OriginalFilename: title + ".pdf",
		StoredPDFName:    title + ".pdf",
		FileSize:         128,
		ContentType:      "application/pdf",
	})
	if err != nil {
		t.Fatalf("CreatePaper(%s) error = %v", title, err)
	}
	return paper
}

func setRowUpdatedAt(t *testing.T, repo *LibraryRepository, table string, id int64, ts string) {
	t.Helper()
	if _, err := repo.DB().Exec("UPDATE "+table+" SET updated_at = ? WHERE id = ?", ts, id); err != nil {
		t.Fatalf("set %s.updated_at error = %v", table, err)
	}
}

func mustParseChangedSinceTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse("2006-01-02 15:04:05", value)
	if err != nil {
		t.Fatalf("parse time %q error = %v", value, err)
	}
	return parsed
}

func assertChangedSinceIDs(t *testing.T, label string, got, want []int64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s ids = %v, want %v", label, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s ids = %v, want %v", label, got, want)
		}
	}
}

func TestListPapersChangedSinceKeysetPaging(t *testing.T) {
	repo := newTestRepository(t)

	p1 := mustCreateChangedSincePaper(t, repo, "Changed Since A")
	p2 := mustCreateChangedSincePaper(t, repo, "Changed Since B")
	p3 := mustCreateChangedSincePaper(t, repo, "Changed Since C")
	p4 := mustCreateChangedSincePaper(t, repo, "Changed Since D")

	setRowUpdatedAt(t, repo, "papers", p1.ID, "2026-01-01 10:00:00")
	setRowUpdatedAt(t, repo, "papers", p2.ID, "2026-01-01 10:00:05")
	setRowUpdatedAt(t, repo, "papers", p3.ID, "2026-01-01 10:00:05")
	setRowUpdatedAt(t, repo, "papers", p4.ID, "2026-01-01 10:00:10")

	since := mustParseChangedSinceTime(t, "2025-12-31 23:59:59")

	page1, err := repo.ListPapersChangedSince(since, 0, 3)
	if err != nil {
		t.Fatalf("ListPapersChangedSince(page1) error = %v", err)
	}
	got1 := []int64{}
	for _, paper := range page1 {
		got1 = append(got1, paper.ID)
	}
	assertChangedSinceIDs(t, "page1", got1, []int64{p1.ID, p2.ID, p3.ID})
	if page1[0].Title != p1.Title {
		t.Fatalf("page1[0].Title = %q, want %q", page1[0].Title, p1.Title)
	}
	if got := page1[1].UpdatedAt.UTC().Format("2006-01-02 15:04:05"); got != "2026-01-01 10:00:05" {
		t.Fatalf("page1[1].UpdatedAt = %q, want 2026-01-01 10:00:05", got)
	}

	// 用上一页最后一条的 (updated_at, id) 续页，跨越相同时间戳的并列行
	last := page1[len(page1)-1]
	page2, err := repo.ListPapersChangedSince(last.UpdatedAt, last.ID, 3)
	if err != nil {
		t.Fatalf("ListPapersChangedSince(page2) error = %v", err)
	}
	got2 := []int64{}
	for _, paper := range page2 {
		got2 = append(got2, paper.ID)
	}
	assertChangedSinceIDs(t, "page2", got2, []int64{p4.ID})

	// 两页合并后不重不漏
	seen := map[int64]int{}
	for _, id := range append(got1, got2...) {
		seen[id]++
	}
	for _, id := range []int64{p1.ID, p2.ID, p3.ID, p4.ID} {
		if seen[id] != 1 {
			t.Fatalf("paper %d seen %d times across pages, want exactly 1", id, seen[id])
		}
	}

	// 从并列时间戳中间续页：只返回之后的行
	rest, err := repo.ListPapersChangedSince(mustParseChangedSinceTime(t, "2026-01-01 10:00:05"), p2.ID, 10)
	if err != nil {
		t.Fatalf("ListPapersChangedSince(tie resume) error = %v", err)
	}
	gotRest := []int64{}
	for _, paper := range rest {
		gotRest = append(gotRest, paper.ID)
	}
	assertChangedSinceIDs(t, "tie resume", gotRest, []int64{p3.ID, p4.ID})
}

func TestListFiguresChangedSinceKeysetPaging(t *testing.T) {
	repo := newTestRepository(t)

	paper, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Changed Since Figures",
		OriginalFilename: "changed-since-figures.pdf",
		StoredPDFName:    "changed-since-figures.pdf",
		FileSize:         128,
		ContentType:      "application/pdf",
		Figures: []FigureUpsertInput{
			{Filename: "cs_figure_1.png", OriginalName: "cs-figure-1.png", ContentType: "image/png", PageNumber: 1, FigureIndex: 1},
			{Filename: "cs_figure_2.png", OriginalName: "cs-figure-2.png", ContentType: "image/png", PageNumber: 2, FigureIndex: 1},
			{Filename: "cs_figure_3.png", OriginalName: "cs-figure-3.png", ContentType: "image/png", PageNumber: 3, FigureIndex: 1},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}
	if len(paper.Figures) != 3 {
		t.Fatalf("CreatePaper() figures = %d, want 3", len(paper.Figures))
	}

	f1 := paper.Figures[0]
	f2 := paper.Figures[1]
	f3 := paper.Figures[2]

	setRowUpdatedAt(t, repo, "paper_figures", f1.ID, "2026-01-01 10:00:00")
	setRowUpdatedAt(t, repo, "paper_figures", f2.ID, "2026-01-01 10:00:05")
	setRowUpdatedAt(t, repo, "paper_figures", f3.ID, "2026-01-01 10:00:05")

	since := mustParseChangedSinceTime(t, "2025-12-31 23:59:59")

	page1, err := repo.ListFiguresChangedSince(since, 0, 2)
	if err != nil {
		t.Fatalf("ListFiguresChangedSince(page1) error = %v", err)
	}
	got1 := []int64{}
	for _, figure := range page1 {
		got1 = append(got1, figure.ID)
	}
	assertChangedSinceIDs(t, "page1", got1, []int64{f1.ID, f2.ID})
	if page1[0].Filename != f1.Filename {
		t.Fatalf("page1[0].Filename = %q, want %q", page1[0].Filename, f1.Filename)
	}

	last := page1[len(page1)-1]
	page2, err := repo.ListFiguresChangedSince(last.UpdatedAt, last.ID, 2)
	if err != nil {
		t.Fatalf("ListFiguresChangedSince(page2) error = %v", err)
	}
	got2 := []int64{}
	for _, figure := range page2 {
		got2 = append(got2, figure.ID)
	}
	assertChangedSinceIDs(t, "page2", got2, []int64{f3.ID})

	seen := map[int64]int{}
	for _, id := range append(got1, got2...) {
		seen[id]++
	}
	for _, id := range []int64{f1.ID, f2.ID, f3.ID} {
		if seen[id] != 1 {
			t.Fatalf("figure %d seen %d times across pages, want exactly 1", id, seen[id])
		}
	}
}

func TestListPDFAnnotationsChangedSinceKeysetPaging(t *testing.T) {
	repo := newTestRepository(t)

	paper := mustCreateChangedSincePaper(t, repo, "Changed Since Annotations")

	createAnnotation := func(quote string) *model.PDFAnnotation {
		t.Helper()
		annotation, err := repo.PDFAnnotation.Create(paper.ID, PDFAnnotationCreateInput{
			Type:      "highlight",
			QuoteText: quote,
			Color:     "yellow",
			Fragments: []model.PDFAnnotationFragment{
				{Page: 1, Left: 0.1, Top: 0.2, Width: 0.3, Height: 0.02},
			},
		})
		if err != nil {
			t.Fatalf("Create(%s) error = %v", quote, err)
		}
		return annotation
	}

	a1 := createAnnotation("alpha")
	a2 := createAnnotation("beta")
	a3 := createAnnotation("gamma")

	setRowUpdatedAt(t, repo, "pdf_annotations", a1.ID, "2026-01-01 10:00:00")
	setRowUpdatedAt(t, repo, "pdf_annotations", a2.ID, "2026-01-01 10:00:05")
	setRowUpdatedAt(t, repo, "pdf_annotations", a3.ID, "2026-01-01 10:00:05")

	since := mustParseChangedSinceTime(t, "2025-12-31 23:59:59")

	page1, err := repo.ListPDFAnnotationsChangedSince(since, 0, 2)
	if err != nil {
		t.Fatalf("ListPDFAnnotationsChangedSince(page1) error = %v", err)
	}
	got1 := []int64{}
	for _, annotation := range page1 {
		got1 = append(got1, annotation.ID)
	}
	assertChangedSinceIDs(t, "page1", got1, []int64{a1.ID, a2.ID})
	if page1[0].QuoteText != "alpha" {
		t.Fatalf("page1[0].QuoteText = %q, want %q", page1[0].QuoteText, "alpha")
	}

	last := page1[len(page1)-1]
	page2, err := repo.ListPDFAnnotationsChangedSince(last.UpdatedAt, last.ID, 2)
	if err != nil {
		t.Fatalf("ListPDFAnnotationsChangedSince(page2) error = %v", err)
	}
	got2 := []int64{}
	for _, annotation := range page2 {
		got2 = append(got2, annotation.ID)
	}
	assertChangedSinceIDs(t, "page2", got2, []int64{a3.ID})

	seen := map[int64]int{}
	for _, id := range append(got1, got2...) {
		seen[id]++
	}
	for _, id := range []int64{a1.ID, a2.ID, a3.ID} {
		if seen[id] != 1 {
			t.Fatalf("annotation %d seen %d times across pages, want exactly 1", id, seen[id])
		}
	}
}

func TestUpdatePaperPDFTextWithPages(t *testing.T) {
	repo := newTestRepository(t)
	paper := mustCreateChangedSincePaper(t, repo, "PDF Page Texts")

	// 从未写入逐页文本时返回 nil, nil
	pageTexts, err := repo.GetPaperPDFPageTexts(paper.ID)
	if err != nil {
		t.Fatalf("GetPaperPDFPageTexts() error = %v", err)
	}
	if pageTexts != nil {
		t.Fatalf("GetPaperPDFPageTexts() = %v, want nil", pageTexts)
	}

	// 写入全文 + 逐页文本并回读
	if err := repo.UpdatePaperPDFTextWithPages(paper.ID, "full text", []string{"page one", "page two"}); err != nil {
		t.Fatalf("UpdatePaperPDFTextWithPages() error = %v", err)
	}
	pageTexts, err = repo.GetPaperPDFPageTexts(paper.ID)
	if err != nil {
		t.Fatalf("GetPaperPDFPageTexts() error = %v", err)
	}
	if len(pageTexts) != 2 || pageTexts[0] != "page one" || pageTexts[1] != "page two" {
		t.Fatalf("GetPaperPDFPageTexts() = %v, want [page one page two]", pageTexts)
	}
	detail, err := repo.GetPaperDetail(paper.ID)
	if err != nil {
		t.Fatalf("GetPaperDetail() error = %v", err)
	}
	if detail.PDFText != "full text" {
		t.Fatalf("GetPaperDetail() pdf_text = %q, want %q", detail.PDFText, "full text")
	}

	// pageTexts 为 nil 时只更新全文，保留逐页文本
	if err := repo.UpdatePaperPDFTextWithPages(paper.ID, "updated text", nil); err != nil {
		t.Fatalf("UpdatePaperPDFTextWithPages(nil pages) error = %v", err)
	}
	pageTexts, err = repo.GetPaperPDFPageTexts(paper.ID)
	if err != nil {
		t.Fatalf("GetPaperPDFPageTexts() error = %v", err)
	}
	if len(pageTexts) != 2 || pageTexts[0] != "page one" || pageTexts[1] != "page two" {
		t.Fatalf("GetPaperPDFPageTexts() after nil update = %v, want [page one page two]", pageTexts)
	}
	detail, err = repo.GetPaperDetail(paper.ID)
	if err != nil {
		t.Fatalf("GetPaperDetail() error = %v", err)
	}
	if detail.PDFText != "updated text" {
		t.Fatalf("GetPaperDetail() pdf_text = %q, want %q", detail.PDFText, "updated text")
	}

	// 旧的 UpdatePaperPDFText 行为不变，也不触碰逐页文本
	if _, err := repo.UpdatePaperPDFText(paper.ID, "legacy text"); err != nil {
		t.Fatalf("UpdatePaperPDFText() error = %v", err)
	}
	pageTexts, err = repo.GetPaperPDFPageTexts(paper.ID)
	if err != nil {
		t.Fatalf("GetPaperPDFPageTexts() error = %v", err)
	}
	if len(pageTexts) != 2 || pageTexts[0] != "page one" || pageTexts[1] != "page two" {
		t.Fatalf("GetPaperPDFPageTexts() after legacy update = %v, want [page one page two]", pageTexts)
	}
	detail, err = repo.GetPaperDetail(paper.ID)
	if err != nil {
		t.Fatalf("GetPaperDetail() error = %v", err)
	}
	if detail.PDFText != "legacy text" {
		t.Fatalf("GetPaperDetail() pdf_text = %q, want %q", detail.PDFText, "legacy text")
	}
}

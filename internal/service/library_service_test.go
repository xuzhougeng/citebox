package service

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/config"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/weixin"
	wolaiapi "github.com/xuzhougeng/citebox/internal/wolai"
)

func newTestService(t *testing.T) (*LibraryService, *repository.LibraryRepository, *config.Config) {
	t.Helper()

	root := t.TempDir()
	cfg := &config.Config{
		StorageDir:              filepath.Join(root, "storage"),
		DatabasePath:            filepath.Join(root, "library.db"),
		MaxUploadSize:           10 << 20,
		AdminUsername:           "citebox",
		AdminPassword:           "citebox123",
		ExtractorTimeoutSeconds: 1,
		ExtractorPollInterval:   1,
		ExtractorFileField:      "file",
	}

	repo, err := repository.NewLibraryRepository(cfg.DatabasePath)
	if err != nil {
		t.Fatalf("NewLibraryRepository() error = %v", err)
	}
	t.Cleanup(func() {
		_ = repo.Close()
	})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := NewLibraryService(repo, cfg, WithLogger(logger), WithoutBackgroundJobs())
	if err != nil {
		t.Fatalf("NewLibraryService() error = %v", err)
	}

	return svc, repo, cfg
}

func waitForPaperPDFText(t *testing.T, svc *LibraryService, paperID int64) string {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		paper, err := svc.GetPaper(paperID)
		if err == nil && paper != nil {
			pdfText := strings.TrimSpace(paper.PDFText)
			if pdfText != "" {
				return pdfText
			}
		}
		time.Sleep(20 * time.Millisecond)
	}

	paper, err := svc.GetPaper(paperID)
	if err != nil {
		t.Fatalf("GetPaper() after waiting error = %v", err)
	}
	t.Fatalf("paper %d pdf_text still empty after waiting; paper = %+v", paperID, paper)
	return ""
}

func createTestPaper(t *testing.T, repo *repository.LibraryRepository) *model.Paper {
	t.Helper()

	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Atlas Study",
		AuthorsText:      "Ada Lovelace, Alan Turing",
		Journal:          "Nature Communications",
		PublishedAt:      "2023-01-18",
		OriginalFilename: "atlas-study.pdf",
		StoredPDFName:    "paper_test.pdf",
		FileSize:         512,
		ContentType:      "application/pdf",
		PDFText:          "Atlas full text",
		AbstractText:     "Atlas abstract",
		NotesText:        "Atlas notes",
		PaperNotesText:   "Atlas paper notes",
		ExtractionStatus: "completed",
		Tags: []repository.TagUpsertInput{
			{Name: "Atlas", Color: "#123456"},
		},
		Figures: []repository.FigureUpsertInput{
			{
				Filename:     "figure_test.png",
				OriginalName: "figure-original.png",
				ContentType:  "image/png",
				PageNumber:   1,
				FigureIndex:  1,
				Caption:      "Figure",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	return paper
}

type testMultipartFile struct {
	*bytes.Reader
}

func wolaiBlockTypes(blocks []map[string]any) []string {
	types := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if blockType, ok := block["type"].(string); ok {
			types = append(types, blockType)
		}
	}
	return types
}

func wolaiBlockContents(blocks []map[string]any) []string {
	contents := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if content := wolaiBlockTitle(block); content != "" {
			contents = append(contents, content)
		}
	}
	return contents
}

func wolaiBlockTitle(block map[string]any) string {
	switch content := block["content"].(type) {
	case string:
		return strings.TrimSpace(content)
	case map[string]any:
		if title, ok := content["title"].(string); ok {
			return strings.TrimSpace(title)
		}
	}
	return ""
}

func (f *testMultipartFile) Close() error {
	return nil
}

func testPNGDataURL(t *testing.T, width, height int) string {
	t.Helper()

	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(testPNGBytes(t, width, height))
}

func testPNGBytes(t *testing.T, width, height int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(10 + x), G: uint8(20 + y), B: 180, A: 255})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode() error = %v", err)
	}

	return buf.Bytes()
}

func writeTestPNGFile(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", path, err)
	}
	if err := os.WriteFile(path, testPNGBytes(t, 8, 8), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func testPDFBytes() []byte {
	return []byte("%PDF-1.4\n1 0 obj\n<< /Type /Catalog >>\nendobj\ntrailer\n<< /Root 1 0 R >>\n%%EOF\n")
}

func useWeixinBindingTestServer(t *testing.T, svc *LibraryService, handler http.Handler) {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	svc.weixinClientFactory = func(token string) weixinBindingClient {
		return weixin.NewClient(server.URL, token, server.Client())
	}
}

type stubWolaiClient struct {
	getBlockFunc            func(id string) (map[string]any, error)
	createBlocksFunc        func(parentID string, blocks any) ([]wolaiapi.CreatedBlock, error)
	createUploadSessionFunc func(input wolaiapi.UploadSessionRequest) (*wolaiapi.UploadSession, error)
	uploadFileFunc          func(session wolaiapi.UploadSession, filename, contentType string, file io.Reader) error
	updateBlockFileFunc     func(blockID, fileID string) error
}

func (c *stubWolaiClient) GetBlock(id string) (map[string]any, error) {
	if c.getBlockFunc != nil {
		return c.getBlockFunc(id)
	}
	return map[string]any{"id": id}, nil
}

func (c *stubWolaiClient) CreateBlocks(parentID string, blocks any) ([]wolaiapi.CreatedBlock, error) {
	if c.createBlocksFunc != nil {
		return c.createBlocksFunc(parentID, blocks)
	}
	return nil, nil
}

func (c *stubWolaiClient) CreateUploadSession(input wolaiapi.UploadSessionRequest) (*wolaiapi.UploadSession, error) {
	if c.createUploadSessionFunc != nil {
		return c.createUploadSessionFunc(input)
	}
	return &wolaiapi.UploadSession{}, nil
}

func (c *stubWolaiClient) UploadFile(session wolaiapi.UploadSession, filename, contentType string, file io.Reader) error {
	if c.uploadFileFunc != nil {
		return c.uploadFileFunc(session, filename, contentType, file)
	}
	return nil
}

func (c *stubWolaiClient) UpdateBlockFile(blockID, fileID string) error {
	if c.updateBlockFileFunc != nil {
		return c.updateBlockFileFunc(blockID, fileID)
	}
	return nil
}

func TestListPapersAppliesDefaultsAndDecoratesURLs(t *testing.T) {
	svc, repo, _ := newTestService(t)
	createTestPaper(t, repo)

	result, err := svc.ListPapers(model.PaperFilter{})
	if err != nil {
		t.Fatalf("ListPapers() error = %v", err)
	}

	if result.Page != 1 || result.PageSize != 12 || result.Total != 1 || result.TotalPages != 1 {
		t.Fatalf("ListPapers() pagination = %+v", result)
	}
	if got := result.Papers[0].PDFURL; got != "/files/papers/paper_test.pdf" {
		t.Fatalf("ListPapers() pdf_url = %q, want %q", got, "/files/papers/paper_test.pdf")
	}
}

func TestGetPaperDecoratesFigureURLs(t *testing.T) {
	svc, repo, _ := newTestService(t)
	paper := createTestPaper(t, repo)

	got, err := svc.GetPaper(paper.ID)
	if err != nil {
		t.Fatalf("GetPaper() error = %v", err)
	}

	if got.PDFURL != "/files/papers/paper_test.pdf" {
		t.Fatalf("GetPaper() pdf_url = %q, want %q", got.PDFURL, "/files/papers/paper_test.pdf")
	}
	if len(got.Figures) != 1 || got.Figures[0].ImageURL != "/files/figures/figure_test.png" {
		t.Fatalf("GetPaper() figures = %+v", got.Figures)
	}
	if got.Figures[0].Source != "auto" {
		t.Fatalf("GetPaper() figure source = %q, want %q", got.Figures[0].Source, "auto")
	}
}

func TestDeletePaperRemovesFilesAndReturnsNotFound(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createTestPaper(t, repo)

	if err := os.WriteFile(filepath.Join(cfg.PapersDir(), paper.StoredPDFName), []byte("pdf"), 0o644); err != nil {
		t.Fatalf("WriteFile(pdf) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename), []byte("img"), 0o644); err != nil {
		t.Fatalf("WriteFile(figure) error = %v", err)
	}

	if err := svc.DeletePaper(paper.ID); err != nil {
		t.Fatalf("DeletePaper() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(cfg.PapersDir(), paper.StoredPDFName)); !os.IsNotExist(err) {
		t.Fatalf("paper file still exists, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename)); !os.IsNotExist(err) {
		t.Fatalf("figure file still exists, stat err = %v", err)
	}

	if err := svc.DeletePaper(paper.ID); !apperr.IsCode(err, apperr.CodeNotFound) {
		t.Fatalf("DeletePaper() missing code = %q, want %q", apperr.CodeOf(err), apperr.CodeNotFound)
	}
}

func TestUpdatePaperValidationErrors(t *testing.T) {
	svc, _, _ := newTestService(t)

	if _, err := svc.UpdatePaper(1, UpdatePaperParams{Title: "   "}); !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("UpdatePaper() empty title code = %q, want %q", apperr.CodeOf(err), apperr.CodeInvalidArgument)
	}

	groupID := int64(999)
	if _, err := svc.UpdatePaper(1, UpdatePaperParams{Title: "Valid", GroupID: &groupID}); !apperr.IsCode(err, apperr.CodeNotFound) {
		t.Fatalf("UpdatePaper() missing group code = %q, want %q", apperr.CodeOf(err), apperr.CodeNotFound)
	}
}

func TestUpdatePaperPersistsMetadata(t *testing.T) {
	svc, repo, _ := newTestService(t)
	paper := createTestPaper(t, repo)
	nextPDFText := "Updated full text"
	doi := "https://doi.org/10.4000/ATLAS-REVISED"

	updated, err := svc.UpdatePaper(paper.ID, UpdatePaperParams{
		Title:          "Atlas Study Revised",
		DOI:            &doi,
		PDFText:        &nextPDFText,
		AuthorsText:    "Grace Hopper, Donald Knuth",
		Journal:        "Journal of Computing",
		PublishedAt:    "2024-05",
		AbstractText:   "Updated abstract",
		NotesText:      "Updated notes",
		PaperNotesText: "Updated paper notes",
		Tags:           []string{"Atlas", "Revised"},
	})
	if err != nil {
		t.Fatalf("UpdatePaper() error = %v", err)
	}

	if updated.AbstractText != "Updated abstract" || updated.NotesText != "Updated notes" || updated.PaperNotesText != "Updated paper notes" {
		t.Fatalf("UpdatePaper() metadata = (%q, %q, %q), want updated values", updated.AbstractText, updated.NotesText, updated.PaperNotesText)
	}
	if updated.PDFText != nextPDFText {
		t.Fatalf("UpdatePaper() pdf_text = %q, want %q", updated.PDFText, nextPDFText)
	}
	if updated.DOI != "10.4000/atlas-revised" {
		t.Fatalf("UpdatePaper() doi = %q, want %q", updated.DOI, "10.4000/atlas-revised")
	}
	if updated.AuthorsText != "Grace Hopper, Donald Knuth" || updated.Journal != "Journal of Computing" || updated.PublishedAt != "2024-05" {
		t.Fatalf("UpdatePaper() doi metadata = (%q, %q, %q), want updated author/journal/published_at", updated.AuthorsText, updated.Journal, updated.PublishedAt)
	}
	if len(updated.Tags) != 2 {
		t.Fatalf("UpdatePaper() tags = %d, want 2", len(updated.Tags))
	}
}

func TestUpdatePaperKeepsPDFTextWhenOmitted(t *testing.T) {
	svc, repo, _ := newTestService(t)
	paper := createTestPaper(t, repo)

	updated, err := svc.UpdatePaper(paper.ID, UpdatePaperParams{
		Title:        "Atlas Study Retitled",
		AbstractText: "Fresh abstract",
	})
	if err != nil {
		t.Fatalf("UpdatePaper() error = %v", err)
	}

	if updated.PDFText != "Atlas full text" {
		t.Fatalf("UpdatePaper() pdf_text = %q, want %q", updated.PDFText, "Atlas full text")
	}
}

func TestUpdatePaperPDFTextOnlyPersistsText(t *testing.T) {
	svc, repo, _ := newTestService(t)
	paper := createTestPaper(t, repo)

	updated, err := svc.UpdatePaperPDFText(paper.ID, "  Extracted PDF full text  ")
	if err != nil {
		t.Fatalf("UpdatePaperPDFText() error = %v", err)
	}

	if updated.PDFText != "Extracted PDF full text" {
		t.Fatalf("UpdatePaperPDFText() pdf_text = %q, want %q", updated.PDFText, "Extracted PDF full text")
	}
	if updated.Title != paper.Title || updated.AbstractText != paper.AbstractText || updated.NotesText != paper.NotesText {
		t.Fatalf("UpdatePaperPDFText() mutated metadata: %+v", updated)
	}
}

func TestUpdatePaperPDFTextRejectsEmpty(t *testing.T) {
	svc, _, _ := newTestService(t)

	if _, err := svc.UpdatePaperPDFText(1, "   "); !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("UpdatePaperPDFText() code = %q, want %q", apperr.CodeOf(err), apperr.CodeInvalidArgument)
	}
}

func TestUpdateFigureTagsOnlyTouchesSelectedFigure(t *testing.T) {
	svc, repo, _ := newTestService(t)

	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Figure Tags",
		OriginalFilename: "figure-tags.pdf",
		StoredPDFName:    "figure-tags.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		Figures: []repository.FigureUpsertInput{
			{Filename: "figure_a.png", ContentType: "image/png", PageNumber: 1, FigureIndex: 1, Caption: "A"},
			{Filename: "figure_b.png", ContentType: "image/png", PageNumber: 1, FigureIndex: 2, Caption: "B"},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	updated, err := svc.UpdateFigureTags(paper.Figures[0].ID, []string{"signal"})
	if err != nil {
		t.Fatalf("UpdateFigureTags() error = %v", err)
	}

	if got := len(updated.Figures[0].Tags); got != 1 {
		t.Fatalf("updated first figure tags = %d, want 1", got)
	}
	if got := len(updated.Figures[1].Tags); got != 0 {
		t.Fatalf("updated second figure tags = %d, want 0", got)
	}
	if got := updated.Figures[0].ImageURL; got != "/files/figures/figure_a.png" {
		t.Fatalf("updated first figure image_url = %q, want %q", got, "/files/figures/figure_a.png")
	}

	tagID := updated.Figures[0].Tags[0].ID
	result, err := svc.ListFigures(model.FigureFilter{TagID: &tagID})
	if err != nil {
		t.Fatalf("ListFigures() error = %v", err)
	}
	if result.Total != 1 || len(result.Figures) != 1 {
		t.Fatalf("ListFigures() total=%d len=%d, want 1/1", result.Total, len(result.Figures))
	}
	if result.Figures[0].ID != paper.Figures[0].ID {
		t.Fatalf("ListFigures() figure id = %d, want %d", result.Figures[0].ID, paper.Figures[0].ID)
	}
}

func TestUpdateFigureNotesAreSearchable(t *testing.T) {
	svc, repo, _ := newTestService(t)

	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Figure Notes",
		OriginalFilename: "figure-notes.pdf",
		StoredPDFName:    "figure-notes.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		Figures: []repository.FigureUpsertInput{
			{Filename: "figure_a.png", ContentType: "image/png", PageNumber: 1, FigureIndex: 1, Caption: "A"},
			{Filename: "figure_b.png", ContentType: "image/png", PageNumber: 1, FigureIndex: 2, Caption: "B"},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	notes := "AI 总结：该图片强调了微环境重塑。"
	updated, err := svc.UpdateFigure(paper.Figures[0].ID, UpdateFigureParams{NotesText: &notes})
	if err != nil {
		t.Fatalf("UpdateFigure() error = %v", err)
	}

	if updated.Figures[0].NotesText != notes {
		t.Fatalf("updated first figure notes_text = %q, want %q", updated.Figures[0].NotesText, notes)
	}
	if updated.Figures[0].Caption != "A" {
		t.Fatalf("updated first figure caption = %q, want %q", updated.Figures[0].Caption, "A")
	}
	if updated.Figures[1].NotesText != "" {
		t.Fatalf("updated second figure notes_text = %q, want empty", updated.Figures[1].NotesText)
	}
	if updated.Figures[1].Caption != "B" {
		t.Fatalf("updated second figure caption = %q, want %q", updated.Figures[1].Caption, "B")
	}

	result, err := svc.ListFigures(model.FigureFilter{Keyword: "微环境重塑"})
	if err != nil {
		t.Fatalf("ListFigures() error = %v", err)
	}
	if result.Total != 1 || len(result.Figures) != 1 {
		t.Fatalf("ListFigures() total=%d len=%d, want 1/1", result.Total, len(result.Figures))
	}
	if result.Figures[0].ID != paper.Figures[0].ID {
		t.Fatalf("ListFigures() figure id = %d, want %d", result.Figures[0].ID, paper.Figures[0].ID)
	}
	if result.Figures[0].NotesText != notes {
		t.Fatalf("ListFigures() notes_text = %q, want %q", result.Figures[0].NotesText, notes)
	}
}

func TestPurgeLibraryRemovesStoredAssets(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createTestPaper(t, repo)

	if err := os.WriteFile(filepath.Join(cfg.PapersDir(), paper.StoredPDFName), []byte("pdf"), 0o644); err != nil {
		t.Fatalf("WriteFile(pdf) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename), []byte("img"), 0o644); err != nil {
		t.Fatalf("WriteFile(figure) error = %v", err)
	}

	if err := svc.PurgeLibrary(); err != nil {
		t.Fatalf("PurgeLibrary() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(cfg.PapersDir(), paper.StoredPDFName)); !os.IsNotExist(err) {
		t.Fatalf("paper file still exists, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename)); !os.IsNotExist(err) {
		t.Fatalf("figure file still exists, stat err = %v", err)
	}

	result, err := svc.ListPapers(model.PaperFilter{})
	if err != nil {
		t.Fatalf("ListPapers() error = %v", err)
	}
	if result.Total != 0 || len(result.Papers) != 0 {
		t.Fatalf("ListPapers() after purge = total:%d len:%d", result.Total, len(result.Papers))
	}
}

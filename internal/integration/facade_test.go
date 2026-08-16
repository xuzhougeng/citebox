package integration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/config"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service"
)

func newTestFacade(t *testing.T) (*Facade, *repository.LibraryRepository, *config.Config) {
	t.Helper()

	root := t.TempDir()
	cfg := &config.Config{
		StorageDir:    filepath.Join(root, "storage"),
		DatabasePath:  filepath.Join(root, "library.db"),
		MaxUploadSize: 10 << 20,
	}

	repo, err := repository.NewLibraryRepository(cfg.DatabasePath)
	if err != nil {
		t.Fatalf("NewLibraryRepository() error = %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := service.NewLibraryService(repo, cfg, service.WithLogger(logger), service.WithoutBackgroundJobs())
	if err != nil {
		t.Fatalf("NewLibraryService() error = %v", err)
	}

	facade := NewFacade(svc, repo, NewAssetStore(), NewService(repo.Setting))
	return facade, repo, cfg
}

func createFacadePaper(t *testing.T, repo *repository.LibraryRepository, title string) *model.Paper {
	t.Helper()
	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            title,
		OriginalFilename: title + ".pdf",
		StoredPDFName:    title + ".pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
	})
	if err != nil {
		t.Fatalf("CreatePaper(%s) error = %v", title, err)
	}
	return paper
}

func createFacadeAnnotation(t *testing.T, repo *repository.LibraryRepository, paperID int64, quote string) *model.PDFAnnotation {
	t.Helper()
	annotation, err := repo.PDFAnnotation.Create(paperID, repository.PDFAnnotationCreateInput{
		Type:      "highlight",
		QuoteText: quote,
		Color:     "yellow",
		Fragments: []model.PDFAnnotationFragment{
			{Page: 1, Left: 0.1, Top: 0.2, Width: 0.3, Height: 0.02},
		},
	})
	if err != nil {
		t.Fatalf("Create annotation(%s) error = %v", quote, err)
	}
	return annotation
}

func setFacadeRowUpdatedAt(t *testing.T, repo *repository.LibraryRepository, table string, id int64, ts string) {
	t.Helper()
	if _, err := repo.DB().Exec("UPDATE "+table+" SET updated_at = ? WHERE id = ?", ts, id); err != nil {
		t.Fatalf("set %s.updated_at error = %v", table, err)
	}
}

func testFacadePNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 6))
	for y := 0; y < 6; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: uint8(30 + x), G: uint8(40 + y), B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode() error = %v", err)
	}
	return buf.Bytes()
}

func TestFacadeGetCapabilities(t *testing.T) {
	facade, _, _ := newTestFacade(t)

	caps := facade.GetCapabilities()
	if caps["research_context_schema"] != ResearchContextSchema {
		t.Fatalf("research_context_schema = %v, want %q", caps["research_context_schema"], ResearchContextSchema)
	}
	if caps["transfer_package_schema"] != service.FigureTransferSchemaName {
		t.Fatalf("transfer_package_schema = %v, want %q", caps["transfer_package_schema"], service.FigureTransferSchemaName)
	}
	tools, ok := caps["tools"].([]string)
	if !ok || len(tools) != 7 {
		t.Fatalf("tools = %v, want 7 tool names", caps["tools"])
	}
	scopes, ok := caps["scopes"].([]string)
	if !ok || len(scopes) != 4 {
		t.Fatalf("scopes = %v, want 4 scopes", caps["scopes"])
	}
}

func TestFacadeSearchLibraryMergeAndCursor(t *testing.T) {
	facade, repo, _ := newTestFacade(t)

	// 文献 A 带笔记（同时出现在 paper 和 note 结果中），文献 B 无笔记
	paperA, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Merge Paper A",
		OriginalFilename: "merge-a.pdf",
		StoredPDFName:    "merge-a.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		AbstractText:     "abstract A",
		PaperNotesText:   "paper notes A",
		Figures: []repository.FigureUpsertInput{
			{Filename: "merge_a_fig.png", OriginalName: "merge-a-fig.png", ContentType: "image/png", PageNumber: 1, FigureIndex: 1, Caption: "Figure A1"},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper(A) error = %v", err)
	}
	paperB := createFacadePaper(t, repo, "Merge Paper B")
	annotation := createFacadeAnnotation(t, repo, paperB.ID, "merge annotation quote")

	// 固定合并顺序 paper(2) → figure(1) → note(1) → annotation(1)，共 5 条
	first, err := facade.SearchLibrary(SearchLibraryParams{Limit: 3})
	if err != nil {
		t.Fatalf("SearchLibrary(page1) error = %v", err)
	}
	if len(first.Items) != 3 {
		t.Fatalf("SearchLibrary(page1) items = %d, want 3", len(first.Items))
	}
	wantTypes := []string{EntityTypePaper, EntityTypePaper, EntityTypeFigure}
	for i, want := range wantTypes {
		if first.Items[i].EntityType != want {
			t.Fatalf("page1 item %d type = %q, want %q", i, first.Items[i].EntityType, want)
		}
	}
	if first.NextCursor == "" {
		t.Fatal("SearchLibrary(page1) next_cursor empty, want non-empty")
	}

	second, err := facade.SearchLibrary(SearchLibraryParams{Limit: 3, Cursor: first.NextCursor})
	if err != nil {
		t.Fatalf("SearchLibrary(page2) error = %v", err)
	}
	if len(second.Items) != 2 {
		t.Fatalf("SearchLibrary(page2) items = %d, want 2", len(second.Items))
	}
	if second.Items[0].EntityType != EntityTypeNote || second.Items[1].EntityType != EntityTypeAnnotation {
		t.Fatalf("page2 types = %q,%q, want note,annotation", second.Items[0].EntityType, second.Items[1].EntityType)
	}
	if second.NextCursor != "" {
		t.Fatalf("SearchLibrary(page2) next_cursor = %q, want empty", second.NextCursor)
	}

	// 两页合并后 source_id 不重不漏
	seen := map[string]int{}
	for _, item := range append(first.Items, second.Items...) {
		seen[item.SourceID]++
	}
	wantIDs := []string{
		PaperSourceID(paperA.ID),
		PaperSourceID(paperB.ID),
		FigureSourceID(paperA.Figures[0].ID),
		PaperNoteSourceID(paperA.ID),
		AnnotationSourceID(annotation.ID),
	}
	if len(seen) != len(wantIDs) {
		t.Fatalf("merged source ids = %v, want %d unique", seen, len(wantIDs))
	}
	for _, id := range wantIDs {
		if seen[id] != 1 {
			t.Fatalf("source id %s seen %d times, want 1", id, seen[id])
		}
	}

	// note 结果的 data 带笔记文本
	if data, ok := second.Items[0].Data.(map[string]any); !ok || data["text"] != "paper notes A" {
		t.Fatalf("note item data = %v, want text paper notes A", second.Items[0].Data)
	}
}

func TestFacadeSearchLibraryFiltersAndValidation(t *testing.T) {
	facade, repo, _ := newTestFacade(t)
	paper := createFacadePaper(t, repo, "Filter Paper")
	createFacadeAnnotation(t, repo, paper.ID, "filter annotation")

	// 限定实体类型
	result, err := facade.SearchLibrary(SearchLibraryParams{EntityTypes: []string{EntityTypeAnnotation}})
	if err != nil {
		t.Fatalf("SearchLibrary(annotation only) error = %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].EntityType != EntityTypeAnnotation {
		t.Fatalf("SearchLibrary(annotation only) items = %+v, want 1 annotation", result.Items)
	}

	// 关键词检索（FTS）
	result, err = facade.SearchLibrary(SearchLibraryParams{Query: "Filter Paper", EntityTypes: []string{EntityTypePaper}})
	if err != nil {
		t.Fatalf("SearchLibrary(keyword) error = %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].Title != "Filter Paper" {
		t.Fatalf("SearchLibrary(keyword) items = %+v, want Filter Paper", result.Items)
	}

	// 未知实体类型、非法游标
	if _, err := facade.SearchLibrary(SearchLibraryParams{EntityTypes: []string{"video"}}); !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("SearchLibrary(unknown type) code = %q, want %q", apperr.CodeOf(err), apperr.CodeInvalidArgument)
	}
	if _, err := facade.SearchLibrary(SearchLibraryParams{Cursor: "!!bad-cursor!!"}); !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("SearchLibrary(bad cursor) code = %q, want %q", apperr.CodeOf(err), apperr.CodeInvalidArgument)
	}

	// UpdatedAfter 后置过滤：时间戳在未来时结果为空
	future := time.Now().Add(24 * time.Hour)
	result, err = facade.SearchLibrary(SearchLibraryParams{UpdatedAfter: &future})
	if err != nil {
		t.Fatalf("SearchLibrary(updated_after future) error = %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("SearchLibrary(updated_after future) items = %d, want 0", len(result.Items))
	}
}

func TestFacadeGetPaperContextIncludes(t *testing.T) {
	facade, repo, _ := newTestFacade(t)

	tag, err := repo.CreateTag(model.TagScopePaper, "context-tag", "#112233")
	if err != nil {
		t.Fatalf("CreateTag() error = %v", err)
	}
	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Context Paper",
		OriginalFilename: "context.pdf",
		StoredPDFName:    "context.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		AbstractText:     "context abstract",
		PaperNotesText:   "context paper notes",
		Tags:             []repository.TagUpsertInput{{Name: "context-tag", Color: "#112233"}},
		Figures: []repository.FigureUpsertInput{
			{Filename: "context_fig.png", OriginalName: "context-fig.png", ContentType: "image/png", PageNumber: 1, FigureIndex: 1, Caption: "Context figure"},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}
	_ = tag
	annotation := createFacadeAnnotation(t, repo, paper.ID, "context annotation")

	// 空 Include = 全部包含
	full, err := facade.GetPaperContext(GetPaperContextParams{PaperID: paper.ID})
	if err != nil {
		t.Fatalf("GetPaperContext(full) error = %v", err)
	}
	if full.SchemaVersion != ResearchContextSchema || full.EntityType != EntityTypePaper {
		t.Fatalf("envelope header = %q/%q", full.SchemaVersion, full.EntityType)
	}
	if full.SourceID != PaperSourceID(paper.ID) {
		t.Fatalf("SourceID = %q, want %q", full.SourceID, PaperSourceID(paper.ID))
	}
	data, ok := full.Data.(map[string]any)
	if !ok {
		t.Fatalf("Data type = %T, want map", full.Data)
	}
	for _, key := range []string{"metadata", "abstract", "paper_notes", "figure_notes", "figures", "annotations", "tags", "group"} {
		if _, ok := data[key]; !ok {
			t.Fatalf("full data missing key %q", key)
		}
	}
	if data["abstract"] != "context abstract" || data["paper_notes"] != "context paper notes" {
		t.Fatalf("abstract/paper_notes = %v/%v", data["abstract"], data["paper_notes"])
	}
	figures, ok := data["figures"].([]map[string]any)
	if !ok || len(figures) != 1 {
		t.Fatalf("figures = %v, want 1 entry", data["figures"])
	}
	annotations, ok := data["annotations"].([]model.PDFAnnotation)
	if !ok || len(annotations) != 1 || annotations[0].ID != annotation.ID {
		t.Fatalf("annotations = %v, want 1 entry", data["annotations"])
	}
	// relations 覆盖 figure/annotation/tag；assets 提供 figure_image 描述符
	if len(full.Relations) != 3 {
		t.Fatalf("relations = %v, want 3 entries (figure+annotation+tag)", full.Relations)
	}
	if len(full.Assets) != 1 {
		t.Fatalf("assets = %v, want 1 entry", full.Assets)
	}
	asset, ok := full.Assets[0].(map[string]any)
	if !ok || asset["kind"] != AssetKindFigureImage || asset["figure_id"] != paper.Figures[0].ID {
		t.Fatalf("asset descriptor = %v", full.Assets[0])
	}

	// 裁剪 Include：只要 metadata 和 tags
	trimmed, err := facade.GetPaperContext(GetPaperContextParams{PaperID: paper.ID, Include: []string{"metadata", "tags"}})
	if err != nil {
		t.Fatalf("GetPaperContext(trimmed) error = %v", err)
	}
	trimmedData, ok := trimmed.Data.(map[string]any)
	if !ok {
		t.Fatalf("trimmed Data type = %T", trimmed.Data)
	}
	if len(trimmedData) != 2 {
		t.Fatalf("trimmed data keys = %v, want exactly metadata+tags", trimmedData)
	}
	if _, ok := trimmedData["metadata"]; !ok {
		t.Fatal("trimmed data missing metadata")
	}
	if _, ok := trimmedData["tags"]; !ok {
		t.Fatal("trimmed data missing tags")
	}
	// 未包含 figures 时不产出资产描述符和 figure/annotation 关系
	if len(trimmed.Assets) != 0 {
		t.Fatalf("trimmed assets = %v, want empty", trimmed.Assets)
	}
	for _, relation := range trimmed.Relations {
		if rel, ok := relation.(map[string]any); ok && rel["type"] != "tag" {
			t.Fatalf("trimmed relation = %v, want only tag relations", relation)
		}
	}

	if _, err := facade.GetPaperContext(GetPaperContextParams{PaperID: 0}); !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("GetPaperContext(0) code = %q, want %q", apperr.CodeOf(err), apperr.CodeInvalidArgument)
	}
	if _, err := facade.GetPaperContext(GetPaperContextParams{PaperID: 99999}); !apperr.IsCode(err, apperr.CodeNotFound) {
		t.Fatalf("GetPaperContext(missing) code = %q, want %q", apperr.CodeOf(err), apperr.CodeNotFound)
	}
}

func TestFacadeSearchPaperTextWithPageTexts(t *testing.T) {
	facade, repo, _ := newTestFacade(t)
	paper := createFacadePaper(t, repo, "Page Text Paper")

	pageTexts := []string{
		"page one has the Target phrase here",
		"page two has nothing",
		"page three also mentions target again",
	}
	if err := repo.UpdatePaperPDFTextWithPages(paper.ID, "full text", pageTexts); err != nil {
		t.Fatalf("UpdatePaperPDFTextWithPages() error = %v", err)
	}

	result, err := facade.SearchPaperText(SearchPaperTextParams{PaperIDs: []int64{paper.ID}, Query: "TARGET"})
	if err != nil {
		t.Fatalf("SearchPaperText() error = %v", err)
	}
	if len(result.Matches) != 2 {
		t.Fatalf("matches = %d, want 2", len(result.Matches))
	}
	// 页码为 1 起始
	if result.Matches[0].Page == nil || *result.Matches[0].Page != 1 {
		t.Fatalf("match[0].Page = %v, want 1", result.Matches[0].Page)
	}
	if result.Matches[1].Page == nil || *result.Matches[1].Page != 3 {
		t.Fatalf("match[1].Page = %v, want 3", result.Matches[1].Page)
	}
	if !strings.Contains(result.Matches[0].Snippet, "Target") {
		t.Fatalf("match[0].Snippet = %q, want context around Target", result.Matches[0].Snippet)
	}
	if result.Matches[0].SourceID != PaperSourceID(paper.ID) {
		t.Fatalf("match[0].SourceID = %q", result.Matches[0].SourceID)
	}
	if result.Matches[0].Revision == "" {
		t.Fatal("match[0].Revision empty")
	}
}

func TestFacadeSearchPaperTextWithoutPageTexts(t *testing.T) {
	facade, repo, _ := newTestFacade(t)

	// 只有整篇 PDFText，没有逐页文本
	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Whole Text Paper",
		OriginalFilename: "whole.pdf",
		StoredPDFName:    "whole.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		PDFText:          "the whole pdf text with a Target inside",
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	result, err := facade.SearchPaperText(SearchPaperTextParams{PaperIDs: []int64{paper.ID}, Query: "target"})
	if err != nil {
		t.Fatalf("SearchPaperText() error = %v", err)
	}
	if len(result.Matches) != 1 {
		t.Fatalf("matches = %d, want 1", len(result.Matches))
	}
	if result.Matches[0].Page != nil {
		t.Fatalf("match.Page = %v, want nil", result.Matches[0].Page)
	}
	if !strings.Contains(result.Matches[0].Snippet, "Target") {
		t.Fatalf("match.Snippet = %q", result.Matches[0].Snippet)
	}

	// 参数校验
	if _, err := facade.SearchPaperText(SearchPaperTextParams{PaperIDs: []int64{paper.ID}}); !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("SearchPaperText(empty query) code = %q, want %q", apperr.CodeOf(err), apperr.CodeInvalidArgument)
	}
	if _, err := facade.SearchPaperText(SearchPaperTextParams{Query: "x"}); !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("SearchPaperText(no papers) code = %q, want %q", apperr.CodeOf(err), apperr.CodeInvalidArgument)
	}
}

func TestFacadeGetEntity(t *testing.T) {
	facade, repo, _ := newTestFacade(t)

	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Entity Paper",
		OriginalFilename: "entity.pdf",
		StoredPDFName:    "entity.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		PaperNotesText:   "entity paper notes",
		Figures: []repository.FigureUpsertInput{
			{Filename: "entity_fig.png", OriginalName: "entity-fig.png", ContentType: "image/png", PageNumber: 1, FigureIndex: 1, Caption: "Entity figure"},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}
	figure := paper.Figures[0]
	if _, err := repo.UpdateFigure(figure.ID, repository.FigureUpdateInput{Caption: "Entity figure", NotesText: "entity figure notes"}); err != nil {
		t.Fatalf("UpdateFigure() error = %v", err)
	}
	annotation := createFacadeAnnotation(t, repo, paper.ID, "entity annotation")

	paperEnv, err := facade.GetEntity(PaperSourceID(paper.ID))
	if err != nil {
		t.Fatalf("GetEntity(paper) error = %v", err)
	}
	if paperEnv.EntityType != EntityTypePaper || paperEnv.SourceID != PaperSourceID(paper.ID) {
		t.Fatalf("paper envelope = %q/%q", paperEnv.EntityType, paperEnv.SourceID)
	}
	if _, ok := paperEnv.Data.(*model.Paper); !ok {
		t.Fatalf("paper data type = %T, want *model.Paper", paperEnv.Data)
	}

	figureEnv, err := facade.GetEntity(FigureSourceID(figure.ID))
	if err != nil {
		t.Fatalf("GetEntity(figure) error = %v", err)
	}
	if figureEnv.EntityType != EntityTypeFigure {
		t.Fatalf("figure envelope type = %q", figureEnv.EntityType)
	}
	if got, ok := figureEnv.Data.(*model.FigureListItem); !ok || got.ID != figure.ID {
		t.Fatalf("figure data = %v", figureEnv.Data)
	}

	annotationEnv, err := facade.GetEntity(AnnotationSourceID(annotation.ID))
	if err != nil {
		t.Fatalf("GetEntity(annotation) error = %v", err)
	}
	if got, ok := annotationEnv.Data.(*model.PDFAnnotation); !ok || got.ID != annotation.ID {
		t.Fatalf("annotation data = %v", annotationEnv.Data)
	}

	noteEnv, err := facade.GetEntity(PaperNoteSourceID(paper.ID))
	if err != nil {
		t.Fatalf("GetEntity(paper note) error = %v", err)
	}
	if noteEnv.EntityType != EntityTypeNote {
		t.Fatalf("note envelope type = %q", noteEnv.EntityType)
	}
	if data, ok := noteEnv.Data.(map[string]any); !ok || data["text"] != "entity paper notes" {
		t.Fatalf("paper note data = %v", noteEnv.Data)
	}

	figureNoteEnv, err := facade.GetEntity(FigureNoteSourceID(figure.ID))
	if err != nil {
		t.Fatalf("GetEntity(figure note) error = %v", err)
	}
	if data, ok := figureNoteEnv.Data.(map[string]any); !ok || data["text"] != "entity figure notes" {
		t.Fatalf("figure note data = %v", figureNoteEnv.Data)
	}

	// 错误路径：格式错误 → CodeInvalidArgument；不存在 → CodeNotFound
	if _, err := facade.GetEntity("not-a-source-id"); !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("GetEntity(garbage) code = %q, want %q", apperr.CodeOf(err), apperr.CodeInvalidArgument)
	}
	if _, err := facade.GetEntity(PaperSourceID(99999)); !apperr.IsCode(err, apperr.CodeNotFound) {
		t.Fatalf("GetEntity(missing paper) code = %q, want %q", apperr.CodeOf(err), apperr.CodeNotFound)
	}
	if _, err := facade.GetEntity(FigureSourceID(99999)); !apperr.IsCode(err, apperr.CodeNotFound) {
		t.Fatalf("GetEntity(missing figure) code = %q, want %q", apperr.CodeOf(err), apperr.CodeNotFound)
	}
	if _, err := facade.GetEntity(AnnotationSourceID(99999)); !apperr.IsCode(err, apperr.CodeNotFound) {
		t.Fatalf("GetEntity(missing annotation) code = %q, want %q", apperr.CodeOf(err), apperr.CodeNotFound)
	}
	if _, err := facade.GetEntity("citebox:note:paper:42:side"); !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("GetEntity(unknown note name) code = %q, want %q", apperr.CodeOf(err), apperr.CodeInvalidArgument)
	}
}

func TestFacadeExportAsset(t *testing.T) {
	facade, repo, cfg := newTestFacade(t)

	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Asset Paper",
		OriginalFilename: "asset.pdf",
		StoredPDFName:    "asset.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		Figures: []repository.FigureUpsertInput{
			{Filename: "asset_fig.png", OriginalName: "asset-fig.png", ContentType: "image/png", PageNumber: 1, FigureIndex: 1, Caption: "Asset figure"},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}
	figure := paper.Figures[0]
	imageData := testFacadePNG(t)
	if err := os.MkdirAll(cfg.FiguresDir(), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.FiguresDir(), figure.Filename), imageData, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// figure_image：sha256 与字节内容一致，资产可通过 AssetStore 取回
	imageExport, err := facade.ExportAsset(AssetKindFigureImage, figure.ID)
	if err != nil {
		t.Fatalf("ExportAsset(figure_image) error = %v", err)
	}
	wantSHA := sha256.Sum256(imageData)
	if imageExport.SHA256 != hex.EncodeToString(wantSHA[:]) {
		t.Fatalf("figure_image sha256 = %q, want %q", imageExport.SHA256, hex.EncodeToString(wantSHA[:]))
	}
	if imageExport.ByteSize != len(imageData) || imageExport.MediaType != "image/png" {
		t.Fatalf("figure_image export = %+v", imageExport)
	}
	if !strings.HasPrefix(imageExport.URL, "http://127.0.0.1:19831/assets/") {
		t.Fatalf("figure_image url = %q", imageExport.URL)
	}
	assetID := strings.TrimPrefix(imageExport.URL, "http://127.0.0.1:19831/assets/")
	stored, mediaType, _, ok := facade.assets.Get(assetID)
	if !ok {
		t.Fatal("AssetStore.Get() miss for exported image")
	}
	if !bytes.Equal(stored, imageData) || mediaType != "image/png" {
		t.Fatalf("stored asset bytes/media = %d/%q", len(stored), mediaType)
	}
	if time.Until(imageExport.ExpiresAt) <= 0 || time.Until(imageExport.ExpiresAt) > DefaultAssetTTL+time.Minute {
		t.Fatalf("expires_at = %v, want within ~10min", imageExport.ExpiresAt)
	}

	// figure_transfer_package：application/zip，sha256 正确
	pkgExport, err := facade.ExportAsset(AssetKindFigureTransferPackage, figure.ID)
	if err != nil {
		t.Fatalf("ExportAsset(figure_transfer_package) error = %v", err)
	}
	if pkgExport.MediaType != "application/zip" {
		t.Fatalf("package media type = %q, want application/zip", pkgExport.MediaType)
	}
	pkgID := strings.TrimPrefix(pkgExport.URL, "http://127.0.0.1:19831/assets/")
	storedPkg, pkgMedia, pkgFilename, ok := facade.assets.Get(pkgID)
	if !ok {
		t.Fatal("AssetStore.Get() miss for exported package")
	}
	pkgSHA := sha256.Sum256(storedPkg)
	if pkgExport.SHA256 != hex.EncodeToString(pkgSHA[:]) || pkgExport.ByteSize != len(storedPkg) {
		t.Fatalf("package export = %+v", pkgExport)
	}
	if pkgMedia != "application/zip" || !strings.HasSuffix(pkgFilename, ".zip") {
		t.Fatalf("package stored media/filename = %q/%q", pkgMedia, pkgFilename)
	}

	// 未知资产类型
	if _, err := facade.ExportAsset("paper_pdf", paper.ID); !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("ExportAsset(unknown) code = %q, want %q", apperr.CodeOf(err), apperr.CodeInvalidArgument)
	}
}

func TestFacadeListChangesPaging(t *testing.T) {
	facade, repo, _ := newTestFacade(t)

	p1 := createFacadePaper(t, repo, "Changes Paper 1")
	p2 := createFacadePaper(t, repo, "Changes Paper 2")
	p3, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Changes Paper 3",
		OriginalFilename: "changes-3.pdf",
		StoredPDFName:    "changes-3.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		Figures: []repository.FigureUpsertInput{
			{Filename: "changes_fig.png", OriginalName: "changes-fig.png", ContentType: "image/png", PageNumber: 1, FigureIndex: 1},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper(3) error = %v", err)
	}
	figure := p3.Figures[0]
	annotation := createFacadeAnnotation(t, repo, p1.ID, "changes annotation")

	// 制造跨类型的相同时间戳并列
	setFacadeRowUpdatedAt(t, repo, "papers", p1.ID, "2026-01-01 10:00:00")
	setFacadeRowUpdatedAt(t, repo, "papers", p2.ID, "2026-01-01 10:00:05")
	setFacadeRowUpdatedAt(t, repo, "papers", p3.ID, "2026-01-01 10:00:05")
	setFacadeRowUpdatedAt(t, repo, "paper_figures", figure.ID, "2026-01-01 10:00:05")
	setFacadeRowUpdatedAt(t, repo, "pdf_annotations", annotation.ID, "2026-01-01 10:00:05")

	// 固定顺序 paper(3) → figure(1) → annotation(1)，limit=2 翻 3 页
	var all []ChangeItem
	cursor := ""
	for page := 0; page < 3; page++ {
		result, err := facade.ListChanges(ListChangesParams{Cursor: cursor, Limit: 2})
		if err != nil {
			t.Fatalf("ListChanges(page %d) error = %v", page, err)
		}
		if len(result.Changes) == 0 {
			t.Fatalf("ListChanges(page %d) empty, want changes", page)
		}
		all = append(all, result.Changes...)
		cursor = result.NextCursor
		if cursor == "" {
			t.Fatalf("ListChanges(page %d) next_cursor empty, want non-empty", page)
		}
	}

	if len(all) != 5 {
		t.Fatalf("total changes = %d, want 5", len(all))
	}
	// 类型顺序 paper×3 → figure → annotation
	wantOrder := []string{
		PaperSourceID(p1.ID), PaperSourceID(p2.ID), PaperSourceID(p3.ID),
		FigureSourceID(figure.ID), AnnotationSourceID(annotation.ID),
	}
	seen := map[string]int{}
	for i, change := range all {
		if change.Operation != "updated" {
			t.Fatalf("change %d operation = %q, want updated", i, change.Operation)
		}
		if change.Revision == "" {
			t.Fatalf("change %d revision empty", i)
		}
		if change.SourceID != wantOrder[i] {
			t.Fatalf("change %d source_id = %q, want %q", i, change.SourceID, wantOrder[i])
		}
		seen[change.SourceID]++
	}
	for _, id := range wantOrder {
		if seen[id] != 1 {
			t.Fatalf("source id %s seen %d times, want 1（跨时间戳并列不重不漏）", id, seen[id])
		}
	}

	// 水位线之后再次调用返回空（不重复投递）
	final, err := facade.ListChanges(ListChangesParams{Cursor: cursor, Limit: 100})
	if err != nil {
		t.Fatalf("ListChanges(final) error = %v", err)
	}
	if len(final.Changes) != 0 {
		t.Fatalf("ListChanges(final) changes = %d, want 0", len(final.Changes))
	}

	// 非法游标
	if _, err := facade.ListChanges(ListChangesParams{Cursor: "!!bad!!"}); !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("ListChanges(bad cursor) code = %q, want %q", apperr.CodeOf(err), apperr.CodeInvalidArgument)
	}

	// 限定实体类型
	onlyFigures, err := facade.ListChanges(ListChangesParams{EntityTypes: []string{EntityTypeFigure}})
	if err != nil {
		t.Fatalf("ListChanges(figures only) error = %v", err)
	}
	if len(onlyFigures.Changes) != 1 || onlyFigures.Changes[0].SourceID != FigureSourceID(figure.ID) {
		t.Fatalf("ListChanges(figures only) = %+v", onlyFigures.Changes)
	}
}

package service

import (
	"archive/zip"

	"bytes"
	"encoding/json"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

func TestOfficialFigureLabel(t *testing.T) {
	cases := []struct {
		caption  string
		fallback string
		want     string
	}{
		{"Extended Data Fig. 3 | MetaNeighbor analysis", "Fig 1", "Extended Data Fig. 3"},
		{"Figure 2a. Something", "Fig 2", "Figure 2a"},
		{"Fig. 1 Endothelial overview", "Fig 1", "Fig. 1"},
		{"图 3 结果", "Fig 3", "图 3"},
		{"Cell states", "Fig 2", "Fig 2"},
		{"", "Fig 4", "Fig 4"},
	}
	for _, tc := range cases {
		if got := officialFigureLabel(tc.caption, tc.fallback); got != tc.want {
			t.Fatalf("officialFigureLabel(%q) = %q, want %q", tc.caption, got, tc.want)
		}
	}
}

func TestExportFigureTransferPackageMainFigure(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createFigureTransferTestPaper(t, repo)
	imageData := testPNGBytes(t, 40, 30)
	if err := os.WriteFile(filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename), imageData, 0o644); err != nil {
		t.Fatalf("WriteFile(figure) error = %v", err)
	}

	pkg, err := svc.ExportFigureTransferPackage(paper.Figures[0].ID)
	if err != nil {
		t.Fatalf("ExportFigureTransferPackage() error = %v", err)
	}
	manifest, err := ValidateFigureTransferPackage(pkg.Data)
	if err != nil {
		t.Fatalf("ValidateFigureTransferPackage() error = %v", err)
	}

	wantSourceID := "citebox:figure:" + strconv.FormatInt(paper.Figures[0].ID, 10)
	if manifest.Schema != FigureTransferSchemaName || manifest.Version != FigureTransferSchemaVersion {
		t.Fatalf("schema = %q version = %d", manifest.Schema, manifest.Version)
	}
	if manifest.Entry != figureTransferHandoffName {
		t.Fatalf("entry = %q", manifest.Entry)
	}
	if manifest.Producer.Name != "CiteBox" || manifest.Producer.Version == "" {
		t.Fatalf("producer = %+v", manifest.Producer)
	}
	if _, err := time.Parse(time.RFC3339, manifest.ExportedAt); err != nil {
		t.Fatalf("exportedAt = %q, parse error = %v", manifest.ExportedAt, err)
	}
	if manifest.Source.SourceID != wantSourceID || manifest.Source.FigureID != paper.Figures[0].ID || manifest.Source.ExtractionMethod != "auto" {
		t.Fatalf("source = %+v", manifest.Source)
	}
	if manifest.Figure.Kind != "figure" || manifest.Source.ParentFigureID != nil || len(manifest.Source.SubfigureLabels) != 0 {
		t.Fatalf("figure identifiers = %+v, source = %+v", manifest.Figure, manifest.Source)
	}
	if manifest.Figure.Number != 2 || manifest.Source.FigureLabel != "Fig 2" || manifest.Source.OfficialLabel != "Fig 2" || manifest.Source.Page == nil || *manifest.Source.Page != 7 || manifest.Source.Caption != "Cell states across tissues" {
		t.Fatalf("figure metadata = %+v, source = %+v", manifest.Figure, manifest.Source)
	}
	transferPaper := manifest.Source.Paper
	if transferPaper.ID != paper.ID || transferPaper.Title != "Transfer Atlas" || strings.Join(transferPaper.Authors, "; ") != "Ada Lovelace; Alan Turing" {
		t.Fatalf("paper metadata = %+v", transferPaper)
	}
	if transferPaper.Journal == nil || *transferPaper.Journal != "Journal of Tests" || transferPaper.Year == nil || *transferPaper.Year != 2024 || transferPaper.PublishedAt != "2024-06-03" {
		t.Fatalf("paper publication metadata = %+v", transferPaper)
	}
	if transferPaper.DOI == nil || *transferPaper.DOI != "10.1234/atlas.2024.7" || transferPaper.URL == nil || *transferPaper.URL != "https://doi.org/10.1234/atlas.2024.7" {
		t.Fatalf("paper identifiers = %+v", transferPaper)
	}
	if manifest.Source.License.Scope != "unknown" || manifest.Source.License.Text != nil {
		t.Fatalf("license = %+v", manifest.Source)
	}
	if manifest.Figure.File != "figure.png" || manifest.Figure.MediaType != "image/png" || manifest.Figure.Bytes != int64(len(imageData)) || len(manifest.Figure.SHA256) != 64 {
		t.Fatalf("figure file metadata = %+v", manifest.Figure)
	}

	entries := readFigureTransferEntries(t, pkg.Data)
	if len(entries) != 4 || !bytes.Equal(entries[manifest.Figure.File], imageData) {
		t.Fatalf("package entries = %v", transferEntryNames(entries))
	}

	var contextDoc FigureResearchContext
	if err := json.Unmarshal(entries[figureTransferContextName], &contextDoc); err != nil {
		t.Fatalf("unmarshal research context: %v", err)
	}
	if contextDoc.Paper.DOI == nil || *contextDoc.Paper.DOI != "10.1234/atlas.2024.7" || contextDoc.Paper.Abstract != "Atlas abstract about cell states." {
		t.Fatalf("research paper = %+v", contextDoc.Paper)
	}
	if !contextDoc.Completeness.DOI || !contextDoc.Completeness.Abstract || contextDoc.Replication.Mode != "visual_reference_only" || contextDoc.Replication.HasSourceData || contextDoc.Replication.HasCode {
		t.Fatalf("completeness/replication = %+v %+v", contextDoc.Completeness, contextDoc.Replication)
	}

	handoff := string(entries[figureTransferHandoffName])
	if !strings.Contains(handoff, "![Fig 2](figure.png)") || !strings.Contains(handoff, "### 原始图注") || !strings.Contains(handoff, "Cell states across tissues") {
		t.Fatalf("handoff missing figure/caption:\n%s", handoff)
	}
	if !strings.Contains(handoff, "10.1234/atlas.2024.7") || !strings.Contains(handoff, "Atlas abstract about cell states.") {
		t.Fatalf("handoff missing bibliography/abstract:\n%s", handoff)
	}
	if !strings.Contains(handoff, "visual reference") {
		t.Fatalf("handoff missing replication boundary:\n%s", handoff)
	}

	repeat, err := svc.ExportFigureTransferPackage(paper.Figures[0].ID)
	if err != nil {
		t.Fatalf("second ExportFigureTransferPackage() error = %v", err)
	}
	repeatEntries := readFigureTransferEntries(t, repeat.Data)
	if !bytes.Equal(entries[figureTransferContextName], repeatEntries[figureTransferContextName]) {
		t.Fatal("research-context.json is not deterministic")
	}
	if !bytes.Equal(entries[figureTransferHandoffName], repeatEntries[figureTransferHandoffName]) {
		t.Fatal("handoff.md is not deterministic")
	}

	manifestJSON := string(entries[figureTransferManifestName])
	contextJSON := string(entries[figureTransferContextName])
	for _, privateValue := range []string{cfg.StorageDir, paper.StoredPDFName, paper.Figures[0].Filename, paper.Figures[0].OriginalName} {
		if strings.Contains(manifestJSON, privateValue) || strings.Contains(contextJSON, privateValue) || strings.Contains(handoff, privateValue) {
			t.Fatalf("package leaks private file value %q", privateValue)
		}
	}
}

func TestExportFigureTransferPackageIncludesOfficialLabelAndExcerpts(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Kidney Atlas",
		DOI:              "10.1038/s41588-025-02285-0",
		AuthorsText:      "Klötzer, Abedini",
		Journal:          "Nature Genetics",
		PublishedAt:      "2025-08-01",
		AbstractText:     "Cross-species kidney atlas abstract.",
		OriginalFilename: "kidney.pdf",
		StoredPDFName:    "kidney.pdf",
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		Figures: []repository.FigureUpsertInput{{
			Filename:     "private-edfig.png",
			OriginalName: "/private/original-edfig.png",
			ContentType:  "image/png",
			PageNumber:   22,
			FigureIndex:  1,
			Source:       "auto",
			Caption:      "Extended Data Fig. 3 | MetaNeighbor-based cell type similarity analysis across species and datasets.",
		}},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}
	if err := repo.UpdatePaperPDFTextWithPages(paper.ID, "full", []string{
		"Introduction without the figure",
		"Results mention Extended Data Fig. 3 as the MetaNeighbor heatmap",
		"Discussion cites Figure 10 which must not steal Fig 1 matches",
	}); err != nil {
		t.Fatalf("UpdatePaperPDFTextWithPages() error = %v", err)
	}
	imageData := testPNGBytes(t, 16, 12)
	if err := os.WriteFile(filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename), imageData, 0o644); err != nil {
		t.Fatalf("WriteFile(figure) error = %v", err)
	}

	pkg, err := svc.ExportFigureTransferPackage(paper.Figures[0].ID)
	if err != nil {
		t.Fatalf("ExportFigureTransferPackage() error = %v", err)
	}
	manifest, err := ValidateFigureTransferPackage(pkg.Data)
	if err != nil {
		t.Fatalf("ValidateFigureTransferPackage() error = %v", err)
	}
	if manifest.Source.FigureLabel != "Fig 1" || manifest.Source.OfficialLabel != "Extended Data Fig. 3" {
		t.Fatalf("labels = %+v", manifest.Source)
	}

	entries := readFigureTransferEntries(t, pkg.Data)
	var contextDoc FigureResearchContext
	if err := json.Unmarshal(entries[figureTransferContextName], &contextDoc); err != nil {
		t.Fatalf("unmarshal research context: %v", err)
	}
	if !contextDoc.Completeness.Excerpts || !contextDoc.Completeness.PageTexts {
		t.Fatalf("completeness = %+v", contextDoc.Completeness)
	}
	foundOfficial := false
	for _, excerpt := range contextDoc.Excerpts {
		if strings.Contains(excerpt.Snippet, "Figure 10") && !strings.Contains(excerpt.Snippet, "Fig 1") && !strings.Contains(excerpt.Snippet, "Extended Data Fig. 3") {
			t.Fatalf("Fig 1 query leaked Figure 10: %+v", excerpt)
		}
		if excerpt.Page != nil && *excerpt.Page == 2 && strings.Contains(excerpt.Snippet, "Extended Data Fig. 3") {
			foundOfficial = true
		}
	}
	if !foundOfficial {
		t.Fatalf("excerpts = %+v, want page 2 official label hit", contextDoc.Excerpts)
	}
	handoff := string(entries[figureTransferHandoffName])
	if !strings.Contains(handoff, "Extended Data Fig. 3") || !strings.Contains(handoff, "### 原始图注") {
		t.Fatalf("handoff missing official caption:\n%s", handoff)
	}
}

func TestExportFigureTransferPackageAllowsIncompleteBibliography(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Klötzer 等   2025   Untitled atlas",
		OriginalFilename: "missing-doi.pdf",
		StoredPDFName:    "missing-doi.pdf",
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		Figures: []repository.FigureUpsertInput{{
			Filename:     "private-missing.png",
			OriginalName: "/private/missing.png",
			ContentType:  "image/png",
			PageNumber:   3,
			FigureIndex:  1,
			Source:       "auto",
			Caption:      "A figure without bibliographic identifiers.",
		}},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename), testPNGBytes(t, 8, 8), 0o644); err != nil {
		t.Fatalf("WriteFile(figure) error = %v", err)
	}

	pkg, err := svc.ExportFigureTransferPackage(paper.Figures[0].ID)
	if err != nil {
		t.Fatalf("ExportFigureTransferPackage() error = %v", err)
	}
	if _, err := ValidateFigureTransferPackage(pkg.Data); err != nil {
		t.Fatalf("ValidateFigureTransferPackage() error = %v", err)
	}
	entries := readFigureTransferEntries(t, pkg.Data)
	var contextDoc FigureResearchContext
	if err := json.Unmarshal(entries[figureTransferContextName], &contextDoc); err != nil {
		t.Fatalf("unmarshal research context: %v", err)
	}
	if contextDoc.Completeness.DOI || contextDoc.Completeness.Authors || contextDoc.Completeness.Journal || contextDoc.Completeness.Abstract {
		t.Fatalf("completeness = %+v, want missing bibliography", contextDoc.Completeness)
	}
	handoff := string(entries[figureTransferHandoffName])
	if !strings.Contains(handoff, "DOI：（缺失）") || !strings.Contains(handoff, "摘要") || !strings.Contains(handoff, missingHandoffValue) {
		t.Fatalf("handoff missing incomplete markers:\n%s", handoff)
	}
}

func TestExportFigureTransferPackageSubfigurePreservesParentIdentity(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createTestPaper(t, repo)
	parentData := testPNGBytes(t, 100, 80)
	if err := os.WriteFile(filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename), parentData, 0o644); err != nil {
		t.Fatalf("WriteFile(parent figure) error = %v", err)
	}

	updated, _, err := svc.CreateSubfigures(paper.Figures[0].ID, CreateSubfiguresParams{
		Regions: []model.SubfigureExtractionRegion{{
			X: 0.1, Y: 0.2, Width: 0.3, Height: 0.25, Label: "B", Caption: "Treatment panel",
		}},
	})
	if err != nil {
		t.Fatalf("CreateSubfigures() error = %v", err)
	}
	var child *model.Figure
	for i := range updated.Figures {
		if updated.Figures[i].ParentFigureID != nil {
			child = &updated.Figures[i]
			break
		}
	}
	if child == nil {
		t.Fatal("CreateSubfigures() did not return a subfigure")
	}

	pkg, err := svc.ExportFigureTransferPackage(child.ID)
	if err != nil {
		t.Fatalf("ExportFigureTransferPackage() error = %v", err)
	}
	manifest, err := ValidateFigureTransferPackage(pkg.Data)
	if err != nil {
		t.Fatalf("ValidateFigureTransferPackage() error = %v", err)
	}
	if manifest.Figure.Kind != "subfigure" || manifest.Source.ParentFigureID == nil || *manifest.Source.ParentFigureID != paper.Figures[0].ID {
		t.Fatalf("subfigure identifiers = %+v, source = %+v", manifest.Figure, manifest.Source)
	}
	if len(manifest.Source.SubfigureLabels) != 1 || manifest.Source.SubfigureLabels[0] != "b" || manifest.Source.FigureLabel != "Fig 1b" {
		t.Fatalf("subfigure relationship = %+v", manifest.Source)
	}
	if manifest.Figure.File != "figure.png" || manifest.Figure.MediaType != "image/png" {
		t.Fatalf("image metadata = %+v", manifest.Figure)
	}

	imageData := readFigureTransferEntries(t, pkg.Data)[manifest.Figure.File]
	image, err := png.Decode(bytes.NewReader(imageData))
	if err != nil {
		t.Fatalf("decode exported subfigure error = %v", err)
	}
	if image.Bounds().Dx() != 30 || image.Bounds().Dy() != 20 {
		t.Fatalf("exported subfigure bounds = %v, want 30x20", image.Bounds())
	}
}

func TestValidateFigureTransferPackageRejectsChangedImage(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createFigureTransferTestPaper(t, repo)
	if err := os.WriteFile(filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename), testPNGBytes(t, 16, 12), 0o644); err != nil {
		t.Fatalf("WriteFile(figure) error = %v", err)
	}
	pkg, err := svc.ExportFigureTransferPackage(paper.Figures[0].ID)
	if err != nil {
		t.Fatalf("ExportFigureTransferPackage() error = %v", err)
	}

	corrupted := rewriteFigureTransferImage(t, pkg.Data, pkg.Manifest.Figure.File)
	_, err = ValidateFigureTransferPackage(corrupted)
	if !apperr.IsCode(err, apperr.CodeInvalidArgument) || !strings.Contains(apperr.Message(err), "sha256 mismatch") {
		t.Fatalf("ValidateFigureTransferPackage() error = %v, want sha256 mismatch", err)
	}
}

func createFigureTransferTestPaper(t *testing.T, repo *repository.LibraryRepository) *model.Paper {
	t.Helper()
	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Transfer Atlas",
		DOI:              "10.1234/atlas.2024.7",
		AuthorsText:      "Ada Lovelace, Alan Turing",
		Journal:          "Journal of Tests",
		PublishedAt:      "2024-06-03",
		AbstractText:     "Atlas abstract about cell states.",
		OriginalFilename: "private-source.pdf",
		StoredPDFName:    "private-stored-name.pdf",
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		Figures: []repository.FigureUpsertInput{{
			Filename:     "private-figure-storage-name.png",
			OriginalName: "/private/machine/path/original-figure.png",
			ContentType:  "image/png",
			PageNumber:   7,
			FigureIndex:  2,
			Source:       "auto",
			Caption:      "Cell states across tissues",
		}},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}
	return paper
}

func readFigureTransferEntries(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip.NewReader() error = %v", err)
	}
	entries := make(map[string][]byte, len(reader.File))
	for _, file := range reader.File {
		entry, err := file.Open()
		if err != nil {
			t.Fatalf("Open(%q) error = %v", file.Name, err)
		}
		content, err := io.ReadAll(entry)
		_ = entry.Close()
		if err != nil {
			t.Fatalf("ReadAll(%q) error = %v", file.Name, err)
		}
		entries[file.Name] = content
	}
	return entries
}

func rewriteFigureTransferImage(t *testing.T, data []byte, imageName string) []byte {
	t.Helper()
	entries := readFigureTransferEntries(t, data)
	imageData := append([]byte(nil), entries[imageName]...)
	imageData[0] ^= 0xff
	entries[imageName] = imageData

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for _, name := range []string{figureTransferManifestName, figureTransferContextName, figureTransferHandoffName, imageName} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("Create(%q) error = %v", name, err)
		}
		if _, err := entry.Write(entries[name]); err != nil {
			t.Fatalf("Write(%q) error = %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil
	}
	return buf.Bytes()
}

func transferEntryNames(entries map[string][]byte) []string {
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	return names
}

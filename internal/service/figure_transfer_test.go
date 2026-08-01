package service

import (
	"archive/zip"
	"bytes"
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
	if manifest.Schema.Name != FigureTransferSchemaName || manifest.Schema.Version != FigureTransferSchemaVersion {
		t.Fatalf("schema = %+v", manifest.Schema)
	}
	if manifest.Source.ID != wantSourceID || manifest.Source.System != "citebox" || manifest.Source.URL != "https://doi.org/10.1234/atlas.2024.7" {
		t.Fatalf("source = %+v", manifest.Source)
	}
	if manifest.Figure.ID != paper.Figures[0].ID || manifest.Figure.Kind != "figure" || manifest.Figure.ParentID != nil || manifest.Figure.ParentSourceID != nil {
		t.Fatalf("figure identifiers = %+v", manifest.Figure)
	}
	if manifest.Figure.Number != 2 || manifest.Figure.DisplayLabel != "Fig 2" || manifest.Figure.PageNumber != 7 || manifest.Figure.Caption != "Cell states" {
		t.Fatalf("figure metadata = %+v", manifest.Figure)
	}
	if manifest.Paper.ID != paper.ID || manifest.Paper.Title != "Transfer Atlas" || manifest.Paper.Authors != "Ada Lovelace, Alan Turing" || manifest.Paper.Journal != "Journal of Tests" {
		t.Fatalf("paper metadata = %+v", manifest.Paper)
	}
	if manifest.Paper.Year == nil || *manifest.Paper.Year != 2024 || manifest.Paper.PublishedAt != "2024-06-03" || manifest.Paper.DOI != "10.1234/atlas.2024.7" {
		t.Fatalf("paper publication metadata = %+v", manifest.Paper)
	}
	if manifest.Rights.License != "unknown" || manifest.Rights.Statement != "unknown" {
		t.Fatalf("rights = %+v", manifest.Rights)
	}
	if _, err := time.Parse(time.RFC3339, manifest.Export.ExportedAt); err != nil || manifest.Export.CiteBoxVersion == "" {
		t.Fatalf("export metadata = %+v, parse error = %v", manifest.Export, err)
	}

	entries := readFigureTransferEntries(t, pkg.Data)
	if len(entries) != 2 || !bytes.Equal(entries[manifest.Image.Filename], imageData) {
		t.Fatalf("package entries = %v", transferEntryNames(entries))
	}
	manifestJSON := string(entries[figureTransferManifestName])
	for _, privateValue := range []string{cfg.StorageDir, paper.StoredPDFName, paper.Figures[0].Filename, paper.Figures[0].OriginalName} {
		if strings.Contains(manifestJSON, privateValue) {
			t.Fatalf("manifest leaks private file value %q: %s", privateValue, manifestJSON)
		}
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
	if manifest.Figure.Kind != "subfigure" || manifest.Figure.ParentID == nil || *manifest.Figure.ParentID != paper.Figures[0].ID {
		t.Fatalf("subfigure identifiers = %+v", manifest.Figure)
	}
	wantParentSourceID := "citebox:figure:" + strconv.FormatInt(paper.Figures[0].ID, 10)
	if manifest.Figure.ParentSourceID == nil || *manifest.Figure.ParentSourceID != wantParentSourceID || manifest.Figure.SubfigureLabel != "b" || manifest.Figure.DisplayLabel != "Fig 1b" {
		t.Fatalf("subfigure relationship = %+v", manifest.Figure)
	}
	if manifest.Image.Filename != "figure.png" || manifest.Image.MediaType != "image/png" {
		t.Fatalf("image metadata = %+v", manifest.Image)
	}

	imageData := readFigureTransferEntries(t, pkg.Data)[manifest.Image.Filename]
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

	corrupted := rewriteFigureTransferImage(t, pkg.Data, pkg.Manifest.Image.Filename)
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
			Caption:      "Cell states",
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
	for _, name := range []string{figureTransferManifestName, imageName} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("Create(%q) error = %v", name, err)
		}
		if _, err := entry.Write(entries[name]); err != nil {
			t.Fatalf("Write(%q) error = %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("zip Close() error = %v", err)
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

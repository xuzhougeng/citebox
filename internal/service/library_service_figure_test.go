package service

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

func TestDeleteFigureRemovesFileAndReturnsUpdatedPaper(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createTestPaper(t, repo)

	figurePath := filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename)
	if err := os.WriteFile(figurePath, []byte("img"), 0o644); err != nil {
		t.Fatalf("WriteFile(figure) error = %v", err)
	}

	updated, err := svc.DeleteFigure(paper.Figures[0].ID)
	if err != nil {
		t.Fatalf("DeleteFigure() error = %v", err)
	}
	if len(updated.Figures) != 0 {
		t.Fatalf("DeleteFigure() figures = %d, want 0", len(updated.Figures))
	}
	if _, err := os.Stat(figurePath); !os.IsNotExist(err) {
		t.Fatalf("figure file still exists, stat err = %v", err)
	}
}

func TestCreateSubfiguresAssignsLabelAndDecoratesParent(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createTestPaper(t, repo)

	parentPath := filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename)
	parentData, err := decodeBase64(testPNGDataURL(t, 80, 60))
	if err != nil {
		t.Fatalf("decodeBase64() error = %v", err)
	}
	if err := os.WriteFile(parentPath, parentData, 0o644); err != nil {
		t.Fatalf("WriteFile(parent figure) error = %v", err)
	}

	updated, addedCount, err := svc.CreateSubfigures(paper.Figures[0].ID, CreateSubfiguresParams{
		Regions: []model.SubfigureExtractionRegion{
			{
				X:         0.12,
				Y:         0.18,
				Width:     0.4,
				Height:    0.45,
				ImageData: testPNGDataURL(t, 20, 16),
				Caption:   "Panel A",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateSubfigures() error = %v", err)
	}
	if addedCount != 1 {
		t.Fatalf("CreateSubfigures() addedCount = %d, want 1", addedCount)
	}
	if len(updated.Figures) != 2 {
		t.Fatalf("CreateSubfigures() figures = %d, want 2", len(updated.Figures))
	}

	var parentFigure *model.Figure
	var childFigure *model.Figure
	for i := range updated.Figures {
		figure := &updated.Figures[i]
		if figure.ID == paper.Figures[0].ID {
			parentFigure = figure
		}
		if figure.ParentFigureID != nil {
			childFigure = figure
		}
	}
	if parentFigure == nil || childFigure == nil {
		t.Fatalf("CreateSubfigures() figures = %+v, want parent and child", updated.Figures)
	}
	if childFigure.SubfigureLabel != "a" {
		t.Fatalf("CreateSubfigures() subfigure_label = %q, want %q", childFigure.SubfigureLabel, "a")
	}
	if childFigure.DisplayLabel != "Fig 1a" {
		t.Fatalf("CreateSubfigures() display_label = %q, want %q", childFigure.DisplayLabel, "Fig 1a")
	}
	if childFigure.ParentDisplayLabel != "Fig 1" {
		t.Fatalf("CreateSubfigures() parent_display_label = %q, want %q", childFigure.ParentDisplayLabel, "Fig 1")
	}
	if len(parentFigure.Subfigures) != 1 || parentFigure.Subfigures[0].ID != childFigure.ID {
		t.Fatalf("CreateSubfigures() parent subfigures = %+v, want child %d", parentFigure.Subfigures, childFigure.ID)
	}
	if !strings.HasPrefix(childFigure.Filename, virtualSubfigureFilenamePrefix) {
		t.Fatalf("CreateSubfigures() filename = %q, want virtual metadata filename", childFigure.Filename)
	}
	if childFigure.ImageURL != "/api/figures/"+strconv.FormatInt(childFigure.ID, 10)+"/image" {
		t.Fatalf("CreateSubfigures() image_url = %q, want dynamic image route", childFigure.ImageURL)
	}
}

func TestCreateSubfiguresUsesManualLabelAndAutoFallback(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createTestPaper(t, repo)

	parentPath := filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename)
	parentData, err := decodeBase64(testPNGDataURL(t, 80, 60))
	if err != nil {
		t.Fatalf("decodeBase64() error = %v", err)
	}
	if err := os.WriteFile(parentPath, parentData, 0o644); err != nil {
		t.Fatalf("WriteFile(parent figure) error = %v", err)
	}

	updated, addedCount, err := svc.CreateSubfigures(paper.Figures[0].ID, CreateSubfiguresParams{
		Regions: []model.SubfigureExtractionRegion{
			{
				X:      0.08,
				Y:      0.10,
				Width:  0.22,
				Height: 0.28,
				Label:  "B",
			},
			{
				X:      0.40,
				Y:      0.20,
				Width:  0.20,
				Height: 0.25,
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateSubfigures() error = %v", err)
	}
	if addedCount != 2 {
		t.Fatalf("CreateSubfigures() addedCount = %d, want 2", addedCount)
	}

	var labels []string
	var displayLabels []string
	for _, figure := range updated.Figures {
		if figure.ParentFigureID == nil {
			continue
		}
		labels = append(labels, figure.SubfigureLabel)
		displayLabels = append(displayLabels, figure.DisplayLabel)
	}
	if !containsString(labels, "b") {
		t.Fatalf("CreateSubfigures() labels = %+v, want normalized manual label b", labels)
	}
	if !containsString(labels, "a") {
		t.Fatalf("CreateSubfigures() labels = %+v, want auto fallback label a", labels)
	}
	if !containsString(displayLabels, "Fig 1b") {
		t.Fatalf("CreateSubfigures() displayLabels = %+v, want manual display label Fig 1b", displayLabels)
	}
	if !containsString(displayLabels, "Fig 1a") {
		t.Fatalf("CreateSubfigures() displayLabels = %+v, want auto display label", displayLabels)
	}
}

func TestCreateSubfiguresAllowsAddingAAfterExistingB(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createTestPaper(t, repo)

	parentPath := filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename)
	parentData, err := decodeBase64(testPNGDataURL(t, 80, 60))
	if err != nil {
		t.Fatalf("decodeBase64() error = %v", err)
	}
	if err := os.WriteFile(parentPath, parentData, 0o644); err != nil {
		t.Fatalf("WriteFile(parent figure) error = %v", err)
	}

	if _, addedCount, err := svc.CreateSubfigures(paper.Figures[0].ID, CreateSubfiguresParams{
		Regions: []model.SubfigureExtractionRegion{
			{
				X:      0.08,
				Y:      0.10,
				Width:  0.22,
				Height: 0.28,
				Label:  "b",
			},
		},
	}); err != nil {
		t.Fatalf("CreateSubfigures(first) error = %v", err)
	} else if addedCount != 1 {
		t.Fatalf("CreateSubfigures(first) addedCount = %d, want 1", addedCount)
	}

	updated, addedCount, err := svc.CreateSubfigures(paper.Figures[0].ID, CreateSubfiguresParams{
		Regions: []model.SubfigureExtractionRegion{
			{
				X:      0.40,
				Y:      0.20,
				Width:  0.20,
				Height: 0.25,
				Label:  "a",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateSubfigures(second) error = %v", err)
	}
	if addedCount != 1 {
		t.Fatalf("CreateSubfigures(second) addedCount = %d, want 1", addedCount)
	}

	var labels []string
	for _, figure := range updated.Figures {
		if figure.ParentFigureID == nil {
			continue
		}
		labels = append(labels, figure.SubfigureLabel)
	}
	if !containsString(labels, "a") || !containsString(labels, "b") {
		t.Fatalf("CreateSubfigures(second) labels = %+v, want both a and b", labels)
	}
}

func TestGetFigureImageRendersSubfigureCrop(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createTestPaper(t, repo)

	parentPath := filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename)
	parentData, err := decodeBase64(testPNGDataURL(t, 100, 80))
	if err != nil {
		t.Fatalf("decodeBase64() error = %v", err)
	}
	if err := os.WriteFile(parentPath, parentData, 0o644); err != nil {
		t.Fatalf("WriteFile(parent figure) error = %v", err)
	}

	updated, _, err := svc.CreateSubfigures(paper.Figures[0].ID, CreateSubfiguresParams{
		Regions: []model.SubfigureExtractionRegion{
			{
				X:       0.1,
				Y:       0.2,
				Width:   0.3,
				Height:  0.25,
				Caption: "Panel A",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateSubfigures() error = %v", err)
	}

	var childFigure *model.Figure
	for i := range updated.Figures {
		if updated.Figures[i].ParentFigureID != nil {
			childFigure = &updated.Figures[i]
			break
		}
	}
	if childFigure == nil {
		t.Fatalf("CreateSubfigures() figures = %+v, want child figure", updated.Figures)
	}

	data, contentType, filename, err := svc.GetFigureImage(childFigure.ID)
	if err != nil {
		t.Fatalf("GetFigureImage() error = %v", err)
	}
	if contentType != "image/png" {
		t.Fatalf("GetFigureImage() content_type = %q, want %q", contentType, "image/png")
	}
	if filename != "figure-original_a.png" {
		t.Fatalf("GetFigureImage() filename = %q, want %q", filename, "figure-original_a.png")
	}

	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("png.Decode() error = %v", err)
	}
	if got := img.Bounds().Dx(); got != 30 {
		t.Fatalf("GetFigureImage() width = %d, want 30", got)
	}
	if got := img.Bounds().Dy(); got != 20 {
		t.Fatalf("GetFigureImage() height = %d, want 20", got)
	}
}

func TestCreateSubfiguresRejectsNonAlphabeticManualLabel(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createTestPaper(t, repo)

	parentPath := filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename)
	parentData, err := decodeBase64(testPNGDataURL(t, 64, 48))
	if err != nil {
		t.Fatalf("decodeBase64() error = %v", err)
	}
	if err := os.WriteFile(parentPath, parentData, 0o644); err != nil {
		t.Fatalf("WriteFile(parent figure) error = %v", err)
	}

	_, _, err = svc.CreateSubfigures(paper.Figures[0].ID, CreateSubfiguresParams{
		Regions: []model.SubfigureExtractionRegion{
			{
				X:      0.12,
				Y:      0.15,
				Width:  0.25,
				Height: 0.25,
				Label:  "12345",
			},
		},
	})
	if !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("CreateSubfigures() code = %q, want %q", apperr.CodeOf(err), apperr.CodeInvalidArgument)
	}
}

func TestDeleteFigureRemovesParentFileForSubfigureBranch(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createTestPaper(t, repo)

	parentPath := filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename)
	parentData, err := decodeBase64(testPNGDataURL(t, 64, 48))
	if err != nil {
		t.Fatalf("decodeBase64() error = %v", err)
	}
	if err := os.WriteFile(parentPath, parentData, 0o644); err != nil {
		t.Fatalf("WriteFile(parent figure) error = %v", err)
	}

	updated, _, err := svc.CreateSubfigures(paper.Figures[0].ID, CreateSubfiguresParams{
		Regions: []model.SubfigureExtractionRegion{
			{
				X:         0.1,
				Y:         0.1,
				Width:     0.35,
				Height:    0.35,
				ImageData: testPNGDataURL(t, 18, 12),
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateSubfigures() error = %v", err)
	}

	var childFigure *model.Figure
	for i := range updated.Figures {
		if updated.Figures[i].ParentFigureID != nil {
			childFigure = &updated.Figures[i]
			break
		}
	}
	if childFigure == nil {
		t.Fatalf("CreateSubfigures() missing child figure: %+v", updated.Figures)
	}
	if !strings.HasPrefix(childFigure.Filename, virtualSubfigureFilenamePrefix) {
		t.Fatalf("CreateSubfigures() filename = %q, want virtual metadata filename", childFigure.Filename)
	}

	result, err := svc.DeleteFigure(paper.Figures[0].ID)
	if err != nil {
		t.Fatalf("DeleteFigure() error = %v", err)
	}
	if len(result.Figures) != 0 {
		t.Fatalf("DeleteFigure() figures = %d, want 0", len(result.Figures))
	}
	if _, err := os.Stat(parentPath); !os.IsNotExist(err) {
		t.Fatalf("parent figure file still exists, stat err = %v", err)
	}
}

func TestCreateOrUpdateFigurePaletteBindsPaletteToSubfigure(t *testing.T) {
	svc, repo, _ := newTestService(t)
	paper := createTestPaper(t, repo)

	updated, _, err := svc.CreateSubfigures(paper.Figures[0].ID, CreateSubfiguresParams{
		Regions: []model.SubfigureExtractionRegion{
			{
				X:         0.15,
				Y:         0.2,
				Width:     0.3,
				Height:    0.35,
				ImageData: testPNGDataURL(t, 24, 18),
				Caption:   "Panel A",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateSubfigures() error = %v", err)
	}

	var childFigure *model.Figure
	for i := range updated.Figures {
		if updated.Figures[i].ParentFigureID != nil {
			childFigure = &updated.Figures[i]
			break
		}
	}
	if childFigure == nil {
		t.Fatalf("CreateSubfigures() figures = %+v, want child figure", updated.Figures)
	}

	palette, refreshedPaper, err := svc.CreateOrUpdateFigurePalette(childFigure.ID, CreatePaletteParams{
		Colors: []string{"#aabbcc", "#DDEEFF", "#ddeeff"},
	})
	if err != nil {
		t.Fatalf("CreateOrUpdateFigurePalette() error = %v", err)
	}
	if palette.Name != "Fig 1a 配色" {
		t.Fatalf("CreateOrUpdateFigurePalette() name = %q, want %q", palette.Name, "Fig 1a 配色")
	}
	if len(palette.Colors) != 2 || palette.Colors[0] != "#AABBCC" || palette.Colors[1] != "#DDEEFF" {
		t.Fatalf("CreateOrUpdateFigurePalette() colors = %+v, want normalized unique colors", palette.Colors)
	}

	var refreshedChild *model.Figure
	for i := range refreshedPaper.Figures {
		if refreshedPaper.Figures[i].ID == childFigure.ID {
			refreshedChild = &refreshedPaper.Figures[i]
			break
		}
	}
	if refreshedChild == nil {
		t.Fatalf("CreateOrUpdateFigurePalette() paper figures = %+v, want refreshed child", refreshedPaper.Figures)
	}
	if refreshedChild.PaletteCount != 1 {
		t.Fatalf("CreateOrUpdateFigurePalette() palette_count = %d, want 1", refreshedChild.PaletteCount)
	}
	if refreshedChild.PaletteName != "Fig 1a 配色" {
		t.Fatalf("CreateOrUpdateFigurePalette() palette_name = %q, want %q", refreshedChild.PaletteName, "Fig 1a 配色")
	}
	if len(refreshedChild.PaletteColors) != 2 || refreshedChild.PaletteColors[0] != "#AABBCC" || refreshedChild.PaletteColors[1] != "#DDEEFF" {
		t.Fatalf("CreateOrUpdateFigurePalette() palette_colors = %+v, want persisted colors", refreshedChild.PaletteColors)
	}
}

func TestCreateOrUpdateFigurePaletteRejectsParentFigure(t *testing.T) {
	svc, repo, _ := newTestService(t)
	paper := createTestPaper(t, repo)

	_, _, err := svc.CreateOrUpdateFigurePalette(paper.Figures[0].ID, CreatePaletteParams{
		Colors: []string{"#112233"},
	})
	if !apperr.IsCode(err, apperr.CodeFailedPrecondition) {
		t.Fatalf("CreateOrUpdateFigurePalette() code = %q, want %q", apperr.CodeOf(err), apperr.CodeFailedPrecondition)
	}
}

func TestListFiguresExcludesSubfigures(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createTestPaper(t, repo)

	parentPath := filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename)
	parentData, err := decodeBase64(testPNGDataURL(t, 60, 40))
	if err != nil {
		t.Fatalf("decodeBase64() error = %v", err)
	}
	if err := os.WriteFile(parentPath, parentData, 0o644); err != nil {
		t.Fatalf("WriteFile(parent figure) error = %v", err)
	}

	if _, _, err := svc.CreateSubfigures(paper.Figures[0].ID, CreateSubfiguresParams{
		Regions: []model.SubfigureExtractionRegion{
			{
				X:      0.15,
				Y:      0.2,
				Width:  0.3,
				Height: 0.35,
			},
		},
	}); err != nil {
		t.Fatalf("CreateSubfigures() error = %v", err)
	}

	result, err := svc.ListFigures(model.FigureFilter{})
	if err != nil {
		t.Fatalf("ListFigures() error = %v", err)
	}
	if result.Total != 1 || len(result.Figures) != 1 {
		t.Fatalf("ListFigures() total=%d len=%d, want 1/1", result.Total, len(result.Figures))
	}
	if result.Figures[0].ParentFigureID != nil {
		t.Fatalf("ListFigures() returned subfigure: %+v", result.Figures[0])
	}
}

func TestNormalizeManualRegionRejectsMissingImageData(t *testing.T) {
	if _, err := normalizeManualRegion(model.ManualExtractionRegion{
		PageNumber: 1,
		X:          0.1,
		Y:          0.1,
		Width:      0.4,
		Height:     0.4,
	}); !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("normalizeManualRegion() code = %q, want %q", apperr.CodeOf(err), apperr.CodeInvalidArgument)
	}
}

func TestNormalizeManualRegionMapsLLMSourceToAuto(t *testing.T) {
	region, err := normalizeManualRegion(model.ManualExtractionRegion{
		PageNumber: 1,
		X:          0.1,
		Y:          0.1,
		Width:      0.4,
		Height:     0.4,
		Source:     manualFigureSourceLLM,
		ImageData:  testPNGDataURL(t, 12, 10),
	})
	if err != nil {
		t.Fatalf("normalizeManualRegion() error = %v", err)
	}
	if region.Source != figureSourceAuto {
		t.Fatalf("normalizeManualRegion() source = %q, want %q", region.Source, figureSourceAuto)
	}
}

func TestManualExtractFiguresStoresClientRenderedImage(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createTestPaper(t, repo)

	if err := os.WriteFile(filepath.Join(cfg.PapersDir(), paper.StoredPDFName), []byte("%PDF-1.4 test"), 0o644); err != nil {
		t.Fatalf("WriteFile(pdf) error = %v", err)
	}

	updated, addedCount, err := svc.ManualExtractFigures(paper.ID, ManualExtractParams{
		Regions: []model.ManualExtractionRegion{
			{
				PageNumber: 1,
				X:          0.1,
				Y:          0.2,
				Width:      0.3,
				Height:     0.4,
				ImageData:  testPNGDataURL(t, 24, 18),
				Caption:    "Manual figure",
			},
		},
	})
	if err != nil {
		t.Fatalf("ManualExtractFigures() error = %v", err)
	}

	if addedCount != 1 {
		t.Fatalf("ManualExtractFigures() addedCount = %d, want 1", addedCount)
	}
	if len(updated.Figures) != 2 {
		t.Fatalf("ManualExtractFigures() figures = %d, want 2", len(updated.Figures))
	}

	var manualFigure *model.Figure
	for i := range updated.Figures {
		if updated.Figures[i].Source == "manual" {
			manualFigure = &updated.Figures[i]
			break
		}
	}
	if manualFigure == nil {
		t.Fatalf("ManualExtractFigures() missing manual figure: %+v", updated.Figures)
	}
	if manualFigure.Caption != "Manual figure" {
		t.Fatalf("ManualExtractFigures() caption = %q, want %q", manualFigure.Caption, "Manual figure")
	}
	if !strings.HasSuffix(manualFigure.Filename, ".png") {
		t.Fatalf("ManualExtractFigures() filename = %q, want .png suffix", manualFigure.Filename)
	}
	if _, err := os.Stat(filepath.Join(cfg.FiguresDir(), manualFigure.Filename)); err != nil {
		t.Fatalf("stored manual figure missing, stat err = %v", err)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

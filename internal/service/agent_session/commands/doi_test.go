package commands

import (
	"context"
	"errors"
	"testing"

	"github.com/xuzhougeng/citebox/internal/model"
)

// fakeDup mimics *service.DuplicatePaperError: it is an error AND exposes the
// existing paper via DuplicatePaper().
type fakeDup struct {
	paper *model.Paper
}

func (d *fakeDup) Error() string                { return "duplicate" }
func (d *fakeDup) DuplicatePaper() *model.Paper { return d.paper }

type fakeDOIImporter struct {
	normalized    string
	normalizeErr  error
	paper         *model.Paper
	importErr     error
	dup           *fakeDup
	lastNormalize string
	lastImport    ImportPaperByDOIParams
}

func (f *fakeDOIImporter) NormalizeDOI(input string) (string, error) {
	f.lastNormalize = input
	if f.normalizeErr != nil {
		return "", f.normalizeErr
	}
	if f.normalized != "" {
		return f.normalized, nil
	}
	return input, nil
}

func (f *fakeDOIImporter) ImportPaperByDOI(_ context.Context, p ImportPaperByDOIParams) (*model.Paper, error) {
	f.lastImport = p
	if f.dup != nil {
		return nil, f.dup
	}
	if f.importErr != nil {
		return nil, f.importErr
	}
	return f.paper, nil
}

func TestDOIImportsAndSetsCurrent(t *testing.T) {
	imp := &fakeDOIImporter{
		normalized: "10.1038/s41586-023-06000-1",
		paper:      &model.Paper{ID: 314, Title: "Imported Title"},
	}
	fs := &fakeState{}
	cmd := &DOICommand{Importer: imp}
	res, err := cmd.Execute(context.Background(),
		"https://doi.org/10.1038/s41586-023-06000-1",
		RuntimeCtx{Surface: "wechat", State: fs})
	if err != nil {
		t.Fatal(err)
	}
	if !res.UsedShortcut {
		t.Error("want UsedShortcut=true")
	}
	if imp.lastImport.DOI != "10.1038/s41586-023-06000-1" {
		t.Errorf("want normalized DOI passed to importer, got %q", imp.lastImport.DOI)
	}
	if fs.paperID != 314 || fs.title != "Imported Title" {
		t.Errorf("want current paper set to 314/Imported Title, got %d/%q", fs.paperID, fs.title)
	}
	if len(res.ChunksText) != 1 || !contains(res.ChunksText[0], "Imported Title") || !contains(res.ChunksText[0], "#314") {
		t.Fatalf("want import-success message, got %q", res.ChunksText)
	}
}

func TestDOIDuplicateSwitchesToExisting(t *testing.T) {
	existing := &model.Paper{ID: 7, Title: "Existing Paper"}
	imp := &fakeDOIImporter{
		normalized: "10.1000/xyz",
		dup:        &fakeDup{paper: existing},
	}
	fs := &fakeState{}
	cmd := &DOICommand{Importer: imp}
	res, err := cmd.Execute(context.Background(), "10.1000/xyz",
		RuntimeCtx{Surface: "wechat", State: fs})
	if err != nil {
		t.Fatalf("duplicate path should not surface error, got %v", err)
	}
	if !res.UsedShortcut {
		t.Error("want UsedShortcut=true on duplicate hit")
	}
	if fs.paperID != 7 || fs.title != "Existing Paper" {
		t.Errorf("want current paper switched to existing 7, got %d/%q", fs.paperID, fs.title)
	}
	if len(res.ChunksText) != 1 ||
		!contains(res.ChunksText[0], "已在文献库") ||
		!contains(res.ChunksText[0], "#7") ||
		!contains(res.ChunksText[0], "Existing Paper") {
		t.Fatalf("want duplicate message, got %q", res.ChunksText)
	}
}

func TestDOIInvalidNormalizeErrors(t *testing.T) {
	imp := &fakeDOIImporter{normalizeErr: errors.New("bad")}
	fs := &fakeState{}
	cmd := &DOICommand{Importer: imp}
	if _, err := cmd.Execute(context.Background(), "garbage",
		RuntimeCtx{Surface: "wechat", State: fs}); err == nil {
		t.Fatal("expected error for invalid DOI")
	}
}

func TestDOIPropagatesImportError(t *testing.T) {
	imp := &fakeDOIImporter{
		normalized: "10.1000/xyz",
		importErr:  errors.New("download failed"),
	}
	fs := &fakeState{}
	cmd := &DOICommand{Importer: imp}
	if _, err := cmd.Execute(context.Background(), "10.1000/xyz",
		RuntimeCtx{Surface: "wechat", State: fs}); err == nil {
		t.Fatal("expected import error to propagate")
	}
}

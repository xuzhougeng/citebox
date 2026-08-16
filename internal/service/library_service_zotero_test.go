package service

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

func TestZoteroSettingsRejectRemoteURL(t *testing.T) {
	svc, _, _ := newTestService(t)
	if _, err := svc.UpdateZoteroSettings(model.ZoteroSettings{BaseURL: "http://example.com:23119/api"}); err == nil {
		t.Fatal("expected remote Zotero URL to be rejected")
	}
}

func TestZoteroImportDedupAndMissingPDF(t *testing.T) {
	svc, repo, _ := newTestService(t)
	pdfPath := filepath.Join(t.TempDir(), "atlas.pdf")
	if err := os.WriteFile(pdfPath, testPDFBytes(), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	server := newFakeZoteroServer(t, pdfPath)
	if _, err := svc.UpdateZoteroSettings(model.ZoteroSettings{
		BaseURL:         server.URL + "/api",
		IncludeChildren: true,
	}); err != nil {
		t.Fatalf("UpdateZoteroSettings() error = %v", err)
	}

	status, err := svc.GetZoteroStatus(context.Background())
	if err != nil || !status.Reachable {
		t.Fatalf("GetZoteroStatus() = %+v, err=%v", status, err)
	}
	collections, err := svc.ListZoteroCollections(context.Background())
	if err != nil || len(collections) != 1 || len(collections[0].Children) != 1 {
		t.Fatalf("ListZoteroCollections() = %+v, err=%v", collections, err)
	}

	preview, err := svc.PreviewZoteroImport(context.Background(), []string{"C1"}, true)
	if err != nil {
		t.Fatalf("PreviewZoteroImport() error = %v", err)
	}
	if preview.Summary.Total != 2 {
		t.Fatalf("preview total = %d, want 2: %+v", preview.Summary.Total, preview.Items)
	}

	run, err := svc.StartZoteroImport(context.Background(), []string{"C1"}, true)
	if err != nil {
		t.Fatalf("StartZoteroImport() error = %v", err)
	}
	run = waitForZoteroRun(t, svc, run.ID)
	if run.Summary.Imported != 1 || run.Summary.MissingPDF != 1 {
		t.Fatalf("run summary = %+v items=%+v", run.Summary, run.Items)
	}

	papers, _, err := repo.ListPapers(model.PaperFilter{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListPapers() error = %v", err)
	}
	if len(papers) != 1 {
		t.Fatalf("papers = %d, want 1", len(papers))
	}
	if papers[0].GroupName != "Lymph/T cell" {
		t.Fatalf("group = %q, want Lymph/T cell", papers[0].GroupName)
	}
	external, err := repo.ExternalID.GetBySourceKey(model.ExternalSourceZotero, "users/0", "P1")
	if err != nil || external == nil || external.PaperID != papers[0].ID {
		t.Fatalf("external id = %+v err=%v", external, err)
	}

	run2, err := svc.StartZoteroImport(context.Background(), []string{"C1"}, true)
	if err != nil {
		t.Fatalf("second StartZoteroImport() error = %v", err)
	}
	run2 = waitForZoteroRun(t, svc, run2.ID)
	if run2.Summary.SkippedExisting != 1 {
		t.Fatalf("second run summary = %+v", run2.Summary)
	}
	papers, _, err = repo.ListPapers(model.PaperFilter{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListPapers() after rerun error = %v", err)
	}
	if len(papers) != 1 {
		t.Fatalf("papers after rerun = %d, want 1", len(papers))
	}

	missing := findZoteroItem(run, "P2")
	if missing.ItemKey == "" || missing.HasLocalPDF {
		t.Fatalf("missing item = %+v", missing)
	}
	body, header := multipartPDF(t, "manual.pdf", testPDFBytesVariant(2))
	attached, err := svc.AttachZoteroImportPDF(run.ID, "P2", body, header)
	if err != nil {
		t.Fatalf("AttachZoteroImportPDF() error = %v", err)
	}
	if attached.Summary.Imported != 2 {
		t.Fatalf("attached summary = %+v", attached.Summary)
	}
	papers, _, err = repo.ListPapers(model.PaperFilter{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListPapers() after attach error = %v", err)
	}
	if len(papers) != 2 {
		t.Fatalf("papers after attach = %d, want 2", len(papers))
	}
}

func TestZoteroIngestBindsExistingDOI(t *testing.T) {
	svc, repo, _ := newTestService(t)
	existing, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Existing DOI",
		DOI:              "10.1000/test",
		OriginalFilename: "existing.pdf",
		StoredPDFName:    "existing.pdf",
		PDFSHA256:        "abc123",
		ExtractionStatus: "completed",
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}
	body, header := multipartPDF(t, "dup.pdf", append([]byte("%PDF-1.4\n"), []byte("different-bytes")...))
	paper, status, err := svc.IngestZoteroPaperFromReader(body, header, model.ZoteroIngestInput{
		ItemKey:        "ZX1",
		LibraryID:      "users/0",
		Title:          "Incoming",
		DOI:            "10.1000/test",
		CollectionPath: "Lymph",
	})
	if err != nil {
		t.Fatalf("IngestZoteroPaperFromReader() error = %v", err)
	}
	if status != zoteroItemSkippedExisting || paper.ID != existing.ID {
		t.Fatalf("ingest status=%s paper=%d want skipped existing %d", status, paper.ID, existing.ID)
	}
	external, err := repo.ExternalID.GetBySourceKey(model.ExternalSourceZotero, "users/0", "ZX1")
	if err != nil || external == nil || external.PaperID != existing.ID {
		t.Fatalf("external id = %+v err=%v", external, err)
	}
}

func TestZoteroImportItemByDOI(t *testing.T) {
	svc, repo, _ := newTestService(t)
	svc.config.OAContactEmail = "ops@example.com"
	pdfPath := filepath.Join(t.TempDir(), "unused.pdf")
	if err := os.WriteFile(pdfPath, testPDFBytes(), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	zoteroServer := newFakeZoteroServer(t, pdfPath)
	if _, err := svc.UpdateZoteroSettings(model.ZoteroSettings{BaseURL: zoteroServer.URL + "/api", IncludeChildren: true}); err != nil {
		t.Fatalf("UpdateZoteroSettings() error = %v", err)
	}

	var oaServer *httptest.Server
	oaServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/crossref/works/"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"message":{"title":["OA Missing"],"abstract":"<jats:p>abs</jats:p>","author":[{"given":"Ada","family":"Lovelace"}],"container-title":["Nature"],"published-online":{"date-parts":[[2024,1,1]]}}}`))
		case strings.HasPrefix(r.URL.Path, "/unpaywall/v2/"):
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"doi":"10.2000/missing","title":"OA Missing","best_oa_location":{"url_for_pdf":%q}}`, oaServer.URL+"/files/oa.pdf")
		case r.URL.Path == "/files/oa.pdf":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write(testPDFBytesVariant(3))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(oaServer.Close)
	originalUnpaywall := unpaywallAPIBaseURL
	originalEuropePMC := europePMCSearchURL
	originalPMCID := pmcIDConvURL
	originalCrossref := crossrefWorksAPIBaseURL
	unpaywallAPIBaseURL = oaServer.URL + "/unpaywall/v2/"
	europePMCSearchURL = oaServer.URL + "/europe-pmc/search"
	pmcIDConvURL = oaServer.URL + "/pmc/idconv"
	crossrefWorksAPIBaseURL = oaServer.URL + "/crossref/works/"
	t.Cleanup(func() {
		unpaywallAPIBaseURL = originalUnpaywall
		europePMCSearchURL = originalEuropePMC
		pmcIDConvURL = originalPMCID
		crossrefWorksAPIBaseURL = originalCrossref
	})

	run, err := svc.StartZoteroImport(context.Background(), []string{"C1"}, true)
	if err != nil {
		t.Fatalf("StartZoteroImport() error = %v", err)
	}
	run = waitForZoteroRun(t, svc, run.ID)
	updated, err := svc.ImportZoteroItemByDOI(context.Background(), run.ID, "P2")
	if err != nil {
		t.Fatalf("ImportZoteroItemByDOI() error = %v", err)
	}
	item := findZoteroItem(updated, "P2")
	if item.Status != zoteroItemImported || item.PaperID == 0 {
		t.Fatalf("doi item = %+v", item)
	}
	external, err := repo.ExternalID.GetBySourceKey(model.ExternalSourceZotero, "users/0", "P2")
	if err != nil || external == nil {
		t.Fatalf("external id missing: %+v %v", external, err)
	}
}

func TestZoteroSchemaCreatesExternalIDTable(t *testing.T) {
	_, repo, _ := newTestService(t)
	var count int
	if err := repo.DB().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='paper_external_ids'`).Scan(&count); err != nil {
		t.Fatalf("query table error = %v", err)
	}
	if count != 1 {
		t.Fatalf("paper_external_ids count = %d", count)
	}
}

func newFakeZoteroServer(t *testing.T, pdfPath string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/users/0/collections"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"key":"C1","data":{"key":"C1","name":"Lymph","parentCollection":false}},
				{"key":"C2","data":{"key":"C2","name":"T cell","parentCollection":"C1"}}
			]`))
		case strings.HasSuffix(r.URL.Path, "/users/0/items"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"key":"P1","data":{"key":"P1","itemType":"journalArticle","title":"Atlas","DOI":"10.1000/test","collections":["C2"],"creators":[{"creatorType":"author","firstName":"Ada","lastName":"Lovelace"}],"tags":[{"tag":"sc"}],"abstractNote":"abs","publicationTitle":"Nature"}},
				{"key":"A1","data":{"key":"A1","itemType":"attachment","parentItem":"P1","contentType":"application/pdf","filename":"atlas.pdf","linkMode":"imported_file"}},
				{"key":"N1","data":{"key":"N1","itemType":"note","parentItem":"P1","note":"<p>Zotero note</p>"}},
				{"key":"P2","data":{"key":"P2","itemType":"journalArticle","title":"No PDF","DOI":"10.2000/missing","collections":["C1"]}}
			]`))
		case strings.HasSuffix(r.URL.Path, "/users/0/items/A1/file/view/url"):
			_, _ = w.Write([]byte("file://" + pdfPath))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func waitForZoteroRun(t *testing.T, svc *LibraryService, id string) *model.ZoteroImportRun {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		run, err := svc.GetZoteroImportRun(id)
		if err != nil {
			t.Fatalf("GetZoteroImportRun() error = %v", err)
		}
		if run.Status == zoteroRunCompleted || run.Status == zoteroRunFailed {
			return run
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("import run %s did not finish", id)
	return nil
}

func findZoteroItem(run *model.ZoteroImportRun, key string) model.ZoteroImportItem {
	if run == nil {
		return model.ZoteroImportItem{}
	}
	for _, item := range run.Items {
		if item.ItemKey == key {
			return item
		}
	}
	return model.ZoteroImportItem{}
}

func testPDFBytesVariant(n byte) []byte {
	return append(testPDFBytes(), n)
}

func multipartPDF(t *testing.T, name string, content []byte) (multipart.File, *multipart.FileHeader) {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("pdf", name)
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/upload", bytes.NewReader(buf.Bytes()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if err := req.ParseMultipartForm(10 << 20); err != nil {
		t.Fatalf("ParseMultipartForm() error = %v", err)
	}
	file, header, err := req.FormFile("pdf")
	if err != nil {
		t.Fatalf("FormFile() error = %v", err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file, header
}

func TestZoteroHasRunningBlocksSecondImport(t *testing.T) {
	svc, repo, _ := newTestService(t)
	pdfPath := filepath.Join(t.TempDir(), "atlas.pdf")
	if err := os.WriteFile(pdfPath, testPDFBytes(), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	server := newFakeZoteroServer(t, pdfPath)
	if _, err := svc.UpdateZoteroSettings(model.ZoteroSettings{
		BaseURL:         server.URL + "/api",
		IncludeChildren: true,
	}); err != nil {
		t.Fatalf("UpdateZoteroSettings() error = %v", err)
	}

	if _, err := svc.PreviewZoteroImport(context.Background(), nil, true); err == nil || !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("empty preview error = %v", err)
	}

	if err := repo.ZoteroImport.Save(&model.ZoteroImportRun{
		ID:     "run-already-running",
		Status: zoteroRunRunning,
	}); err != nil {
		t.Fatalf("Save running import error = %v", err)
	}
	if _, err := svc.StartZoteroImport(context.Background(), []string{"C1"}, true); err == nil || !apperr.IsCode(err, apperr.CodeFailedPrecondition) {
		t.Fatalf("second StartZoteroImport() error = %v", err)
	}
}

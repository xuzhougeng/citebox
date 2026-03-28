package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/repository"
)

func TestImportPaperByDOIUsesUnpaywallAndPersistsDOI(t *testing.T) {
	svc, _, cfg := newTestService(t)
	svc.config.OAContactEmail = "ops@example.com"

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/crossref/works/"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"message":{"title":["Open Access Atlas"],"abstract":"<jats:p>Atlas abstract</jats:p>","author":[{"given":"Ada","family":"Lovelace"},{"given":"Alan","family":"Turing"}],"container-title":["Nature Communications"],"published-online":{"date-parts":[[2023,1,18]]}}}`))
		case strings.HasPrefix(r.URL.Path, "/unpaywall/v2/"):
			if r.URL.Query().Get("email") == "" {
				t.Fatalf("unpaywall email query missing")
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"doi":"10.1000/test-doi","title":"Open Access Atlas","best_oa_location":{"url_for_pdf":%q}}`, server.URL+"/files/test.pdf")
		case r.URL.Path == "/files/test.pdf":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write(testPDFBytes())
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	originalUnpaywall := unpaywallAPIBaseURL
	originalEuropePMC := europePMCSearchURL
	originalPMCID := pmcIDConvURL
	originalCrossref := crossrefWorksAPIBaseURL
	unpaywallAPIBaseURL = server.URL + "/unpaywall/v2/"
	europePMCSearchURL = server.URL + "/europe-pmc/search"
	pmcIDConvURL = server.URL + "/pmc/idconv"
	crossrefWorksAPIBaseURL = server.URL + "/crossref/works/"
	defer func() {
		unpaywallAPIBaseURL = originalUnpaywall
		europePMCSearchURL = originalEuropePMC
		pmcIDConvURL = originalPMCID
		crossrefWorksAPIBaseURL = originalCrossref
	}()

	paper, err := svc.ImportPaperByDOI(context.Background(), ImportPaperByDOIParams{
		DOI:            "https://doi.org/10.1000/TEST-DOI",
		Tags:           []string{"oa"},
		ExtractionMode: "manual",
	})
	if err != nil {
		t.Fatalf("ImportPaperByDOI() error = %v", err)
	}

	if paper.DOI != "10.1000/test-doi" {
		t.Fatalf("paper doi = %q, want %q", paper.DOI, "10.1000/test-doi")
	}
	if paper.Title != "Open Access Atlas" {
		t.Fatalf("paper title = %q, want %q", paper.Title, "Open Access Atlas")
	}
	if paper.AbstractText != "Atlas abstract" {
		t.Fatalf("paper abstract = %q, want %q", paper.AbstractText, "Atlas abstract")
	}
	if paper.AuthorsText != "Ada Lovelace, Alan Turing" {
		t.Fatalf("paper authors = %q, want %q", paper.AuthorsText, "Ada Lovelace, Alan Turing")
	}
	if paper.Journal != "Nature Communications" {
		t.Fatalf("paper journal = %q, want %q", paper.Journal, "Nature Communications")
	}
	if paper.PublishedAt != "2023-01-18" {
		t.Fatalf("paper published_at = %q, want %q", paper.PublishedAt, "2023-01-18")
	}
	if paper.OriginalFilename != "test.pdf" {
		t.Fatalf("paper original filename = %q, want %q", paper.OriginalFilename, "test.pdf")
	}
	if len(paper.Tags) != 1 || paper.Tags[0].Name != "oa" {
		t.Fatalf("paper tags = %+v, want one oa tag", paper.Tags)
	}
	if _, err := os.Stat(filepath.Join(cfg.PapersDir(), paper.StoredPDFName)); err != nil {
		t.Fatalf("stored pdf stat error = %v", err)
	}
}

func TestImportPaperByDOIFallsBackToEuropePMC(t *testing.T) {
	svc, _, _ := newTestService(t)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/europe-pmc/search":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"resultList":{"result":[{"doi":"10.2000/fallback","title":"Fallback Article","isOpenAccess":"Y","hasPDF":"Y","fullTextUrlList":{"fullTextUrl":[{"availabilityCode":"OA","documentStyle":"pdf","url":%q}]}}]}}`, server.URL+"/files/fallback.pdf")
		case r.URL.Path == "/files/fallback.pdf":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write(testPDFBytes())
		case r.URL.Path == "/pmc/idconv":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"records":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	originalUnpaywall := unpaywallAPIBaseURL
	originalEuropePMC := europePMCSearchURL
	originalPMCID := pmcIDConvURL
	originalCrossref := crossrefWorksAPIBaseURL
	unpaywallAPIBaseURL = server.URL + "/unpaywall/v2/"
	europePMCSearchURL = server.URL + "/europe-pmc/search"
	pmcIDConvURL = server.URL + "/pmc/idconv"
	crossrefWorksAPIBaseURL = server.URL + "/crossref/works/"
	defer func() {
		unpaywallAPIBaseURL = originalUnpaywall
		europePMCSearchURL = originalEuropePMC
		pmcIDConvURL = originalPMCID
		crossrefWorksAPIBaseURL = originalCrossref
	}()

	paper, err := svc.ImportPaperByDOI(context.Background(), ImportPaperByDOIParams{
		DOI:            "10.2000/FALLBACK",
		ExtractionMode: "manual",
	})
	if err != nil {
		t.Fatalf("ImportPaperByDOI() error = %v", err)
	}
	if paper.Title != "Fallback Article" {
		t.Fatalf("paper title = %q, want %q", paper.Title, "Fallback Article")
	}
	if paper.DOI != "10.2000/fallback" {
		t.Fatalf("paper doi = %q, want %q", paper.DOI, "10.2000/fallback")
	}
}

func TestImportPaperByDOIKeepsDownloadContextUntilBodyClose(t *testing.T) {
	svc, _, cfg := newTestService(t)
	svc.config.OAContactEmail = "ops@example.com"

	pdfBytes := testPDFBytes()
	splitAt := len(pdfBytes) / 2
	if splitAt < 1 {
		splitAt = 1
	}

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/unpaywall/v2/"):
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"doi":"10.1111/slow-stream","title":"Slow Stream Paper","best_oa_location":{"url_for_pdf":%q}}`, server.URL+"/files/slow.pdf")
		case r.URL.Path == "/files/slow.pdf":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write(pdfBytes[:splitAt])
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			time.Sleep(50 * time.Millisecond)
			_, _ = w.Write(pdfBytes[splitAt:])
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	originalUnpaywall := unpaywallAPIBaseURL
	originalEuropePMC := europePMCSearchURL
	originalPMCID := pmcIDConvURL
	originalCrossref := crossrefWorksAPIBaseURL
	unpaywallAPIBaseURL = server.URL + "/unpaywall/v2/"
	europePMCSearchURL = server.URL + "/europe-pmc/search"
	pmcIDConvURL = server.URL + "/pmc/idconv"
	crossrefWorksAPIBaseURL = server.URL + "/crossref/works/"
	defer func() {
		unpaywallAPIBaseURL = originalUnpaywall
		europePMCSearchURL = originalEuropePMC
		pmcIDConvURL = originalPMCID
		crossrefWorksAPIBaseURL = originalCrossref
	}()

	paper, err := svc.ImportPaperByDOI(context.Background(), ImportPaperByDOIParams{
		DOI:            "10.1111/slow-stream",
		ExtractionMode: "manual",
	})
	if err != nil {
		t.Fatalf("ImportPaperByDOI() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.PapersDir(), paper.StoredPDFName)); err != nil {
		t.Fatalf("stored pdf stat error = %v", err)
	}
}

func TestImportPaperByDOIReturnsNotFoundWhenNoOpenAccessPDF(t *testing.T) {
	svc, _, _ := newTestService(t)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/europe-pmc/search":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"resultList":{"result":[]}}`))
		case "/pmc/idconv":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"records":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	originalUnpaywall := unpaywallAPIBaseURL
	originalEuropePMC := europePMCSearchURL
	originalPMCID := pmcIDConvURL
	originalCrossref := crossrefWorksAPIBaseURL
	unpaywallAPIBaseURL = server.URL + "/unpaywall/v2/"
	europePMCSearchURL = server.URL + "/europe-pmc/search"
	pmcIDConvURL = server.URL + "/pmc/idconv"
	crossrefWorksAPIBaseURL = server.URL + "/crossref/works/"
	defer func() {
		unpaywallAPIBaseURL = originalUnpaywall
		europePMCSearchURL = originalEuropePMC
		pmcIDConvURL = originalPMCID
		crossrefWorksAPIBaseURL = originalCrossref
	}()

	_, err := svc.ImportPaperByDOI(context.Background(), ImportPaperByDOIParams{
		DOI:            "10.3000/missing",
		ExtractionMode: "manual",
	})
	if !apperr.IsCode(err, apperr.CodeNotFound) {
		t.Fatalf("ImportPaperByDOI() code = %q, want %q", apperr.CodeOf(err), apperr.CodeNotFound)
	}
}

func TestImportPaperByDOIReturnsDuplicateWhenDOIAlreadyExists(t *testing.T) {
	svc, repo, _ := newTestService(t)

	existing, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Existing DOI",
		DOI:              "10.4000/existing",
		OriginalFilename: "existing.pdf",
		StoredPDFName:    "existing.pdf",
		FileSize:         128,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	_, err = svc.ImportPaperByDOI(context.Background(), ImportPaperByDOIParams{
		DOI:            "10.4000/EXISTING",
		ExtractionMode: "manual",
	})
	var duplicateErr *DuplicatePaperError
	if !errors.As(err, &duplicateErr) {
		t.Fatalf("ImportPaperByDOI() error = %T %v, want DuplicatePaperError", err, err)
	}
	if duplicateErr.Paper == nil || duplicateErr.Paper.ID != existing.ID {
		t.Fatalf("duplicate paper = %+v, want id %d", duplicateErr.Paper, existing.ID)
	}
}

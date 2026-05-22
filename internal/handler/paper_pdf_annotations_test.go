package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/config"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service"
)

func newPaperHandlerForTest(t *testing.T) (*PaperHandler, *repository.LibraryRepository) {
	t.Helper()
	root := t.TempDir()
	cfg := &config.Config{
		StorageDir:              filepath.Join(root, "storage"),
		DatabasePath:            filepath.Join(root, "library.db"),
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
	t.Cleanup(func() { _ = repo.Close() })
	librarySvc, err := service.NewLibraryService(repo, cfg, service.WithoutBackgroundJobs())
	if err != nil {
		t.Fatalf("NewLibraryService() error = %v", err)
	}
	return NewPaperHandler(librarySvc), repo
}

func createHandlerTestPaper(t *testing.T, repo *repository.LibraryRepository) *model.Paper {
	t.Helper()
	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Handler Paper",
		OriginalFilename: "handler.pdf",
		StoredPDFName:    "handler.pdf",
		FileSize:         128,
		ContentType:      "application/pdf",
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}
	return paper
}

func TestPaperHandlerPDFAnnotationsCreateListDelete(t *testing.T) {
	h, repo := newPaperHandlerForTest(t)
	paper := createHandlerTestPaper(t, repo)
	paperID := strconv.FormatInt(paper.ID, 10)

	createReq := httptest.NewRequest(http.MethodPost, "/api/papers/"+paperID+"/pdf-annotations", bytes.NewBufferString(`{
		"type":"highlight",
		"quote_text":"selected text",
		"color":"yellow",
		"fragments":[{"page":3,"left":0.12,"top":0.34,"width":0.28,"height":0.018}]
	}`))
	createRec := httptest.NewRecorder()
	h.CreatePDFAnnotation(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("create status = %d body = %s", createRec.Code, createRec.Body.String())
	}
	var createdBody struct {
		Success    bool                `json:"success"`
		Annotation model.PDFAnnotation `json:"annotation"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createdBody); err != nil {
		t.Fatalf("Decode(create) error = %v", err)
	}
	if !createdBody.Success || createdBody.Annotation.ID == 0 || createdBody.Annotation.PaperID != paper.ID {
		t.Fatalf("created body = %+v", createdBody)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/papers/"+paperID+"/pdf-annotations", nil)
	listRec := httptest.NewRecorder()
	h.ListPDFAnnotations(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d body = %s", listRec.Code, listRec.Body.String())
	}
	var listBody struct {
		Success     bool                  `json:"success"`
		Annotations []model.PDFAnnotation `json:"annotations"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&listBody); err != nil {
		t.Fatalf("Decode(list) error = %v", err)
	}
	if len(listBody.Annotations) != 1 || listBody.Annotations[0].ID != createdBody.Annotation.ID {
		t.Fatalf("list body = %+v", listBody)
	}

	deletePath := "/api/papers/" + paperID + "/pdf-annotations/" + strconv.FormatInt(createdBody.Annotation.ID, 10)
	deleteReq := httptest.NewRequest(http.MethodDelete, deletePath, nil)
	deleteRec := httptest.NewRecorder()
	h.DeletePDFAnnotation(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete status = %d body = %s", deleteRec.Code, deleteRec.Body.String())
	}

	afterDelete, err := repo.PDFAnnotation.ListByPaperID(paper.ID)
	if err != nil {
		t.Fatalf("ListByPaperID() error = %v", err)
	}
	if len(afterDelete) != 0 {
		t.Fatalf("annotations after delete = %d, want 0", len(afterDelete))
	}
}

func TestPaperHandlerPDFAnnotationRejectsBadJSON(t *testing.T) {
	h, repo := newPaperHandlerForTest(t)
	paper := createHandlerTestPaper(t, repo)
	req := httptest.NewRequest(http.MethodPost, "/api/papers/"+strconv.FormatInt(paper.ID, 10)+"/pdf-annotations", strings.NewReader(`{`))
	rec := httptest.NewRecorder()

	h.CreatePDFAnnotation(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestPaperHandlerListPDFAnnotationsGlobal(t *testing.T) {
	h, repo := newPaperHandlerForTest(t)
	paper := createHandlerTestPaper(t, repo)
	if _, err := repo.PDFAnnotation.Create(paper.ID, repository.PDFAnnotationCreateInput{
		Type:      "highlight",
		QuoteText: "handler global text",
		Color:     "yellow",
		Fragments: []model.PDFAnnotationFragment{
			{Page: 6, Left: 0.12, Top: 0.34, Width: 0.28, Height: 0.018},
		},
	}); err != nil {
		t.Fatalf("Create annotation error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/pdf-annotations?query=global&page=1&page_size=25&sort=created_desc", nil)
	rec := httptest.NewRecorder()
	h.ListPDFAnnotationsGlobal(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Success     bool                              `json:"success"`
		Annotations []model.PDFAnnotationListItem     `json:"annotations"`
		Pagination  model.PDFAnnotationListPagination `json:"pagination"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if !body.Success || len(body.Annotations) != 1 {
		t.Fatalf("body = %+v", body)
	}
	if body.Annotations[0].PaperTitle != "Handler Paper" || body.Annotations[0].PaperPDFURL != "/files/papers/handler.pdf" {
		t.Fatalf("annotation paper fields = %+v", body.Annotations[0])
	}
	if body.Pagination.Page != 1 || body.Pagination.PageSize != 25 || body.Pagination.Total != 1 {
		t.Fatalf("pagination = %+v", body.Pagination)
	}
}

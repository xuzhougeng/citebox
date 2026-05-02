package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/service/research"
)

type stubResearchService struct {
	searchResult research.PaperList
	searchErr    error
	getResult    research.Paper
	getErr       error
}

func (s *stubResearchService) Search(ctx context.Context, q string, opts research.SearchOpts) (research.PaperList, error) {
	return s.searchResult, s.searchErr
}
func (s *stubResearchService) GetPaper(ctx context.Context, id string) (research.Paper, error) {
	return s.getResult, s.getErr
}
func (s *stubResearchService) References(ctx context.Context, id string, off, lim int) (research.PaperList, error) {
	return research.PaperList{}, nil
}
func (s *stubResearchService) Citations(ctx context.Context, id string, off, lim int, opts research.CitationOpts) (research.PaperList, error) {
	return research.PaperList{}, nil
}
func (s *stubResearchService) Recommendations(ctx context.Context, id string) ([]research.Paper, error) {
	return nil, nil
}
func (s *stubResearchService) RecommendationsForList(ctx context.Context, pos, neg []string) ([]research.Paper, error) {
	return nil, nil
}
func (s *stubResearchService) Autocomplete(ctx context.Context, q string) ([]research.AutocompleteItem, error) {
	return nil, nil
}
func (s *stubResearchService) SnippetSearch(ctx context.Context, q string, opts research.SnippetSearchOpts) (research.SnippetList, error) {
	return research.SnippetList{}, nil
}

func TestResearchHandlerSearch(t *testing.T) {
	stub := &stubResearchService{
		searchResult: research.PaperList{
			Items: []research.Paper{{PaperID: "p1", Title: "Hello"}},
			Total: 1,
		},
	}
	h := NewResearchHandler(stub, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/research/search?q=hello", nil)
	rec := httptest.NewRecorder()
	h.Search(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Items []research.Paper `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Total != 1 || len(body.Items) != 1 {
		t.Fatalf("body = %+v", body)
	}
}

func TestResearchHandlerSearchMissingQuery(t *testing.T) {
	stub := &stubResearchService{}
	h := NewResearchHandler(stub, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/research/search", nil)
	rec := httptest.NewRecorder()
	h.Search(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestResearchHandlerGetPaperNotFound(t *testing.T) {
	stub := &stubResearchService{getErr: research.ErrPaperNotFound}
	h := NewResearchHandler(stub, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/research/paper/DOI:nope", nil)
	rec := httptest.NewRecorder()
	h.GetPaper(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestResearchHandlerGetPaperRoute(t *testing.T) {
	stub := &stubResearchService{getResult: research.Paper{PaperID: "p1", Title: "Found"}}
	h := NewResearchHandler(stub, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/research/paper/DOI:10.1%2Fabc", nil)
	rec := httptest.NewRecorder()
	h.GetPaper(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"paperId":"p1"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestResearchHandlerSearchRateLimitedReportsAPIKeyUsage(t *testing.T) {
	stub := &stubResearchService{searchErr: &research.RateLimitedError{UsedAPIKey: true}}
	h := NewResearchHandler(stub, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/research/search?q=hello", nil)
	rec := httptest.NewRecorder()
	h.Search(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Code       string `json:"code"`
		Error      string `json:"error"`
		UsedAPIKey *bool  `json:"used_api_key"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Code == "" || body.Error == "" {
		t.Fatalf("missing error payload: %+v", body)
	}
	if body.UsedAPIKey == nil || !*body.UsedAPIKey {
		t.Fatalf("used_api_key = %+v, want true", body.UsedAPIKey)
	}
}

func TestResearchHandlerSearchRateLimitedReportsAnonymousUsage(t *testing.T) {
	stub := &stubResearchService{searchErr: &research.RateLimitedError{UsedAPIKey: false}}
	h := NewResearchHandler(stub, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/research/search?q=hello", nil)
	rec := httptest.NewRecorder()
	h.Search(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		UsedAPIKey *bool `json:"used_api_key"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.UsedAPIKey == nil || *body.UsedAPIKey {
		t.Fatalf("used_api_key = %+v, want false", body.UsedAPIKey)
	}
}

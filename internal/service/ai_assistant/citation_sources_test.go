package ai_assistant

import (
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/service/research"
	"strings"
	"testing"
)

type citationPaperStore struct {
	paper *model.Paper
	pages []string
}

func (s citationPaperStore) GetPaperDetail(int64) (*model.Paper, error)   { return s.paper, nil }
func (s citationPaperStore) GetPaperPDFPageTexts(int64) ([]string, error) { return s.pages, nil }

func TestCitationSourceCapturesUniquePageAndTextRevision(t *testing.T) {
	store := citationPaperStore{paper: &model.Paper{ID: 7, Title: "Study", StoredPDFName: "study.pdf", PDFText: "Introduction\nNegative result."}, pages: []string{"Introduction", "Negative result."}}
	cites := []Citation{{I: 1, PaperID: 7, Source: "local", Snippet: research.Snippet{Text: "Negative result.", SnippetKind: "body"}}}
	EnrichCitationSources(store, cites)
	if cites[0].Page == nil || *cites[0].Page != 2 || !strings.Contains(cites[0].SourceURL, "page=2") || !strings.HasPrefix(cites[0].SourceRevision, "sha256:") {
		t.Fatalf("source: %+v", cites)
	}
	store.pages = []string{"Negative result.", "Negative result."}
	cites[0].Page = nil
	EnrichCitationSources(store, cites)
	if cites[0].Page != nil {
		t.Fatal("ambiguous page guessed")
	}
}

func TestExternalEvidenceDoesNotInheritPinnedPaperRevision(t *testing.T) {
	cites := []Citation{{PaperID: 7, Source: "external:pubmed", SourceURL: "https://pubmed.ncbi.nlm.nih.gov/1/", Title: "External study"}}
	EnrichCitationSources(citationPaperStore{paper: &model.Paper{ID: 7, Title: "Local study"}}, cites)
	if cites[0].SourceRevision != "" || cites[0].Title != "External study" {
		t.Fatal("external source overwritten with local metadata")
	}
}

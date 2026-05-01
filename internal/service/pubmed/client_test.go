package pubmed

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestClientSearchHydratesPubMedArticle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/entrez/eutils/esearch.fcgi":
			if got := r.URL.Query().Get("term"); got != "cell fate" {
				http.Error(w, "unexpected term "+got, http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"esearchresult":{"idlist":["12345"]}}`))
		case "/entrez/eutils/efetch.fcgi":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(pubmedFetchXML))
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, MinInterval: 0})
	res, err := client.Search(context.Background(), "cell fate", SearchOptions{Limit: 5})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("len = %d, want 1", len(res.Items))
	}
	p := res.Items[0]
	if p.PMID != "12345" || p.Title != "Cell fate control" || p.DOI != "10.1/example" || p.PMCID != "PMC123" {
		t.Fatalf("paper = %+v", p)
	}
	if p.Year != 2024 || p.Journal != "Nature Medicine" {
		t.Fatalf("paper = %+v", p)
	}
	if !strings.Contains(p.Abstract, "cell fate decisions") {
		t.Fatalf("abstract = %q", p.Abstract)
	}
	if len(p.Authors) != 2 || p.Authors[0] != "Ada Lovelace" || p.Authors[1] != "Alan Turing" {
		t.Fatalf("authors = %#v", p.Authors)
	}
}

func TestClientSearchHydratesPubMedOnlineAndIssueYears(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/entrez/eutils/esearch.fcgi":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"esearchresult":{"idlist":["67890"]}}`))
		case "/entrez/eutils/efetch.fcgi":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0"?>
<PubmedArticleSet>
  <PubmedArticle>
    <MedlineCitation>
      <PMID>67890</PMID>
      <Article>
        <ArticleTitle>Dual year article</ArticleTitle>
        <Journal>
          <Title>Science</Title>
          <JournalIssue><PubDate><Year>2026</Year></PubDate></JournalIssue>
        </Journal>
        <ArticleDate DateType="Electronic">
          <Year>2025</Year>
          <Month>12</Month>
          <Day>31</Day>
        </ArticleDate>
      </Article>
    </MedlineCitation>
    <PubmedData>
      <ArticleIdList>
        <ArticleId IdType="doi">10.1/dual-year</ArticleId>
      </ArticleIdList>
    </PubmedData>
  </PubmedArticle>
</PubmedArticleSet>`))
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, MinInterval: 0})
	res, err := client.Search(context.Background(), "dual year", SearchOptions{Limit: 5})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("len = %d, want 1", len(res.Items))
	}
	p := res.Items[0]
	if p.OnlineYear != 2025 {
		t.Fatalf("OnlineYear = %d, want 2025", p.OnlineYear)
	}
	if p.IssueYear != 2026 {
		t.Fatalf("IssueYear = %d, want 2026", p.IssueYear)
	}
	if p.YearLabel != "2025 online / 2026 issue" {
		t.Fatalf("YearLabel = %q, want %q", p.YearLabel, "2025 online / 2026 issue")
	}
}

func TestClientSearchReturnsEmptyWithoutFetching(t *testing.T) {
	fetchCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/entrez/eutils/efetch.fcgi" {
			fetchCalled = true
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"esearchresult":{"idlist":[]}}`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, MinInterval: 0})
	res, err := client.Search(context.Background(), "nothing", SearchOptions{Limit: 5})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(res.Items) != 0 || fetchCalled {
		t.Fatalf("items = %#v fetchCalled = %v", res.Items, fetchCalled)
	}
}

func TestClientRetriesRateLimitOnce(t *testing.T) {
	calls := 0
	oldBackoff := retryBackoff
	retryBackoff = time.Millisecond
	t.Cleanup(func() { retryBackoff = oldBackoff })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			http.Error(w, "rate", http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"esearchresult":{"idlist":[]}}`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, MinInterval: 0})
	if _, err := client.Search(context.Background(), "retry", SearchOptions{Limit: 1}); err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestClientSetSettingsUpdatesManagedRateInterval(t *testing.T) {
	client := NewClient(Config{APIKey: "", MinInterval: RateInterval("")})
	defer client.Close()

	if got := client.currentRateInterval(); got != RateInterval("") {
		t.Fatalf("initial interval = %v, want %v", got, RateInterval(""))
	}

	client.SetSettings("key", "email@example.org", "tool")

	if got := client.currentRateInterval(); got != RateInterval("key") {
		t.Fatalf("interval with key = %v, want %v", got, RateInterval("key"))
	}

	client.SetSettings("", "", "")

	if got := client.currentRateInterval(); got != RateInterval("") {
		t.Fatalf("interval without key = %v, want %v", got, RateInterval(""))
	}
}

func TestClientSetSettingsConcurrentWithSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"esearchresult":{"idlist":[]}}`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, MinInterval: 0})
	errs := make(chan error, 8)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 250; i++ {
			client.SetSettings("key", "email@example.com", "tool")
			client.SetSettings("", "", "")
		}
	}()

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				if _, err := client.Search(context.Background(), "race", SearchOptions{Limit: 1}); err != nil {
					errs <- err
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("Search() error = %v", err)
	}
}

const pubmedFetchXML = `<?xml version="1.0"?>
<PubmedArticleSet>
  <PubmedArticle>
    <MedlineCitation>
      <PMID>12345</PMID>
      <Article>
        <ArticleTitle>Cell fate control</ArticleTitle>
        <Journal>
          <Title>Nature Medicine</Title>
          <JournalIssue><PubDate><Year>2024</Year></PubDate></JournalIssue>
        </Journal>
        <Abstract><AbstractText>Signals control cell fate decisions.</AbstractText></Abstract>
        <AuthorList>
          <Author><LastName>Lovelace</LastName><ForeName>Ada</ForeName></Author>
          <Author><LastName>Turing</LastName><ForeName>Alan</ForeName></Author>
        </AuthorList>
        <ELocationID EIdType="doi">10.1/example</ELocationID>
      </Article>
    </MedlineCitation>
    <PubmedData>
      <ArticleIdList>
        <ArticleId IdType="doi">10.1/example</ArticleId>
        <ArticleId IdType="pmc">PMC123</ArticleId>
      </ArticleIdList>
    </PubmedData>
  </PubmedArticle>
</PubmedArticleSet>`

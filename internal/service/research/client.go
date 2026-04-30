package research

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrPaperNotFound is returned by Get / References / Citations when S2 returns 404.
var ErrPaperNotFound = errors.New("research: paper not found")

// ErrRateLimited is returned when S2 returns 429. doJSON retries once before
// surfacing this error, so callers should treat it as a hard failure.
var ErrRateLimited = errors.New("research: rate limited")

// paperBatchChunk is the upper bound on ids per /paper/batch call (S2 docs say ≤500).
const paperBatchChunk = 500

// Config configures the Semantic Scholar client.
type Config struct {
	BaseURL     string        // default https://api.semanticscholar.org
	APIKey      string        // optional; sent as x-api-key
	HTTPClient  *http.Client  // optional override
	MinInterval time.Duration // minimum delay between consecutive requests
}

// Client wraps the Semantic Scholar Graph API. All methods are safe for
// concurrent use; an internal ticker enforces MinInterval globally.
type Client struct {
	baseURL     string
	apiKeyMu    sync.RWMutex
	apiKey      string
	httpClient  *http.Client
	tokens      chan struct{} // ticker buffer; nil if rate limiting disabled
	closeTicker chan struct{}
}

// NewClient constructs a Client. If MinInterval > 0, a goroutine drips tokens.
func NewClient(cfg Config) *Client {
	c := &Client{
		baseURL:    strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		apiKey:     strings.TrimSpace(cfg.APIKey),
		httpClient: cfg.HTTPClient,
	}
	if c.baseURL == "" {
		c.baseURL = "https://api.semanticscholar.org"
	}
	if c.httpClient == nil {
		c.httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	if cfg.MinInterval > 0 {
		c.tokens = make(chan struct{}, 1)
		c.closeTicker = make(chan struct{})
		go c.refillTokens(cfg.MinInterval)
	}
	return c
}

// Close releases the rate-limit ticker goroutine.
func (c *Client) Close() {
	if c.closeTicker != nil {
		close(c.closeTicker)
	}
}

// SetAPIKey updates the API key used by subsequent requests. Safe for
// concurrent use. Pass "" to revert to anonymous access.
func (c *Client) SetAPIKey(key string) {
	c.apiKeyMu.Lock()
	c.apiKey = strings.TrimSpace(key)
	c.apiKeyMu.Unlock()
}

// currentAPIKey returns the active API key under the read lock.
func (c *Client) currentAPIKey() string {
	c.apiKeyMu.RLock()
	defer c.apiKeyMu.RUnlock()
	return c.apiKey
}

func (c *Client) refillTokens(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	// seed one token immediately so first call doesn't wait
	select {
	case c.tokens <- struct{}{}:
	default:
	}
	for {
		select {
		case <-c.closeTicker:
			return
		case <-ticker.C:
			select {
			case c.tokens <- struct{}{}:
			default:
			}
		}
	}
}

func (c *Client) takeToken(ctx context.Context) error {
	if c.tokens == nil {
		return nil
	}
	select {
	case <-c.tokens:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// doJSON performs a GET request, retrying once on a 429 response. The retry
// re-acquires a token first, so when MinInterval > 0 the second attempt is
// naturally spaced.
func (c *Client) doJSON(ctx context.Context, path string, query url.Values, out interface{}) error {
	err := c.doJSONOnce(ctx, path, query, out)
	if errors.Is(err, ErrRateLimited) {
		err = c.doJSONOnce(ctx, path, query, out)
	}
	return err
}

func (c *Client) doJSONOnce(ctx context.Context, path string, query url.Values, out interface{}) error {
	if err := c.takeToken(ctx); err != nil {
		return err
	}
	full := c.baseURL + path
	if len(query) > 0 {
		full += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return err
	}
	if k := c.currentAPIKey(); k != "" {
		req.Header.Set("x-api-key", k)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return json.NewDecoder(resp.Body).Decode(out)
	case http.StatusNotFound:
		return ErrPaperNotFound
	case http.StatusTooManyRequests:
		return ErrRateLimited
	default:
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("research: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
}

// rawSearchResponse maps the S2 search response shape.
type rawSearchResponse struct {
	Total  int        `json:"total"`
	Offset int        `json:"offset"`
	Next   int        `json:"next"`
	Data   []rawPaper `json:"data"`
}

type rawPaper struct {
	PaperID                  string         `json:"paperId"`
	ExternalIDs              map[string]any `json:"externalIds"` // S2 mixes strings and numbers (e.g. CorpusId, PubMed)
	Title                    string         `json:"title"`
	Abstract                 string         `json:"abstract"`
	Year                     int            `json:"year"`
	Venue                    string         `json:"venue"`
	Authors                  []Author       `json:"authors"`
	CitationCount            int            `json:"citationCount"`
	ReferenceCount           int            `json:"referenceCount"`
	InfluentialCitationCount int            `json:"influentialCitationCount"`
	OpenAccessPDF            *struct {
		URL string `json:"url"`
	} `json:"openAccessPdf"`
	TLDR *struct {
		Text string `json:"text"`
	} `json:"tldr"`
	FieldsOfStudy []string `json:"fieldsOfStudy"`
}

// externalIDString coerces a value from S2's externalIds map (which may be a
// string or number depending on the source ID) into a string.
func externalIDString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		// Use %v so integers don't get a ".000000" suffix from %f.
		return strconv.FormatFloat(x, 'f', -1, 64)
	case json.Number:
		return x.String()
	default:
		return ""
	}
}

func (rp rawPaper) toPaper() Paper {
	out := Paper{
		PaperID:          rp.PaperID,
		Title:            rp.Title,
		Abstract:         rp.Abstract,
		Year:             rp.Year,
		Venue:            rp.Venue,
		Authors:          rp.Authors,
		CitationCount:    rp.CitationCount,
		ReferenceCount:   rp.ReferenceCount,
		InfluentialCount: rp.InfluentialCitationCount,
		FieldsOfStudy:    rp.FieldsOfStudy,
	}
	if rp.OpenAccessPDF != nil {
		out.OpenAccessPDFURL = rp.OpenAccessPDF.URL
	}
	if rp.TLDR != nil {
		out.TLDR = rp.TLDR.Text
	}
	if v, ok := rp.ExternalIDs["DOI"]; ok {
		out.ExternalIDs.DOI = externalIDString(v)
	}
	if v, ok := rp.ExternalIDs["ArXiv"]; ok {
		out.ExternalIDs.ArXiv = externalIDString(v)
	}
	if v, ok := rp.ExternalIDs["PubMed"]; ok {
		out.ExternalIDs.PubMed = externalIDString(v)
	}
	return out
}

// defaultPaperFields is the field selection used by Get/Search.
const defaultPaperFields = "paperId,externalIds,title,abstract,year,venue,authors,citationCount,referenceCount,influentialCitationCount,openAccessPdf,tldr,fieldsOfStudy"

// paperFieldsWithoutTLDR is used for references/citations and recommendations.
// Semantic Scholar accepts tldr on direct Graph paper responses, but rejects it
// on edge-list and recommendation endpoints.
const paperFieldsWithoutTLDR = "paperId,externalIds,title,abstract,year,venue,authors,citationCount,referenceCount,influentialCitationCount,openAccessPdf,fieldsOfStudy"

// Search executes a paper/search query.
func (c *Client) Search(ctx context.Context, query string, opts SearchOpts) (PaperList, error) {
	q := url.Values{}
	q.Set("query", query)
	q.Set("fields", defaultPaperFields)
	if opts.Year != "" {
		q.Set("year", opts.Year)
	}
	if opts.FieldsOfStudy != "" {
		q.Set("fieldsOfStudy", opts.FieldsOfStudy)
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Offset > 0 {
		q.Set("offset", strconv.Itoa(opts.Offset))
	}

	var raw rawSearchResponse
	if err := c.doJSON(ctx, "/graph/v1/paper/search", q, &raw); err != nil {
		return PaperList{}, err
	}
	out := PaperList{Items: make([]Paper, 0, len(raw.Data)), Offset: raw.Offset, Next: raw.Next, Total: raw.Total}
	for _, rp := range raw.Data {
		out.Items = append(out.Items, rp.toPaper())
	}
	return out, nil
}

// Get fetches a single paper by S2-supported ID (DOI:..., ArXiv:..., paperId, etc).
// fields is optional; pass nil for the default selection.
func (c *Client) Get(ctx context.Context, id string, fields []string) (Paper, error) {
	q := url.Values{}
	if len(fields) > 0 {
		q.Set("fields", strings.Join(fields, ","))
	} else {
		q.Set("fields", defaultPaperFields)
	}
	var rp rawPaper
	path := "/graph/v1/paper/" + url.PathEscape(id)
	if err := c.doJSON(ctx, path, q, &rp); err != nil {
		return Paper{}, err
	}
	return rp.toPaper(), nil
}

type rawListResponse struct {
	Offset int       `json:"offset"`
	Next   int       `json:"next"`
	Data   []rawEdge `json:"data"`
}

// rawEdge is a row in references / citations responses; one of citedPaper /
// citingPaper is populated. We unify into a Paper.
type rawEdge struct {
	CitedPaper    *rawPaper `json:"citedPaper,omitempty"`
	CitingPaper   *rawPaper `json:"citingPaper,omitempty"`
	IsInfluential bool      `json:"isInfluential"`
	Intents       []string  `json:"intents"`
}

func (e rawEdge) paper() (Paper, bool) {
	switch {
	case e.CitedPaper != nil:
		return e.CitedPaper.toPaper(), e.IsInfluential
	case e.CitingPaper != nil:
		return e.CitingPaper.toPaper(), e.IsInfluential
	}
	return Paper{}, false
}

// References returns papers that the given paper cites.
func (c *Client) References(ctx context.Context, paperID string, offset, limit int) (PaperList, error) {
	return c.fetchPaperList(ctx, "/graph/v1/paper/"+url.PathEscape(paperID)+"/references", "citedPaper", offset, limit, false)
}

// Citations returns papers that cite the given paper. If opts.InfluentialOnly,
// non-influential edges are dropped *after* fetch (S2 has no server-side filter).
func (c *Client) Citations(ctx context.Context, paperID string, offset, limit int, opts CitationOpts) (PaperList, error) {
	return c.fetchPaperList(ctx, "/graph/v1/paper/"+url.PathEscape(paperID)+"/citations", "citingPaper", offset, limit, opts.InfluentialOnly)
}

func (c *Client) fetchPaperList(ctx context.Context, path, paperPrefix string, offset, limit int, influentialOnly bool) (PaperList, error) {
	q := url.Values{}
	q.Set("fields", "isInfluential,intents,"+prefixPaperFields(paperFieldsWithoutTLDR, paperPrefix))
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	var raw rawListResponse
	if err := c.doJSON(ctx, path, q, &raw); err != nil {
		return PaperList{}, err
	}
	out := PaperList{Offset: raw.Offset, Next: raw.Next, Items: make([]Paper, 0, len(raw.Data))}
	for _, edge := range raw.Data {
		paper, influential := edge.paper()
		if paper.PaperID == "" {
			continue
		}
		if influentialOnly && !influential {
			continue
		}
		out.Items = append(out.Items, paper)
	}
	return out, nil
}

// RateInterval returns the minimum delay between requests appropriate for the
// given API key state. With a key, allow ~5 req/s; without, fall back to ~1.
func RateInterval(apiKey string) time.Duration {
	if strings.TrimSpace(apiKey) != "" {
		return 200 * time.Millisecond
	}
	return time.Second
}

// prefixPaperFields prefixes every comma-delimited field with the given edge
// paper object name. The S2 API expects endpoint-specific fields, e.g.
// `citedPaper.title` for references and `citingPaper.title` for citations.
func prefixPaperFields(fields, prefix string) string {
	parts := strings.Split(fields, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, prefix+"."+p)
	}
	return strings.Join(out, ",")
}

// recommendationsResponse maps both recommendation endpoints (they share shape).
type recommendationsResponse struct {
	RecommendedPapers []rawPaper `json:"recommendedPapers"`
}

// Recommendations returns S2 recommendations for a single seed paper.
func (c *Client) Recommendations(ctx context.Context, paperID string) ([]Paper, error) {
	q := url.Values{}
	q.Set("fields", paperFieldsWithoutTLDR)
	var raw recommendationsResponse
	if err := c.doJSON(ctx, "/recommendations/v1/papers/forpaper/"+url.PathEscape(paperID), q, &raw); err != nil {
		return nil, err
	}
	out := make([]Paper, 0, len(raw.RecommendedPapers))
	for _, rp := range raw.RecommendedPapers {
		out = append(out, rp.toPaper())
	}
	return out, nil
}

// RecommendationsForList POSTs a positive/negative list of paper IDs and
// returns recommendations.
func (c *Client) RecommendationsForList(ctx context.Context, positive, negative []string) ([]Paper, error) {
	if err := c.takeToken(ctx); err != nil {
		return nil, err
	}
	body := map[string][]string{
		"positivePaperIds": positive,
		"negativePaperIds": negative,
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("fields", paperFieldsWithoutTLDR)
	full := c.baseURL + "/recommendations/v1/papers?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, full, strings.NewReader(string(buf)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if k := c.currentAPIKey(); k != "" {
		req.Header.Set("x-api-key", k)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrPaperNotFound
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, ErrRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("research: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var raw recommendationsResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := make([]Paper, 0, len(raw.RecommendedPapers))
	for _, rp := range raw.RecommendedPapers {
		out = append(out, rp.toPaper())
	}
	return out, nil
}

// GetBatch fetches metadata for up to paperBatchChunk papers per call. The
// caller may pass more ids; the slice is split into chunks of paperBatchChunk
// transparently. Missing ids are returned as zero-valued Paper entries with an
// empty PaperID, in the same position as the input slice. The fields argument
// is optional — pass nil for the default selection.
func (c *Client) GetBatch(ctx context.Context, ids []string, fields []string) ([]Paper, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	fieldsParam := defaultPaperFields
	if len(fields) > 0 {
		fieldsParam = strings.Join(fields, ",")
	}
	out := make([]Paper, 0, len(ids))
	for start := 0; start < len(ids); start += paperBatchChunk {
		end := start + paperBatchChunk
		if end > len(ids) {
			end = len(ids)
		}
		chunk, err := c.postPaperBatch(ctx, ids[start:end], fieldsParam)
		if err != nil {
			return nil, err
		}
		out = append(out, chunk...)
	}
	return out, nil
}

func (c *Client) postPaperBatch(ctx context.Context, ids []string, fields string) ([]Paper, error) {
	if err := c.takeToken(ctx); err != nil {
		return nil, err
	}
	body, err := json.Marshal(map[string][]string{"ids": ids})
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("fields", fields)
	full := c.baseURL + "/graph/v1/paper/batch?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, full, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if k := c.currentAPIKey(); k != "" {
		req.Header.Set("x-api-key", k)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusTooManyRequests:
		return nil, ErrRateLimited
	default:
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("research: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	// Response is a JSON array; missing ids appear as null entries.
	var rows []*rawPaper
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, err
	}
	out := make([]Paper, len(rows))
	for i, rp := range rows {
		if rp == nil {
			continue
		}
		out[i] = rp.toPaper()
	}
	return out, nil
}

// Autocomplete returns paper-title suggestions for an in-progress query.
// S2 silently truncates query to 100 characters.
func (c *Client) Autocomplete(ctx context.Context, query string) ([]AutocompleteItem, error) {
	q := url.Values{}
	q.Set("query", query)
	var raw struct {
		Matches []struct {
			ID          string `json:"id"`
			Title       string `json:"title"`
			AuthorsYear string `json:"authorsYear"`
		} `json:"matches"`
	}
	if err := c.doJSON(ctx, "/graph/v1/paper/autocomplete", q, &raw); err != nil {
		return nil, err
	}
	out := make([]AutocompleteItem, 0, len(raw.Matches))
	for _, m := range raw.Matches {
		out = append(out, AutocompleteItem{PaperID: m.ID, Title: m.Title, AuthorsYear: m.AuthorsYear})
	}
	return out, nil
}

// rawSnippetResponse maps /snippet/search; matches the swagger SnippetMatch wrapper.
type rawSnippetResponse struct {
	Data []struct {
		Paper   rawPaper `json:"paper"`
		Snippet Snippet  `json:"snippet"`
		Score   float64  `json:"score"`
	} `json:"data"`
	RetrievalVersion string `json:"retrievalVersion"`
}

// SnippetSearch returns ~500-word excerpts that best match the query, optionally
// constrained to a small set of papers (≤100 ids).
func (c *Client) SnippetSearch(ctx context.Context, query string, opts SnippetSearchOpts) (SnippetList, error) {
	q := url.Values{}
	q.Set("query", query)
	if len(opts.PaperIDs) > 0 {
		q.Set("paperIds", strings.Join(opts.PaperIDs, ","))
	}
	if opts.Year != "" {
		q.Set("year", opts.Year)
	}
	if opts.Venue != "" {
		q.Set("venue", opts.Venue)
	}
	if opts.FieldsOfStudy != "" {
		q.Set("fieldsOfStudy", opts.FieldsOfStudy)
	}
	if opts.MinCitationCount > 0 {
		q.Set("minCitationCount", strconv.Itoa(opts.MinCitationCount))
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	q.Set("fields", "snippet,paper.title,paper.year,paper.authors,paper.externalIds,paper.openAccessPdf")

	var raw rawSnippetResponse
	if err := c.doJSON(ctx, "/graph/v1/snippet/search", q, &raw); err != nil {
		return SnippetList{}, err
	}
	out := SnippetList{
		Matches:          make([]SnippetMatch, 0, len(raw.Data)),
		RetrievalVersion: raw.RetrievalVersion,
	}
	for _, row := range raw.Data {
		paper := row.Paper.toPaper()
		out.Matches = append(out.Matches, SnippetMatch{
			PaperID: paper.PaperID,
			Paper:   paper,
			Snippet: row.Snippet,
			Score:   row.Score,
		})
	}
	return out, nil
}

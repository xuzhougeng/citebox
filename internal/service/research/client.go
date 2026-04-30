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

// ErrRateLimited is returned when S2 returns 429 even after a single retry.
var ErrRateLimited = errors.New("research: rate limited")

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

// doJSON performs the request, decodes JSON into out, and maps known errors.
func (c *Client) doJSON(ctx context.Context, path string, query url.Values, out interface{}) error {
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
const defaultPaperFields = "paperId,externalIds,title,abstract,year,venue,authors,citationCount,influentialCitationCount,openAccessPdf,tldr,fieldsOfStudy"

// paperFieldsWithoutTLDR is used for references/citations and recommendations.
// Semantic Scholar accepts tldr on direct Graph paper responses, but rejects it
// on edge-list and recommendation endpoints.
const paperFieldsWithoutTLDR = "paperId,externalIds,title,abstract,year,venue,authors,citationCount,influentialCitationCount,openAccessPdf,fieldsOfStudy"

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

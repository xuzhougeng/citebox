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
	if c.apiKey != "" {
		req.Header.Set("x-api-key", c.apiKey)
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
	PaperID                  string            `json:"paperId"`
	ExternalIDs              map[string]string `json:"externalIds"`
	Title                    string            `json:"title"`
	Abstract                 string            `json:"abstract"`
	Year                     int               `json:"year"`
	Venue                    string            `json:"venue"`
	Authors                  []Author          `json:"authors"`
	CitationCount            int               `json:"citationCount"`
	InfluentialCitationCount int               `json:"influentialCitationCount"`
	OpenAccessPDF            *struct {
		URL string `json:"url"`
	} `json:"openAccessPdf"`
	TLDR *struct {
		Text string `json:"text"`
	} `json:"tldr"`
	FieldsOfStudy []string `json:"fieldsOfStudy"`
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
		out.ExternalIDs.DOI = v
	}
	if v, ok := rp.ExternalIDs["ArXiv"]; ok {
		out.ExternalIDs.ArXiv = v
	}
	if v, ok := rp.ExternalIDs["PubMed"]; ok {
		out.ExternalIDs.PubMed = v
	}
	return out
}

// defaultPaperFields is the field selection used by Get/Search/References/Citations.
const defaultPaperFields = "paperId,externalIds,title,abstract,year,venue,authors,citationCount,influentialCitationCount,openAccessPdf,tldr,fieldsOfStudy"

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

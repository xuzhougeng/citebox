package pubmed

import (
	"context"
	"encoding/json"
	"encoding/xml"
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

const (
	defaultBaseURL = "https://eutils.ncbi.nlm.nih.gov"
	defaultTool    = "citebox"
	defaultLimit   = 20
	maxLimit       = 100
)

var (
	ErrRateLimited = errors.New("pubmed rate limited")
	retryBackoff   = 500 * time.Millisecond
)

type Config struct {
	BaseURL     string
	APIKey      string
	Email       string
	Tool        string
	HTTPClient  *http.Client
	MinInterval time.Duration
}

type Client struct {
	baseURL     string
	settingsMu  sync.RWMutex
	apiKey      string
	email       string
	tool        string
	httpClient  *http.Client
	rateMu      sync.RWMutex
	ticker      *time.Ticker
	tickerDone  chan struct{}
	minInterval time.Duration
	autoRate    bool
}

type settingsSnapshot struct {
	apiKey string
	email  string
	tool   string
}

func NewClient(cfg Config) *Client {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	tool := strings.TrimSpace(cfg.Tool)
	if tool == "" {
		tool = defaultTool
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	c := &Client{
		baseURL:    baseURL,
		apiKey:     strings.TrimSpace(cfg.APIKey),
		email:      strings.TrimSpace(cfg.Email),
		tool:       tool,
		httpClient: httpClient,
	}
	c.autoRate = cfg.MinInterval == RateInterval(cfg.APIKey)
	c.setRateInterval(cfg.MinInterval)
	return c
}

func RateInterval(apiKey string) time.Duration {
	if strings.TrimSpace(apiKey) != "" {
		return 100 * time.Millisecond
	}
	return 350 * time.Millisecond
}

func (c *Client) Close() {
	c.setRateInterval(0)
}

func (c *Client) SetSettings(apiKey, email, tool string) {
	apiKey = strings.TrimSpace(apiKey)
	c.settingsMu.Lock()
	c.apiKey = apiKey
	c.email = strings.TrimSpace(email)
	c.tool = strings.TrimSpace(tool)
	if c.tool == "" {
		c.tool = defaultTool
	}
	c.settingsMu.Unlock()

	if c.autoRate {
		c.SetRateInterval(RateInterval(apiKey))
	}
}

func (c *Client) currentSettings() settingsSnapshot {
	c.settingsMu.RLock()
	defer c.settingsMu.RUnlock()

	return settingsSnapshot{
		apiKey: c.apiKey,
		email:  c.email,
		tool:   c.tool,
	}
}

func (c *Client) Search(ctx context.Context, query string, opts SearchOptions) (SearchResult, error) {
	ids, err := c.esearch(ctx, query, opts)
	if err != nil {
		return SearchResult{}, err
	}
	if len(ids) == 0 {
		return SearchResult{}, nil
	}

	items, err := c.efetch(ctx, ids)
	if err != nil {
		return SearchResult{}, err
	}
	return SearchResult{Items: items}, nil
}

func (c *Client) esearch(ctx context.Context, query string, opts SearchOptions) ([]string, error) {
	values := url.Values{}
	values.Set("db", "pubmed")
	values.Set("retmode", "json")
	values.Set("term", query)
	values.Set("retmax", strconv.Itoa(clampLimit(opts.Limit)))

	var res esearchResponse
	if err := c.do(ctx, "/entrez/eutils/esearch.fcgi", values, &res); err != nil {
		return nil, err
	}
	return res.ESearchResult.IDList, nil
}

func (c *Client) efetch(ctx context.Context, ids []string) ([]Paper, error) {
	values := url.Values{}
	values.Set("db", "pubmed")
	values.Set("retmode", "xml")
	values.Set("id", strings.Join(ids, ","))

	var res pubmedArticleSet
	if err := c.do(ctx, "/entrez/eutils/efetch.fcgi", values, &res); err != nil {
		return nil, err
	}
	return res.papers(), nil
}

func (c *Client) do(ctx context.Context, path string, values url.Values, dst any) error {
	if err := c.doOnce(ctx, path, values, dst); err != nil {
		if !errors.Is(err, ErrRateLimited) {
			return err
		}
		timer := time.NewTimer(retryBackoff)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
		return c.doOnce(ctx, path, values, dst)
	}
	return nil
}

func (c *Client) doOnce(ctx context.Context, path string, values url.Values, dst any) error {
	if err := c.takeToken(ctx); err != nil {
		return err
	}

	c.addConfiguredParams(values)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path+"?"+values.Encode(), nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return ErrRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("pubmed request failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	switch v := dst.(type) {
	case *esearchResponse:
		return json.NewDecoder(resp.Body).Decode(v)
	case *pubmedArticleSet:
		return xml.NewDecoder(resp.Body).Decode(v)
	default:
		return fmt.Errorf("unsupported pubmed decode target %T", dst)
	}
}

func (c *Client) SetRateInterval(interval time.Duration) {
	c.setRateInterval(interval)
}

func (c *Client) setRateInterval(interval time.Duration) {
	c.rateMu.Lock()
	defer c.rateMu.Unlock()

	if c.ticker != nil {
		c.ticker.Stop()
	}
	if c.tickerDone != nil {
		close(c.tickerDone)
	}
	c.ticker = nil
	c.tickerDone = nil
	c.minInterval = interval

	if interval <= 0 {
		return
	}
	c.ticker = time.NewTicker(interval)
	c.tickerDone = make(chan struct{})
}

func (c *Client) currentRateInterval() time.Duration {
	c.rateMu.RLock()
	defer c.rateMu.RUnlock()
	return c.minInterval
}

func (c *Client) takeToken(ctx context.Context) error {
	for {
		c.rateMu.RLock()
		ticker := c.ticker
		done := c.tickerDone
		c.rateMu.RUnlock()
		if ticker == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-done:
		case <-ticker.C:
			return nil
		}
	}
}

func (c *Client) addConfiguredParams(values url.Values) {
	settings := c.currentSettings()
	if settings.apiKey != "" {
		values.Set("api_key", settings.apiKey)
	}
	if settings.email != "" {
		values.Set("email", settings.email)
	}
	if settings.tool != "" {
		values.Set("tool", settings.tool)
	}
}

func clampLimit(limit int) int {
	if limit <= 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

func normalizeSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

type esearchResponse struct {
	ESearchResult struct {
		IDList []string `json:"idlist"`
	} `json:"esearchresult"`
}

type pubmedArticleSet struct {
	Articles []pubmedArticle `xml:"PubmedArticle"`
}

func (s pubmedArticleSet) papers() []Paper {
	papers := make([]Paper, 0, len(s.Articles))
	for _, article := range s.Articles {
		p := Paper{
			PMID:     normalizeSpace(article.MedlineCitation.PMID),
			Title:    normalizeSpace(article.MedlineCitation.Article.Title),
			Abstract: normalizeSpace(strings.Join(article.MedlineCitation.Article.Abstract.Texts, " ")),
			Journal:  normalizeSpace(article.MedlineCitation.Article.Journal.Title),
			Year:     article.MedlineCitation.Article.Journal.Year(),
			Authors:  article.MedlineCitation.Article.Authors.names(),
		}
		p.DOI = normalizeSpace(article.PubmedData.ArticleIDs.find("doi"))
		if p.DOI == "" {
			p.DOI = normalizeSpace(article.MedlineCitation.Article.ELocationIDs.find("doi"))
		}
		p.PMCID = normalizeSpace(article.PubmedData.ArticleIDs.find("pmc"))
		if p.PMID != "" {
			p.URL = "https://pubmed.ncbi.nlm.nih.gov/" + p.PMID + "/"
		}
		papers = append(papers, p)
	}
	return papers
}

type pubmedArticle struct {
	MedlineCitation medlineCitation `xml:"MedlineCitation"`
	PubmedData      pubmedData      `xml:"PubmedData"`
}

type medlineCitation struct {
	PMID    string         `xml:"PMID"`
	Article articlePayload `xml:"Article"`
}

type articlePayload struct {
	Title        string             `xml:"ArticleTitle"`
	Journal      journalPayload     `xml:"Journal"`
	Abstract     abstractPayload    `xml:"Abstract"`
	Authors      authorList         `xml:"AuthorList"`
	ELocationIDs eLocationIDPayload `xml:"ELocationID"`
}

type journalPayload struct {
	Title string `xml:"Title"`
	Issue struct {
		PubDate pubDatePayload `xml:"PubDate"`
	} `xml:"JournalIssue"`
}

func (j journalPayload) Year() int {
	year, _ := strconv.Atoi(normalizeSpace(j.Issue.PubDate.Year))
	return year
}

type pubDatePayload struct {
	Year string `xml:"Year"`
}

type abstractPayload struct {
	Texts []string `xml:"AbstractText"`
}

type authorList struct {
	Authors []authorPayload `xml:"Author"`
}

func (l authorList) names() []string {
	names := make([]string, 0, len(l.Authors))
	for _, author := range l.Authors {
		name := normalizeSpace(strings.TrimSpace(author.ForeName + " " + author.LastName))
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

type authorPayload struct {
	LastName string `xml:"LastName"`
	ForeName string `xml:"ForeName"`
}

type eLocationIDPayload []typedValue

func (ids eLocationIDPayload) find(idType string) string {
	for _, id := range ids {
		if strings.EqualFold(id.typeName(), idType) {
			return id.Value
		}
	}
	return ""
}

type pubmedData struct {
	ArticleIDs articleIDPayload `xml:"ArticleIdList>ArticleId"`
}

type articleIDPayload []typedValue

func (ids articleIDPayload) find(idType string) string {
	for _, id := range ids {
		if strings.EqualFold(id.typeName(), idType) {
			return id.Value
		}
	}
	return ""
}

type typedValue struct {
	IDType  string `xml:"IdType,attr"`
	EIDType string `xml:"EIdType,attr"`
	Value   string `xml:",chardata"`
}

func (v typedValue) typeName() string {
	if v.IDType != "" {
		return v.IDType
	}
	return v.EIDType
}

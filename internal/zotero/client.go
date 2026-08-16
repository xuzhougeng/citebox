package zotero

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	http    *http.Client
	baseURL *url.URL
}

type Collection struct {
	Key       string
	Name      string
	ParentKey string
}

type Item struct {
	Key              string
	ItemType         string
	Title            string
	AbstractNote     string
	PublicationTitle string
	ConferenceName   string
	BookTitle        string
	Publisher        string
	Date             string
	DOI              string
	Extra            string
	URL              string
	Filename         string
	ContentType      string
	LinkMode         string
	ParentItem       string
	Note             string
	Collections      []string
	Creators         []Creator
	Tags             []string
}

type Creator struct {
	CreatorType string
	FirstName   string
	LastName    string
	Name        string
}

type Status struct {
	LibraryPrefix   string
	CollectionCount int
}

func NewClient(rawBaseURL string) (*Client, error) {
	parsed, err := ParseAndValidateBaseURL(rawBaseURL)
	if err != nil {
		return nil, err
	}
	return &Client{
		http: &http.Client{
			Timeout: 20 * time.Second,
		},
		baseURL: parsed,
	}, nil
}

func (c *Client) BaseURL() string {
	if c == nil || c.baseURL == nil {
		return DefaultBaseURL
	}
	return strings.TrimRight(c.baseURL.String(), "/")
}

func (c *Client) Probe(ctx context.Context) (Status, error) {
	var lastErr error
	for _, prefix := range []string{"users/0", "users/local"} {
		collections, err := c.ListCollections(ctx, prefix)
		if err != nil {
			lastErr = err
			continue
		}
		return Status{LibraryPrefix: prefix, CollectionCount: len(collections)}, nil
	}
	if lastErr != nil {
		return Status{}, lastErr
	}
	return Status{}, fmt.Errorf("无法连接 Zotero Local API")
}

func (c *Client) ListCollections(ctx context.Context, libraryPrefix string) ([]Collection, error) {
	var raw []apiObject
	if err := c.getAll(ctx, c.path(libraryPrefix, "collections"), &raw); err != nil {
		return nil, err
	}
	out := make([]Collection, 0, len(raw))
	for _, item := range raw {
		var data collectionData
		if err := json.Unmarshal(item.Data, &data); err != nil {
			continue
		}
		parentKey := ""
		if data.ParentCollection != nil && data.ParentCollection.Valid && !data.ParentCollection.False {
			parentKey = data.ParentCollection.Key
		}
		out = append(out, Collection{
			Key:       firstNonEmpty(data.Key, item.Key),
			Name:      strings.TrimSpace(data.Name),
			ParentKey: parentKey,
		})
	}
	return out, nil
}

func (c *Client) ListItems(ctx context.Context, libraryPrefix string) ([]Item, error) {
	var raw []apiObject
	if err := c.getAll(ctx, c.path(libraryPrefix, "items"), &raw); err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(raw))
	for _, item := range raw {
		parsed, err := parseItem(item)
		if err != nil {
			continue
		}
		out = append(out, parsed)
	}
	return out, nil
}

func (c *Client) AttachmentFilePath(ctx context.Context, libraryPrefix, itemKey string) (string, error) {
	itemKey = strings.TrimSpace(itemKey)
	if itemKey == "" {
		return "", fmt.Errorf("缺少附件 key")
	}
	body, err := c.getBytes(ctx, c.path(libraryPrefix, "items", itemKey, "file", "view", "url"))
	if err != nil {
		return "", err
	}
	raw := strings.TrimSpace(string(body))
	if raw == "" {
		return "", fmt.Errorf("Zotero 未返回本地文件路径")
	}
	if strings.HasPrefix(raw, "{") {
		var payload struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(body, &payload); err == nil && strings.TrimSpace(payload.URL) != "" {
			raw = strings.TrimSpace(payload.URL)
		}
	}
	return ResolveLocalFilePath(raw)
}

func parseItem(raw apiObject) (Item, error) {
	var data itemData
	if err := json.Unmarshal(raw.Data, &data); err != nil {
		return Item{}, err
	}
	tags := make([]string, 0, len(data.Tags))
	for _, tag := range data.Tags {
		name := strings.TrimSpace(tag.Tag)
		if name != "" {
			tags = append(tags, name)
		}
	}
	creators := make([]Creator, 0, len(data.Creators))
	for _, creator := range data.Creators {
		creators = append(creators, Creator{
			CreatorType: strings.TrimSpace(creator.CreatorType),
			FirstName:   strings.TrimSpace(creator.FirstName),
			LastName:    strings.TrimSpace(creator.LastName),
			Name:        strings.TrimSpace(creator.Name),
		})
	}
	return Item{
		Key:              firstNonEmpty(data.Key, raw.Key),
		ItemType:         strings.TrimSpace(data.ItemType),
		Title:            strings.TrimSpace(data.Title),
		AbstractNote:     strings.TrimSpace(data.AbstractNote),
		PublicationTitle: strings.TrimSpace(data.PublicationTitle),
		ConferenceName:   strings.TrimSpace(data.ConferenceName),
		BookTitle:        strings.TrimSpace(data.BookTitle),
		Publisher:        strings.TrimSpace(data.Publisher),
		Date:             strings.TrimSpace(data.Date),
		DOI:              firstNonEmpty(strings.TrimSpace(data.DOI), doiFromExtra(data.Extra)),
		Extra:            strings.TrimSpace(data.Extra),
		URL:              strings.TrimSpace(data.URL),
		Filename:         strings.TrimSpace(data.Filename),
		ContentType:      strings.TrimSpace(data.ContentType),
		LinkMode:         strings.TrimSpace(data.LinkMode),
		ParentItem:       strings.TrimSpace(data.ParentItem),
		Note:             strings.TrimSpace(data.Note),
		Collections:      data.Collections,
		Creators:         creators,
		Tags:             tags,
	}, nil
}

func doiFromExtra(extra string) string {
	for _, line := range strings.Split(extra, "\n") {
		line = strings.TrimSpace(line)
		if len(line) < 5 {
			continue
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "doi:") {
			return strings.TrimSpace(line[4:])
		}
	}
	return ""
}

type apiObject struct {
	Key  string          `json:"key"`
	Data json.RawMessage `json:"data"`
}

type collectionData struct {
	Key              string             `json:"key"`
	Name             string             `json:"name"`
	ParentCollection *flexibleParentKey `json:"parentCollection"`
}

type itemData struct {
	ItemType         string        `json:"itemType"`
	Title            string        `json:"title"`
	AbstractNote     string        `json:"abstractNote"`
	PublicationTitle string        `json:"publicationTitle"`
	ConferenceName   string        `json:"conferenceName"`
	BookTitle        string        `json:"bookTitle"`
	Publisher        string        `json:"publisher"`
	Date             string        `json:"date"`
	DOI              string        `json:"DOI"`
	Extra            string        `json:"extra"`
	URL              string        `json:"url"`
	Filename         string        `json:"filename"`
	ContentType      string        `json:"contentType"`
	LinkMode         string        `json:"linkMode"`
	ParentItem       string        `json:"parentItem"`
	Note             string        `json:"note"`
	Collections      []string      `json:"collections"`
	Creators         []creatorData `json:"creators"`
	Tags             []tagData     `json:"tags"`
	Key              string        `json:"key"`
}

type creatorData struct {
	CreatorType string `json:"creatorType"`
	FirstName   string `json:"firstName"`
	LastName    string `json:"lastName"`
	Name        string `json:"name"`
}

type tagData struct {
	Tag string `json:"tag"`
}

type flexibleParentKey struct {
	Key   string
	Valid bool
	False bool
}

func (p *flexibleParentKey) UnmarshalJSON(data []byte) error {
	raw := strings.TrimSpace(string(data))
	if raw == "" || raw == "null" || raw == "false" {
		p.False = raw == "false"
		p.Valid = false
		return nil
	}
	if raw == "true" {
		p.Valid = false
		return nil
	}
	var key string
	if err := json.Unmarshal(data, &key); err == nil {
		p.Key = strings.TrimSpace(key)
		p.Valid = p.Key != ""
		return nil
	}
	return nil
}

func (c *Client) path(parts ...string) string {
	cleaned := make([]string, 0, len(parts)+1)
	cleaned = append(cleaned, strings.Trim(c.baseURL.Path, "/"))
	for _, part := range parts {
		part = strings.Trim(part, "/")
		if part == "" {
			continue
		}
		cleaned = append(cleaned, part)
	}
	return "/" + path.Join(cleaned...)
}

func (c *Client) getAll(ctx context.Context, requestPath string, dest *[]apiObject) error {
	start := 0
	for {
		query := url.Values{}
		query.Set("limit", "100")
		query.Set("start", strconv.Itoa(start))
		query.Set("includeTrashed", "0")
		var page []apiObject
		if err := c.getJSON(ctx, requestPath, query, &page); err != nil {
			return err
		}
		*dest = append(*dest, page...)
		if len(page) < 100 {
			return nil
		}
		start += len(page)
	}
}

func (c *Client) getJSON(ctx context.Context, requestPath string, query url.Values, dest any) error {
	body, err := c.getBytesQuery(ctx, requestPath, query)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("解析 Zotero 响应失败: %w", err)
	}
	return nil
}

func (c *Client) getBytes(ctx context.Context, requestPath string) ([]byte, error) {
	return c.getBytesQuery(ctx, requestPath, nil)
}

func (c *Client) getBytesQuery(ctx context.Context, requestPath string, query url.Values) ([]byte, error) {
	endpoint := *c.baseURL
	endpoint.Path = requestPath
	if query != nil {
		endpoint.RawQuery = query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Zotero-API-Version", "3")
	req.Header.Set("Accept", "application/json, text/plain;q=0.9,*/*;q=0.8")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("无法连接 Zotero Local API，请确认 Zotero 已打开并启用本地 API: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("读取 Zotero 响应失败: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Zotero Local API 返回 %d", resp.StatusCode)
	}
	return body, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

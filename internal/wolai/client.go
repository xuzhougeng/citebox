package wolai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"
)

const DefaultBaseURL = "https://openapi.wolai.com"
const DefaultAPIBaseURL = "https://api.wolai.com"

type Config struct {
	Token      string
	BaseURL    string
	APIBaseURL string
	Timeout    time.Duration
}

type Client struct {
	httpClient *http.Client
	baseURL    string
	apiBaseURL string
	token      string
}

type apiEnvelope struct {
	Data    json.RawMessage `json:"data"`
	Message string          `json:"message"`
	Error   string          `json:"error"`
}

type UploadSessionRequest struct {
	SpaceID  string `json:"spaceId"`
	FileSize int64  `json:"fileSize"`
	BlockID  string `json:"blockId,omitempty"`
	Type     string `json:"type"`
	FileName string `json:"fileName"`
	OSSPath  string `json:"OSSPath,omitempty"`
}

type UploadSession struct {
	FileID     string       `json:"fileId"`
	FileURL    string       `json:"fileUrl"`
	PolicyData UploadPolicy `json:"policyData"`
}

type UploadPolicy struct {
	URL      string            `json:"url"`
	Bucket   string            `json:"bucket"`
	FormData map[string]string `json:"formData"`
}

type CreatedBlock struct {
	ID   string `json:"id"`
	Type string `json:"type,omitempty"`
	URL  string `json:"url,omitempty"`
}

func NewClient(cfg Config) (*Client, error) {
	token := strings.TrimSpace(cfg.Token)
	if token == "" {
		return nil, errors.New("missing Wolai token")
	}

	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	return &Client{
		httpClient: &http.Client{Timeout: timeout},
		baseURL:    baseURL,
		apiBaseURL: normalizeAPIBaseURL(cfg.APIBaseURL, baseURL),
		token:      token,
	}, nil
}

func (c *Client) GetBlock(id string) (map[string]any, error) {
	var data map[string]any
	if err := c.doJSON(http.MethodGet, "/v1/blocks/"+url.PathEscape(strings.TrimSpace(id)), nil, &data); err != nil {
		return nil, err
	}
	return data, nil
}

func (c *Client) CreateBlocks(parentID string, blocks any) ([]CreatedBlock, error) {
	payload := map[string]any{
		"parent_id": strings.TrimSpace(parentID),
		"blocks":    blocks,
	}
	env, err := c.doEnvelope(http.MethodPost, "/v1/blocks", payload)
	if err != nil {
		return nil, err
	}
	return decodeCreatedBlocks(env.Data)
}

func (c *Client) CreateUploadSession(input UploadSessionRequest) (*UploadSession, error) {
	input.SpaceID = strings.TrimSpace(input.SpaceID)
	input.BlockID = strings.TrimSpace(input.BlockID)
	input.Type = strings.TrimSpace(input.Type)
	input.FileName = strings.TrimSpace(input.FileName)
	input.OSSPath = strings.TrimSpace(input.OSSPath)

	if input.SpaceID == "" {
		return nil, errors.New("missing Wolai space ID")
	}
	if input.FileSize <= 0 {
		return nil, errors.New("missing Wolai file size")
	}
	if input.Type == "" {
		return nil, errors.New("missing Wolai file type")
	}
	if input.FileName == "" {
		return nil, errors.New("missing Wolai file name")
	}

	var session UploadSession
	if err := c.doJSONWithBase(c.apiBaseURL, http.MethodPost, "/v1/file/getSignedPostUrl", input, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func (c *Client) UploadFile(session UploadSession, filename, contentType string, file io.Reader) error {
	if strings.TrimSpace(session.PolicyData.URL) == "" {
		return errors.New("missing Wolai upload URL")
	}
	if strings.TrimSpace(session.FileURL) == "" {
		return errors.New("missing Wolai file URL")
	}
	if file == nil {
		return errors.New("missing upload file reader")
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	for key, value := range session.PolicyData.FormData {
		if strings.TrimSpace(key) == "" {
			continue
		}
		if err := writer.WriteField(key, value); err != nil {
			return fmt.Errorf("write Wolai upload form field %q: %w", key, err)
		}
	}
	if err := writer.WriteField("key", session.FileURL); err != nil {
		return fmt.Errorf("write Wolai upload key: %w", err)
	}
	if err := writer.WriteField("success_action_status", "200"); err != nil {
		return fmt.Errorf("write Wolai upload success status: %w", err)
	}

	part, err := createMultipartFilePart(writer, filename, contentType)
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("copy upload file payload: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close Wolai upload body: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, session.PolicyData.URL, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		text := strings.TrimSpace(string(data))
		if text == "" {
			text = http.StatusText(resp.StatusCode)
		}
		return fmt.Errorf("wolai upload error (%d): %s", resp.StatusCode, text)
	}
	return nil
}

func (c *Client) UpdateBlockFile(blockID, fileID string) error {
	blockID = strings.TrimSpace(blockID)
	fileID = strings.TrimSpace(fileID)
	if blockID == "" {
		return errors.New("missing Wolai block ID")
	}
	if fileID == "" {
		return errors.New("missing Wolai file ID")
	}

	payload := map[string]any{
		"file_id": fileID,
	}
	_, err := c.doEnvelope(http.MethodPatch, "/v1/blocks/"+url.PathEscape(blockID), payload)
	return err
}

func (c *Client) doJSON(method, path string, payload any, out any) error {
	return c.doJSONWithBase(c.baseURL, method, path, payload, out)
}

func (c *Client) doJSONWithBase(baseURL, method, path string, payload any, out any) error {
	env, err := c.doEnvelopeWithBase(baseURL, method, path, payload)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if len(env.Data) == 0 {
		return nil
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return fmt.Errorf("decode response payload: %w", err)
	}
	return nil
}

func (c *Client) doEnvelope(method, path string, payload any) (apiEnvelope, error) {
	return c.doEnvelopeWithBase(c.baseURL, method, path, payload)
}

func (c *Client) doEnvelopeWithBase(baseURL, method, path string, payload any) (apiEnvelope, error) {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return apiEnvelope{}, err
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, strings.TrimRight(baseURL, "/")+path, body)
	if err != nil {
		return apiEnvelope{}, err
	}
	req.Header.Set("authorization", c.token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return apiEnvelope{}, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return apiEnvelope{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return apiEnvelope{}, decodeAPIError(resp.StatusCode, data)
	}

	var env apiEnvelope
	if len(data) == 0 {
		return env, nil
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return apiEnvelope{}, fmt.Errorf("decode response envelope: %w", err)
	}
	return env, nil
}

func decodeAPIError(status int, body []byte) error {
	var env apiEnvelope
	if err := json.Unmarshal(body, &env); err == nil {
		if message := strings.TrimSpace(firstNonEmpty(env.Message, env.Error)); message != "" {
			return fmt.Errorf("wolai api error (%d): %s", status, message)
		}
	}

	text := strings.TrimSpace(string(body))
	if text == "" {
		text = http.StatusText(status)
	}
	return fmt.Errorf("wolai api error (%d): %s", status, text)
}

func normalizeAPIBaseURL(apiBaseURL, openAPIBaseURL string) string {
	apiBaseURL = strings.TrimRight(strings.TrimSpace(apiBaseURL), "/")
	if apiBaseURL != "" {
		return apiBaseURL
	}
	if openAPIBaseURL == "" || openAPIBaseURL == DefaultBaseURL {
		return DefaultAPIBaseURL
	}

	parsed, err := url.Parse(openAPIBaseURL)
	if err != nil {
		return openAPIBaseURL
	}
	if strings.HasPrefix(parsed.Host, "openapi.") {
		parsed.Host = "api." + strings.TrimPrefix(parsed.Host, "openapi.")
		return strings.TrimRight(parsed.String(), "/")
	}
	return openAPIBaseURL
}

func createMultipartFilePart(writer *multipart.Writer, filename, contentType string) (io.Writer, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		filename = "upload.bin"
	}
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, filename))
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return nil, fmt.Errorf("create Wolai upload file part: %w", err)
	}
	return part, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func decodeCreatedBlocks(data json.RawMessage) ([]CreatedBlock, error) {
	if len(data) == 0 {
		return nil, nil
	}

	var urlList []string
	if err := json.Unmarshal(data, &urlList); err == nil {
		blocks := make([]CreatedBlock, 0, len(urlList))
		for _, rawURL := range urlList {
			rawURL = strings.TrimSpace(rawURL)
			if rawURL == "" {
				continue
			}
			blocks = append(blocks, CreatedBlock{
				ID:  extractBlockIDFromURL(rawURL),
				URL: rawURL,
			})
		}
		return blocks, nil
	}

	var payload struct {
		Blocks []CreatedBlock `json:"blocks"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("decode created blocks payload: %w", err)
	}
	for i := range payload.Blocks {
		if payload.Blocks[i].ID == "" && payload.Blocks[i].URL != "" {
			payload.Blocks[i].ID = extractBlockIDFromURL(payload.Blocks[i].URL)
		}
	}
	return payload.Blocks, nil
}

func extractBlockIDFromURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	if fragment := strings.TrimSpace(parsed.Fragment); fragment != "" {
		return fragment
	}
	path := strings.Trim(parsed.Path, "/")
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	return strings.TrimSpace(parts[len(parts)-1])
}

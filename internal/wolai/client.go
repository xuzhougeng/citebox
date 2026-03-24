package wolai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const DefaultBaseURL = "https://openapi.wolai.com"

type Config struct {
	Token   string
	BaseURL string
	Timeout time.Duration
}

type Client struct {
	httpClient *http.Client
	baseURL    string
	token      string
}

type apiEnvelope struct {
	Data    json.RawMessage `json:"data"`
	Message string          `json:"message"`
	Error   string          `json:"error"`
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

func (c *Client) CreateBlocks(parentID string, blocks any) error {
	payload := map[string]any{
		"parent_id": strings.TrimSpace(parentID),
		"blocks":    blocks,
	}
	_, err := c.doEnvelope(http.MethodPost, "/v1/blocks", payload)
	return err
}

func (c *Client) doJSON(method, path string, payload any, out any) error {
	env, err := c.doEnvelope(method, path, payload)
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
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return apiEnvelope{}, err
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.baseURL+path, body)
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

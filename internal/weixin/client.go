package weixin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

const (
	DefaultBaseURL = "https://ilinkai.weixin.qq.com"
	BotType        = "3"
)

// Client wraps the subset of the iLink Bot HTTP API used for binding.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewClient(baseURL, token string, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	return &Client{
		baseURL:    baseURL,
		token:      token,
		httpClient: httpClient,
	}
}

func (c *Client) BaseURL() string {
	return c.baseURL
}

func (c *Client) GetQRCode(ctx context.Context) (*QRCodeResponse, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fmt.Sprintf("%s/ilink/bot/get_bot_qrcode?bot_type=%s", c.baseURL, BotType),
		nil,
	)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("iLink API get_bot_qrcode returned %d", resp.StatusCode)
	}

	var result QRCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetQRCodeStatus(ctx context.Context, qrcode string) (*QRCodeStatusResponse, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fmt.Sprintf("%s/ilink/bot/get_qrcode_status?qrcode=%s", c.baseURL, qrcode),
		nil,
	)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("iLink API get_qrcode_status returned %d", resp.StatusCode)
	}

	var result QRCodeStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("AuthorizationType", "ilink_bot_token")
	uin := strconv.FormatUint(uint64(rand.Uint32()), 10)
	req.Header.Set("X-WECHAT-UIN", base64.StdEncoding.EncodeToString([]byte(uin)))
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}

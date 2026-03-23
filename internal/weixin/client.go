package weixin

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

const (
	DefaultBaseURL = "https://ilinkai.weixin.qq.com"
	BotType        = "3"
)

// Client wraps the subset of the iLink Bot HTTP API used by CiteBox.
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

func (c *Client) GetUpdates(ctx context.Context, buf string) (*GetUpdatesResponse, error) {
	body := GetUpdatesRequest{
		GetUpdatesBuf: buf,
		BaseInfo:      baseInfo(),
	}
	var result GetUpdatesResponse
	if err := c.post(ctx, "/ilink/bot/getupdates", body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) SendTextMessage(ctx context.Context, toUserID, text, contextToken string) error {
	body := SendMessageRequest{
		Msg: Message{
			ToUserID:     toUserID,
			ClientID:     generateClientID(),
			MessageType:  MessageTypeBot,
			MessageState: MessageStateFinish,
			ContextToken: contextToken,
			ItemList: []MessageItem{
				{
					Type:     ItemTypeText,
					TextItem: &TextItem{Text: text},
				},
			},
		},
		BaseInfo: baseInfo(),
	}
	var result SendMessageResponse
	if err := c.post(ctx, "/ilink/bot/sendmessage", body, &result); err != nil {
		return err
	}
	if result.Ret != 0 {
		return fmt.Errorf("sendmessage ret=%d errcode=%d: %s", result.Ret, result.ErrCode, result.Message)
	}
	return nil
}

func (c *Client) post(ctx context.Context, path string, body any, result any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("iLink API %s returned %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(result)
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

func baseInfo() BaseInfo {
	return BaseInfo{ChannelVersion: ChannelVersion}
}

func generateClientID() string {
	return fmt.Sprintf("openclaw-weixin-%d-%d", time.Now().UnixMilli(), rand.Intn(100000))
}

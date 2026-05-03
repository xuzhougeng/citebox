package ai_image_gen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

type APIClient interface {
	Generate(ctx context.Context, settings model.AIImageGenSettings, prompt string) ([]byte, error)
}

type Client struct {
	http *http.Client
}

func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{http: httpClient}
}

func (c *Client) Generate(ctx context.Context, settings model.AIImageGenSettings, prompt string) ([]byte, error) {
	if strings.TrimSpace(settings.APIKey) == "" {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "图像生成 API key 未配置")
	}
	payload := map[string]interface{}{
		"model":   settings.Model,
		"prompt":  prompt,
		"size":    settings.Size,
		"quality": settings.Quality,
		"n":       1,
	}
	if !strings.HasPrefix(strings.TrimSpace(settings.Model), "gpt-image-") {
		payload["response_format"] = "b64_json"
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "序列化图像生成请求失败", err)
	}
	endpoint := strings.TrimRight(settings.BaseURL, "/") + "/v1/images/generations"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "构造图像生成请求失败", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+settings.APIKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeUnavailable, "调用图像生成接口失败", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeUnavailable, "读取图像生成响应失败", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, apperr.New(apperr.CodeUnavailable, fmt.Sprintf("图像生成接口返回 %d: %s", resp.StatusCode, extractAPIErrorMessage(respBody)))
	}

	var parsed struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, apperr.Wrap(apperr.CodeUnavailable, "解析图像生成响应失败", err)
	}
	if len(parsed.Data) == 0 || parsed.Data[0].B64JSON == "" {
		return nil, apperr.New(apperr.CodeUnavailable, "图像生成响应未包含 b64_json")
	}
	pngBytes, err := base64.StdEncoding.DecodeString(parsed.Data[0].B64JSON)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeUnavailable, "图像 base64 解码失败", err)
	}
	return pngBytes, nil
}

func extractAPIErrorMessage(body []byte) string {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return "空响应"
	}
	var nested struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &nested); err == nil && strings.TrimSpace(nested.Error.Message) != "" {
		return nested.Error.Message
	}
	return string(body)
}

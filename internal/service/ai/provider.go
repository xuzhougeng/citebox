package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

// ProviderMode 返回提供商的调用模式
func ProviderMode(settings model.AISettings) string {
	switch settings.Provider {
	case model.AIProviderOpenAI:
		if settings.OpenAILegacyMode {
			return "chat_completions"
		}
		return "responses"
	case model.AIProviderAnthropic:
		return "messages"
	case model.AIProviderGemini:
		return "generate_content"
	default:
		return ""
	}
}

// CallOpenAIResponses 调用 OpenAI Responses API
func CallOpenAIResponses(client *http.Client, settings model.AISettings, systemPrompt, userPrompt string, images []ImageInput) (string, error) {
	content := []map[string]interface{}{
		{"type": "input_text", "text": userPrompt},
	}
	for _, image := range images {
		content = append(content, map[string]interface{}{
			"type":      "input_image",
			"image_url": "data:" + image.MIMEType + ";base64," + image.Data,
		})
	}

	payload := map[string]interface{}{
		"model":             settings.Model,
		"instructions":      systemPrompt,
		"input":             []map[string]interface{}{{"role": "user", "content": content}},
		"temperature":       settings.Temperature,
		"max_output_tokens": settings.MaxOutputTokens,
	}

	body, err := postJSON(client, settings.BaseURL+"/v1/responses", map[string]string{
		"Authorization": "Bearer " + settings.APIKey,
	}, payload)
	if err != nil {
		return "", err
	}

	var response struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", apperr.Wrap(apperr.CodeUnavailable, "解析 OpenAI Responses 响应失败", err)
	}
	if strings.TrimSpace(response.OutputText) != "" {
		return response.OutputText, nil
	}
	for _, item := range response.Output {
		for _, content := range item.Content {
			if strings.TrimSpace(content.Text) != "" {
				return content.Text, nil
			}
		}
	}
	return "", apperr.New(apperr.CodeUnavailable, "OpenAI Responses 未返回文本内容")
}

// CallOpenAIChatCompletions 调用 OpenAI Chat Completions API
func CallOpenAIChatCompletions(client *http.Client, settings model.AISettings, systemPrompt, userPrompt string, images []ImageInput) (string, error) {
	userContent := []map[string]interface{}{
		{"type": "text", "text": userPrompt},
	}
	for _, image := range images {
		userContent = append(userContent, map[string]interface{}{
			"type": "image_url",
			"image_url": map[string]interface{}{
				"url": "data:" + image.MIMEType + ";base64," + image.Data,
			},
		})
	}

	payload := map[string]interface{}{
		"model": settings.Model,
		"messages": []map[string]interface{}{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userContent},
		},
		"temperature": settings.Temperature,
		"max_tokens":  settings.MaxOutputTokens,
	}

	body, err := postJSON(client, settings.BaseURL+"/v1/chat/completions", map[string]string{
		"Authorization": "Bearer " + settings.APIKey,
	}, payload)
	if err != nil {
		return "", err
	}

	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", apperr.Wrap(apperr.CodeUnavailable, "解析 OpenAI Chat Completions 响应失败", err)
	}
	if len(response.Choices) == 0 || strings.TrimSpace(response.Choices[0].Message.Content) == "" {
		return "", apperr.New(apperr.CodeUnavailable, "OpenAI Chat Completions 未返回文本内容")
	}
	return response.Choices[0].Message.Content, nil
}

// CallAnthropicMessages 调用 Anthropic Messages API
func CallAnthropicMessages(client *http.Client, settings model.AISettings, systemPrompt, userPrompt string, images []ImageInput) (string, error) {
	content := []map[string]interface{}{
		{"type": "text", "text": userPrompt},
	}
	for _, image := range images {
		content = append(content, map[string]interface{}{
			"type": "image",
			"source": map[string]interface{}{
				"type":       "base64",
				"media_type": image.MIMEType,
				"data":       image.Data,
			},
		})
	}

	payload := map[string]interface{}{
		"model":       settings.Model,
		"max_tokens":  settings.MaxOutputTokens,
		"temperature": settings.Temperature,
		"system":      systemPrompt,
		"messages": []map[string]interface{}{
			{"role": "user", "content": content},
		},
	}

	body, err := postJSON(client, settings.BaseURL+"/v1/messages", map[string]string{
		"x-api-key":         settings.APIKey,
		"anthropic-version": "2023-06-01",
	}, payload)
	if err != nil {
		return "", err
	}

	var response struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", apperr.Wrap(apperr.CodeUnavailable, "解析 Anthropic 响应失败", err)
	}
	for _, item := range response.Content {
		if strings.TrimSpace(item.Text) != "" {
			return item.Text, nil
		}
	}
	return "", apperr.New(apperr.CodeUnavailable, "Anthropic 未返回文本内容")
}

// CallGeminiGenerateContent 调用 Gemini GenerateContent API
func CallGeminiGenerateContent(client *http.Client, settings model.AISettings, systemPrompt, userPrompt string, images []ImageInput) (string, error) {
	parts := []map[string]interface{}{
		{"text": userPrompt},
	}
	for _, image := range images {
		parts = append(parts, map[string]interface{}{
			"inline_data": map[string]interface{}{
				"mime_type": image.MIMEType,
				"data":      image.Data,
			},
		})
	}

	payload := map[string]interface{}{
		"system_instruction": map[string]interface{}{
			"parts": []map[string]interface{}{
				{"text": systemPrompt},
			},
		},
		"contents": []map[string]interface{}{
			{"parts": parts},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     settings.Temperature,
			"maxOutputTokens": settings.MaxOutputTokens,
		},
	}

	endpoint := settings.BaseURL + "/v1beta/models/" + url.PathEscape(settings.Model) + ":generateContent"
	endpointWithKey, err := addQuery(endpoint, "key", settings.APIKey)
	if err != nil {
		return "", apperr.Wrap(apperr.CodeInvalidArgument, "Gemini Base URL 无效", err)
	}

	body, err := postJSON(client, endpointWithKey, nil, payload)
	if err != nil {
		return "", err
	}

	var response struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", apperr.Wrap(apperr.CodeUnavailable, "解析 Gemini 响应失败", err)
	}
	for _, candidate := range response.Candidates {
		for _, part := range candidate.Content.Parts {
			if strings.TrimSpace(part.Text) != "" {
				return part.Text, nil
			}
		}
	}
	return "", apperr.New(apperr.CodeUnavailable, "Gemini 未返回文本内容")
}

// postJSON 发送 JSON POST 请求
func postJSON(client *http.Client, endpoint string, headers map[string]string, payload interface{}) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "序列化 AI 请求失败", err)
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "创建 AI 请求失败", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeUnavailable, "调用 AI 接口失败", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeUnavailable, "读取 AI 接口响应失败", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, apperr.New(apperr.CodeUnavailable, fmt.Sprintf("AI 接口返回 %d: %s", resp.StatusCode, extractError(respBody)))
	}

	return respBody, nil
}

// postJSONStream 发送 JSON POST 请求并处理流式响应
func PostJSONStream(client *http.Client, endpoint string, headers map[string]string, payload interface{}, onEvent func(eventType, data string) error) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return apperr.Wrap(apperr.CodeInternal, "序列化 AI 流式请求失败", err)
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return apperr.Wrap(apperr.CodeInternal, "创建 AI 流式请求失败", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return context.Canceled
		}
		return apperr.Wrap(apperr.CodeUnavailable, "调用 AI 流式接口失败", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return apperr.Wrap(apperr.CodeUnavailable, "读取 AI 流式接口错误响应失败", readErr)
		}
		return apperr.New(apperr.CodeUnavailable, fmt.Sprintf("AI 接口返回 %d: %s", resp.StatusCode, extractError(respBody)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	eventType := ""
	dataLines := make([]string, 0, 4)
	flushEvent := func() error {
		if eventType == "" && len(dataLines) == 0 {
			return nil
		}
		data := strings.Join(dataLines, "\n")
		eventType = strings.TrimSpace(eventType)
		dataLines = dataLines[:0]
		currentEvent := eventType
		eventType = ""
		if strings.TrimSpace(data) == "" {
			return nil
		}
		return onEvent(currentEvent, data)
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := flushEvent(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, context.Canceled) {
			return context.Canceled
		}
		return apperr.Wrap(apperr.CodeUnavailable, "读取 AI 流式响应失败", err)
	}
	return flushEvent()
}

// addQuery 添加 URL 查询参数
func addQuery(endpoint, key, value string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set(key, value)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// extractError 提取错误信息
func extractError(body []byte) string {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return "空响应"
	}

	var payload struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err == nil {
		if msg := strings.TrimSpace(payload.Error); msg != "" {
			return msg
		}
		if msg := strings.TrimSpace(payload.Message); msg != "" {
			return msg
		}
	}

	return string(body)
}

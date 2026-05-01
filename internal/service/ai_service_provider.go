package service

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

func (s *AIService) CheckModel(ctx context.Context, input model.AIModelConfig) (*model.AIModelCheckResponse, error) {
	normalized, err := normalizeAIModelConfig(input, model.DefaultAISettings().Models[0], 1)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(normalized.APIKey) == "" {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "请先填写 API Key 再检查模型")
	}

	runtimeSettings := model.DefaultAISettings()
	applyAIModelConfig(&runtimeSettings, normalized)
	mode := aiProviderMode(runtimeSettings)

	rawText, providerMode, err := s.callProvider(ctx, &aiReadPrepared{
		settings:     runtimeSettings,
		systemPrompt: "你是模型联通性检查助手。请只回复 OK。",
		userPrompt:   "请只回复 OK",
	})
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(rawText) == "" {
		return nil, apperr.New(apperr.CodeUnavailable, "模型检查未返回文本内容")
	}

	return &model.AIModelCheckResponse{
		Success:  true,
		Provider: normalized.Provider,
		Model:    normalized.Model,
		Mode:     firstNonEmpty(providerMode, mode),
		Message:  "模型检查通过",
	}, nil
}

func (s *AIService) callProvider(ctx context.Context, prepared *aiReadPrepared) (string, string, error) {
	mode := aiProviderMode(prepared.settings)
	switch prepared.settings.Provider {
	case model.AIProviderOpenAI:
		if prepared.settings.OpenAILegacyMode {
			text, err := s.callOpenAIChatCompletions(ctx, prepared.settings, prepared.systemPrompt, prepared.userPrompt, prepared.images)
			return text, mode, err
		}
		text, err := s.callOpenAIResponses(ctx, prepared.settings, prepared.systemPrompt, prepared.userPrompt, prepared.images)
		return text, mode, err
	case model.AIProviderAnthropic:
		text, err := s.callAnthropicMessages(ctx, prepared.settings, prepared.systemPrompt, prepared.userPrompt, prepared.images)
		return text, mode, err
	case model.AIProviderGemini:
		text, err := s.callGeminiGenerateContent(ctx, prepared.settings, prepared.systemPrompt, prepared.userPrompt, prepared.images)
		return text, mode, err
	default:
		return "", "", apperr.New(apperr.CodeInvalidArgument, "暂不支持该 AI 提供商")
	}
}

func (s *AIService) callProviderStream(ctx context.Context, prepared *aiReadPrepared, onDelta func(string) error) (string, error) {
	switch prepared.settings.Provider {
	case model.AIProviderOpenAI:
		if prepared.settings.OpenAILegacyMode {
			return s.callOpenAIChatCompletionsStream(ctx, prepared.settings, prepared.systemPrompt, prepared.userPrompt, prepared.images, onDelta)
		}
		return s.callOpenAIResponsesStream(ctx, prepared.settings, prepared.systemPrompt, prepared.userPrompt, prepared.images, onDelta)
	case model.AIProviderAnthropic:
		return s.callAnthropicMessagesStream(ctx, prepared.settings, prepared.systemPrompt, prepared.userPrompt, prepared.images, onDelta)
	case model.AIProviderGemini:
		return s.callGeminiGenerateContentStream(ctx, prepared.settings, prepared.systemPrompt, prepared.userPrompt, prepared.images, onDelta)
	default:
		return "", apperr.New(apperr.CodeInvalidArgument, "暂不支持该 AI 提供商")
	}
}

func aiProviderMode(settings model.AISettings) string {
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

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func applyTemperature(payload map[string]interface{}, settings model.AISettings) {
	if shouldOmitTemperature(settings) {
		return
	}
	payload["temperature"] = settings.Temperature
}

func applyOpenAIRequestOptions(payload map[string]interface{}, settings model.AISettings, mode string) {
	applyTemperature(payload, settings)

	effort := strings.TrimSpace(settings.ReasoningEffort)
	if mode == "responses" {
		if settings.ThinkingEnabled && effort == "" {
			effort = "medium"
		}
		if effort != "" {
			payload["reasoning"] = map[string]interface{}{"effort": effort}
		}
		return
	}

	if effort != "" {
		payload["reasoning_effort"] = effort
	}
	if mode == "chat_completions" && settings.ThinkingEnabled {
		payload["thinking"] = map[string]interface{}{"type": "enabled"}
	}
}

func shouldOmitTemperature(settings model.AISettings) bool {
	if settings.OmitTemperature {
		return true
	}
	if settings.Provider != model.AIProviderOpenAI {
		return false
	}
	return openAIModelOmitsTemperature(settings.Model)
}

func openAIModelOmitsTemperature(modelName string) bool {
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	modelName = strings.TrimPrefix(modelName, "openai/")
	for _, prefix := range []string{"gpt-5", "o1", "o3", "o4", "o5"} {
		if strings.HasPrefix(modelName, prefix) {
			return true
		}
	}
	return false
}

func (s *AIService) callTextProvider(ctx context.Context, settings model.AISettings, systemPrompt, userPrompt string, images []aiImageInput) (string, error) {
	switch settings.Provider {
	case model.AIProviderOpenAI:
		if settings.OpenAILegacyMode {
			return s.callOpenAIChatCompletions(ctx, settings, systemPrompt, userPrompt, images)
		}
		return s.callOpenAIResponses(ctx, settings, systemPrompt, userPrompt, images)
	case model.AIProviderAnthropic:
		return s.callAnthropicMessages(ctx, settings, systemPrompt, userPrompt, images)
	case model.AIProviderGemini:
		return s.callGeminiGenerateContent(ctx, settings, systemPrompt, userPrompt, images)
	default:
		return "", apperr.New(apperr.CodeInvalidArgument, "暂不支持该 AI 提供商")
	}
}

func (s *AIService) callOpenAIResponses(ctx context.Context, settings model.AISettings, systemPrompt, userPrompt string, images []aiImageInput) (string, error) {
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
		"max_output_tokens": settings.MaxOutputTokens,
	}
	applyOpenAIRequestOptions(payload, settings, "responses")

	body, err := s.postJSON(
		ctx,
		joinProviderURL(settings.BaseURL, defaultAIBaseURL(model.AIProviderOpenAI), "/v1/responses"),
		map[string]string{
			"Authorization": "Bearer " + settings.APIKey,
		},
		payload,
	)
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

func (s *AIService) callOpenAIChatCompletions(ctx context.Context, settings model.AISettings, systemPrompt, userPrompt string, images []aiImageInput) (string, error) {
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
		"max_tokens": settings.MaxOutputTokens,
	}
	applyOpenAIRequestOptions(payload, settings, "chat_completions")

	body, err := s.postJSON(
		ctx,
		joinProviderURL(settings.BaseURL, defaultAIBaseURL(model.AIProviderOpenAI), "/v1/chat/completions"),
		map[string]string{
			"Authorization": "Bearer " + settings.APIKey,
		},
		payload,
	)
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

func (s *AIService) callAnthropicMessages(ctx context.Context, settings model.AISettings, systemPrompt, userPrompt string, images []aiImageInput) (string, error) {
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
		"model":      settings.Model,
		"max_tokens": settings.MaxOutputTokens,
		"system":     systemPrompt,
		"messages": []map[string]interface{}{
			{"role": "user", "content": content},
		},
	}
	applyTemperature(payload, settings)

	body, err := s.postJSON(
		ctx,
		joinProviderURL(settings.BaseURL, defaultAIBaseURL(model.AIProviderAnthropic), "/v1/messages"),
		map[string]string{
			"x-api-key":         settings.APIKey,
			"anthropic-version": "2023-06-01",
		},
		payload,
	)
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

func (s *AIService) callGeminiGenerateContent(ctx context.Context, settings model.AISettings, systemPrompt, userPrompt string, images []aiImageInput) (string, error) {
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
			"maxOutputTokens": settings.MaxOutputTokens,
		},
	}
	applyTemperature(payload["generationConfig"].(map[string]interface{}), settings)

	endpoint := joinProviderURL(settings.BaseURL, defaultAIBaseURL(model.AIProviderGemini), "/v1beta/models/"+url.PathEscape(settings.Model)+":generateContent")
	endpointWithKey, err := addQuery(endpoint, "key", settings.APIKey)
	if err != nil {
		return "", apperr.Wrap(apperr.CodeInvalidArgument, "Gemini Base URL 无效", err)
	}

	body, err := s.postJSON(ctx, endpointWithKey, nil, payload)
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

func (s *AIService) callOpenAIResponsesStream(ctx context.Context, settings model.AISettings, systemPrompt, userPrompt string, images []aiImageInput, onDelta func(string) error) (string, error) {
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
		"max_output_tokens": settings.MaxOutputTokens,
		"stream":            true,
	}
	applyOpenAIRequestOptions(payload, settings, "responses")

	var raw strings.Builder
	err := s.postJSONStream(
		ctx,
		joinProviderURL(settings.BaseURL, defaultAIBaseURL(model.AIProviderOpenAI), "/v1/responses"),
		map[string]string{
			"Authorization": "Bearer " + settings.APIKey,
		},
		payload,
		func(eventType, data string) error {
			delta, snapshot, err := extractOpenAIResponsesStreamText(eventType, data)
			if err != nil {
				return err
			}
			if delta == "" {
				return nil
			}
			if snapshot && raw.Len() > 0 {
				return nil
			}
			raw.WriteString(delta)
			return onDelta(delta)
		},
	)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(raw.String()) == "" {
		return "", apperr.New(apperr.CodeUnavailable, "OpenAI Responses 未返回文本内容")
	}
	return raw.String(), nil
}

func (s *AIService) callOpenAIChatCompletionsStream(ctx context.Context, settings model.AISettings, systemPrompt, userPrompt string, images []aiImageInput, onDelta func(string) error) (string, error) {
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
		"max_tokens": settings.MaxOutputTokens,
		"stream":     true,
	}
	applyOpenAIRequestOptions(payload, settings, "chat_completions")

	var raw strings.Builder
	err := s.postJSONStream(
		ctx,
		joinProviderURL(settings.BaseURL, defaultAIBaseURL(model.AIProviderOpenAI), "/v1/chat/completions"),
		map[string]string{
			"Authorization": "Bearer " + settings.APIKey,
		},
		payload,
		func(_ string, data string) error {
			delta, err := extractOpenAIChatCompletionsStreamDelta(data)
			if err != nil {
				return err
			}
			if delta == "" {
				return nil
			}
			raw.WriteString(delta)
			return onDelta(delta)
		},
	)
	if err != nil {
		return "", err
	}
	return raw.String(), nil
}

func (s *AIService) callAnthropicMessagesStream(ctx context.Context, settings model.AISettings, systemPrompt, userPrompt string, images []aiImageInput, onDelta func(string) error) (string, error) {
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
		"model":      settings.Model,
		"max_tokens": settings.MaxOutputTokens,
		"system":     systemPrompt,
		"stream":     true,
		"messages": []map[string]interface{}{
			{"role": "user", "content": content},
		},
	}
	applyTemperature(payload, settings)

	var raw strings.Builder
	err := s.postJSONStream(
		ctx,
		joinProviderURL(settings.BaseURL, defaultAIBaseURL(model.AIProviderAnthropic), "/v1/messages"),
		map[string]string{
			"x-api-key":         settings.APIKey,
			"anthropic-version": "2023-06-01",
		},
		payload,
		func(eventType, data string) error {
			delta, err := extractAnthropicMessagesStreamDelta(eventType, data)
			if err != nil {
				return err
			}
			if delta == "" {
				return nil
			}
			raw.WriteString(delta)
			return onDelta(delta)
		},
	)
	if err != nil {
		return "", err
	}
	return raw.String(), nil
}

func (s *AIService) callGeminiGenerateContentStream(ctx context.Context, settings model.AISettings, systemPrompt, userPrompt string, images []aiImageInput, onDelta func(string) error) (string, error) {
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
			"maxOutputTokens": settings.MaxOutputTokens,
		},
	}
	applyTemperature(payload["generationConfig"].(map[string]interface{}), settings)

	endpoint := joinProviderURL(settings.BaseURL, defaultAIBaseURL(model.AIProviderGemini), "/v1beta/models/"+url.PathEscape(settings.Model)+":streamGenerateContent")
	endpointWithKey, err := addQuery(endpoint, "key", settings.APIKey)
	if err != nil {
		return "", apperr.Wrap(apperr.CodeInvalidArgument, "Gemini Base URL 无效", err)
	}
	endpointWithAlt, err := addQuery(endpointWithKey, "alt", "sse")
	if err != nil {
		return "", apperr.Wrap(apperr.CodeInvalidArgument, "Gemini 流式 URL 无效", err)
	}

	var raw strings.Builder
	err = s.postJSONStream(ctx, endpointWithAlt, nil, payload, func(_ string, data string) error {
		chunk, err := extractGeminiStreamChunk(data)
		if err != nil {
			return err
		}
		delta := diffAccumulatedChunk(raw.String(), chunk)
		if delta == "" {
			return nil
		}
		raw.WriteString(delta)
		return onDelta(delta)
	})
	if err != nil {
		return "", err
	}
	return raw.String(), nil
}

func (s *AIService) postJSON(ctx context.Context, endpoint string, headers map[string]string, payload interface{}) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "序列化 AI 请求失败", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "创建 AI 请求失败", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeUnavailable, "调用 AI 接口失败", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeUnavailable, "读取 AI 接口响应失败", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, apperr.New(apperr.CodeUnavailable, fmt.Sprintf("AI 接口返回 %d: %s", resp.StatusCode, formatProviderError(respBody)))
	}

	return respBody, nil
}

func (s *AIService) postJSONStream(ctx context.Context, endpoint string, headers map[string]string, payload interface{}, onEvent func(eventType, data string) error) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return apperr.Wrap(apperr.CodeInternal, "序列化 AI 流式请求失败", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return apperr.Wrap(apperr.CodeInternal, "创建 AI 流式请求失败", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := s.httpClient.Do(req)
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
		return apperr.New(apperr.CodeUnavailable, fmt.Sprintf("AI 接口返回 %d: %s", resp.StatusCode, formatProviderError(respBody)))
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
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return context.Canceled
		}
		return apperr.Wrap(apperr.CodeUnavailable, "读取 AI 流式响应失败", err)
	}
	return flushEvent()
}

func extractOpenAIResponsesStreamDelta(eventType, data string) (string, error) {
	text, _, err := extractOpenAIResponsesStreamText(eventType, data)
	return text, err
}

func extractOpenAIResponsesStreamText(eventType, data string) (string, bool, error) {
	if strings.TrimSpace(data) == "" || strings.TrimSpace(data) == "[DONE]" {
		return "", false, nil
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return "", false, apperr.Wrap(apperr.CodeUnavailable, "解析 OpenAI Responses 流式事件失败", err)
	}

	typeName := firstString(payload["type"])
	if eventType == "" {
		eventType = typeName
	}
	if strings.HasSuffix(eventType, "output_text.delta") || strings.HasSuffix(typeName, "output_text.delta") {
		if delta, ok := payload["delta"].(string); ok {
			return delta, false, nil
		}
		if text, ok := payload["text"].(string); ok {
			return text, false, nil
		}
	}
	if strings.HasSuffix(eventType, "output_text.done") || strings.HasSuffix(typeName, "output_text.done") {
		if text, ok := payload["text"].(string); ok {
			return text, true, nil
		}
	}
	if strings.HasSuffix(eventType, "response.completed") || strings.HasSuffix(typeName, "response.completed") {
		if text := extractOpenAIResponsesCompletedText(payload); text != "" {
			return text, true, nil
		}
	}
	return "", false, nil
}

func extractOpenAIResponsesCompletedText(payload map[string]interface{}) string {
	if text := firstString(payload["output_text"]); strings.TrimSpace(text) != "" {
		return text
	}
	if response, ok := payload["response"].(map[string]interface{}); ok {
		if text := firstString(response["output_text"]); strings.TrimSpace(text) != "" {
			return text
		}
		if text := extractOpenAIResponsesOutputText(response["output"]); text != "" {
			return text
		}
	}
	return extractOpenAIResponsesOutputText(payload["output"])
}

func extractOpenAIResponsesOutputText(value interface{}) string {
	output, ok := value.([]interface{})
	if !ok {
		return ""
	}
	var b strings.Builder
	for _, item := range output {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		content, ok := obj["content"].([]interface{})
		if !ok {
			continue
		}
		for _, part := range content {
			partObj, ok := part.(map[string]interface{})
			if !ok {
				continue
			}
			text := firstString(partObj["text"])
			if strings.TrimSpace(text) != "" {
				b.WriteString(text)
			}
		}
	}
	return b.String()
}

func extractOpenAIChatCompletionsStreamDelta(data string) (string, error) {
	if strings.TrimSpace(data) == "" || strings.TrimSpace(data) == "[DONE]" {
		return "", nil
	}

	var payload struct {
		Choices []struct {
			Delta struct {
				Content interface{} `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return "", apperr.Wrap(apperr.CodeUnavailable, "解析 OpenAI Chat Completions 流式事件失败", err)
	}
	if len(payload.Choices) == 0 {
		return "", nil
	}
	return stringifyContentDelta(payload.Choices[0].Delta.Content), nil
}

func extractAnthropicMessagesStreamDelta(eventType, data string) (string, error) {
	if strings.TrimSpace(data) == "" || strings.TrimSpace(data) == "[DONE]" {
		return "", nil
	}
	if eventType != "content_block_delta" {
		return "", nil
	}

	var payload struct {
		Delta struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta"`
	}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return "", apperr.Wrap(apperr.CodeUnavailable, "解析 Anthropic 流式事件失败", err)
	}
	if payload.Delta.Type != "text_delta" {
		return "", nil
	}
	return payload.Delta.Text, nil
}

func extractGeminiStreamChunk(data string) (string, error) {
	if strings.TrimSpace(data) == "" || strings.TrimSpace(data) == "[DONE]" {
		return "", nil
	}

	var payload struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return "", apperr.Wrap(apperr.CodeUnavailable, "解析 Gemini 流式事件失败", err)
	}

	parts := make([]string, 0, 4)
	for _, candidate := range payload.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				parts = append(parts, part.Text)
			}
		}
	}
	return strings.Join(parts, ""), nil
}

func diffAccumulatedChunk(accumulated, chunk string) string {
	if chunk == "" {
		return ""
	}
	if accumulated != "" && strings.HasPrefix(chunk, accumulated) {
		return chunk[len(accumulated):]
	}
	return chunk
}

func stringifyContentDelta(content interface{}) string {
	switch typed := content.(type) {
	case string:
		return typed
	case []interface{}:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if segment, ok := item.(map[string]interface{}); ok {
				if firstString(segment["type"]) == "text" {
					if text, ok := segment["text"].(string); ok && text != "" {
						parts = append(parts, text)
					}
				}
			}
		}
		return strings.Join(parts, "")
	default:
		return ""
	}
}

func extractProviderError(body []byte) string {
	message := strings.TrimSpace(string(body))

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return fallbackText(message, "未知错误")
	}

	if value := firstString(
		payload["message"],
		payload["error"],
		payload["detail"],
	); value != "" {
		return value
	}

	if nested, ok := payload["error"].(map[string]interface{}); ok {
		if value := firstString(nested["message"], nested["type"]); value != "" {
			return value
		}
	}

	return fallbackText(message, "未知错误")
}

func formatProviderError(body []byte) string {
	message := extractProviderError(body)
	if hint := providerUnsupportedParameterHint(message); hint != "" {
		return message + "；" + hint
	}
	return message
}

func providerUnsupportedParameterHint(message string) string {
	lowerMessage := strings.ToLower(strings.TrimSpace(message))
	if lowerMessage == "" {
		return ""
	}
	if !strings.Contains(lowerMessage, "parameter") {
		return ""
	}
	if !strings.Contains(lowerMessage, "unsupported") && !strings.Contains(lowerMessage, "not supported") {
		return ""
	}

	parameter := strings.ToLower(extractUnsupportedParameterName(message))
	switch parameter {
	case "temperature":
		return "提示：该模型不支持 temperature，请在模型配置中勾选「不发送 temperature 参数」。"
	case "thinking":
		return "提示：该接口不支持 thinking，请关闭 thinking 参数；DeepSeek Pro 需要 OpenAI 兼容提供商、传统 Chat Completions 模式和正确 Base URL。"
	case "reasoning", "reasoning_effort", "reasoning.effort":
		return "提示：该接口不支持 reasoning_effort，请清空 Reasoning Effort，或确认当前模型和接口支持该参数。"
	case "max_tokens", "max_output_tokens":
		return "提示：该接口不支持当前 token 参数，请检查是否选错 Responses 或 Chat Completions 模式。"
	default:
		if parameter != "" {
			return fmt.Sprintf("提示：请在模型配置中关闭或调整参数 %s。", parameter)
		}
	}
	return ""
}

func extractUnsupportedParameterName(message string) string {
	lowerMessage := strings.ToLower(message)
	for _, marker := range []string{"unsupported parameter:", "unsupported parameter", "unknown parameter:", "unknown parameter", "parameter:"} {
		index := strings.Index(lowerMessage, marker)
		if index < 0 {
			continue
		}
		remainder := strings.TrimSpace(message[index+len(marker):])
		if parameter := trimProviderParameterToken(remainder); parameter != "" {
			return parameter
		}
	}
	return ""
}

func trimProviderParameterToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if quote := value[0]; quote == '\'' || quote == '"' || quote == '`' {
		if end := strings.IndexByte(value[1:], quote); end >= 0 {
			return strings.TrimSpace(value[1 : 1+end])
		}
	}
	value = strings.TrimLeft(value, "'\"`:")
	for index, r := range value {
		if !isProviderParameterRune(r) {
			return strings.TrimSpace(value[:index])
		}
	}
	return strings.TrimSpace(value)
}

func isProviderParameterRune(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '_' ||
		r == '-' ||
		r == '.'
}

func joinProviderURL(baseURL, defaultBase, endpoint string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = defaultBase
	}

	switch {
	case isDeepSeekRootBase(base) && endpoint == "/v1/chat/completions":
		return base + "/chat/completions"
	case strings.HasSuffix(base, "/v1") && strings.HasPrefix(endpoint, "/v1/"):
		return base + strings.TrimPrefix(endpoint, "/v1")
	case strings.HasSuffix(base, "/v1beta") && strings.HasPrefix(endpoint, "/v1beta/"):
		return base + strings.TrimPrefix(endpoint, "/v1beta")
	default:
		return base + endpoint
	}
}

func isDeepSeekRootBase(base string) bool {
	normalized := strings.ToLower(strings.TrimRight(strings.TrimSpace(base), "/"))
	return normalized == "https://api.deepseek.com" || normalized == "http://api.deepseek.com"
}

func addQuery(rawURL, key, value string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set(key, value)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

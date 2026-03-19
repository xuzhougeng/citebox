package ai

import (
	"encoding/json"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
)

// ExtractOpenAIResponsesStreamDelta 提取 OpenAI Responses 流式响应增量
func ExtractOpenAIResponsesStreamDelta(eventType, data string) (string, error) {
	if strings.TrimSpace(data) == "" || strings.TrimSpace(data) == "[DONE]" {
		return "", nil
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return "", apperr.Wrap(apperr.CodeUnavailable, "解析 OpenAI Responses 流式事件失败", err)
	}

	typeName := firstString(payload["type"])
	if eventType == "" {
		eventType = typeName
	}
	if strings.HasSuffix(eventType, "output_text.delta") || strings.HasSuffix(typeName, "output_text.delta") {
		if delta, ok := payload["delta"].(string); ok {
			return delta, nil
		}
		if text, ok := payload["text"].(string); ok {
			return text, nil
		}
	}
	return "", nil
}

// ExtractOpenAIChatCompletionsStreamDelta 提取 OpenAI Chat Completions 流式响应增量
func ExtractOpenAIChatCompletionsStreamDelta(data string) (string, error) {
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

// ExtractAnthropicMessagesStreamDelta 提取 Anthropic Messages 流式响应增量
func ExtractAnthropicMessagesStreamDelta(eventType, data string) (string, error) {
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

// ExtractGeminiStreamChunk 提取 Gemini 流式响应块
func ExtractGeminiStreamChunk(data string) (string, error) {
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

// DiffAccumulatedChunk 计算累积块差异
func DiffAccumulatedChunk(accumulated, chunk string) string {
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

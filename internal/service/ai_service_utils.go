package service

import (
	"encoding/json"
	"strconv"
	"strings"
)

type aiStructuredResult struct {
	Answer         string
	SuggestedTags  []string
	SuggestedGroup string
}

func extractStructuredAIResult(text string) aiStructuredResult {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return aiStructuredResult{}
	}

	for _, candidate := range []string{
		trimmed,
		trimCodeFence(trimmed),
		trimJSONObject(trimmed),
	} {
		result, ok := parseStructuredAIJSON(candidate)
		if ok {
			return result
		}
	}

	for _, candidate := range []string{
		trimmed,
		trimCodeFence(trimmed),
	} {
		result, ok := parseStructuredAIJSONLoose(candidate)
		if ok {
			return result
		}
	}

	return aiStructuredResult{Answer: trimmed}
}

func parseStructuredAIJSON(candidate string) (aiStructuredResult, bool) {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return aiStructuredResult{}, false
	}

	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(candidate), &raw); err != nil {
		return aiStructuredResult{}, false
	}

	result := aiStructuredResult{
		Answer:         firstString(raw["answer"], raw["response"], raw["analysis"]),
		SuggestedTags:  toStringSlice(raw["suggested_tags"], raw["tags"]),
		SuggestedGroup: firstString(raw["suggested_group"], raw["group"]),
	}
	return result, true
}

func parseStructuredAIJSONLoose(candidate string) (aiStructuredResult, bool) {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return aiStructuredResult{}, false
	}
	if !strings.Contains(candidate, "\"answer\"") &&
		!strings.Contains(candidate, "\"response\"") &&
		!strings.Contains(candidate, "\"analysis\"") &&
		!strings.Contains(candidate, "\"suggested_tags\"") &&
		!strings.Contains(candidate, "\"suggested_group\"") {
		return aiStructuredResult{}, false
	}

	result := aiStructuredResult{}
	found := false

	if answer, ok := extractPartialJSONStringField(candidate, "answer", "response", "analysis"); ok {
		result.Answer = answer
		found = true
	}
	if tags, ok := extractPartialJSONStringArrayField(candidate, "suggested_tags", "tags"); ok {
		result.SuggestedTags = tags
		found = true
	}
	if group, ok := extractPartialJSONStringField(candidate, "suggested_group", "group"); ok {
		result.SuggestedGroup = group
		found = true
	}

	return result, found
}

func extractPartialJSONStringField(text string, keys ...string) (string, bool) {
	for _, key := range keys {
		start, ok := findJSONFieldValueStart(text, key, '"')
		if !ok {
			continue
		}
		value, _, _ := decodePartialJSONString(text, start)
		return value, true
	}
	return "", false
}

func extractPartialJSONStringArrayField(text string, keys ...string) ([]string, bool) {
	for _, key := range keys {
		start, ok := findJSONFieldValueStart(text, key, '[')
		if !ok {
			continue
		}
		values := make([]string, 0, 4)
		i := start
		for i < len(text) {
			i = skipJSONWhitespace(text, i)
			if i >= len(text) || text[i] == ']' {
				break
			}
			if text[i] == ',' {
				i++
				continue
			}
			if text[i] != '"' {
				break
			}

			value, next, _ := decodePartialJSONString(text, i+1)
			if strings.TrimSpace(value) != "" {
				values = append(values, value)
			}
			i = next
		}

		if len(values) > 0 {
			return values, true
		}
	}
	return nil, false
}

func findJSONFieldValueStart(text, key string, opening byte) (int, bool) {
	pattern := `"` + key + `"`
	searchFrom := 0
	for searchFrom < len(text) {
		relative := strings.Index(text[searchFrom:], pattern)
		if relative < 0 {
			return 0, false
		}
		index := searchFrom + relative + len(pattern)
		index = skipJSONWhitespace(text, index)
		if index >= len(text) || text[index] != ':' {
			searchFrom += relative + len(pattern)
			continue
		}
		index++
		index = skipJSONWhitespace(text, index)
		if index >= len(text) || text[index] != opening {
			searchFrom += relative + len(pattern)
			continue
		}
		return index + 1, true
	}
	return 0, false
}

func skipJSONWhitespace(text string, index int) int {
	for index < len(text) {
		switch text[index] {
		case ' ', '\n', '\r', '\t':
			index++
		default:
			return index
		}
	}
	return index
}

func decodePartialJSONString(text string, start int) (string, int, bool) {
	var builder strings.Builder
	for i := start; i < len(text); i++ {
		ch := text[i]
		if ch == '"' {
			return builder.String(), i + 1, true
		}
		if ch != '\\' {
			builder.WriteByte(ch)
			continue
		}

		i++
		if i >= len(text) {
			return builder.String(), len(text), false
		}

		switch text[i] {
		case '"', '\\', '/':
			builder.WriteByte(text[i])
		case 'b':
			builder.WriteByte('\b')
		case 'f':
			builder.WriteByte('\f')
		case 'n':
			builder.WriteByte('\n')
		case 'r':
			builder.WriteByte('\r')
		case 't':
			builder.WriteByte('\t')
		case 'u':
			if i+4 >= len(text) {
				return builder.String(), len(text), false
			}
			if value, ok := parseJSONUnicodeEscape(text[i+1 : i+5]); ok {
				builder.WriteRune(value)
				i += 4
				continue
			}
			builder.WriteString(`\u`)
		default:
			builder.WriteByte(text[i])
		}
	}

	return builder.String(), len(text), false
}

func parseJSONUnicodeEscape(text string) (rune, bool) {
	if len(text) != 4 {
		return 0, false
	}

	var value rune
	for _, ch := range text {
		value <<= 4
		switch {
		case ch >= '0' && ch <= '9':
			value += ch - '0'
		case ch >= 'a' && ch <= 'f':
			value += ch - 'a' + 10
		case ch >= 'A' && ch <= 'F':
			value += ch - 'A' + 10
		default:
			return 0, false
		}
	}
	return value, true
}

func trimCodeFence(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "```") {
		return ""
	}
	lines := strings.Split(text, "\n")
	if len(lines) < 3 {
		return ""
	}
	return strings.Join(lines[1:len(lines)-1], "\n")
}

func trimJSONObject(text string) string {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return ""
	}
	return text[start : end+1]
}

func firstString(values ...interface{}) string {
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return strings.TrimSpace(typed)
			}
		case float64:
			return strconv.FormatInt(int64(typed), 10)
		case int:
			return strconv.Itoa(typed)
		case int64:
			return strconv.FormatInt(typed, 10)
		case map[string]interface{}:
			if name := firstString(typed["name"], typed["title"]); name != "" {
				return name
			}
		}
	}
	return ""
}

func toStringSlice(values ...interface{}) []string {
	for _, value := range values {
		switch typed := value.(type) {
		case []interface{}:
			result := make([]string, 0, len(typed))
			for _, item := range typed {
				if text := firstString(item); text != "" {
					result = append(result, text)
				}
			}
			if len(result) > 0 {
				return result
			}
		case []string:
			result := make([]string, 0, len(typed))
			for _, item := range typed {
				if trimmed := strings.TrimSpace(item); trimmed != "" {
					result = append(result, trimmed)
				}
			}
			if len(result) > 0 {
				return result
			}
		case string:
			parts := strings.Split(typed, ",")
			result := make([]string, 0, len(parts))
			for _, part := range parts {
				if trimmed := strings.TrimSpace(part); trimmed != "" {
					result = append(result, trimmed)
				}
			}
			if len(result) > 0 {
				return result
			}
		}
	}
	return []string{}
}

func joinOrFallback(values []string, fallback string) string {
	if len(values) == 0 {
		return fallback
	}
	return strings.Join(values, "，")
}

func fallbackText(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

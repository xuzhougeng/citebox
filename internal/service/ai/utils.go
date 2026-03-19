package ai

import (
	"encoding/json"
	"regexp"
	"strings"
)

var markdownFigureReferencePattern = regexp.MustCompile(`!\[([^\]]*)\]\(figure://([0-9]+)\)`)

// ExtractStructuredResult 从 AI 响应中提取结构化结果
type StructuredResult struct {
	Answer         string
	SuggestedTags  []string
	SuggestedGroup string
}

// ExtractStructuredResult 从文本中提取结构化结果
func ExtractStructuredResult(text string) StructuredResult {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return StructuredResult{}
	}

	for _, candidate := range []string{
		trimmed,
		trimCodeFence(trimmed),
		trimJSONObject(trimmed),
	} {
		result, ok := parseStructuredJSON(candidate)
		if ok {
			return result
		}
	}

	for _, candidate := range []string{
		trimmed,
		trimCodeFence(trimmed),
	} {
		result, ok := parseStructuredJSONLoose(candidate)
		if ok {
			return result
		}
	}

	return StructuredResult{Answer: trimmed}
}

func parseStructuredJSON(candidate string) (StructuredResult, bool) {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return StructuredResult{}, false
	}

	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(candidate), &raw); err != nil {
		return StructuredResult{}, false
	}

	result := StructuredResult{
		Answer:         firstString(raw["answer"], raw["response"], raw["analysis"]),
		SuggestedTags:  toStringSlice(raw["suggested_tags"], raw["tags"]),
		SuggestedGroup: firstString(raw["suggested_group"], raw["group"]),
	}
	return result, true
}

func parseStructuredJSONLoose(candidate string) (StructuredResult, bool) {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return StructuredResult{}, false
	}
	if !strings.Contains(candidate, "\"answer\"") &&
		!strings.Contains(candidate, "\"response\"") &&
		!strings.Contains(candidate, "\"analysis\"") &&
		!strings.Contains(candidate, "\"suggested_tags\"") &&
		!strings.Contains(candidate, "\"suggested_group\"") {
		return StructuredResult{}, false
	}

	result := StructuredResult{}
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
			if i >= len(text) || text[i] != '"' {
				break
			}
			i++
			value, nextIdx, ok := decodePartialJSONString(text, i)
			if ok && strings.TrimSpace(value) != "" {
				values = append(values, value)
			}
			i = nextIdx
			if i < len(text) && text[i] == ',' {
				i++
				continue
			}
			if i < len(text) && text[i] == ']' {
				break
			}
		}
		return values, true
	}
	return nil, false
}

func findJSONFieldValueStart(text, key string, valueStart byte) (int, bool) {
	pattern := `"` + regexp.QuoteMeta(key) + `"\s*:\s*`
	re := regexp.MustCompile(pattern)
	loc := re.FindStringIndex(text)
	if loc == nil {
		return 0, false
	}
	start := loc[1]
	for start < len(text) && (text[start] == ' ' || text[start] == '\t' || text[start] == '\n' || text[start] == '\r') {
		start++
	}
	if start >= len(text) || text[start] != valueStart {
		return 0, false
	}
	if valueStart == '"' {
		return start + 1, true
	}
	return start, true
}

func decodePartialJSONString(text string, start int) (string, int, bool) {
	var result strings.Builder
	i := start
	for i < len(text) {
		c := text[i]
		if c == '"' {
			return result.String(), i + 1, true
		}
		if c == '\\' && i+1 < len(text) {
			next := text[i+1]
			switch next {
			case '"', '\\', '/':
				result.WriteByte(next)
				i += 2
			case 'n':
				result.WriteByte('\n')
				i += 2
			case 't':
				result.WriteByte('\t')
				i += 2
			case 'r':
				result.WriteByte('\r')
				i += 2
			case 'b':
				result.WriteByte('\b')
				i += 2
			case 'f':
				result.WriteByte('\f')
				i += 2
			case 'u':
				if i+5 < len(text) {
					code := text[i+2 : i+6]
					if r, err := decodeHex(code); err == nil {
						result.WriteRune(r)
					}
					i += 6
				} else {
					i += 2
				}
			default:
				result.WriteByte(next)
				i += 2
			}
			continue
		}
		result.WriteByte(c)
		i++
	}
	return result.String(), i, false
}

func decodeHex(s string) (rune, error) {
	if len(s) != 4 {
		return 0, nil
	}
	var result rune
	for _, c := range s {
		result <<= 4
		switch {
		case c >= '0' && c <= '9':
			result += rune(c - '0')
		case c >= 'a' && c <= 'f':
			result += rune(c - 'a' + 10)
		case c >= 'A' && c <= 'F':
			result += rune(c - 'A' + 10)
		default:
			return 0, nil
		}
	}
	return result, nil
}

func skipJSONWhitespace(text string, i int) int {
	for i < len(text) {
		c := text[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			i++
		} else {
			break
		}
	}
	return i
}

func trimCodeFence(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) < 2 {
		return s
	}
	first := strings.TrimSpace(lines[0])
	if strings.HasPrefix(first, "```") {
		lines = lines[1:]
	}
	last := strings.TrimSpace(lines[len(lines)-1])
	if last == "```" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

func trimJSONObject(s string) string {
	start := strings.Index(s, "{")
	if start == -1 {
		return s
	}
	depth := 0
	end := -1
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i + 1
				break
			}
		}
		if end != -1 {
			break
		}
	}
	if end == -1 {
		return s
	}
	return s[start:end]
}

func firstString(values ...interface{}) string {
	for _, v := range values {
		switch val := v.(type) {
		case string:
			if strings.TrimSpace(val) != "" {
				return strings.TrimSpace(val)
			}
		}
	}
	return ""
}

func toStringSlice(values ...interface{}) []string {
	for _, v := range values {
		switch val := v.(type) {
		case []interface{}:
			result := make([]string, 0, len(val))
			for _, item := range val {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					result = append(result, strings.TrimSpace(s))
				}
			}
			return result
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// MarkdownFigureReferencePattern 返回 Markdown 图片引用正则表达式
func MarkdownFigureReferencePattern() *regexp.Regexp {
	return markdownFigureReferencePattern
}

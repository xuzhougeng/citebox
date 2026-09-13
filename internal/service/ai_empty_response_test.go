package service

import (
	"strings"
	"testing"
)

func TestChatCompletionEmptyResponseDiagnostics(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"no choices", `{"choices":[]}`, "choices 为空"},
		{"token limit", `{"choices":[{"finish_reason":"length","message":{"content":null,"reasoning_content":"private reasoning"}}]}`, "finish_reason=length"},
		{"filtered", `{"choices":[{"finish_reason":"content_filter","message":{"content":""}}]}`, "过滤或拒绝"},
		{"refusal", `{"choices":[{"message":{"content":null,"refusal":"private refusal"}}]}`, "过滤或拒绝"},
		{"reasoning only", `{"choices":[{"finish_reason":"stop","message":{"content":"","reasoning_content":"private reasoning"}}]}`, "仅返回推理内容"},
		{"empty stop", `{"choices":[{"finish_reason":"stop","message":{"content":" "}}]}`, "模型能力及接口兼容性"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseOpenAIChatCompletionText([]byte(tc.body))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v; want %s", err, tc.want)
			}
			if strings.Contains(err.Error(), "private") {
				t.Fatal("error exposed private response content")
			}
		})
	}
	// A valid no-figure JSON answer must not be confused with an empty response.
	got, err := parseOpenAIChatCompletionText([]byte(`{"choices":[{"message":{"content":"{\"figures\":[]}"}}]}`))
	if err != nil || got != `{"figures":[]}` {
		t.Fatalf("got %q, %v", got, err)
	}
}

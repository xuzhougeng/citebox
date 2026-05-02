package agent_session

import (
	"context"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service/agent_session/commands"
)

func TestParseSlashRecognizesFullwidth(t *testing.T) {
	cmd, arg, ok := parseSlash("／help")
	if !ok || cmd != "/help" || arg != "" {
		t.Fatalf("got cmd=%q arg=%q ok=%v", cmd, arg, ok)
	}
}

func TestParseSlashSplitsArg(t *testing.T) {
	cmd, arg, ok := parseSlash("/note 这是笔记")
	if !ok || cmd != "/note" || arg != "这是笔记" {
		t.Fatalf("got cmd=%q arg=%q ok=%v", cmd, arg, ok)
	}
}

func TestParseSlashRejectsPlainText(t *testing.T) {
	_, _, ok := parseSlash("hello")
	if ok {
		t.Fatal("plain text should not be recognized as slash")
	}
}

func TestHandleFreeTextWithoutFreeTextHandlerReturnsError(t *testing.T) {
	repo := repository.NewTestAIConversationRepo(t)
	registry := commands.NewRegistry()
	svc := New(repo, registry, nil, nil, nil)
	_, err := svc.Handle(context.Background(), AgentRequest{
		Surface:      SurfaceWeChat,
		Conversation: ConversationRef{Kind: KindMainWeChat},
		Input:        Input{Text: "free text question"},
	})
	if err == nil {
		t.Fatal("expected error when freeText handler is nil")
	}
	if !strings.Contains(err.Error(), "AI 助手未配置") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLooksLikeDOI(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"10.1038/s41586-023-06000-1", true},
		{"https://doi.org/10.1038/s41586-023-06000-1", true},
		{"doi:10.1038/s41586-023-06000-1", true},
		{"DOI:10.1000/xyz123", true},
		{"  10.1234/abc.def  ", true},
		{"hello world", false},
		{"/help", false},
		{"", false},
		{"just text mentioning 10.something", false},
		// Sentences with embedded DOIs must NOT route to DOI import — the
		// user is asking a question about a DOI, not pasting one.
		{"请解释一下 10.1038/nature1234 的主要结论", false},
		{"see 10.1038/nature1234 please", false},
		{"10.1038/nature1234\nfollowed by another line", false},
	}
	for _, tc := range cases {
		if got := looksLikeDOI(tc.in); got != tc.want {
			t.Errorf("looksLikeDOI(%q)=%v want %v", tc.in, got, tc.want)
		}
	}
}

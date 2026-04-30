package ai_conversation

import (
	"context"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

func TestSummarizerCompressOldHalf(t *testing.T) {
	caller := &stubNonStreamCaller{staticReply: "压缩摘要内容"}
	settings := model.DefaultAISettings()
	msgs := []repository.AIMessage{
		{ID: 1, Role: "user", Content: "Q1"},
		{ID: 2, Role: "assistant", Content: "A1"},
		{ID: 3, Role: "user", Content: "Q2"},
		{ID: 4, Role: "assistant", Content: "A2"},
		{ID: 5, Role: "user", Content: "Q3"},
		{ID: 6, Role: "assistant", Content: "A3"},
	}
	summary, throughID, err := summarize(context.Background(), caller, settings, "已有摘要", msgs)
	if err != nil {
		t.Fatalf("summarize: %v", err)
	}
	if !strings.Contains(summary, "压缩摘要内容") {
		t.Fatalf("summary = %q", summary)
	}
	// Compresses oldest half (id 1..3 → 3 messages of 6).
	if throughID != 3 {
		t.Fatalf("throughID = %d, want 3", throughID)
	}
}

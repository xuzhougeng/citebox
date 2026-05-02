package agent_session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/xuzhougeng/citebox/internal/repository"
)

func TestMigrateLegacyWeixinContext(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "im_context.json")
	payload := map[string]any{
		"current_paper_id":  42,
		"current_figure_id": 7,
		"qa_history": []map[string]any{
			{"role": "user", "content": "你好"},
			{"role": "assistant", "content": "你好。"},
		},
		"search_paper_ids": []int{1, 2},
	}
	raw, _ := json.Marshal(payload)
	if err := os.WriteFile(legacy, raw, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	repo := repository.NewTestAIConversationRepo(t)
	state, err := NewSurfaceStateStore(filepath.Join(dir, "wx_state.json"))
	if err != nil {
		t.Fatalf("NewSurfaceStateStore: %v", err)
	}

	if err := MigrateLegacyWeixinContext(legacy, repo, state, nil); err != nil {
		t.Fatalf("MigrateLegacyWeixinContext: %v", err)
	}

	cid, err := repo.FindOrCreateByKind("main_wechat", "wechat")
	if err != nil {
		t.Fatalf("FindOrCreateByKind: %v", err)
	}
	msgs, err := repo.ListMessagesAfterBarrier(cid, 100)
	if err != nil {
		t.Fatalf("ListMessagesAfterBarrier: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("want 2 migrated messages, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "你好" {
		t.Fatalf("first message = %+v, want user/你好", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "你好。" {
		t.Fatalf("second message = %+v, want assistant/你好。", msgs[1])
	}

	sc, _ := state.Get("wechat")
	if sc.CurrentPaperID != 42 || sc.CurrentFigureID != 7 {
		t.Fatalf("state not migrated: %+v", sc)
	}
	if len(sc.RecentSearchPaperIDs) != 2 || sc.RecentSearchPaperIDs[0] != 1 || sc.RecentSearchPaperIDs[1] != 2 {
		t.Fatalf("search paper ids not migrated: %+v", sc.RecentSearchPaperIDs)
	}

	if _, err := os.Stat(legacy + ".migrated.bak"); err != nil {
		t.Fatalf("expected legacy file to be renamed: %v", err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("expected legacy path to be gone, got err=%v", err)
	}
}

func TestMigrateLegacyWeixinContextHandlesQATurnShape(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "im_context.json")
	payload := map[string]any{
		"qa_history": []map[string]any{
			{"question": "Q1", "answer": "A1"},
			{"question": "Q2", "answer": ""},
		},
	}
	raw, _ := json.Marshal(payload)
	if err := os.WriteFile(legacy, raw, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	repo := repository.NewTestAIConversationRepo(t)
	state, err := NewSurfaceStateStore(filepath.Join(dir, "wx.json"))
	if err != nil {
		t.Fatalf("NewSurfaceStateStore: %v", err)
	}

	if err := MigrateLegacyWeixinContext(legacy, repo, state, nil); err != nil {
		t.Fatalf("MigrateLegacyWeixinContext: %v", err)
	}

	cid, _ := repo.FindOrCreateByKind("main_wechat", "wechat")
	msgs, err := repo.ListMessagesAfterBarrier(cid, 100)
	if err != nil {
		t.Fatalf("ListMessagesAfterBarrier: %v", err)
	}
	// Q1+A1 = 2 messages, Q2 (no answer) = 1 message → 3 total.
	if len(msgs) != 3 {
		t.Fatalf("want 3 migrated messages, got %d (%+v)", len(msgs), msgs)
	}
}

func TestMigrateLegacyWeixinContextNoFile(t *testing.T) {
	dir := t.TempDir()
	repo := repository.NewTestAIConversationRepo(t)
	state, err := NewSurfaceStateStore(filepath.Join(dir, "wx.json"))
	if err != nil {
		t.Fatalf("NewSurfaceStateStore: %v", err)
	}

	if err := MigrateLegacyWeixinContext(filepath.Join(dir, "missing.json"), repo, state, nil); err != nil {
		t.Fatalf("MigrateLegacyWeixinContext on missing path: %v", err)
	}
}

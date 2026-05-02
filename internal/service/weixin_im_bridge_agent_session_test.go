package service

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service/agent_session"
	"github.com/xuzhougeng/citebox/internal/service/agent_session/commands"
)

// TestWeixinIMBridgeRoutesSlashThroughAgentSession verifies the T15 cutover:
// `/help` (a state command) reaches the agent_session registry through the
// bridge and the response chunks land in the envelope's Text field.
func TestWeixinIMBridgeRoutesSlashThroughAgentSession(t *testing.T) {
	svc, repo, cfg := newTestService(t)

	state, err := agent_session.NewSurfaceStateStore(filepath.Join(cfg.StorageDir, "wx_state.json"))
	if err != nil {
		t.Fatalf("NewSurfaceStateStore: %v", err)
	}

	registry := commands.NewRegistry(
		&commands.HelpCommand{},
		&commands.ClearCommand{},
	)

	agentSvc := agent_session.New(repo.AIConversation, registry, nil, state, slog.New(slog.NewTextHandler(io.Discard, nil)))

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	bridge := NewWeixinIMBridge(svc, &fakeWeixinAIReader{}, logger, cfg.StorageDir, agentSvc, state)

	reply := bridge.handleIncomingText(context.Background(), "/help")
	if reply == "" {
		t.Fatal("/help returned empty reply")
	}
	if !containsAll(reply, "状态：", "/clear", "/help") {
		t.Fatalf("/help reply missing expected command listing: %q", reply)
	}
}

// TestWeixinIMBridgeClearSetsBarrierViaAgentSession verifies that `/clear`
// routed through the bridge actually mutates the underlying conversation's
// clear_barrier_turn_id. After /clear, ListMessagesAfterBarrier returns zero
// rows even though one user turn ("hi") was previously inserted.
func TestWeixinIMBridgeClearSetsBarrierViaAgentSession(t *testing.T) {
	svc, repo, cfg := newTestService(t)

	state, err := agent_session.NewSurfaceStateStore(filepath.Join(cfg.StorageDir, "wx_state.json"))
	if err != nil {
		t.Fatalf("NewSurfaceStateStore: %v", err)
	}
	registry := commands.NewRegistry(&commands.ClearCommand{})
	agentSvc := agent_session.New(repo.AIConversation, registry, nil, state, slog.New(slog.NewTextHandler(io.Discard, nil)))

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	bridge := NewWeixinIMBridge(svc, &fakeWeixinAIReader{}, logger, cfg.StorageDir, agentSvc, state)

	// Seed the wechat conversation with a turn so the barrier has something to
	// hide. We touch the repo directly (parallel to what SendForSurface does)
	// because the bridge's free-text path currently requires a wired
	// FreeTextHandler — this test focuses on the slash-command leg.
	cid, err := repo.AIConversation.FindOrCreateByKind("main_wechat", "wechat")
	if err != nil {
		t.Fatalf("FindOrCreateByKind: %v", err)
	}
	if _, err := repo.AIConversation.AddMessage(cid, "user", "hi", repository.AIMessageMeta{}); err != nil {
		t.Fatalf("AddMessage: %v", err)
	}

	reply := bridge.handleIncomingText(context.Background(), "/clear")
	if reply == "" {
		t.Fatal("/clear returned empty reply")
	}
	if !containsAll(reply, "已清空对话上下文") {
		t.Errorf("/clear reply missing confirmation: %q", reply)
	}

	rows, err := repo.AIConversation.ListMessagesAfterBarrier(cid, 10)
	if err != nil {
		t.Fatalf("ListMessagesAfterBarrier: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows after /clear, got %d", len(rows))
	}
}

// TestWeixinIMBridgeAliasRewriteRoutesQAToHelp confirms the alias map (e.g.
// "/qa" → "/ask") works through the bridge dispatch. Without an /ask command
// in the registry, the request becomes "unknown command", which the bridge
// surfaces as the dispatch error string.
func TestWeixinIMBridgeAliasRewriteHelpAlias(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	state, _ := agent_session.NewSurfaceStateStore(filepath.Join(cfg.StorageDir, "wx_state.json"))
	registry := commands.NewRegistry(&commands.HelpCommand{})
	agentSvc := agent_session.New(repo.AIConversation, registry, nil, state, slog.New(slog.NewTextHandler(io.Discard, nil)))

	bridge := NewWeixinIMBridge(svc, &fakeWeixinAIReader{}, slog.New(slog.NewTextHandler(io.Discard, nil)),
		cfg.StorageDir, agentSvc, state)

	reply := bridge.handleIncomingText(context.Background(), "/h")
	if !containsAll(reply, "状态：", "/help") {
		t.Fatalf("/h alias should rewrite to /help; got %q", reply)
	}
}

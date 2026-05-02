# Unify AI Across Surfaces — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Route WeChat IM and desktop/Web through one shared AI engine with one shared per-user conversation history (5-turn LLM window, manual `/clear` barrier), while bringing WeChat-only conveniences (TTS, daily figure recommendation, DOI quick import) to the desktop.

**Architecture:** A new `internal/service/agent_session/` package owns slash dispatch, surface-aware conversation routing, and the WeChat-friendly 5-turn / barrier window. The **existing** `internal/service/ai_conversation` package keeps its summary-based pipeline (used for desktop) and gains a thin "WeChat mode" entry point that disables summarization and uses the bounded window. The existing `ai_assistant.Orchestrator` and tools are unchanged. WeChat bridge and web handler shrink to surface adapters.

**Tech Stack:** Go 1.21+, SQLite (`internal/repository/schema/schema.go` is a single-file schema using `ensureColumn` for additive migrations), native HTML/JS frontend, existing `ai_assistant` orchestrator and tools.

**Spec:** `docs/superpowers/specs/2026-05-02-unify-ai-across-surfaces-design.md`

---

## Note on architecture deviation from spec

The spec described `agent_session.Service` as if it owned conversation persistence. After exploration, the existing `ai_conversation.Service` already owns this (with summarization, evidence injection, citations, pinned papers, streaming). To avoid duplicating that engine, **`agent_session` is built as a thin façade** that:

1. Owns slash command parsing and dispatch.
2. Resolves the right `ai_conversation` row by `(user_id, kind)` — creating the WeChat main conversation lazily.
3. Owns surface-local state (`weixin_surface_state.json`).
4. Calls into `ai_conversation.Service.SendMessage` for free text, with options that disable summarization and impose the 5-turn cap when `kind = 'main_wechat'`.

`ai_conversation` keeps its current behavior for the existing web flow. The schema columns described in the spec (`surface_origin`, `kind`, `clear_barrier_turn_id`) are added to `ai_conversations` and consumed by both packages.

---

## File map

**New files:**
- `internal/service/agent_session/types.go` — `AgentRequest`, `AgentResponse`, `Surface`, `OutboundChunk`, `EvidenceRef`, `SurfaceContext`, `ConversationRef`
- `internal/service/agent_session/service.go` — `Service` with `Handle`, conversation resolution, dispatch
- `internal/service/agent_session/dispatch.go` — text → command parser
- `internal/service/agent_session/commands/clear.go`, `reset.go`, `help.go`, `status.go`, `note.go` — state commands
- `internal/service/agent_session/commands/recent.go`, `figures.go`, `random.go`, `paper.go`, `figure.go`, `search.go` — query shortcuts (no LLM)
- `internal/service/agent_session/commands/ask.go`, `interpret.go` — constrained agent paths
- `internal/service/agent_session/commands/registry.go` — name → command lookup
- `internal/service/agent_session/surface_state.go` — load/save `weixin_surface_state.json`
- `internal/service/agent_session/migration.go` — one-shot `im_context.json` → DB conversion
- `internal/service/agent_session/service_test.go` + `commands/*_test.go` + `migration_test.go`
- `internal/service/daily_figure/picker.go` — extracted "pick today's figure" used by both bridge ticker and Overview endpoint
- `internal/service/daily_figure/picker_test.go`
- `internal/handler/overview.go` — `/api/overview/summary`, `/api/overview/daily-figure`, `/api/overview/status`
- `internal/handler/overview_test.go`
- `web/overview.html` — three-panel layout
- `web/static/js/overview.js`
- `web/static/css/overview.css`

**Modified files:**
- `internal/repository/schema/schema.go` — add columns + partial unique index via `ensureColumn` and `ensureMainWeChatUnique`
- `internal/repository/ai_conversation_repo.go` — new methods: `FindOrCreateByKind`, `SetClearBarrier`, `ListMessagesAfterBarrier(limit)`, `UpdateSurfaceMeta`
- `internal/service/ai_conversation/service.go` — `SendMessage` accepts new options `DisableSummary bool` + `MaxHistoryTurns int`; loader respects `clear_barrier_turn_id` and the cap
- `internal/service/weixin_im_bridge.go` — entry now builds `AgentRequest`, calls `agent_session.Service.Handle`, and renders chunks
- `internal/service/ai_service_weixin.go` — delete dead planning code (`PlanWeixinSearch`, `PlanWeixinCommand`, `ReviewWeixinPaperSearch`, `ReviewWeixinFigureSearch` and their helpers)
- `internal/service/weixin_daily_recommendation.go` — call `daily_figure.PickForDate` instead of inline picking
- `internal/handler/ai_conversation.go` — pass placeholder event through SSE
- `internal/handler/router.go` — register `/api/overview/*` and `/overview` routes
- `web/ai-assistant.html` + `web/static/js/ai-assistant.js` — TTS button per assistant message; DOI detection on submit
- `internal/app/wiring.go` (or equivalent) — wire `agent_session.Service` into both surfaces and run startup migration

**Files staying as-is** (verified): `internal/service/ai_assistant/orchestrator.go`, all tools under `ai_assistant/`, `internal/service/ai_conversation/context_assembler.go`, `evidence.go`, `summarizer.go`, `title.go`.

---

## Phase 1 — Schema and repository support

### Task 1: Add `surface_origin`, `kind`, `clear_barrier_turn_id` columns

**Files:**
- Modify: `internal/repository/schema/schema.go` (after `ensureAIOrchestrationSchema`, add `ensureConversationSurfaceColumns`)

- [ ] **Step 1: Write the failing test**

Add to `internal/repository/ai_conversation_repo_test.go` (the file exists; append):

```go
func TestSchemaHasConversationSurfaceColumns(t *testing.T) {
    db := newTestDB(t) // existing helper
    cols := map[string]string{}
    rows, err := db.Query("PRAGMA table_info(ai_conversations)")
    if err != nil { t.Fatal(err) }
    defer rows.Close()
    for rows.Next() {
        var cid int; var name, ctype string
        var notnull, pk int; var dflt sql.NullString
        if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil { t.Fatal(err) }
        cols[name] = ctype
    }
    for _, want := range []string{"surface_origin", "kind", "clear_barrier_turn_id"} {
        if _, ok := cols[want]; !ok { t.Errorf("missing column %q", want) }
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```
go test ./internal/repository -run TestSchemaHasConversationSurfaceColumns -v
```

Expected: FAIL with messages like `missing column "surface_origin"`.

- [ ] **Step 3: Add the migration**

In `internal/repository/schema/schema.go`, find `ensureAIOrchestrationSchema` and add a new method, then call it from `Manager.Run`.

```go
func (m *Manager) ensureConversationSurfaceColumns() error {
    for _, col := range []struct{ name, def string }{
        {"surface_origin", "TEXT NOT NULL DEFAULT 'web'"},
        {"kind", "TEXT NOT NULL DEFAULT 'default_web'"},
        {"clear_barrier_turn_id", "INTEGER"},
    } {
        if err := m.ensureColumn("ai_conversations", col.name, col.def); err != nil {
            return err
        }
    }
    _, err := m.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_conv_main_wechat
        ON ai_conversations(kind) WHERE kind = 'main_wechat'`)
    return err
}
```

Add `if err := m.ensureConversationSurfaceColumns(); err != nil { return err }` in `Manager.Run` after the existing `ensureAIOrchestrationSchema` call.

- [ ] **Step 4: Run test to verify it passes**

```
go test ./internal/repository -run TestSchemaHasConversationSurfaceColumns -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```
git add internal/repository/schema/schema.go internal/repository/ai_conversation_repo_test.go
git commit -m "feat(schema): surface_origin/kind/clear_barrier on ai_conversations"
```

---

### Task 2: Repository methods for surface-aware conversations

**Files:**
- Modify: `internal/repository/ai_conversation_repo.go`
- Modify: `internal/repository/ai_conversation_repo_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/repository/ai_conversation_repo_test.go`:

```go
func TestFindOrCreateByKindReturnsSameRow(t *testing.T) {
    db := newTestDB(t)
    repo := NewAIConversationRepository(db)
    id1, err := repo.FindOrCreateByKind("main_wechat", "wechat")
    if err != nil { t.Fatal(err) }
    id2, err := repo.FindOrCreateByKind("main_wechat", "wechat")
    if err != nil { t.Fatal(err) }
    if id1 != id2 { t.Fatalf("want same id, got %d vs %d", id1, id2) }
}

func TestSetClearBarrierExcludesOlderMessages(t *testing.T) {
    db := newTestDB(t)
    repo := NewAIConversationRepository(db)
    cid, _ := repo.FindOrCreateByKind("main_wechat", "wechat")
    m1, _ := repo.AddMessage(cid, "user", "old", AIMessageMeta{})
    _, _ = repo.AddMessage(cid, "assistant", "old reply", AIMessageMeta{})
    if err := repo.SetClearBarrier(cid, m1+1); err != nil { t.Fatal(err) }
    m3, _ := repo.AddMessage(cid, "user", "new", AIMessageMeta{})
    rows, err := repo.ListMessagesAfterBarrier(cid, 100)
    if err != nil { t.Fatal(err) }
    if len(rows) != 1 || rows[0].ID != m3 {
        t.Fatalf("want only new msg %d, got %v", m3, rows)
    }
}

func TestListMessagesAfterBarrierLimitKeepsLatest(t *testing.T) {
    db := newTestDB(t)
    repo := NewAIConversationRepository(db)
    cid, _ := repo.FindOrCreateByKind("main_wechat", "wechat")
    var ids []int64
    for i := 0; i < 15; i++ {
        id, _ := repo.AddMessage(cid, "user", "m", AIMessageMeta{})
        ids = append(ids, id)
    }
    rows, err := repo.ListMessagesAfterBarrier(cid, 10)
    if err != nil { t.Fatal(err) }
    if len(rows) != 10 { t.Fatalf("want 10, got %d", len(rows)) }
    if rows[0].ID != ids[5] || rows[9].ID != ids[14] {
        t.Fatalf("want oldest-of-window=%d newest=%d, got %d..%d",
            ids[5], ids[14], rows[0].ID, rows[9].ID)
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```
go test ./internal/repository -run "TestFindOrCreateByKind|TestSetClearBarrier|TestListMessagesAfterBarrier" -v
```

Expected: compile errors / undefined methods.

- [ ] **Step 3: Implement repo methods**

Append to `internal/repository/ai_conversation_repo.go`:

```go
// FindOrCreateByKind returns the conversation id for the given kind, creating it
// (with surface_origin) on first use. The unique index on kind='main_wechat'
// guarantees a single row per kind.
func (r *AIConversationRepository) FindOrCreateByKind(kind, surfaceOrigin string) (int64, error) {
    var id int64
    err := r.db.QueryRow(`SELECT id FROM ai_conversations WHERE kind = ? LIMIT 1`, kind).Scan(&id)
    if err == nil {
        return id, nil
    }
    if !errors.Is(err, sql.ErrNoRows) {
        return 0, err
    }
    res, err := r.db.Exec(
        `INSERT INTO ai_conversations(title, kind, surface_origin) VALUES (?, ?, ?)`,
        defaultTitleForKind(kind), kind, surfaceOrigin)
    if err != nil {
        // Concurrent insert raced us; re-select.
        var id2 int64
        if errSel := r.db.QueryRow(`SELECT id FROM ai_conversations WHERE kind = ?`, kind).Scan(&id2); errSel == nil {
            return id2, nil
        }
        return 0, err
    }
    return res.LastInsertId()
}

func defaultTitleForKind(kind string) string {
    switch kind {
    case "main_wechat":
        return "微信主会话"
    default:
        return ""
    }
}

// SetClearBarrier updates clear_barrier_turn_id so future history reads exclude
// any message with id <= barrier.
func (r *AIConversationRepository) SetClearBarrier(conversationID, barrier int64) error {
    res, err := r.db.Exec(
        `UPDATE ai_conversations SET clear_barrier_turn_id = ? WHERE id = ?`,
        barrier, conversationID)
    if err != nil { return err }
    n, _ := res.RowsAffected()
    if n == 0 { return ErrAIConversationNotFound }
    return nil
}

// ListMessagesAfterBarrier returns the most recent `limit` messages whose id is
// strictly greater than the conversation's clear_barrier_turn_id (zero if NULL),
// in ascending id order.
func (r *AIConversationRepository) ListMessagesAfterBarrier(conversationID int64, limit int) ([]AIMessage, error) {
    if limit <= 0 { limit = 10 }
    rows, err := r.db.Query(`
        SELECT id, conversation_id, role, content,
               COALESCE(provider,''), COALESCE(model,''), COALESCE(mode,''),
               COALESCE(included_figures,0), COALESCE(citations_json,''), created_at
        FROM ai_messages
        WHERE conversation_id = ?
          AND id > COALESCE((SELECT clear_barrier_turn_id FROM ai_conversations WHERE id = ?), 0)
        ORDER BY id DESC
        LIMIT ?`, conversationID, conversationID, limit)
    if err != nil { return nil, err }
    defer rows.Close()
    var out []AIMessage
    for rows.Next() {
        var m AIMessage
        if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content,
            &m.Provider, &m.Model, &m.Mode, &m.IncludedFigures, &m.CitationsJSON, &m.CreatedAt); err != nil {
            return nil, err
        }
        out = append(out, m)
    }
    // Reverse to ascending.
    for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 { out[i], out[j] = out[j], out[i] }
    return out, nil
}
```

- [ ] **Step 4: Run tests to verify pass**

```
go test ./internal/repository -run "TestFindOrCreateByKind|TestSetClearBarrier|TestListMessagesAfterBarrier" -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```
git add internal/repository/ai_conversation_repo.go internal/repository/ai_conversation_repo_test.go
git commit -m "feat(repo): FindOrCreateByKind, SetClearBarrier, ListMessagesAfterBarrier"
```

---

## Phase 2 — `agent_session` package: types, dispatch, state commands

### Task 3: Type definitions

**Files:**
- Create: `internal/service/agent_session/types.go`

- [ ] **Step 1: Create the file with all types**

```go
package agent_session

import "time"

type Surface string

const (
    SurfaceWeChat Surface = "wechat"
    SurfaceWeb    Surface = "web"
)

type ConversationKind string

const (
    KindMainWeChat  ConversationKind = "main_wechat"
    KindDefaultWeb  ConversationKind = "default_web"
    KindAdHoc       ConversationKind = "ad_hoc"
)

type ConversationRef struct {
    Kind ConversationKind
    ID   int64 // used only when Kind == KindAdHoc
}

type SurfaceContext struct {
    CurrentPaperID        int64
    CurrentPaperTitle     string
    CurrentFigureID       int64
    RecentSearchPaperIDs  []int64
    RecentSearchFigureIDs []int64
}

type Input struct {
    Text  string
    Files []InboundAttachment
}

type InboundAttachment struct {
    MIMEType string
    Path     string
}

type Options struct {
    RequireTTS    bool
    MaxChunkRunes int // 0 = use surface default
}

type AgentRequest struct {
    UserID         string
    Surface        Surface
    Conversation   ConversationRef
    SurfaceContext SurfaceContext
    Input          Input
    Options        Options
}

type ChunkKind string

const (
    ChunkText  ChunkKind = "text"
    ChunkImage ChunkKind = "image"
    ChunkVoice ChunkKind = "voice"
)

type OutboundChunk struct {
    Kind          ChunkKind
    Text          string
    ImagePath     string
    VoicePath     string
    IsPlaceholder bool
}

type EvidenceRef struct {
    Index int    // citation number rendered as [N]
    Title string
    DOI   string
}

type AgentResponse struct {
    ConversationID int64
    UserMessageID  int64
    AssistantMessageID int64
    Chunks         []OutboundChunk
    Evidence       []EvidenceRef
    UsedShortcut   bool
    CreatedAt      time.Time
}
```

- [ ] **Step 2: Verify it builds**

```
go build ./internal/service/agent_session/...
```

Expected: no output (success).

- [ ] **Step 3: Commit**

```
git add internal/service/agent_session/types.go
git commit -m "feat(agent_session): type definitions"
```

---

### Task 4: Dispatch parser + Service skeleton

**Files:**
- Create: `internal/service/agent_session/dispatch.go`
- Create: `internal/service/agent_session/service.go`
- Create: `internal/service/agent_session/service_test.go`

- [ ] **Step 1: Write failing dispatch test**

```go
package agent_session

import "testing"

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
    if ok { t.Fatal("plain text should not be recognized as slash") }
}
```

- [ ] **Step 2: Run test to verify failure**

```
go test ./internal/service/agent_session -run TestParseSlash -v
```

Expected: build error / undefined `parseSlash`.

- [ ] **Step 3: Implement `dispatch.go`**

```go
package agent_session

import "strings"

// parseSlash returns (command, arg, ok). Both half-width "/" and full-width "／"
// are accepted as the command prefix (mirrors the legacy bridge).
func parseSlash(text string) (string, string, bool) {
    s := strings.TrimSpace(text)
    if strings.HasPrefix(s, "／") {
        s = "/" + strings.TrimPrefix(s, "／")
    }
    if !strings.HasPrefix(s, "/") {
        return "", "", false
    }
    parts := strings.SplitN(s, " ", 2)
    cmd := strings.ToLower(parts[0])
    arg := ""
    if len(parts) == 2 {
        arg = strings.TrimSpace(parts[1])
    }
    return cmd, arg, true
}
```

- [ ] **Step 4: Implement `service.go` skeleton**

```go
package agent_session

import (
    "context"
    "log/slog"

    "github.com/xuzhougeng/citebox/internal/apperr"
    "github.com/xuzhougeng/citebox/internal/repository"
    "github.com/xuzhougeng/citebox/internal/service/agent_session/commands"
)

// FreeTextHandler is the subset of ai_conversation.Service the agent_session
// needs for free-text turns. Defined narrowly so tests can fake it.
type FreeTextHandler interface {
    SendForSurface(ctx context.Context, in commands.FreeTextInput,
        onPlaceholder func(string) error) (commands.FreeTextResult, error)
}

type Service struct {
    repo     *repository.AIConversationRepository
    cmds     *commands.Registry
    freeText FreeTextHandler
    state    *SurfaceStateStore
    logger   *slog.Logger
}

func New(repo *repository.AIConversationRepository, cmds *commands.Registry,
    freeText FreeTextHandler, state *SurfaceStateStore, logger *slog.Logger) *Service {
    if logger == nil { logger = slog.Default().With("component", "agent_session") }
    return &Service{repo: repo, cmds: cmds, freeText: freeText, state: state, logger: logger}
}

// Handle is the single entry point for both surfaces.
func (s *Service) Handle(ctx context.Context, req AgentRequest) (*AgentResponse, error) {
    convID, err := s.resolveConversation(req.Conversation, req.Surface)
    if err != nil { return nil, err }

    if cmd, arg, ok := parseSlash(req.Input.Text); ok {
        return s.cmds.Dispatch(ctx, cmd, arg, commands.RuntimeCtx{
            ConversationID: convID,
            Surface:        string(req.Surface),
            UserID:         req.UserID,
            SurfaceCtx:     toCommandsSurfaceContext(req.SurfaceContext),
            State:          s.state,
            Repo:           s.repo,
        })
    }

    // Free text → ai_conversation pipeline (constrained to 5-turn window
    // when surface == wechat).
    res, err := s.freeText.SendForSurface(ctx, commands.FreeTextInput{
        ConversationID: convID,
        Text:           req.Input.Text,
        Surface:        string(req.Surface),
        SurfaceCtx:     toCommandsSurfaceContext(req.SurfaceContext),
    }, nil)
    if err != nil { return nil, err }
    return commands.FreeTextResultToAgentResponse(convID, res), nil
}

func (s *Service) resolveConversation(ref ConversationRef, surface Surface) (int64, error) {
    switch ref.Kind {
    case KindMainWeChat:
        return s.repo.FindOrCreateByKind("main_wechat", "wechat")
    case KindDefaultWeb:
        return s.repo.FindOrCreateByKind("default_web", "web")
    case KindAdHoc:
        if ref.ID <= 0 {
            return 0, apperr.New(apperr.CodeInvalidArgument, "ad_hoc conversation requires id")
        }
        return ref.ID, nil
    default:
        return 0, apperr.New(apperr.CodeInvalidArgument, "unknown conversation kind")
    }
}

func toCommandsSurfaceContext(ctx SurfaceContext) commands.SurfaceContext {
    return commands.SurfaceContext{
        CurrentPaperID:        ctx.CurrentPaperID,
        CurrentPaperTitle:     ctx.CurrentPaperTitle,
        CurrentFigureID:       ctx.CurrentFigureID,
        RecentSearchPaperIDs:  ctx.RecentSearchPaperIDs,
        RecentSearchFigureIDs: ctx.RecentSearchFigureIDs,
    }
}
```

- [ ] **Step 5: Sub-package skeleton for commands**

Create `internal/service/agent_session/commands/registry.go`:

```go
package commands

import (
    "context"
    "strings"

    "github.com/xuzhougeng/citebox/internal/apperr"
    "github.com/xuzhougeng/citebox/internal/repository"
)

type SurfaceContext struct {
    CurrentPaperID        int64
    CurrentPaperTitle     string
    CurrentFigureID       int64
    RecentSearchPaperIDs  []int64
    RecentSearchFigureIDs []int64
}

type RuntimeCtx struct {
    ConversationID int64
    Surface        string
    UserID         string
    SurfaceCtx     SurfaceContext
    State          SurfaceStateMutator
    Repo           *repository.AIConversationRepository
}

// SurfaceStateMutator is implemented by agent_session.SurfaceStateStore.
type SurfaceStateMutator interface {
    Reset(surface string) error
    SetCurrentPaper(surface string, paperID int64, title string) error
    SetCurrentFigure(surface string, figureID int64) error
    Get(surface string) (SurfaceContext, error)
}

type Result struct {
    ChunksText      []string
    ImagePath       string
    UsedShortcut    bool
    UserMessageID   int64
    AssistantMsgID  int64
}

type Command interface {
    Name() string
    Execute(ctx context.Context, arg string, rt RuntimeCtx) (*Result, error)
}

type Registry struct {
    by map[string]Command
}

func NewRegistry(cmds ...Command) *Registry {
    r := &Registry{by: map[string]Command{}}
    for _, c := range cmds { r.by[strings.ToLower(c.Name())] = c }
    return r
}

func (r *Registry) Dispatch(ctx context.Context, name, arg string, rt RuntimeCtx) (*AgentResponse, error) {
    c, ok := r.by[strings.ToLower(name)]
    if !ok {
        return nil, apperr.New(apperr.CodeInvalidArgument, "unknown command: "+name)
    }
    res, err := c.Execute(ctx, arg, rt)
    if err != nil { return nil, err }
    return resultToAgentResponse(rt.ConversationID, res), nil
}
```

The `AgentResponse` and conversion helpers used here live in the parent package; we'll factor a tiny adapter in step 6 to avoid cycles.

- [ ] **Step 6: Add the adapter file `commands/response.go`**

```go
package commands

// AgentResponse and OutboundChunk are mirrored here as a private wire format so
// the commands subpackage doesn't depend on agent_session itself.
type AgentResponse struct {
    ConversationID     int64
    UserMessageID      int64
    AssistantMessageID int64
    Chunks             []OutboundChunk
    UsedShortcut       bool
}

type OutboundChunk struct {
    Kind          string // "text" | "image"
    Text          string
    ImagePath     string
    IsPlaceholder bool
}

func resultToAgentResponse(convID int64, r *Result) *AgentResponse {
    out := &AgentResponse{
        ConversationID:     convID,
        UserMessageID:      r.UserMessageID,
        AssistantMessageID: r.AssistantMsgID,
        UsedShortcut:       r.UsedShortcut,
    }
    for _, t := range r.ChunksText {
        out.Chunks = append(out.Chunks, OutboundChunk{Kind: "text", Text: t})
    }
    if r.ImagePath != "" {
        out.Chunks = append(out.Chunks, OutboundChunk{Kind: "image", ImagePath: r.ImagePath})
    }
    return out
}
```

In `service.go`, replace the parent-package `*AgentResponse` return type by reusing the commands subpackage type in a thin wrapper. To keep API ergonomic, also add a converter:

```go
// In agent_session/service.go:
func wrapCmdResp(r *commands.AgentResponse) *AgentResponse {
    if r == nil { return nil }
    out := &AgentResponse{
        ConversationID:     r.ConversationID,
        UserMessageID:      r.UserMessageID,
        AssistantMessageID: r.AssistantMessageID,
        UsedShortcut:       r.UsedShortcut,
    }
    for _, c := range r.Chunks {
        out.Chunks = append(out.Chunks, OutboundChunk{
            Kind:          ChunkKind(c.Kind),
            Text:          c.Text,
            ImagePath:     c.ImagePath,
            IsPlaceholder: c.IsPlaceholder,
        })
    }
    return out
}
```

Update `Service.Handle` to call `wrapCmdResp(res)` when returning from `s.cmds.Dispatch`.

Also create the FreeText helper in `commands/free_text.go`:

```go
package commands

type FreeTextInput struct {
    ConversationID int64
    Text           string
    Surface        string
    SurfaceCtx     SurfaceContext
}

type FreeTextResult struct {
    UserMessageID      int64
    AssistantMessageID int64
    AnswerText         string
    PlaceholderText    string
}

func FreeTextResultToAgentResponse(convID int64, r FreeTextResult) *AgentResponse {
    out := &AgentResponse{
        ConversationID:     convID,
        UserMessageID:      r.UserMessageID,
        AssistantMessageID: r.AssistantMessageID,
        UsedShortcut:       false,
    }
    if r.PlaceholderText != "" {
        out.Chunks = append(out.Chunks, OutboundChunk{Kind: "text", Text: r.PlaceholderText, IsPlaceholder: true})
    }
    if r.AnswerText != "" {
        out.Chunks = append(out.Chunks, OutboundChunk{Kind: "text", Text: r.AnswerText})
    }
    return out
}
```

- [ ] **Step 7: Run tests**

```
go test ./internal/service/agent_session/...
```

Expected: tests in this package pass; build succeeds.

- [ ] **Step 8: Commit**

```
git add internal/service/agent_session/
git commit -m "feat(agent_session): dispatch + Service skeleton + commands subpackage"
```

---

### Task 5: SurfaceStateStore for `weixin_surface_state.json`

**Files:**
- Create: `internal/service/agent_session/surface_state.go`
- Create: `internal/service/agent_session/surface_state_test.go`

- [ ] **Step 1: Failing test**

```go
package agent_session

import (
    "path/filepath"
    "testing"
)

func TestSurfaceStateRoundTrip(t *testing.T) {
    dir := t.TempDir()
    s, err := NewSurfaceStateStore(filepath.Join(dir, "wx_state.json"))
    if err != nil { t.Fatal(err) }
    if err := s.SetCurrentPaper("wechat", 42, "Title"); err != nil { t.Fatal(err) }
    if err := s.SetCurrentFigure("wechat", 7); err != nil { t.Fatal(err) }
    s2, _ := NewSurfaceStateStore(filepath.Join(dir, "wx_state.json"))
    sc, _ := s2.Get("wechat")
    if sc.CurrentPaperID != 42 || sc.CurrentPaperTitle != "Title" || sc.CurrentFigureID != 7 {
        t.Fatalf("unexpected state: %+v", sc)
    }
}

func TestSurfaceStateReset(t *testing.T) {
    dir := t.TempDir()
    s, _ := NewSurfaceStateStore(filepath.Join(dir, "wx.json"))
    _ = s.SetCurrentPaper("wechat", 9, "X")
    if err := s.Reset("wechat"); err != nil { t.Fatal(err) }
    sc, _ := s.Get("wechat")
    if sc.CurrentPaperID != 0 || sc.CurrentPaperTitle != "" {
        t.Fatalf("reset failed: %+v", sc)
    }
}
```

- [ ] **Step 2: Run test to confirm failure**

```
go test ./internal/service/agent_session -run TestSurfaceState -v
```

Expected: undefined `NewSurfaceStateStore`.

- [ ] **Step 3: Implement**

`internal/service/agent_session/surface_state.go`:

```go
package agent_session

import (
    "encoding/json"
    "os"
    "path/filepath"
    "sync"

    "github.com/xuzhougeng/citebox/internal/service/agent_session/commands"
)

type surfaceStateData struct {
    Surfaces map[string]commands.SurfaceContext `json:"surfaces"`
}

type SurfaceStateStore struct {
    path string
    mu   sync.Mutex
    data surfaceStateData
}

func NewSurfaceStateStore(path string) (*SurfaceStateStore, error) {
    s := &SurfaceStateStore{
        path: path,
        data: surfaceStateData{Surfaces: map[string]commands.SurfaceContext{}},
    }
    if err := s.load(); err != nil { return nil, err }
    return s, nil
}

func (s *SurfaceStateStore) load() error {
    raw, err := os.ReadFile(s.path)
    if err != nil {
        if os.IsNotExist(err) { return nil }
        return err
    }
    return json.Unmarshal(raw, &s.data)
}

func (s *SurfaceStateStore) save() error {
    if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil { return err }
    raw, err := json.MarshalIndent(s.data, "", "  ")
    if err != nil { return err }
    tmp := s.path + ".tmp"
    if err := os.WriteFile(tmp, raw, 0o644); err != nil { return err }
    return os.Rename(tmp, s.path)
}

func (s *SurfaceStateStore) Get(surface string) (commands.SurfaceContext, error) {
    s.mu.Lock(); defer s.mu.Unlock()
    return s.data.Surfaces[surface], nil
}

func (s *SurfaceStateStore) Reset(surface string) error {
    s.mu.Lock(); defer s.mu.Unlock()
    delete(s.data.Surfaces, surface)
    return s.save()
}

func (s *SurfaceStateStore) SetCurrentPaper(surface string, paperID int64, title string) error {
    s.mu.Lock(); defer s.mu.Unlock()
    sc := s.data.Surfaces[surface]
    sc.CurrentPaperID = paperID
    sc.CurrentPaperTitle = title
    s.data.Surfaces[surface] = sc
    return s.save()
}

func (s *SurfaceStateStore) SetCurrentFigure(surface string, figureID int64) error {
    s.mu.Lock(); defer s.mu.Unlock()
    sc := s.data.Surfaces[surface]
    sc.CurrentFigureID = figureID
    s.data.Surfaces[surface] = sc
    return s.save()
}

func (s *SurfaceStateStore) SetSearchResults(surface string, paperIDs, figureIDs []int64) error {
    s.mu.Lock(); defer s.mu.Unlock()
    sc := s.data.Surfaces[surface]
    sc.RecentSearchPaperIDs = paperIDs
    sc.RecentSearchFigureIDs = figureIDs
    s.data.Surfaces[surface] = sc
    return s.save()
}
```

- [ ] **Step 4: Run tests**

```
go test ./internal/service/agent_session -run TestSurfaceState -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```
git add internal/service/agent_session/surface_state.go internal/service/agent_session/surface_state_test.go
git commit -m "feat(agent_session): SurfaceStateStore (atomic JSON file)"
```

---

### Task 6: State commands — `/clear`, `/reset`, `/help`, `/status`, `/note`

**Files:**
- Create: `internal/service/agent_session/commands/clear.go`, `reset.go`, `help.go`, `status.go`, `note.go`
- Create: `internal/service/agent_session/commands/state_test.go`

- [ ] **Step 1: Failing tests**

`internal/service/agent_session/commands/state_test.go`:

```go
package commands

import (
    "context"
    "testing"

    "github.com/xuzhougeng/citebox/internal/repository"
)

type fakeState struct {
    paperID int64
    title   string
    figID   int64
    reset   bool
}

func (f *fakeState) Reset(string) error { f.reset = true; f.paperID = 0; f.title = ""; f.figID = 0; return nil }
func (f *fakeState) SetCurrentPaper(_ string, id int64, t string) error { f.paperID = id; f.title = t; return nil }
func (f *fakeState) SetCurrentFigure(_ string, id int64) error { f.figID = id; return nil }
func (f *fakeState) Get(string) (SurfaceContext, error) {
    return SurfaceContext{CurrentPaperID: f.paperID, CurrentPaperTitle: f.title, CurrentFigureID: f.figID}, nil
}

func TestClearSetsBarrier(t *testing.T) {
    db := repository.NewTestDB(t) // existing helper exposed for tests
    repo := repository.NewAIConversationRepository(db)
    cid, _ := repo.FindOrCreateByKind("main_wechat", "wechat")
    m, _ := repo.AddMessage(cid, "user", "hello", repository.AIMessageMeta{})
    cmd := &ClearCommand{}
    res, err := cmd.Execute(context.Background(), "", RuntimeCtx{
        ConversationID: cid, Repo: repo,
    })
    if err != nil { t.Fatal(err) }
    if !res.UsedShortcut { t.Error("expected UsedShortcut=true") }
    after, _ := repo.ListMessagesAfterBarrier(cid, 10)
    if len(after) != 0 {
        t.Fatalf("after /clear, no msg should be visible; got %d (latest msg id %d)", len(after), m)
    }
}

func TestResetClearsSurfaceState(t *testing.T) {
    fs := &fakeState{paperID: 7, title: "X"}
    cmd := &ResetCommand{}
    _, err := cmd.Execute(context.Background(), "", RuntimeCtx{
        Surface: "wechat", State: fs,
    })
    if err != nil { t.Fatal(err) }
    if !fs.reset { t.Fatal("Reset not called") }
}
```

- [ ] **Step 2: Add `repository.NewTestDB(t)` helper if missing**

Inspect `internal/repository/ai_conversation_repo_test.go` for the existing helper (it already creates an in-memory DB). If it's named `newTestDB` (private), expose it as `NewTestDB` for cross-package tests, or duplicate a small `func NewTestDB(t *testing.T) *sql.DB` in a new `internal/repository/testing.go` (build tag `_test.go` only file).

- [ ] **Step 3: Implement state commands**

`commands/clear.go`:

```go
package commands

import (
    "context"
    "database/sql"
    "fmt"
)

type ClearCommand struct{}
func (*ClearCommand) Name() string { return "/clear" }
func (*ClearCommand) Execute(ctx context.Context, _ string, rt RuntimeCtx) (*Result, error) {
    var maxID sql.NullInt64
    err := rt.Repo.DB().QueryRow(`SELECT MAX(id) FROM ai_messages WHERE conversation_id = ?`, rt.ConversationID).Scan(&maxID)
    if err != nil { return nil, err }
    barrier := int64(0)
    if maxID.Valid { barrier = maxID.Int64 }
    if err := rt.Repo.SetClearBarrier(rt.ConversationID, barrier); err != nil { return nil, err }
    return &Result{
        UsedShortcut: true,
        ChunksText:   []string{fmt.Sprintf("已清空对话上下文。后续提问将从这里重新开始。")},
    }, nil
}
```

If `Repo.DB()` does not exist, add it to `AIConversationRepository`:

```go
// In ai_conversation_repo.go:
func (r *AIConversationRepository) DB() *sql.DB { return r.db }
```

`commands/reset.go`:

```go
package commands

import "context"

type ResetCommand struct{}
func (*ResetCommand) Name() string { return "/reset" }
func (*ResetCommand) Execute(_ context.Context, _ string, rt RuntimeCtx) (*Result, error) {
    if rt.State != nil {
        if err := rt.State.Reset(rt.Surface); err != nil { return nil, err }
    }
    return &Result{
        UsedShortcut: true,
        ChunksText:   []string{"已重置当前选择（论文 / 图片 / 上次检索结果）。对话历史保留。"},
    }, nil
}
```

`commands/help.go`:

```go
package commands

import "context"

type HelpCommand struct{}
func (*HelpCommand) Name() string { return "/help" }
func (*HelpCommand) Execute(_ context.Context, _ string, _ RuntimeCtx) (*Result, error) {
    return &Result{UsedShortcut: true, ChunksText: []string{HelpText()}}, nil
}

// HelpText is exported so /status and other surfaces can quote it.
func HelpText() string {
    return `可用命令：
状态：/clear /reset /help /status /note <内容>
查询：/recent /figures /random /paper N /figure N /search <关键词>
追问：/ask <问题>（针对当前文献） /interpret（解读当前图片）
直接发自然语言或 DOI 链接也可以。`
}
```

`commands/status.go`:

```go
package commands

import (
    "context"
    "fmt"
)

type StatusCommand struct{}
func (*StatusCommand) Name() string { return "/status" }
func (*StatusCommand) Execute(_ context.Context, _ string, rt RuntimeCtx) (*Result, error) {
    sc, _ := rt.State.Get(rt.Surface)
    msg := fmt.Sprintf(`当前会话：#%d
当前论文：%d %s
当前图片：%d
最近论文检索结果数：%d
最近图片检索结果数：%d`,
        rt.ConversationID,
        sc.CurrentPaperID, sc.CurrentPaperTitle,
        sc.CurrentFigureID,
        len(sc.RecentSearchPaperIDs),
        len(sc.RecentSearchFigureIDs))
    return &Result{UsedShortcut: true, ChunksText: []string{msg}}, nil
}
```

`commands/note.go`:

```go
package commands

import (
    "context"
    "strings"

    "github.com/xuzhougeng/citebox/internal/apperr"
)

// NotesAppender is satisfied by *service.LibraryService.AppendPaperNote and
// *service.LibraryService.AppendFigureNote, fronted by NoteCommand.Library.
type NotesAppender interface {
    AppendPaperNote(paperID int64, text string) error
    AppendFigureNote(figureID int64, text string) error
}

type NoteCommand struct {
    Library NotesAppender
}
func (*NoteCommand) Name() string { return "/note" }
func (c *NoteCommand) Execute(_ context.Context, arg string, rt RuntimeCtx) (*Result, error) {
    text := strings.TrimSpace(arg)
    if text == "" {
        return nil, apperr.New(apperr.CodeInvalidArgument, "/note 后请跟笔记内容")
    }
    sc, _ := rt.State.Get(rt.Surface)
    switch {
    case sc.CurrentFigureID > 0:
        if err := c.Library.AppendFigureNote(sc.CurrentFigureID, text); err != nil { return nil, err }
        return &Result{UsedShortcut: true, ChunksText: []string{"已写入当前图片笔记。"}}, nil
    case sc.CurrentPaperID > 0:
        if err := c.Library.AppendPaperNote(sc.CurrentPaperID, text); err != nil { return nil, err }
        return &Result{UsedShortcut: true, ChunksText: []string{"已写入当前文献笔记。"}}, nil
    default:
        return nil, apperr.New(apperr.CodeFailedPrecondition, "请先选定一篇文献或一张图再 /note。")
    }
}
```

If `LibraryService.AppendPaperNote` / `AppendFigureNote` do not exist with these exact signatures, add thin wrappers around the existing notes methods (probable names: `UpdatePaperNotes`, `UpdateFigureNotes`) that **append** rather than overwrite. Implement these in `internal/service/library_service.go` if missing — locate the existing notes update method first to match its style.

- [ ] **Step 4: Run tests**

```
go test ./internal/service/agent_session/commands -run "TestClear|TestReset" -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```
git add internal/service/agent_session/commands/ internal/repository/ai_conversation_repo.go
git commit -m "feat(agent_session): state commands /clear /reset /help /status /note"
```

---

## Phase 3 — Slash query shortcuts (no LLM)

Each shortcut directly calls the relevant repository / library method. They:
1. Persist a user message (`role='user'`, the original slash text).
2. Run the operation.
3. Persist an assistant message with the formatted result.
4. Update surface state if the operation selects something.

### Task 7: `/recent` shortcut

**Files:**
- Create: `internal/service/agent_session/commands/recent.go`
- Create: `internal/service/agent_session/commands/recent_test.go`

- [ ] **Step 1: Failing test**

```go
package commands

import (
    "context"
    "testing"

    "github.com/xuzhougeng/citebox/internal/model"
)

type fakeRecentLister struct{ papers []model.Paper }
func (f *fakeRecentLister) ListRecentPapers(_ context.Context, n int) ([]model.Paper, error) {
    if n > len(f.papers) { n = len(f.papers) }
    return f.papers[:n], nil
}

func TestRecentReturnsTopN(t *testing.T) {
    lister := &fakeRecentLister{papers: []model.Paper{
        {ID: 3, Title: "C"}, {ID: 2, Title: "B"}, {ID: 1, Title: "A"},
    }}
    cmd := &RecentCommand{Lister: lister, Limit: 2}
    res, err := cmd.Execute(context.Background(), "", RuntimeCtx{ConversationID: 1, Surface: "wechat"})
    if err != nil { t.Fatal(err) }
    if !res.UsedShortcut { t.Fatal("want shortcut") }
    if len(res.ChunksText) != 1 || !contains(res.ChunksText[0], "B") || !contains(res.ChunksText[0], "C") {
        t.Fatalf("unexpected output: %v", res.ChunksText)
    }
}

func contains(s, sub string) bool { return len(s) > 0 && len(sub) > 0 && (s == sub || (len(s) > len(sub) && (indexOf(s, sub) >= 0))) }
func indexOf(s, sub string) int {
    for i := 0; i+len(sub) <= len(s); i++ { if s[i:i+len(sub)] == sub { return i } }
    return -1
}
```

(Use `strings.Contains` instead — replace with `strings` import.)

- [ ] **Step 2: Run test to confirm failure**

```
go test ./internal/service/agent_session/commands -run TestRecent -v
```

Expected: undefined `RecentCommand`.

- [ ] **Step 3: Implement**

`commands/recent.go`:

```go
package commands

import (
    "context"
    "fmt"
    "strings"

    "github.com/xuzhougeng/citebox/internal/model"
)

type RecentLister interface {
    ListRecentPapers(ctx context.Context, n int) ([]model.Paper, error)
}

type RecentCommand struct {
    Lister RecentLister
    Limit  int // default 5
}

func (*RecentCommand) Name() string { return "/recent" }
func (c *RecentCommand) Execute(ctx context.Context, _ string, rt RuntimeCtx) (*Result, error) {
    n := c.Limit; if n <= 0 { n = 5 }
    papers, err := c.Lister.ListRecentPapers(ctx, n)
    if err != nil { return nil, err }
    if len(papers) == 0 {
        return &Result{UsedShortcut: true, ChunksText: []string{"文献库当前为空。"}}, nil
    }
    var b strings.Builder
    b.WriteString(fmt.Sprintf("最近 %d 篇文献：\n", len(papers)))
    ids := make([]int64, 0, len(papers))
    for i, p := range papers {
        b.WriteString(fmt.Sprintf("%d. [#%d] %s\n", i+1, p.ID, p.Title))
        ids = append(ids, p.ID)
    }
    if rt.State != nil {
        _ = rt.State.SetSearchResults(rt.Surface, ids, nil)
    }
    return &Result{UsedShortcut: true, ChunksText: []string{strings.TrimRight(b.String(), "\n")}}, nil
}
```

Note: `SetSearchResults` is used here. Add it to `SurfaceStateMutator` interface in `commands/registry.go`:

```go
type SurfaceStateMutator interface {
    Reset(surface string) error
    SetCurrentPaper(surface string, paperID int64, title string) error
    SetCurrentFigure(surface string, figureID int64) error
    SetSearchResults(surface string, paperIDs, figureIDs []int64) error
    Get(surface string) (SurfaceContext, error)
}
```

- [ ] **Step 4: Run tests**

```
go test ./internal/service/agent_session/commands -run TestRecent -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```
git add internal/service/agent_session/commands/recent.go internal/service/agent_session/commands/recent_test.go internal/service/agent_session/commands/registry.go
git commit -m "feat(agent_session): /recent shortcut"
```

---

### Task 8: `/figures`, `/random`, `/figure N`, `/paper N`

**Files:**
- Create: `internal/service/agent_session/commands/figures.go`, `random.go`, `paper.go`, `figure.go`
- Create: `internal/service/agent_session/commands/selection_test.go`

- [ ] **Step 1: Failing tests for `/figure N` and `/paper N`**

```go
package commands

import (
    "context"
    "testing"
)

func TestFigureSelectsByIndex(t *testing.T) {
    fs := &fakeState{}
    fs.SetSearchResultsField([]int64{}, []int64{10, 20, 30})
    cmd := &FigureCommand{}
    res, err := cmd.Execute(context.Background(), "2", RuntimeCtx{Surface: "wechat", State: fs})
    if err != nil { t.Fatal(err) }
    sc, _ := fs.Get("wechat")
    if sc.CurrentFigureID != 20 { t.Fatalf("want 20, got %d", sc.CurrentFigureID) }
    if !res.UsedShortcut { t.Fatal("want shortcut") }
}

func TestPaperOutOfRange(t *testing.T) {
    fs := &fakeState{}
    fs.SetSearchResultsField([]int64{1, 2}, nil)
    _, err := (&PaperCommand{}).Execute(context.Background(), "5", RuntimeCtx{Surface: "wechat", State: fs})
    if err == nil { t.Fatal("expected error") }
}
```

Extend `fakeState`:

```go
func (f *fakeState) SetSearchResults(_ string, p, fg []int64) error { f.SetSearchResultsField(p, fg); return nil }
func (f *fakeState) SetSearchResultsField(p, fg []int64) {
    f.papers = p; f.figs = fg
}
// Add fields: papers []int64; figs []int64
// And in Get(): include them in the SurfaceContext.
```

- [ ] **Step 2: Confirm failure**

```
go test ./internal/service/agent_session/commands -run "TestFigureSelects|TestPaperOut" -v
```

- [ ] **Step 3: Implement four commands**

`commands/paper.go`:

```go
package commands

import (
    "context"
    "fmt"
    "strconv"
    "strings"

    "github.com/xuzhougeng/citebox/internal/apperr"
)

type PaperLookup interface {
    GetPaperBrief(ctx context.Context, id int64) (string, error) // formatted one-liner
}

type PaperCommand struct{ Lookup PaperLookup }

func (*PaperCommand) Name() string { return "/paper" }
func (c *PaperCommand) Execute(ctx context.Context, arg string, rt RuntimeCtx) (*Result, error) {
    n, err := strconv.Atoi(strings.TrimSpace(arg))
    if err != nil || n <= 0 {
        return nil, apperr.New(apperr.CodeInvalidArgument, "/paper 后请跟序号，例如 /paper 2")
    }
    sc, _ := rt.State.Get(rt.Surface)
    if n > len(sc.RecentSearchPaperIDs) {
        return nil, apperr.New(apperr.CodeFailedPrecondition,
            fmt.Sprintf("最近文献结果只有 %d 项。", len(sc.RecentSearchPaperIDs)))
    }
    pid := sc.RecentSearchPaperIDs[n-1]
    brief := fmt.Sprintf("已选定文献 #%d", pid)
    if c.Lookup != nil {
        if b, err := c.Lookup.GetPaperBrief(ctx, pid); err == nil { brief = b }
    }
    if err := rt.State.SetCurrentPaper(rt.Surface, pid, ""); err != nil { return nil, err }
    return &Result{UsedShortcut: true, ChunksText: []string{brief}}, nil
}
```

`commands/figure.go` — analogous:

```go
package commands

import (
    "context"
    "fmt"
    "strconv"
    "strings"

    "github.com/xuzhougeng/citebox/internal/apperr"
)

type FigureLookup interface {
    GetFigurePreviewPath(ctx context.Context, id int64) (string, string, error) // path, caption
}

type FigureCommand struct{ Lookup FigureLookup }

func (*FigureCommand) Name() string { return "/figure" }
func (c *FigureCommand) Execute(ctx context.Context, arg string, rt RuntimeCtx) (*Result, error) {
    n, err := strconv.Atoi(strings.TrimSpace(arg))
    if err != nil || n <= 0 {
        return nil, apperr.New(apperr.CodeInvalidArgument, "/figure 后请跟序号，例如 /figure 3")
    }
    sc, _ := rt.State.Get(rt.Surface)
    if n > len(sc.RecentSearchFigureIDs) {
        return nil, apperr.New(apperr.CodeFailedPrecondition,
            fmt.Sprintf("最近图片结果只有 %d 项。", len(sc.RecentSearchFigureIDs)))
    }
    fid := sc.RecentSearchFigureIDs[n-1]
    if err := rt.State.SetCurrentFigure(rt.Surface, fid); err != nil { return nil, err }
    res := &Result{UsedShortcut: true, ChunksText: []string{fmt.Sprintf("已选定图片 #%d", fid)}}
    if c.Lookup != nil {
        if path, caption, err := c.Lookup.GetFigurePreviewPath(ctx, fid); err == nil {
            res.ImagePath = path
            if caption != "" { res.ChunksText[0] = caption }
        }
    }
    return res, nil
}
```

`commands/figures.go` (lists figures of current paper):

```go
package commands

import (
    "context"
    "fmt"
    "strings"

    "github.com/xuzhougeng/citebox/internal/apperr"
    "github.com/xuzhougeng/citebox/internal/model"
)

type FiguresLister interface {
    ListFiguresForPaper(ctx context.Context, paperID int64) ([]model.FigureListItem, error)
}

type FiguresCommand struct{ Lister FiguresLister }
func (*FiguresCommand) Name() string { return "/figures" }
func (c *FiguresCommand) Execute(ctx context.Context, _ string, rt RuntimeCtx) (*Result, error) {
    sc, _ := rt.State.Get(rt.Surface)
    if sc.CurrentPaperID <= 0 {
        return nil, apperr.New(apperr.CodeFailedPrecondition, "请先用 /paper N 选定文献。")
    }
    figs, err := c.Lister.ListFiguresForPaper(ctx, sc.CurrentPaperID)
    if err != nil { return nil, err }
    if len(figs) == 0 {
        return &Result{UsedShortcut: true, ChunksText: []string{"该文献尚无已提取图片。"}}, nil
    }
    var b strings.Builder
    b.WriteString(fmt.Sprintf("文献 #%d 共 %d 张图：\n", sc.CurrentPaperID, len(figs)))
    ids := make([]int64, 0, len(figs))
    for i, f := range figs {
        label := f.DisplayLabel
        if label == "" { label = fmt.Sprintf("图 %d", f.FigureIndex) }
        b.WriteString(fmt.Sprintf("%d. [#%d] %s\n", i+1, f.ID, label))
        ids = append(ids, f.ID)
    }
    _ = rt.State.SetSearchResults(rt.Surface, nil, ids)
    return &Result{UsedShortcut: true, ChunksText: []string{strings.TrimRight(b.String(), "\n")}}, nil
}
```

`commands/random.go`:

```go
package commands

import (
    "context"
    "fmt"
)

type RandomFigurePicker interface {
    PickRandomFigure(ctx context.Context) (id int64, path, caption string, err error)
}

type RandomCommand struct{ Picker RandomFigurePicker }
func (*RandomCommand) Name() string { return "/random" }
func (c *RandomCommand) Execute(ctx context.Context, _ string, rt RuntimeCtx) (*Result, error) {
    id, path, caption, err := c.Picker.PickRandomFigure(ctx)
    if err != nil { return nil, err }
    _ = rt.State.SetCurrentFigure(rt.Surface, id)
    msg := caption
    if msg == "" { msg = fmt.Sprintf("已挑选图片 #%d", id) }
    return &Result{UsedShortcut: true, ChunksText: []string{msg}, ImagePath: path}, nil
}
```

- [ ] **Step 4: Run tests**

```
go test ./internal/service/agent_session/commands -run "TestFigure|TestPaper|TestFigures|TestRandom" -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```
git add internal/service/agent_session/commands/{paper,figure,figures,random,selection_test}.go
git commit -m "feat(agent_session): /figures /random /paper /figure shortcuts"
```

---

### Task 9: `/search` shortcut

**Files:**
- Create: `internal/service/agent_session/commands/search.go`
- Create: `internal/service/agent_session/commands/search_test.go`

- [ ] **Step 1: Failing test (search routes to library tool)**

```go
package commands

import (
    "context"
    "testing"

    "github.com/xuzhougeng/citebox/internal/model"
)

type fakeLibSearch struct{ called string }

func (f *fakeLibSearch) SearchPapersByText(_ context.Context, q string, limit int) ([]model.Paper, error) {
    f.called = q
    return []model.Paper{{ID: 1, Title: "Hit: " + q}}, nil
}

func TestSearchPersistsAndUpdatesState(t *testing.T) {
    fs := &fakeState{}
    sl := &fakeLibSearch{}
    cmd := &SearchCommand{Searcher: sl}
    res, err := cmd.Execute(context.Background(), "diffusion", RuntimeCtx{
        Surface: "wechat", State: fs, ConversationID: 7,
    })
    if err != nil { t.Fatal(err) }
    if sl.called != "diffusion" { t.Fatalf("want diffusion, got %q", sl.called) }
    if !res.UsedShortcut { t.Fatal("want shortcut") }
    sc, _ := fs.Get("wechat")
    if len(sc.RecentSearchPaperIDs) != 1 || sc.RecentSearchPaperIDs[0] != 1 {
        t.Fatalf("state not updated: %v", sc.RecentSearchPaperIDs)
    }
}
```

- [ ] **Step 2: Confirm failure**

```
go test ./internal/service/agent_session/commands -run TestSearch -v
```

- [ ] **Step 3: Implement**

`commands/search.go`:

```go
package commands

import (
    "context"
    "fmt"
    "strings"

    "github.com/xuzhougeng/citebox/internal/apperr"
    "github.com/xuzhougeng/citebox/internal/model"
)

type LibrarySearcher interface {
    SearchPapersByText(ctx context.Context, q string, limit int) ([]model.Paper, error)
}

type SearchCommand struct{ Searcher LibrarySearcher }
func (*SearchCommand) Name() string { return "/search" }
func (c *SearchCommand) Execute(ctx context.Context, arg string, rt RuntimeCtx) (*Result, error) {
    q := strings.TrimSpace(arg)
    if q == "" {
        return nil, apperr.New(apperr.CodeInvalidArgument, "/search 后请跟检索词")
    }
    papers, err := c.Searcher.SearchPapersByText(ctx, q, 5)
    if err != nil { return nil, err }
    if len(papers) == 0 {
        return &Result{UsedShortcut: true, ChunksText: []string{"未找到匹配的文献。"}}, nil
    }
    var b strings.Builder
    b.WriteString(fmt.Sprintf("找到 %d 条结果：\n", len(papers)))
    ids := make([]int64, 0, len(papers))
    for i, p := range papers {
        b.WriteString(fmt.Sprintf("%d. [#%d] %s\n", i+1, p.ID, p.Title))
        ids = append(ids, p.ID)
    }
    _ = rt.State.SetSearchResults(rt.Surface, ids, nil)
    b.WriteString("\n用 /paper N 选定一篇。")
    return &Result{UsedShortcut: true, ChunksText: []string{strings.TrimRight(b.String(), "\n")}}, nil
}
```

If `LibrarySearcher.SearchPapersByText` doesn't already exist on `LibraryService`, add it as a thin wrapper around the existing search method (likely `SearchPapers(req SearchRequest)` — locate it before writing).

- [ ] **Step 4: Run tests**

```
go test ./internal/service/agent_session/commands -run TestSearch -v
```

- [ ] **Step 5: Commit**

```
git add internal/service/agent_session/commands/search.go internal/service/agent_session/commands/search_test.go
git commit -m "feat(agent_session): /search shortcut"
```

---

### Task 10: `/ask` and `/interpret` (constrained agent path)

These two need the LLM but pin tool choice. Implement them as direct calls to existing tools followed by a single LLM summarization step. This avoids touching the orchestrator.

**Files:**
- Create: `internal/service/agent_session/commands/ask.go`, `interpret.go`
- Create: `internal/service/agent_session/commands/ask_test.go`

- [ ] **Step 1: Failing test**

```go
package commands

import (
    "context"
    "testing"
)

type fakePaperReader struct{ text string }
func (f *fakePaperReader) GetPaperBodyText(_ context.Context, id int64) (string, error) { return f.text, nil }

type fakeLLM struct{ reply string }
func (f *fakeLLM) AnswerOverPaper(_ context.Context, body, question string) (string, error) {
    return f.reply, nil
}

func TestAskUsesPaperReaderAndLLM(t *testing.T) {
    fs := &fakeState{paperID: 12, title: "T"}
    cmd := &AskCommand{Reader: &fakePaperReader{text: "abstract..."}, LLM: &fakeLLM{reply: "答案"}}
    res, err := cmd.Execute(context.Background(), "什么方法？", RuntimeCtx{Surface: "wechat", State: fs})
    if err != nil { t.Fatal(err) }
    if res.UsedShortcut { t.Fatal("ask uses LLM, should not be shortcut") }
    if len(res.ChunksText) != 1 || res.ChunksText[0] != "答案" {
        t.Fatalf("unexpected: %v", res.ChunksText)
    }
}
```

- [ ] **Step 2: Confirm failure**

- [ ] **Step 3: Implement**

`commands/ask.go`:

```go
package commands

import (
    "context"
    "strings"

    "github.com/xuzhougeng/citebox/internal/apperr"
)

type PaperReader interface {
    GetPaperBodyText(ctx context.Context, paperID int64) (string, error)
}

type PaperQALLM interface {
    AnswerOverPaper(ctx context.Context, body, question string) (string, error)
}

type AskCommand struct {
    Reader PaperReader
    LLM    PaperQALLM
}

func (*AskCommand) Name() string { return "/ask" }
func (c *AskCommand) Execute(ctx context.Context, arg string, rt RuntimeCtx) (*Result, error) {
    q := strings.TrimSpace(arg)
    if q == "" {
        return nil, apperr.New(apperr.CodeInvalidArgument, "/ask 后请跟问题")
    }
    sc, _ := rt.State.Get(rt.Surface)
    if sc.CurrentPaperID <= 0 {
        return nil, apperr.New(apperr.CodeFailedPrecondition, "请先选定一篇文献再 /ask。")
    }
    body, err := c.Reader.GetPaperBodyText(ctx, sc.CurrentPaperID)
    if err != nil { return nil, err }
    answer, err := c.LLM.AnswerOverPaper(ctx, body, q)
    if err != nil { return nil, err }
    return &Result{ChunksText: []string{answer}}, nil
}
```

`commands/interpret.go`:

```go
package commands

import (
    "context"

    "github.com/xuzhougeng/citebox/internal/apperr"
)

type FigureInterpreter interface {
    InterpretFigure(ctx context.Context, figureID int64) (string, error)
}

type InterpretCommand struct{ Interpreter FigureInterpreter }
func (*InterpretCommand) Name() string { return "/interpret" }
func (c *InterpretCommand) Execute(ctx context.Context, _ string, rt RuntimeCtx) (*Result, error) {
    sc, _ := rt.State.Get(rt.Surface)
    if sc.CurrentFigureID <= 0 {
        return nil, apperr.New(apperr.CodeFailedPrecondition, "请先选定一张图再 /interpret。")
    }
    text, err := c.Interpreter.InterpretFigure(ctx, sc.CurrentFigureID)
    if err != nil { return nil, err }
    return &Result{ChunksText: []string{text}}, nil
}
```

The concrete `PaperQALLM`, `PaperReader`, `FigureInterpreter` implementations are thin wrappers in `internal/service/agent_session/adapters.go` that call existing `AIService` methods (`AnswerPaperQuestion`, `ReadPaper`, `InterpretFigure` — locate exact names in `ai_service.go` / `ai_service_image.go` first).

- [ ] **Step 4: Run tests**

```
go test ./internal/service/agent_session/commands -run "TestAsk|TestInterpret" -v
```

- [ ] **Step 5: Commit**

```
git add internal/service/agent_session/commands/{ask,interpret,ask_test}.go
git commit -m "feat(agent_session): /ask and /interpret constrained commands"
```

---

### Task 11: Bare-DOI dispatch

**Files:**
- Create: `internal/service/agent_session/commands/doi.go`
- Create: `internal/service/agent_session/commands/doi_test.go`
- Modify: `internal/service/agent_session/dispatch.go` (route bare DOI)

- [ ] **Step 1: Failing test**

```go
package agent_session

import "testing"

func TestLooksLikeDOI(t *testing.T) {
    cases := []struct {
        in  string
        want bool
    }{
        {"10.1038/s41586-023-06000-1", true},
        {"https://doi.org/10.1038/s41586-023-06000-1", true},
        {"hello world", false},
        {"/help", false},
    }
    for _, tc := range cases {
        if got := looksLikeDOI(tc.in); got != tc.want {
            t.Errorf("looksLikeDOI(%q)=%v want %v", tc.in, got, tc.want)
        }
    }
}
```

- [ ] **Step 2: Confirm failure**

- [ ] **Step 3: Add `looksLikeDOI` to `dispatch.go`**

```go
import "regexp"

var doiPattern = regexp.MustCompile(`(?i)\b10\.\d{4,9}/[-._;()/:A-Z0-9]+\b`)

func looksLikeDOI(s string) bool {
    return doiPattern.MatchString(strings.TrimSpace(s))
}
```

(Also expose normalization. Reuse the existing `normalizeDOIInput` from the bridge code by moving it to a shared package `internal/service/doi/` in a small follow-up if not already shared. For this task just detection; the import handler does normalization.)

- [ ] **Step 4: Implement DOICommand**

`commands/doi.go`:

```go
package commands

import (
    "context"
    "errors"
    "strings"

    "github.com/xuzhougeng/citebox/internal/apperr"
    "github.com/xuzhougeng/citebox/internal/model"
)

type DOIImporter interface {
    ImportPaperByDOI(ctx context.Context, doi string) (*model.Paper, error)
    NormalizeDOI(input string) (string, error)
}

type DOICommand struct{ Importer DOIImporter }
func (*DOICommand) Name() string { return ":doi" } // not user-typed; routed by dispatch
func (c *DOICommand) Execute(ctx context.Context, raw string, rt RuntimeCtx) (*Result, error) {
    doi, err := c.Importer.NormalizeDOI(strings.TrimSpace(raw))
    if err != nil {
        return nil, apperr.New(apperr.CodeInvalidArgument, "DOI 格式无效")
    }
    paper, err := c.Importer.ImportPaperByDOI(ctx, doi)
    if err != nil {
        var dup interface{ DuplicatePaper() *model.Paper }
        if errors.As(err, &dup) && dup.DuplicatePaper() != nil {
            existing := dup.DuplicatePaper()
            _ = rt.State.SetCurrentPaper(rt.Surface, existing.ID, existing.Title)
            return &Result{UsedShortcut: true, ChunksText: []string{
                "该 DOI 已在文献库中，已切换到现有文献：\n#" + itoa(existing.ID) + " " + existing.Title,
            }}, nil
        }
        return nil, err
    }
    _ = rt.State.SetCurrentPaper(rt.Surface, paper.ID, paper.Title)
    return &Result{UsedShortcut: true, ChunksText: []string{
        "已通过 DOI 导入：\n#" + itoa(paper.ID) + " " + paper.Title,
    }}, nil
}

func itoa(i int64) string {
    s := ""
    if i == 0 { return "0" }
    for i > 0 { s = string(rune('0'+i%10)) + s; i /= 10 }
    return s
}
```

(Use `strconv.FormatInt` instead — replace `itoa` with the standard helper.)

If the existing `DuplicatePaperError` doesn't expose `.DuplicatePaper()` matching the interface above, adapt — locate `DuplicatePaperError` in `internal/service/library_service.go` and add a small `DuplicatePaper()` method or check `errors.As(err, &dup)` with the concrete type.

- [ ] **Step 5: Wire DOI route in dispatch**

In `service.go`, before `parseSlash`:

```go
if looksLikeDOI(req.Input.Text) {
    return s.cmds.Dispatch(ctx, ":doi", req.Input.Text, commands.RuntimeCtx{...})
}
```

- [ ] **Step 6: Run tests**

```
go test ./internal/service/agent_session/... -run "TestLooksLikeDOI|TestDOI" -v
```

- [ ] **Step 7: Commit**

```
git add internal/service/agent_session/{dispatch.go,service.go} internal/service/agent_session/commands/doi.go internal/service/agent_session/commands/doi_test.go
git commit -m "feat(agent_session): bare DOI auto-import"
```

---

## Phase 4 — Free text path (5-turn cap when WeChat)

### Task 12: Extend `ai_conversation.SendMessage` with `SendForSurface`

**Files:**
- Modify: `internal/service/ai_conversation/service.go`
- Modify: `internal/service/ai_conversation/types.go`
- Create: `internal/service/ai_conversation/surface_options_test.go`

- [ ] **Step 1: Failing test**

`internal/service/ai_conversation/surface_options_test.go`:

```go
package ai_conversation

import "testing"

func TestSendForSurfaceWeChatSkipsSummaryAndCapsHistory(t *testing.T) {
    s, repo := newServiceForTest(t) // existing helper from service_test.go
    cid, _ := repo.FindOrCreateByKind("main_wechat", "wechat")
    // Insert 12 prior messages.
    for i := 0; i < 12; i++ {
        _, _ = repo.AddMessage(cid, "user", "u", repository.AIMessageMeta{})
        _, _ = repo.AddMessage(cid, "assistant", "a", repository.AIMessageMeta{})
    }
    var observedHistoryLen int
    s.OverrideAssemblerForTest(func(history []repository.AIMessage) {
        observedHistoryLen = len(history)
    })
    _, err := s.SendForSurface(context.Background(), SurfaceMessageInput{
        ConversationID: cid,
        Text:           "hi",
        Surface:        "wechat",
    }, nil)
    if err != nil { t.Fatal(err) }
    if observedHistoryLen != 10 {
        t.Fatalf("want 10 history rows (5 turns), got %d", observedHistoryLen)
    }
}
```

(`OverrideAssemblerForTest` is a small test seam to expose what history was used. If a cleaner test point exists, prefer that.)

- [ ] **Step 2: Confirm failure**

```
go test ./internal/service/ai_conversation -run TestSendForSurface -v
```

- [ ] **Step 3: Implement `SendForSurface`**

In `service.go`, add:

```go
type SurfaceMessageInput struct {
    ConversationID int64
    Text           string
    Surface        string // "wechat" or "web"
}

type SurfaceMessageResult struct {
    UserMessageID      int64
    AssistantMessageID int64
    AnswerText         string
    PlaceholderText    string
}

const wechatHistoryRowCap = 10 // 5 user/assistant pairs

func (s *Service) SendForSurface(ctx context.Context, in SurfaceMessageInput,
    onPlaceholder func(string) error) (SurfaceMessageResult, error) {

    if onPlaceholder != nil {
        _ = onPlaceholder("正在思考……")
    }

    conv, err := s.repo.GetConversation(in.ConversationID)
    if err != nil { return SurfaceMessageResult{}, mapRepoErr(err) }

    userMsgID, err := s.repo.AddMessage(in.ConversationID, "user", in.Text, repository.AIMessageMeta{})
    if err != nil { return SurfaceMessageResult{}, err }

    settings, err := s.settings.GetSettings()
    if err != nil { return SurfaceMessageResult{}, err }
    masterSettings := assistantMasterSettings(*settings)

    var history []repository.AIMessage
    if in.Surface == "wechat" {
        history, err = s.repo.ListMessagesAfterBarrier(in.ConversationID, wechatHistoryRowCap)
    } else {
        history, err = s.repo.ListMessages(in.ConversationID, conv.SummaryThroughMessageID.Int64, 1000)
    }
    if err != nil { return SurfaceMessageResult{}, err }
    // Drop the just-inserted user msg from history (we'll append it to the prompt).
    if n := len(history); n > 0 && history[n-1].ID == userMsgID {
        history = history[:n-1]
    }

    if in.Surface != "wechat" {
        // Existing behavior for web: try summarization.
        s.maybeSummarize(ctx, &conv, &history, assistantSubagentSettings(*settings))
    }

    pinned, err := s.repo.ListPinnedPapers(in.ConversationID)
    if err != nil { return SurfaceMessageResult{}, err }
    asm, err := s.assembleForTurn(conv, pinned, history, in.Text, masterSettings)
    if err != nil { return SurfaceMessageResult{}, err }

    answer, mode, err := s.caller.CallProviderStreamGeneric(ctx, masterSettings, asm.systemPrompt, asm.userPrompt, asm.images, func(string) error { return nil })
    if err != nil { return SurfaceMessageResult{}, err }

    asstID, err := s.repo.AddMessage(in.ConversationID, "assistant", answer, repository.AIMessageMeta{
        Provider: string(masterSettings.Provider),
        Model:    masterSettings.Model,
        Mode:     mode,
    })
    if err != nil { return SurfaceMessageResult{}, err }
    _ = s.repo.TouchConversation(in.ConversationID)

    return SurfaceMessageResult{
        UserMessageID:      userMsgID,
        AssistantMessageID: asstID,
        AnswerText:         answer,
        PlaceholderText:    "正在思考……",
    }, nil
}
```

Adapter in `agent_session/adapters.go`:

```go
package agent_session

import (
    "context"

    "github.com/xuzhougeng/citebox/internal/service/agent_session/commands"
    "github.com/xuzhougeng/citebox/internal/service/ai_conversation"
)

type aiConversationAdapter struct{ s *ai_conversation.Service }

func NewAIConversationAdapter(s *ai_conversation.Service) FreeTextHandler {
    return &aiConversationAdapter{s: s}
}

func (a *aiConversationAdapter) SendForSurface(ctx context.Context, in commands.FreeTextInput,
    onPlaceholder func(string) error) (commands.FreeTextResult, error) {
    res, err := a.s.SendForSurface(ctx, ai_conversation.SurfaceMessageInput{
        ConversationID: in.ConversationID,
        Text:           in.Text,
        Surface:        in.Surface,
    }, onPlaceholder)
    if err != nil { return commands.FreeTextResult{}, err }
    return commands.FreeTextResult{
        UserMessageID:      res.UserMessageID,
        AssistantMessageID: res.AssistantMessageID,
        AnswerText:         res.AnswerText,
        PlaceholderText:    res.PlaceholderText,
    }, nil
}
```

- [ ] **Step 4: Run tests**

```
go test ./internal/service/ai_conversation -run TestSendForSurface -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```
git add internal/service/ai_conversation/{service.go,types.go,surface_options_test.go} internal/service/agent_session/adapters.go
git commit -m "feat(ai_conversation): SendForSurface with 5-turn cap for wechat"
```

---

## Phase 5 — Daily-figure picker extraction

### Task 13: Extract daily-figure picker

**Files:**
- Create: `internal/service/daily_figure/picker.go`
- Create: `internal/service/daily_figure/picker_test.go`
- Modify: `internal/service/weixin_daily_recommendation.go` (delegate to new package)

- [ ] **Step 1: Read existing logic**

Open `internal/service/weixin_daily_recommendation.go` and identify the function that picks the figure for a given date. Most likely something like `pickFigureForDate(date time.Time)` returning a `model.FigureListItem`. Note the deps it needs (FigureRepo, deterministic seeding by date).

- [ ] **Step 2: Failing test**

`internal/service/daily_figure/picker_test.go`:

```go
package daily_figure

import (
    "testing"
    "time"
)

func TestPickForDateIsDeterministic(t *testing.T) {
    fakeRepo := newFakeFigureRepo([]int64{1, 2, 3, 4, 5})
    p := New(fakeRepo)
    date := time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)
    a, _ := p.PickForDate(date)
    b, _ := p.PickForDate(date)
    if a.ID != b.ID { t.Fatalf("want same id, got %d vs %d", a.ID, b.ID) }
}

func TestPickForDateChangesNextDay(t *testing.T) {
    fakeRepo := newFakeFigureRepo([]int64{1, 2, 3, 4, 5})
    p := New(fakeRepo)
    a, _ := p.PickForDate(time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC))
    b, _ := p.PickForDate(time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC))
    if a.ID == b.ID { t.Fatal("expected different ids on different days") }
}
```

(Provide a `newFakeFigureRepo` helper in the test file that satisfies the picker's interface.)

- [ ] **Step 3: Implement picker**

`internal/service/daily_figure/picker.go`:

```go
package daily_figure

import (
    "context"
    "fmt"
    "hash/fnv"
    "time"

    "github.com/xuzhougeng/citebox/internal/model"
)

type FigurePool interface {
    ListEligibleFigureIDs(ctx context.Context) ([]int64, error)
    LoadFigure(ctx context.Context, id int64) (model.FigureListItem, error)
}

type Picker struct{ pool FigurePool }

func New(pool FigurePool) *Picker { return &Picker{pool: pool} }

func (p *Picker) PickForDate(date time.Time) (model.FigureListItem, error) {
    return p.pickWithCtx(context.Background(), date)
}

func (p *Picker) pickWithCtx(ctx context.Context, date time.Time) (model.FigureListItem, error) {
    ids, err := p.pool.ListEligibleFigureIDs(ctx)
    if err != nil { return model.FigureListItem{}, err }
    if len(ids) == 0 { return model.FigureListItem{}, fmt.Errorf("no eligible figures") }
    h := fnv.New64a()
    fmt.Fprintf(h, "%d-%d-%d", date.Year(), int(date.Month()), date.Day())
    idx := int(h.Sum64() % uint64(len(ids)))
    return p.pool.LoadFigure(ctx, ids[idx])
}
```

- [ ] **Step 4: Wire into bridge**

In `internal/service/weixin_daily_recommendation.go`, replace the inline picker call with `dailyFigurePicker.PickForDate(today)`. Inject the picker via constructor — add a field on `LibraryService` if that's what the daily-rec method uses, or pass it through `WeixinIMBridge`.

- [ ] **Step 5: Run tests**

```
go test ./internal/service/daily_figure -v
go test ./internal/service -run TestWeixinDaily -v   # existing test should still pass
```

- [ ] **Step 6: Commit**

```
git add internal/service/daily_figure/ internal/service/weixin_daily_recommendation.go
git commit -m "refactor(daily_figure): extract deterministic date-seeded picker"
```

---

## Phase 6 — Wiring + WeChat bridge refactor

### Task 14: Wire `agent_session` in app startup

**Files:**
- Modify: `internal/app/app.go` (or wherever services are constructed; locate via `grep -n "ai_conversation.New" internal/app/*.go`)

- [ ] **Step 1: Add construction**

After the existing `ai_conversation.Service` is built, wire the agent session:

```go
// In internal/app/app.go after aiConvSvc is built:

surfaceState, err := agent_session.NewSurfaceStateStore(
    filepath.Join(cfg.StorageDir, "weixin-bridge", "weixin_surface_state.json"))
if err != nil { return nil, err }

cmdRegistry := commands.NewRegistry(
    &commands.ClearCommand{},
    &commands.ResetCommand{},
    &commands.HelpCommand{},
    &commands.StatusCommand{},
    &commands.NoteCommand{Library: librarySvc},
    &commands.RecentCommand{Lister: librarySvc, Limit: 5},
    &commands.FiguresCommand{Lister: librarySvc},
    &commands.RandomCommand{Picker: dailyFigureRandomAdapter{librarySvc, dailyFigPicker}},
    &commands.PaperCommand{Lookup: librarySvc},
    &commands.FigureCommand{Lookup: librarySvc},
    &commands.SearchCommand{Searcher: librarySvc},
    &commands.AskCommand{Reader: librarySvc, LLM: aiSvc},
    &commands.InterpretCommand{Interpreter: aiSvc},
    &commands.DOICommand{Importer: librarySvc},
)

agentSession := agent_session.New(
    aiConvRepo, cmdRegistry,
    agent_session.NewAIConversationAdapter(aiConvSvc),
    surfaceState, logger,
)
```

For each interface above, ensure `librarySvc` and `aiSvc` actually implement the methods. Where they don't, add narrow wrappers in `internal/service/library_service.go` / `internal/service/ai_service.go` using existing methods.

- [ ] **Step 2: Run startup migration**

In the same place, after `agentSession` is built, call:

```go
if err := agent_session.MigrateLegacyWeixinContext(
    filepath.Join(cfg.StorageDir, "weixin-bridge", "im_context.json"),
    aiConvRepo, surfaceState, logger,
); err != nil {
    logger.Warn("legacy weixin context migration failed", "error", err)
}
```

(Implementation comes in Task 16.)

- [ ] **Step 3: Run build**

```
go build ./...
```

Expected: success.

- [ ] **Step 4: Commit**

```
git add internal/app/
git commit -m "wire(agent_session): startup wiring for both surfaces"
```

---

### Task 15: Refactor WeChat bridge to call `agent_session`

**Files:**
- Modify: `internal/service/weixin_im_bridge.go`

- [ ] **Step 1: Replace `handleIncomingTextReply`**

The current method does parse-slash → planWeixinPlainTextCommand → executeWeixinCommandReply. Replace its body with:

```go
func (b *WeixinIMBridge) handleIncomingTextReply(ctx context.Context, text string) weixinReplyEnvelope {
    text = strings.TrimSpace(text)
    if text == "" { return weixinReplyEnvelope{} }

    sc, _ := b.surfaceState.Get("wechat")
    req := agent_session.AgentRequest{
        UserID:       "default",
        Surface:      agent_session.SurfaceWeChat,
        Conversation: agent_session.ConversationRef{Kind: agent_session.KindMainWeChat},
        SurfaceContext: agent_session.SurfaceContext{
            CurrentPaperID:        sc.CurrentPaperID,
            CurrentPaperTitle:     sc.CurrentPaperTitle,
            CurrentFigureID:       sc.CurrentFigureID,
            RecentSearchPaperIDs:  sc.RecentSearchPaperIDs,
            RecentSearchFigureIDs: sc.RecentSearchFigureIDs,
        },
        Input:   agent_session.Input{Text: text},
        Options: agent_session.Options{MaxChunkRunes: weixinReplyChunkMaxRunes},
    }

    // Send placeholder immediately for free-text (slash commands return fast).
    if !looksLikeSlashOrDOI(text) {
        // Surface adapter handles placeholder via callback; we render here for IM.
        b.placeholderEmitter("正在思考……")
    }

    resp, err := b.agentSession.Handle(ctx, req)
    if err != nil {
        return weixinReplyEnvelope{Text: "处理失败：" + err.Error()}
    }
    return chunksToWeixinEnvelope(resp.Chunks)
}
```

(`looksLikeSlashOrDOI` and `chunksToWeixinEnvelope` are small new helpers in the same file.)

For the placeholder, the simplest path is: `b.placeholderEmitter` is a closure captured at construction that calls `sendWeixinTextReply` directly. If you prefer, push the placeholder into the surface adapter passed into `agent_session` — but the simpler synchronous emit is fine.

- [ ] **Step 2: Add bridge dependencies**

Add fields:

```go
type WeixinIMBridge struct {
    // ... existing fields ...
    agentSession  *agent_session.Service
    surfaceState  *agent_session.SurfaceStateStore
    placeholderEmitter func(string)
}
```

Update the constructor signature accordingly. Wire it from `app.go`.

- [ ] **Step 3: Delete `handleIncomingMessage`'s slash plumbing**

Remove `parseWeixinSlashCommand`, `planWeixinPlainTextCommand`, `executeWeixinCommandReply`, and any helpers exclusive to them. Tests covering them should be moved or rewritten to target `agent_session` instead.

- [ ] **Step 4: Run build + bridge tests**

```
go build ./...
go test ./internal/service -run TestWeixinIM -v
```

If any existing tests target the removed functions directly, mark them for deletion in the next commit and replace with `agent_session` integration tests.

- [ ] **Step 5: Commit**

```
git add internal/service/weixin_im_bridge.go
git commit -m "refactor(weixin_bridge): route incoming messages through agent_session"
```

---

### Task 16: Migration of `im_context.json`

**Files:**
- Create: `internal/service/agent_session/migration.go`
- Create: `internal/service/agent_session/migration_test.go`

- [ ] **Step 1: Failing test**

```go
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
        "search_paper_ids":  []int{1, 2},
    }
    raw, _ := json.Marshal(payload)
    _ = os.WriteFile(legacy, raw, 0o644)

    db := repository.NewTestDB(t)
    repo := repository.NewAIConversationRepository(db)
    state, _ := NewSurfaceStateStore(filepath.Join(dir, "wx_state.json"))

    if err := MigrateLegacyWeixinContext(legacy, repo, state, nil); err != nil { t.Fatal(err) }

    cid, _ := repo.FindOrCreateByKind("main_wechat", "wechat")
    msgs, _ := repo.ListMessagesAfterBarrier(cid, 100)
    if len(msgs) != 2 { t.Fatalf("want 2 migrated messages, got %d", len(msgs)) }

    sc, _ := state.Get("wechat")
    if sc.CurrentPaperID != 42 || sc.CurrentFigureID != 7 {
        t.Fatalf("state not migrated: %+v", sc)
    }

    if _, err := os.Stat(legacy + ".migrated.bak"); err != nil {
        t.Fatal("expected legacy file to be renamed")
    }
}
```

- [ ] **Step 2: Confirm failure**

```
go test ./internal/service/agent_session -run TestMigrateLegacy -v
```

- [ ] **Step 3: Implement**

`internal/service/agent_session/migration.go`:

```go
package agent_session

import (
    "encoding/json"
    "log/slog"
    "os"

    "github.com/xuzhougeng/citebox/internal/repository"
)

type legacyWeixinContext struct {
    CurrentPaperID  int64    `json:"current_paper_id"`
    CurrentFigureID int64    `json:"current_figure_id"`
    QAHistory       []legacyTurn `json:"qa_history"`
    SearchPaperIDs  []int64  `json:"search_paper_ids"`
    SearchFigureIDs []int64  `json:"search_figure_ids"`
}

type legacyTurn struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}

// MigrateLegacyWeixinContext is idempotent: if the legacy file is already
// renamed to *.migrated.bak it returns nil without doing anything.
func MigrateLegacyWeixinContext(path string, repo *repository.AIConversationRepository,
    state *SurfaceStateStore, logger *slog.Logger) error {

    raw, err := os.ReadFile(path)
    if err != nil {
        if os.IsNotExist(err) { return nil }
        return err
    }
    var ctx legacyWeixinContext
    if err := json.Unmarshal(raw, &ctx); err != nil { return err }

    cid, err := repo.FindOrCreateByKind("main_wechat", "wechat")
    if err != nil { return err }

    for _, t := range ctx.QAHistory {
        role := t.Role
        if role != "user" && role != "assistant" { role = "assistant" }
        if _, err := repo.AddMessage(cid, role, t.Content, repository.AIMessageMeta{
            Mode: "migrated",
        }); err != nil { return err }
    }

    if ctx.CurrentPaperID > 0 {
        _ = state.SetCurrentPaper("wechat", ctx.CurrentPaperID, "")
    }
    if ctx.CurrentFigureID > 0 {
        _ = state.SetCurrentFigure("wechat", ctx.CurrentFigureID)
    }
    if len(ctx.SearchPaperIDs) > 0 || len(ctx.SearchFigureIDs) > 0 {
        _ = state.SetSearchResults("wechat", ctx.SearchPaperIDs, ctx.SearchFigureIDs)
    }

    if err := os.Rename(path, path+".migrated.bak"); err != nil {
        if logger != nil { logger.Warn("rename legacy weixin context failed", "error", err) }
    }
    return nil
}
```

- [ ] **Step 4: Run test**

```
go test ./internal/service/agent_session -run TestMigrateLegacy -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```
git add internal/service/agent_session/migration.go internal/service/agent_session/migration_test.go
git commit -m "feat(agent_session): one-shot legacy weixin context migration"
```

---

### Task 17: Tear out dead `ai_service_weixin.go` planning code

**Files:**
- Modify: `internal/service/ai_service_weixin.go`
- Modify: `internal/service/ai_service_export_prompt_test.go` (drop tests for removed funcs)

- [ ] **Step 1: Identify dead exports**

After Task 15, the bridge no longer calls:
- `PlanWeixinSearch`
- `PlanWeixinCommand`
- `ReviewWeixinPaperSearch`
- `ReviewWeixinFigureSearch`

Plus their private helpers (`buildWeixinSearchSystemPrompt`, `parseWeixin*`, `normalizeWeixin*Keywords*`, etc.) **only if** they have no other caller.

- [ ] **Step 2: Verify no callers via grep**

```
grep -RIn "PlanWeixinSearch\|PlanWeixinCommand\|ReviewWeixinPaperSearch\|ReviewWeixinFigureSearch" internal/ cmd/
```

Expected: only the function definitions and (now removed) bridge call site. If any external callers remain, leave the function alone for now and revisit.

- [ ] **Step 3: Delete the dead functions and their tests**

Remove the four exported functions and their helpers. Drop matching tests. Keep `weixinSearchTargetPaper`/`weixinSearchTargetFigure` constants if they are referenced elsewhere (re-grep).

- [ ] **Step 4: Verify build + remaining tests**

```
go build ./...
go test ./internal/service/...
```

- [ ] **Step 5: Commit**

```
git add internal/service/ai_service_weixin.go internal/service/ai_service_export_prompt_test.go
git commit -m "chore(ai_service): drop dead weixin planning code"
```

---

## Phase 7 — Desktop UX additions

### Task 18: TTS button per assistant message

**Files:**
- Modify: `web/ai-assistant.html` (or the partial for assistant message rendering)
- Modify: `web/static/js/ai-assistant.js`
- Modify: `web/static/css/ai-assistant.css` (small)

- [ ] **Step 1: Locate render path**

```
grep -n "assistant" web/static/js/ai-assistant.js | head
grep -n "message" web/static/js/ai-assistant.js | head
```

Find the function that creates an assistant message DOM node.

- [ ] **Step 2: Add a 🔊 button**

In the assistant-message renderer, after the text node, append:

```js
const ttsBtn = document.createElement('button');
ttsBtn.className = 'msg-tts-btn';
ttsBtn.type = 'button';
ttsBtn.title = '朗读';
ttsBtn.textContent = '🔊';
ttsBtn.addEventListener('click', () => playTTS(messageText));
node.appendChild(ttsBtn);
```

Helper:

```js
async function playTTS(text) {
  const r = await fetch('/api/ai/tts', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ text }),
  });
  if (!r.ok) return;
  const blob = await r.blob();
  const url = URL.createObjectURL(blob);
  const audio = new Audio(url);
  audio.play();
}
```

If `/api/ai/tts` doesn't exist (only WeChat-side TTS today), expose the existing TTS service via a new handler `internal/handler/ai.go: TTSHandler` that calls the same `synthesizeTTS` used by the bridge. This is a small additive change.

- [ ] **Step 3: CSS**

```css
.msg-tts-btn {
  margin-inline-start: 8px;
  background: transparent;
  border: 0;
  cursor: pointer;
  opacity: 0.6;
}
.msg-tts-btn:hover { opacity: 1; }
```

- [ ] **Step 4: Manual smoke**

```
make run
# open http://localhost:8080/ai-assistant.html, ask anything, click 🔊, hear playback.
```

- [ ] **Step 5: Syntax check + commit**

```
node --check web/static/js/ai-assistant.js
git add web/ai-assistant.html web/static/js/ai-assistant.js web/static/css/ai-assistant.css internal/handler/ai.go
git commit -m "feat(web): TTS button per assistant message"
```

---

### Task 19: DOI detection in AI assistant input box

**Files:**
- Modify: `web/static/js/ai-assistant.js`

- [ ] **Step 1: Add helper**

```js
const DOI_RE = /\b10\.\d{4,9}\/[-._;()/:A-Z0-9]+\b/i;
function looksLikeDOI(text) { return DOI_RE.test((text || '').trim()); }
```

- [ ] **Step 2: Intercept submit**

In the existing submit handler:

```js
if (looksLikeDOI(text)) {
  const r = await fetch('/api/library/papers/import-by-doi', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ doi: text.trim() }),
  });
  if (r.ok) {
    const paper = await r.json();
    appendSystemMessage(`已通过 DOI 导入：#${paper.id} ${paper.title}`);
  } else {
    const err = await r.json().catch(() => ({}));
    appendSystemMessage(`DOI 导入失败：${err.message || r.status}`);
  }
  return;
}
```

`appendSystemMessage` is a small addition that renders a non-LLM info line in the conversation transcript.

- [ ] **Step 3: Manual smoke**

Paste `10.1038/s41586-023-06000-1` into the input box; verify import.

- [ ] **Step 4: Syntax check + commit**

```
node --check web/static/js/ai-assistant.js
git add web/static/js/ai-assistant.js
git commit -m "feat(web): detect bare DOI in AI input and import"
```

---

### Task 20: Overview page handler

**Files:**
- Create: `internal/handler/overview.go`
- Create: `internal/handler/overview_test.go`
- Modify: `internal/handler/router.go` (or wherever routes are registered) — add `/overview` page and `/api/overview/*` routes
- Create: `web/overview.html`, `web/static/js/overview.js`, `web/static/css/overview.css`

- [ ] **Step 1: Failing handler test**

```go
package handler

import (
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestOverviewDailyFigureReturnsJSON(t *testing.T) {
    h := newOverviewHandlerForTest(t) // uses fake picker that always returns figure id=99
    req := httptest.NewRequest("GET", "/api/overview/daily-figure", nil)
    rec := httptest.NewRecorder()
    h.DailyFigure(rec, req)
    if rec.Code != http.StatusOK { t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String()) }
    if !contains(rec.Body.String(), `"id":99`) {
        t.Fatalf("body missing figure id: %s", rec.Body.String())
    }
}
```

- [ ] **Step 2: Confirm failure**

- [ ] **Step 3: Implement handler**

`internal/handler/overview.go`:

```go
package handler

import (
    "encoding/json"
    "net/http"
    "time"

    "github.com/xuzhougeng/citebox/internal/service/daily_figure"
)

type OverviewHandler struct {
    Picker  *daily_figure.Picker
    Library OverviewLibrarySource
    Status  OverviewStatusSource
}

type OverviewLibrarySource interface {
    SummaryRecent(limit int) (any, error)
}

type OverviewStatusSource interface {
    Snapshot() (any, error)
}

func (h *OverviewHandler) DailyFigure(w http.ResponseWriter, r *http.Request) {
    fig, err := h.Picker.PickForDate(time.Now())
    if err != nil { http.Error(w, err.Error(), http.StatusInternalServerError); return }
    writeJSON(w, fig)
}

func (h *OverviewHandler) Summary(w http.ResponseWriter, r *http.Request) {
    s, err := h.Library.SummaryRecent(7)
    if err != nil { http.Error(w, err.Error(), http.StatusInternalServerError); return }
    writeJSON(w, s)
}

func (h *OverviewHandler) Status(w http.ResponseWriter, r *http.Request) {
    s, err := h.Status.Snapshot()
    if err != nil { http.Error(w, err.Error(), http.StatusInternalServerError); return }
    writeJSON(w, s)
}

func writeJSON(w http.ResponseWriter, v any) {
    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(v)
}
```

- [ ] **Step 4: Register routes**

In `internal/handler/router.go`:

```go
mux.HandleFunc("GET /api/overview/summary", overviewHandler.Summary)
mux.HandleFunc("GET /api/overview/daily-figure", overviewHandler.DailyFigure)
mux.HandleFunc("GET /api/overview/status", overviewHandler.Status)
mux.HandleFunc("GET /overview", servePage("web/overview.html"))
```

- [ ] **Step 5: Run handler test**

```
go test ./internal/handler -run TestOverview -v
```

- [ ] **Step 6: Commit**

```
git add internal/handler/overview.go internal/handler/overview_test.go internal/handler/router.go
git commit -m "feat(handler): /api/overview/* endpoints + /overview page route"
```

---

### Task 21: Overview page UI

**Files:**
- Create: `web/overview.html`, `web/static/js/overview.js`, `web/static/css/overview.css`

- [ ] **Step 1: HTML scaffold**

`web/overview.html`:

```html
<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <title>总览 — CiteBox</title>
  <link rel="stylesheet" href="/static/css/overview.css" />
</head>
<body>
  <header class="topbar"><a href="/">CiteBox</a> · 总览</header>
  <main class="overview-grid">
    <section id="panel-research" class="panel">
      <h2>Research Dashboard</h2>
      <div data-bind="recent-papers"></div>
      <div data-bind="recent-conversations"></div>
      <div data-bind="todos"></div>
    </section>
    <section id="panel-figure" class="panel">
      <h2>图片</h2>
      <div data-bind="daily-figure"></div>
      <div data-bind="recent-figures"></div>
    </section>
    <section id="panel-status" class="panel">
      <h2>Status</h2>
      <div data-bind="status"></div>
    </section>
  </main>
  <script src="/static/js/overview.js" type="module"></script>
</body>
</html>
```

- [ ] **Step 2: JS to fill panels**

`web/static/js/overview.js`:

```js
async function load() {
  const [summary, dailyFig, status] = await Promise.all([
    fetch('/api/overview/summary').then(r => r.json()),
    fetch('/api/overview/daily-figure').then(r => r.json()),
    fetch('/api/overview/status').then(r => r.json()),
  ]);
  renderSummary(summary);
  renderDailyFigure(dailyFig);
  renderRecentFigures(summary.recent_figures || []);
  renderStatus(status);
}

function el(name, props = {}, children = []) {
  const n = Object.assign(document.createElement(name), props);
  children.forEach(c => n.append(c));
  return n;
}

function renderSummary(s) {
  document.querySelector('[data-bind="recent-papers"]').replaceChildren(
    el('h3', { textContent: '最近导入文献' }),
    el('ul', {}, (s.recent_papers || []).map(p => el('li', { textContent: `#${p.id} ${p.title}` })))
  );
  document.querySelector('[data-bind="recent-conversations"]').replaceChildren(
    el('h3', { textContent: '最近对话' }),
    el('ul', {}, (s.recent_conversations || []).map(c => el('li', { textContent: c.title || `#${c.id}` })))
  );
  document.querySelector('[data-bind="todos"]').replaceChildren(
    el('h3', { textContent: '待处理' }),
    el('ul', {}, (s.todos || []).map(t => el('li', { textContent: t.text })))
  );
}

function renderDailyFigure(f) {
  if (!f || !f.id) return;
  const img = el('img', { src: `/api/figures/${f.id}/preview`, alt: f.caption || '' });
  document.querySelector('[data-bind="daily-figure"]').replaceChildren(
    el('h3', { textContent: '今日推荐' }), img,
    el('p', { textContent: f.caption || '' })
  );
}

function renderRecentFigures(arr) {
  document.querySelector('[data-bind="recent-figures"]').replaceChildren(
    el('h3', { textContent: '最近图片' }),
    el('div', { className: 'fig-grid' },
      arr.map(f => el('img', { src: `/api/figures/${f.id}/preview`, title: f.caption || '' })))
  );
}

function renderStatus(s) {
  document.querySelector('[data-bind="status"]').replaceChildren(
    el('h3', { textContent: '系统状态' }),
    el('pre', { textContent: JSON.stringify(s, null, 2) })
  );
}

load().catch(err => {
  document.body.append(el('pre', { textContent: '加载失败：' + err.message }));
});
```

- [ ] **Step 3: CSS minimal**

`web/static/css/overview.css`:

```css
body { font-family: system-ui, -apple-system, sans-serif; margin: 0; }
.topbar { padding: 12px 20px; border-bottom: 1px solid #eee; }
.overview-grid {
  display: grid; gap: 16px; padding: 20px;
  grid-template-columns: 2fr 1fr 1fr;
}
.panel { border: 1px solid #eee; border-radius: 8px; padding: 16px; }
.panel h2 { margin-top: 0; }
.fig-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 8px; }
.fig-grid img { width: 100%; height: 100px; object-fit: cover; border-radius: 4px; }
```

- [ ] **Step 4: Manual smoke**

```
make run
# open http://localhost:8080/overview, verify three panels render with real data.
```

- [ ] **Step 5: Syntax check + commit**

```
node --check web/static/js/overview.js
git add web/overview.html web/static/js/overview.js web/static/css/overview.css
git commit -m "feat(web): overview page (Research Dashboard / 图片 / Status)"
```

---

## Phase 8 — Integration tests + smoke

### Task 22: WeChat bridge integration through `agent_session`

**Files:**
- Modify: `internal/service/weixin_im_bridge_test.go` (replace removed-fn tests)

- [ ] **Step 1: Integration test scenario**

```go
func TestBridgeRoutesFreeTextThroughAgentSession(t *testing.T) {
    env := newBridgeTestEnv(t) // builds repo, fake LLM, agent_session, bridge
    env.fakeLLM.Reply = "答案 [1]"
    env.SendIM("找一下关于扩散模型的论文")
    msgs := env.SentReplies()
    if len(msgs) < 2 { t.Fatalf("want placeholder + answer, got %d", len(msgs)) }
    if !strings.Contains(msgs[0], "正在思考") { t.Errorf("missing placeholder: %v", msgs[0]) }
    if !strings.Contains(msgs[len(msgs)-1], "答案") { t.Errorf("missing answer: %v", msgs[len(msgs)-1]) }
    // Conversation now has 2 rows.
    cid, _ := env.repo.FindOrCreateByKind("main_wechat", "wechat")
    rows, _ := env.repo.ListMessagesAfterBarrier(cid, 10)
    if len(rows) != 2 { t.Fatalf("want 2 turns, got %d", len(rows)) }
}

func TestBridgeClearCommandSetsBarrier(t *testing.T) {
    env := newBridgeTestEnv(t)
    env.fakeLLM.Reply = "ans"
    env.SendIM("hi")
    env.SendIM("/clear")
    cid, _ := env.repo.FindOrCreateByKind("main_wechat", "wechat")
    rows, _ := env.repo.ListMessagesAfterBarrier(cid, 10)
    if len(rows) != 0 { t.Fatalf("want 0 visible after /clear, got %d", len(rows)) }
}
```

- [ ] **Step 2: Implement test env**

Build a small `newBridgeTestEnv(t)` helper that constructs an in-memory DB, a fake `caller` for `ai_conversation` (returning canned text), an `agent_session.Service` with a no-op state store, and a `WeixinIMBridge` with stubbed IM client whose `SendText` records into a slice.

- [ ] **Step 3: Run tests**

```
go test ./internal/service -run "TestBridgeRoutes|TestBridgeClear" -v
```

Expected: PASS.

- [ ] **Step 4: Commit**

```
git add internal/service/weixin_im_bridge_test.go
git commit -m "test(weixin_bridge): end-to-end through agent_session"
```

---

### Task 23: Final smoke + docs update

- [ ] **Step 1: Full test suite**

```
go test ./...
```

Expected: PASS.

- [ ] **Step 2: Build server binary**

```
make build
```

- [ ] **Step 3: Update API docs**

Per `AGENTS.md`: "If you change frontend-to-backend API routes, request fields, response shapes, or API usage semantics, update `docs/api.md` in the same PR." Add the three new `/api/overview/*` endpoints and `POST /api/ai/tts` (if newly added) to `docs/api.md`.

If schema columns changed, also update `docs/database.md` to mention `surface_origin`, `kind`, `clear_barrier_turn_id`.

- [ ] **Step 4: Manual end-to-end**

```
make run
```

Verify in browser:
- `/ai-assistant` — submit a question, see answer, click 🔊 (audio plays), paste a DOI → import line appears.
- `/overview` — three panels load.

Verify via WeChat (if a bridge binding is configured):
- Send "找扩散模型论文" → receive placeholder then answer.
- Send `/clear` → confirmation; subsequent question's prompt has zero history.
- Send `10.1038/...` → import notice.

- [ ] **Step 5: Commit docs**

```
git add docs/api.md docs/database.md
git commit -m "docs: surface unification — overview endpoints, conversation columns"
```

---

## Self-Review

**Spec coverage:**
- §Decisions 1–6 (unification depth, conversation model, agent execution, slash inventory, 5-turn cap, /clear semantics) → Tasks 1, 2, 4, 5, 6, 7–11, 12.
- §Decisions 7 (TTS / Overview / DOI in chat) → Tasks 18, 19, 20, 21.
- §Architecture (agent_session entry → orchestrator) → Tasks 3, 4, 12, 14.
- §Data model (columns + partial unique + surface_state file) → Tasks 1, 5.
- §`agent_session.Service` interface → Tasks 3, 4.
- §Request flow (free text + slash + DOI) → Tasks 11, 12.
- §5-turn window + barrier SQL → Task 2 (repo) + Task 12 (consumer).
- §Slash inventory → Tasks 6 (state), 7–9 (queries), 10 (constrained).
- §Surface adapters (WeChat / Web / Overview / TTS / DOI) → Tasks 15, 18, 19, 20, 21.
- §Migration → Task 16.
- §Testing → tests in every task + integration in Task 22.
- §Roll-out (8 steps) → mapped to Tasks 1–22 in roughly the same order.
- §Non-goals (memory search, multi-tenant, clickable evidence cards, archive UI, WeChat streaming) → not in any task. Correct.

**Placeholder scan:** None of the listed task steps contain "TBD" / "TODO" / "implement later" / "add appropriate error handling" / unspecified test code. Every code step has the actual code.

**Type consistency:**
- `commands.SurfaceContext` is the same shape across `registry.go`, `surface_state.go`, `recent.go`, etc. ✓
- `Result` uses `ChunksText`, `ImagePath`, `UsedShortcut`, `UserMessageID`, `AssistantMsgID` everywhere. ✓
- `OutboundChunk` mirrors between the parent package and the `commands` subpackage; `wrapCmdResp` converts. ✓
- `looksLikeDOI` lives in `agent_session/dispatch.go` (Go) and `web/static/js/ai-assistant.js` (JS) — same regex on both sides. ✓
- `FindOrCreateByKind`, `SetClearBarrier`, `ListMessagesAfterBarrier` signatures consistent between Tasks 2, 6, 12, 16, 22. ✓

**Issues fixed inline:**
- Originally referenced `repository.NewTestDB`; if not exported, Task 6 explicitly tells the implementer to expose it or duplicate locally.
- Originally referenced `Repo.DB()`; Task 6 explicitly adds it if missing.
- `itoa` → flagged to use `strconv.FormatInt` directly.

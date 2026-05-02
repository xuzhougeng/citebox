# Unify AI Across Surfaces (WeChat + Desktop/Web)

Date: 2026-05-02
Status: Draft (awaiting user review)

## Goal

Today CiteBox runs two parallel AI paths that do not share intelligence, memory, or capabilities:

- **Desktop / Web AI assistant** uses the full agent stack: `ai_assistant.Orchestrator`, library / figure / external / paper-read tools, `ai_conversation` persistence, evidence assembly, streaming.
- **WeChat IM bridge** runs an independent path: `weixin_im_bridge.go` plus `ai_service_weixin.go` plan a slash command from the user's text, execute one shallow action, return a reply. Memory is a 6-turn `qa_history` plus `current_paper_id` / `current_figure_id` in `im_context.json`.

The result is that the same question reaches different brains. WeChat cannot do multi-step tool use, has no shared memory with the desktop, and the desktop is missing WeChat-only conveniences (TTS, daily figure recommendation, DOI quick import).

The goal of this work is **one AI, one memory, one capability set, surfaced through any client**. WeChat becomes one input form among others, not a parallel system.

## Non-goals

- **Memory search / RAG over full conversation history.** Schema leaves room; not implemented this round.
- **Multi-tenancy beyond the existing single admin user.** CiteBox stays single-tenant.
- **Clickable evidence cards in WeChat.** Plain text references like `[1]`, `[2]` are sufficient.
- **Advanced management of the WeChat conversation from desktop** (rename, archive, delete) — desktop must be able to view and continue the WeChat thread, nothing more.
- **Streaming inside WeChat IM.** WeChat replies are single-shot after a placeholder; no progress events.

## Decisions (resolved during brainstorming)

1. **Unification depth = full**: intelligence + session memory + operation set all merge.
2. **Conversation model**: each WeChat-bound user gets exactly one persistent "main WeChat" conversation, visible from the desktop. All WeChat input appends here.
3. **Agent execution**: synchronous, non-streaming. Surface sends a placeholder ("正在思考……") immediately, then a single follow-up with the full answer. Multi-step tool use happens inside one request.
4. **Slash commands stay**: state commands (`/clear`, `/reset`, `/help`, `/status`, `/note`) and query shortcuts (`/recent`, `/figures`, `/random`, `/paper N`, `/figure N`, `/search`, `/ask`, `/interpret`) all preserved. Query shortcuts call tools directly, bypassing the LLM. Free text falls through to the agent.
5. **Context window**: hard sliding window of the most recent 5 turns sent to the LLM. No summarization, no anchor preservation. All turns persist in DB for future memory features.
6. **`/clear` semantics**: sets a barrier — subsequent prompts only include turns after the barrier. DB rows are never deleted.
7. **WeChat-exclusive features ported to desktop**:
   - TTS: manual 🔊 button on each assistant message in `ai-assistant.html`. No auto-play setting.
   - Daily figure recommendation: surfaced inside a new Overview page (panels: Research Dashboard / 图片 / Status). Same idempotent picker the WeChat ticker uses.
   - DOI quick import: AI assistant input box detects DOI on submit and triggers the existing `ImportPaperByDOI` flow; result becomes a system turn in the conversation. No new top-level button.

## Architecture

```
┌─────────────────┐  ┌─────────────────┐  ┌──────────────┐
│ weixin_im_bridge│  │ ai_conversation │  │ overview     │
│ (IM surface)    │  │ handler (web)   │  │ handler      │
└────────┬────────┘  └────────┬────────┘  └──────┬───────┘
         │ AgentRequest        │ AgentRequest     │
         └──────────┬──────────┘                  │
                    ▼                              │
         ┌─────────────────────┐                   │
         │ agent_session.Svc   │◄──────────────────┘
         │ - resolve session   │
         │ - load 5-turn ctx   │   ┌────────────────────┐
         │ - dispatch slash    │──►│ commands.* (slash) │
         │ - run agent         │   └────────────────────┘
         │ - persist turn      │   ┌────────────────────┐
         │ - chunk reply       │──►│ ai_assistant.      │
         └─────────────────────┘   │   Orchestrator     │
                    ▲              │ (existing tools)   │
                    │              └────────────────────┘
         ┌──────────┴──────────┐
         │ ConversationStore   │
         │ (SQLite)            │
         └─────────────────────┘
```

The new `agent_session.Service` is the single entry point. WeChat bridge and web handler both shrink to surface adapters: parse incoming form, build an `AgentRequest`, hand off, format the `AgentResponse` for the surface (IM chunk vs. SSE).

### New package: `internal/service/agent_session/`

- `service.go` — `Service.Handle(ctx, AgentRequest) (*AgentResponse, error)`
- `conversation_store.go` — SQLite read/write for conversations and turns, including the 5-turn / barrier query
- `commands/` — one file per slash command (`clear.go`, `reset.go`, `help.go`, `status.go`, `note.go`, `recent.go`, `figures.go`, `random.go`, `paper.go`, `figure.go`, `search.go`, `ask.go`, `interpret.go`)
- `dispatch.go` — text → command parser; falls through to agent for free text
- `agent_runner.go` — wraps the existing `ai_assistant.Orchestrator`; injects 5-turn context as conversation history; collects evidence + tool traces into the turn metadata
- `placeholder.go` — produces the immediate placeholder response shape so surfaces can flush before the agent finishes
- `types.go` — `AgentRequest`, `AgentResponse`, `Surface`, `SurfaceContext`, `ConversationRef`, `OutboundChunk`, `EvidenceRef`

## Data model

### `conversations` (extends existing `ai_conversation` table)

New columns:

- `surface_origin TEXT NOT NULL` — `'wechat'`, `'web'`, `'desktop'`
- `kind TEXT NOT NULL` — `'main_wechat'`, `'default_web'`, `'ad_hoc'`
- `clear_barrier_turn_id INTEGER` — nullable; when set, any turn with `id <= clear_barrier_turn_id` is excluded from prompt assembly

Constraint: partial unique index on `(kind)` where `kind = 'main_wechat'` to guarantee a single WeChat thread for the single-tenant deployment. (If multi-tenancy is added, this becomes `(user_id, kind)`.)

### `conversation_turns` (extends existing turn storage)

Fields:

- `id`, `conversation_id`, `role` (`'user'` / `'assistant'` / `'system'` / `'tool'`)
- `content TEXT` — primary text
- `metadata_json TEXT` — surface label, evidence refs, tool calls, attached figure IDs, placeholder flag
- `created_at`

All turns are persisted; nothing is deleted by `/clear`. The 5-turn cap is a read-time filter, not a write-time prune.

### Surface-local state (not in `conversations`)

- **WeChat**: keep `weixin_surface_state.json` (renamed from `im_context.json` after migration) holding only `current_paper_id`, `current_figure_id`, `search_paper_ids`, `search_figure_ids`. The `qa_history` field is removed — history lives in the DB now.
- **Web**: stays in front-end state (URL, reader open). No server-side mirror.

`SurfaceContext` (the per-request snapshot of selection state) is **not synced across surfaces**. Only conversation history is shared.

## `agent_session.Service` interface

```go
type Surface string

const (
    SurfaceWeChat Surface = "wechat"
    SurfaceWeb    Surface = "web"
)

type SurfaceContext struct {
    CurrentPaperID    int64
    CurrentPaperTitle string
    CurrentFigureID   int64
    RecentSearchPaperIDs  []int64
    RecentSearchFigureIDs []int64
}

type ConversationRef struct {
    Kind ConversationKind // MainWeChat | DefaultWeb | AdHoc(id)
    ID   int64            // ignored unless Kind == AdHoc
}

type Input struct {
    Text  string             // raw user text
    Files []InboundAttachment // images sent over IM, etc.
}

type AgentRequest struct {
    UserID         string
    Surface        Surface
    Conversation   ConversationRef
    SurfaceContext SurfaceContext
    Input          Input
    Options        Options
}

type Options struct {
    RequireTTS    bool
    MaxChunkRunes int
}

type OutboundChunk struct {
    Kind          ChunkKind // Text | Image | Voice
    Text          string
    ImagePath     string
    VoicePath     string
    IsPlaceholder bool
}

type AgentResponse struct {
    Turn         ConversationTurn
    Chunks       []OutboundChunk
    Evidence     []EvidenceRef
    UsedShortcut bool // true when no LLM call was made
}

type Service interface {
    Handle(ctx context.Context, req AgentRequest) (*AgentResponse, error)
}
```

## Request flow

### Free text (NL → agent)

1. Surface adapter builds `AgentRequest` and calls `Service.Handle`.
2. `Service` resolves the `ConversationRef` (creating the main WeChat conversation row if missing).
3. `Service` persists the user turn.
4. `Service` returns immediately to the surface with a `Chunks: [{Text: "正在思考……", IsPlaceholder: true}]` response. The surface flushes this so the user sees activity within a second.
5. Surface calls `Service.Continue(ctx, response)` (or supplies a `FollowUp` callback at request time) to drive the agent run.
6. `agent_runner` loads the most recent 5 turns after the barrier, formats them as conversation history, calls `ai_assistant.Orchestrator.Run(...)`.
7. Orchestrator runs multi-step tool use as it does today (no changes inside the agent itself).
8. `Service` persists the assistant turn (with evidence + tool trace in metadata) and returns the final `AgentResponse`.
9. Surface delivers the final chunks: WeChat sends a separator (`---`) plus the answer text plus optional figure attachment plus optional voice; Web pushes via SSE so the front-end replaces the placeholder.

### Slash command (state)

`/clear`, `/reset`, `/help`, `/status`, `/note`:

1. Surface adapter builds `AgentRequest`.
2. `dispatch.go` recognizes the command and routes to the matching `commands/*.go`.
3. The command performs its action (e.g. `clear.go` writes `clear_barrier_turn_id = max(turn_id)`; `note.go` appends a note row), persists a system turn describing the action, returns a single text chunk confirming what happened.
4. No placeholder, no agent call. `UsedShortcut = true`.

### Slash command (query shortcut)

`/recent`, `/figures`, `/random`, `/paper N`, `/figure N`, `/search …`, `/ask …`, `/interpret`:

1. Surface adapter builds `AgentRequest`.
2. `dispatch.go` routes to the command.
3. Command directly calls the relevant tool (`library_search_tool`, `figure_lookup_tool`, etc.) with hard-coded inputs — no LLM planning or generation.
4. Command persists user + assistant turns (assistant turn carries the tool result) and returns chunks.
5. No placeholder, no agent call. `UsedShortcut = true`.

`/ask` and `/interpret` need the LLM but pin the input: `/ask` answers about the current paper using `paper_read_tool`; `/interpret` describes the current figure using the multimodal path. Both still go through the orchestrator but with a fixed initial tool selection — implementation can either be a constrained agent run or a direct tool call followed by a single LLM summarization step. We will choose during planning based on existing tool boundaries.

### Bare DOI

If the input matches a DOI pattern (existing `looksLikeWeixinDOIText` / `normalizeDOIInput` reused, moved to `internal/service/doi/`), `dispatch.go` routes to a DOI handler that calls `LibraryService.ImportPaperByDOI`. Result is appended as a system turn. Works on both surfaces.

## 5-turn window + `/clear` barrier

When `agent_runner` builds the LLM prompt:

```sql
SELECT id, role, content, metadata_json, created_at
FROM conversation_turns
WHERE conversation_id = ?
  AND id > COALESCE((SELECT clear_barrier_turn_id FROM conversations WHERE id = ?), 0)
ORDER BY id DESC
LIMIT 10        -- 5 user/assistant pairs = up to 10 rows
```

Then reverse to chronological order and pass to the orchestrator.

`/clear` updates `conversations.clear_barrier_turn_id` to the largest existing turn ID for that conversation. The next prompt sees zero history. The next user message becomes the first turn in the new "window".

Desktop view of the WeChat conversation respects the same barrier so what the user sees matches what the model sees. Pre-barrier turns are accessible only via an explicit "归档" toggle (out of scope for this round; UI may simply hide them).

## Slash commands inventory

| Command | Type | Action |
|---|---|---|
| `/help` | state | List available commands. |
| `/status` | state | Report current selection, conversation length, model, TTS status. |
| `/clear` | state | Set conversation barrier. |
| `/reset` | state | Clear surface-local state (`current_paper_id` etc.) without touching conversation. |
| `/note <text>` | state | Append note to current paper / figure. |
| `/recent` | shortcut | Last N papers added; calls library tool with `mode=recent`. |
| `/figures` | shortcut | Figures of current paper; calls figure tool. |
| `/random` | shortcut | Random figure; reuses daily-figure picker logic without "today" lock. |
| `/paper N` | shortcut | Select N-th item from last paper search; updates surface state. |
| `/figure N` | shortcut | Select N-th item from last figure search; updates surface state. |
| `/search <q>` | shortcut | Direct library search; bypasses planner. |
| `/ask <q>` | constrained agent | Q&A scoped to current paper (paper_read_tool). |
| `/interpret` | constrained agent | Multimodal description of current figure. |

Free text → full orchestrator (no tool selection forced).

## Surface adapters

### WeChat (`weixin_im_bridge.go` after refactor)

Responsibilities shrink to:

- Poll WeChat IM (existing logic).
- Map incoming message → `AgentRequest{Surface: SurfaceWeChat, Conversation: {Kind: MainWeChat}, ...}`.
- For each `AgentResponse.Chunks`:
  - Text → `sendWeixinTextReply` (chunked at `weixinReplyChunkMaxRunes`).
  - Image → `client.SendImageFile`.
  - Voice → `client.SendFileAttachment` (synthesized from final answer when `Options.RequireTTS`).
- Daily recommendation ticker keeps running, calls `daily_figure.PickForDate(today)` (extracted from current logic) and pushes via the same chunk path.

What goes away:

- `ai_service_weixin.go`'s plan/review functions for slash-command planning. Replaced by free-text → orchestrator. The "review candidates" step (`ReviewWeixinPaperSearch`, `ReviewWeixinFigureSearch`) is no longer needed because the orchestrator already filters via tool calls.
- `weixinIMContext.QAHistory` — moved to the DB.

### Web (`internal/handler/ai_conversation.go` after refactor)

- POST `/api/ai/conversation/message` (existing) → builds `AgentRequest{Surface: SurfaceWeb, Conversation: {Kind: DefaultWeb} or {Kind: AdHoc, ID: id}}`, returns SSE stream where the placeholder is the first event and the final answer is the second.
- The existing front-end handles SSE; only the event payload needs to allow a placeholder marker.

### Overview (new)

- New `internal/handler/overview.go` plus `web/overview.html`.
- Endpoints:
  - `GET /api/overview/summary` — recent papers, recent conversations, TODOs.
  - `GET /api/overview/daily-figure` — calls `daily_figure.PickForDate(today)`.
  - `GET /api/overview/status` — bridge / model / TTS / disk.
- Page layout: three panels stacked or in a grid, content described in §1. No new mutations on this page.

### TTS button (web)

- Each assistant message in `web/ai-assistant.html` gets a 🔊 button.
- Click calls existing TTS endpoint with the message text; plays the returned audio inline.
- No setting toggle; no auto-play.

### DOI in chat input

- In the AI assistant front-end, before submitting a message, run `looksLikeDOI` (mirror of existing `looksLikeWeixinDOIText`).
- If matched, hit `/api/library/papers/import-by-doi` directly; on success, append a system turn to the conversation and render an "已导入：XXX" line. The user's text is not sent to the agent.

## Migration

One-shot startup migration:

1. If `STORAGE_DIR/weixin-bridge/im_context.json` exists and `STORAGE_DIR/weixin-bridge/.migrated` does not:
   - Locate (or create) the `main_wechat` conversation for the bound user.
   - For each entry in `qa_history`, write a user turn and an assistant turn with `metadata_json.surface = 'wechat'` and a synthetic `created_at` derived from the file's modification time stepped by 1 second per entry (best-effort ordering).
   - Write `current_paper_id` / `current_figure_id` / `search_*_ids` to `weixin_surface_state.json`.
   - Rename `im_context.json` → `im_context.json.migrated.bak`.
   - Touch `.migrated`.

The `ai_conversation` table gains the new columns via a SQLite migration (`internal/repository/migrations/NNN_add_conversation_surface.sql`). Existing rows default to `surface_origin = 'web'`, `kind = 'default_web'`, `clear_barrier_turn_id = NULL`.

## Testing

- **`agent_session/service_test.go`** — covers conversation resolution, 5-turn window assembly, `clear_barrier_turn_id` filter, slash dispatch, placeholder shape, persistence of evidence metadata.
- **`agent_session/commands/*_test.go`** — one focused test per command; assert tool calls, persisted turns, side effects on surface state.
- **`agent_session/migration_test.go`** — feeds a representative `im_context.json` and asserts the resulting DB rows + state file.
- **Bridge integration test** — `weixin_im_bridge_test.go` rewrites the existing flow to drive `agent_session.Service` end-to-end with a fake orchestrator; asserts chunk count, placeholder timing, final reply content.
- **Web integration test** — `ai_conversation_test.go` does the same on the SSE path.
- **Tool tests** — existing `library_search_tool_test.go`, `figure_lookup_tool_test.go`, etc. unchanged; the orchestrator interface they target is not modified.

## Roll-out

This change is large enough to land in stages but small enough not to need separate sub-projects. Suggested order (the implementation plan will enumerate concretely):

1. Schema migration + `agent_session.ConversationStore` skeleton + tests.
2. `agent_session.Service` with command dispatch + state commands; web handler routes through it (parity check).
3. Query-shortcut commands; switch web to use shortcut for `/recent` etc. and verify.
4. Free text → agent path inside `agent_session`; placeholder + follow-up shape; web SSE adapts.
5. WeChat bridge refactor + migration of `im_context.json` + bridge integration test.
6. Daily figure picker extraction + Overview page (panels: Research Dashboard / 图片 / Status) + new endpoints.
7. TTS button on web; DOI detection in AI input box.
8. Tear out `ai_service_weixin.go` planning code that is now dead.

## Open questions to revisit during planning

- Exact column names / migration filename — defer to existing migration conventions in `internal/repository`.
- Whether `/ask` and `/interpret` go through the constrained agent or through a direct-tool + single-summary pattern — pick the simpler one once the tool boundaries are re-examined.
- Whether the placeholder mechanism is a separate `Service.Continue` call or a callback registered with `Service.Handle` — pick the cleaner one in code; the contract above tolerates either.

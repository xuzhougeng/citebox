# AI Assistant Command Center Design

**状态**: 基本已完成，并在后续迭代中继续扩展（2026-05-01 审核）

## Current Completion Status

- **Completed**: orchestrator/tool layer, `intent_hint` routing, full-library search, external search, paper read/compare, figure lookup, process strip, result cards, citation restoration, artifact persistence, frontend shortcuts, API documentation, and no-embedding/no-vector-search constraint.
- **Evolved after this spec**: full-library search now uses a lightweight Master/Sub-Agent style planner/classifier path for local full-text scanning when configured, rather than only rule-based tool dispatch.
- **Recent additions covered by implementation but not by the original spec**: result-card collapse behavior, search-term highlighting in evidence snippets, streaming “thinking” placeholder, and figure lookup fallback through full-text candidate papers.
- **Still a product boundary**: this is not a long-running async task center; tool execution remains request-scoped.

## Purpose

The AI page should become CiteBox's central research dispatcher while still feeling like a traditional chat page. Users should be able to ask natural questions such as "帮我查找关于 ATAC 数据的文章", "查一下外部有没有 single-cell ATAC 综述", "对比这几篇文献", or "看图 1", and the system should route the request to the right internal capability.

The first implementation keeps the UI chat-first. It does not introduce a full task dashboard, real multi-agent runtime, embeddings, or vector search.

## Confirmed Product Decisions

- Use the existing chat page as the primary experience.
- Show a compact process strip per assistant turn, not a full task panel.
- Route by natural language, with four lightweight intent shortcuts above the composer:
  - `查全库`
  - `查外部`
  - `读文献`
  - `看图/图文`
- Remove `内部搜索` / `外部搜索` as primary UI switches. Keep existing backend fields temporarily for compatibility.
- Use an orchestrator/tool architecture rather than continuing to add logic directly to the conversation service.
- Use tool dispatch, not real multi-LLM sub-agents, in the first version.
- Persist process summaries, tool-call summaries, result cards, and citations so reopened conversations restore the same structured state.
- Never use embeddings or a vector database.

## Architecture

### High-Level Components

`AIConversationService` remains responsible for:

- conversation CRUD
- message persistence
- streaming response lifecycle
- title generation and summarization
- persisting run summaries, result cards, citations, and tool summaries

Add a new `AIOrchestrator`, responsible for:

- intent routing
- plan building
- tool selection
- tool execution coordination
- process summary creation
- result card assembly
- final answer context assembly

Add a tool layer with stable contracts:

- `LibrarySearchTool`
- `ExternalSearchTool`
- `PaperReadTool`
- `FigureLookupTool`

The orchestrator should not issue SQL directly or know repository details. It receives typed tool outputs and composes them into an answer context.

### Non-Goals

- No full async task center in the first version.
- No real Master/Sub-Agent runtime in the first version.
- No hidden background workers that outlive the message request.
- No embedding pipeline or vector store.
- No broad UI rewrite into a dashboard.

## User Interaction

The AI page remains a two-pane chat layout: conversations on the left, active chat on the right.

The composer gains four shortcuts. Clicking one sets an `intent_hint` for the next message. Users can ignore the shortcuts and rely on natural language routing.

One assistant turn renders in this order:

1. Compact process strip
2. AI text response
3. Structured result cards
4. Citation interactions

The process strip shows stages and counts, not raw query details. Examples:

- `全库检索 184 篇 · 命中 12 篇 · 读取 6 篇 · 生成回答`
- `外部搜索 20 条 · 命中 8 条 · 生成回答`
- `图文检索 42 张图 · 命中 5 张 · 生成回答`

Raw query terms and detailed tool inputs are persisted internally for future expandable debugging, but are not shown by default.

## Single-Turn Flow

1. Frontend sends:
   - `content`
   - optional `intent_hint`
   - optional `context`, such as current `paper_id`, `figure_id`, and source page
   - existing compatibility fields only when older clients use them

2. `AIConversationService` creates or loads the conversation and persists the user message immediately.

3. `AIOrchestrator` runs:
   - `RouteIntent`
   - `BuildPlan`
   - `RunTools`
   - `BuildProcessSummary`
   - `BuildResultCards`
   - `BuildAnswerContext`

4. The final LLM call receives the assembled evidence and card summary, then streams the assistant response.

5. The service persists:
   - assistant message
   - turn run
   - tool-call summaries
   - result cards
   - citations

6. The frontend renders streamed events and restores the persisted structured state when reopening the conversation.

## API Design

Keep `POST /api/ai/conversations/{id}/messages` as the main entrypoint and continue returning NDJSON.

Request additions:

```json
{
  "content": "帮我查找关于 ATAC 数据的文章",
  "intent_hint": "library_search",
  "context": {
    "source": "library",
    "paper_id": 42,
    "figure_id": 12
  }
}
```

`intent_hint` values:

- `library_search`
- `external_search`
- `paper_read`
- `figure_lookup`
- empty string or omitted for automatic routing

Compatibility fields:

- `strict_evidence`
- `include_external_evidence`

These remain accepted while the new UI migrates away from primary switches.

NDJSON event types:

- `meta`: conversation metadata
- `process`: compact process-strip update
- `delta`: streamed assistant text
- `cards`: result card array
- `citations`: citation array
- `done`: assistant message and turn-run metadata
- `error`: request-level error

## Persistence Model

### `ai_turn_runs`

Stores one orchestrated assistant turn.

Fields:

- `id`
- `conversation_id`
- `user_message_id`
- `assistant_message_id`
- `intent`
- `intent_hint`
- `process_summary_json`
- `status`
- `created_at`
- `updated_at`

`process_summary_json` stores the compact strip data: stages, counts, durations, and recoverable failures.

### `ai_tool_calls`

Stores tool-call summaries for observability and later expandable process views.

Fields:

- `id`
- `turn_run_id`
- `tool_name`
- `input_json`
- `output_summary_json`
- `status`
- `duration_ms`
- `error`
- `created_at`

The first UI does not need to expose the full detail, but the data should be saved.

### `ai_result_cards`

Stores structured result cards.

Fields:

- `id`
- `turn_run_id`
- `card_type`
- `sort_order`
- `payload_json`
- `created_at`

First card types:

- `paper_hit`
- `external_paper`
- `paper_compare`
- `figure_result`
- `report_outline`

`citations_json` remains on assistant messages for citation hover and footnote rendering. Result card payloads may refer to citation indexes for UI linking.

## Tool Contracts

All tools return a shared shape:

- `process`: counts, stages, and status
- `cards`: typed result cards
- `citations`: evidence snippets
- `answer_context`: concise evidence context for final answer generation

### `LibrarySearchTool`

Purpose: "查全库".

Searches the local library using keyword expansion and literal/full-text scanning over available fields. It must not use embeddings.

Inputs:

- user query
- optional candidate limit
- optional paper filters

Outputs:

- paper hit cards
- 1-3 evidence snippets per paper
- hit locations: title, abstract, body, notes, figure caption or figure notes
- relevance reason
- scanned count and hit count

For full-library research, the first response is a hit list. The user can then ask to organize the hits into a report.

### `ExternalSearchTool`

Purpose: "查外部".

The interface is generic, but the first implementation uses existing Semantic Scholar research capabilities:

- `paper/search`
- `snippet/search`
- references
- citations
- recommendations when needed

Outputs:

- external paper cards
- external evidence snippets
- source metadata
- recoverable failure information, such as rate limits

### `PaperReadTool`

Purpose: "读文献" and compare papers.

Inputs:

- one or more local `paper_id` values
- user query

Behavior:

- Performs full-text evidence scanning on selected papers.
- Supports multi-paper full-text comparison.
- Deeply expands 1-2 papers by default.
- For more than 2 papers, first returns full-text evidence scanning and a list-style comparison, then asks the user to choose 1-2 papers for deeper expansion.

Outputs:

- single-paper reading summary
- multi-paper comparison card
- evidence snippets
- unsupported or insufficient-evidence notes

### `FigureLookupTool`

Purpose: "看图/图文".

Modes:

- exact lookup inside current paper, such as "看图 1"
- cross-figure-library search, such as "找所有 ATAC 相关图"

Search fields:

- figure label
- caption
- notes
- parent paper title
- parent paper abstract

Outputs:

- figure cards with image URL
- figure label
- caption
- notes
- parent paper
- open figure/open paper actions

If the image file is missing, the card still renders caption and notes with a visible unavailable-image state.

## Frontend Refactor

The UI remains a traditional chat page. The goal is module clarity, not a visual rewrite.

Recommended module boundaries:

- `ai-reader.js`: page bootstrapping and wiring only
- `ai-conversations.js`: sidebar conversations, search, new, rename, delete
- `ai-composer.js`: composer state, auto-grow, shortcuts, `intent_hint`, send state, mention palette coordination
- `ai-message-list.js`: message rendering, stream deltas, scroll behavior, empty state
- `ai-process-strip.js`: compact process strip rendering
- `ai-result-cards.js`: structured cards
- `ai-evidence.js`: citation hover and citation linking
- `ai-conversation-view.js`: thin controller coordinating the modules

The primary UI no longer shows `内部搜索` and `外部搜索` switches. The old controls may remain internally during migration but should not be the main user-facing model.

Result card rendering:

- `paper_hit`: title, year, DOI, relevance reason, snippets, hit location, open-paper action
- `external_paper`: title, year, venue, source IDs, snippets, open external action
- `paper_compare`: compared papers, dimensions, evidence-backed differences
- `figure_result`: image, label, caption, notes, parent paper, open actions
- `report_outline`: generated outline for follow-up report work

## Error Handling

Tool failures should be recoverable when possible.

- A single tool failure does not fail the entire turn when other evidence exists.
- If all tools fail and the system can only answer normally, it must say no retrieval evidence is available.
- Semantic Scholar rate limits should not block local library search.
- Missing figure image files should not hide figure caption and notes.
- More-than-2-paper deep comparison should degrade to full-text evidence scanning and list-style comparison.
- Stopped streams keep completed process/card/tool summaries and mark the assistant message as `stopped`.

## Implementation Phases

### Phase 1: Data Model

- Add turn runs, tool calls, and result cards tables.
- Add repository methods and tests.
- Extend conversation detail reads to include turn run summaries and cards.

### Phase 2: Tool Layer

- Extract local evidence search into `LibrarySearchTool`.
- Wrap research service as `ExternalSearchTool`.
- Add first versions of `PaperReadTool` and `FigureLookupTool`.
- Add unit tests for each tool without requiring live network calls.

### Phase 3: Orchestrator

- Add rule-based intent routing first.
- Add plan building and tool execution coordination.
- Add answer-context assembly.
- Add tests for representative queries:
  - "帮我查找 ATAC 数据的文章"
  - "查一下外部有没有 single-cell ATAC 综述"
  - "对比这两篇文献"
  - "看图 1"
  - "找所有 ATAC 相关的图"

### Phase 4: Frontend Refactor

- Split composer, message list, process strip, and result cards.
- Add four shortcuts.
- Remove internal/external search switches from the primary UI.
- Preserve compatibility for old messages without process/cards.

### Phase 5: End-to-End Verification

- Run `go test ./...`.
- Run JavaScript syntax checks for touched files.
- Verify locale JSON parsing.
- Use Playwright for:
  - shortcut sends `intent_hint`
  - process strip displays stages and counts
  - full-library search returns paper cards
  - external failure still allows internal results
  - figure result card renders image and caption
  - old conversations still open

## Acceptance Criteria

- The AI page still reads as a traditional conversation page.
- Users do not need to understand `内部搜索` or `外部搜索` switches.
- The four tasks route through the orchestrator:
  - full-library research
  - external search
  - paper reading/comparison
  - figure lookup
- Each orchestrated turn can restore process strip and result cards after reopening.
- Result cards include enough evidence to explain why a paper, external item, comparison, or figure was returned.
- The system does not use embeddings.

## Risks and Mitigations

- **Risk:** Orchestrator becomes another large service.
  **Mitigation:** Keep tools independently testable and keep the orchestrator focused on routing and composition.

- **Risk:** Result card payloads drift by type.
  **Mitigation:** Define explicit payload structs in Go and version card payloads if needed.

- **Risk:** The first route classifier misroutes ambiguous user questions.
  **Mitigation:** Use shortcuts as intent hints and store route decisions for debugging.

- **Risk:** Large full-text comparisons exceed model context.
  **Mitigation:** Scan full text first, pass only selected evidence snippets to the model, and require deeper expansion for more than 2 papers.

- **Risk:** Frontend refactor regresses existing chat behavior.
  **Mitigation:** Keep old message rendering compatible and verify send, stop, delete, rename, pin, export, citations, and legacy conversations.

# AI Assistant Command Center Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

## Current Completion Status

**Status:** Core implementation complete as of 2026-05-01, with follow-up UI and retrieval refinements already applied.

This file is now an archival implementation plan. The detailed step checklist below was not maintained as the live source of truth after implementation; use this status block for current completion state.

- **Completed:** Tasks 1-8 are represented in the current codebase: orchestration persistence, router/types, internal library search, external search, paper read/compare, figure lookup, orchestrator integration, frontend composer/process strip/result cards, locale strings, and API docs.
- **Evolved after the plan:** The library search tool now supports a lightweight Master/Sub-Agent style planner/classifier path for local full-text scanning; result cards are collapsible by default and evidence snippets can highlight search terms.
- **Recent fixes included:** Streaming shows a “thinking” placeholder before text arrives, figure lookup can fall back through full-text candidate papers, and model settings now support provider-specific `thinking`/reasoning behavior.
- **Still intentionally not done:** No embeddings, no vector database, and no long-running background task center.

**Goal:** Rebuild the AI page into a chat-first command center that routes full-library search, external search, paper reading/comparison, and figure lookup through an orchestrator/tool layer with persisted process strips and result cards.

**Architecture:** Keep `AIConversationService` as the conversation and streaming lifecycle owner. Add an `internal/service/ai_assistant/` package for intent routing, tool execution, process summaries, result cards, and answer-context assembly. Persist turn runs, tool-call summaries, result cards, and citations so reopened conversations restore structured state.

**Tech Stack:** Go, SQLite, native HTML/CSS/JavaScript, NDJSON streaming, existing Semantic Scholar research service, existing paper and figure repositories. No embeddings or vector database.

---

## File Map

Backend files to create:

- `internal/service/ai_assistant/types.go`: shared orchestrator/tool/result-card/process types.
- `internal/service/ai_assistant/router.go`: rule-based intent routing.
- `internal/service/ai_assistant/orchestrator.go`: plan, tool execution, process/card/context assembly.
- `internal/service/ai_assistant/library_search_tool.go`: full-library search wrapper around existing evidence scanning.
- `internal/service/ai_assistant/external_search_tool.go`: Semantic Scholar-backed external search tool behind a generic interface.
- `internal/service/ai_assistant/paper_read_tool.go`: selected-paper full-text read/compare tool.
- `internal/service/ai_assistant/figure_lookup_tool.go`: exact figure lookup and cross-figure search.
- `internal/service/ai_assistant/*_test.go`: unit tests for router, tools, and orchestrator.

Backend files to modify:

- `internal/repository/schema/schema.go`: add three AI orchestration tables and migration helpers.
- `internal/repository/ai_conversation_repo.go`: add run/tool/card models and persistence methods.
- `internal/repository/ai_conversation_repo_test.go`: schema and persistence tests.
- `internal/repository/paper_repo.go`: reuse `ListEvidenceCandidatePaperIDs` as the candidate provider for `LibrarySearchTool`.
- `internal/repository/figure_repo.go`: reuse existing `ListFigures` keyword and paper filters through a narrow adapter.
- `internal/service/ai_conversation/types.go`: add turn run and card fields to read models; add `IntentHint` and `Context` to send input.
- `internal/service/ai_conversation/service.go`: call orchestrator before final LLM call and persist orchestration artifacts.
- `internal/service/ai_conversation/evidence.go`: move reusable evidence term/snippet helpers into `ai_assistant` or keep exported wrapper functions.
- `internal/handler/ai_conversation.go`: accept `intent_hint` and `context`, stream `process`, `cards`, and `citations` events.
- `internal/handler/ai_conversation_test.go`: request/stream tests.
- `internal/app/server.go`: wire orchestrator and tools.
- `docs/api.md`: document new request fields and NDJSON events.

Frontend files to create:

- `web/static/js/ai-composer.js`: input, shortcuts, intent hint, send state.
- `web/static/js/ai-message-list.js`: message and stream rendering.
- `web/static/js/ai-process-strip.js`: compact process strip.
- `web/static/js/ai-result-cards.js`: paper/external/compare/figure/report cards.

Frontend files to modify:

- `web/ai.html`: add shortcut container and new scripts; remove primary internal/external switch UI.
- `web/static/js/ai-reader.js`: wiring only.
- `web/static/js/ai-conversation-view.js`: thin controller.
- `web/static/js/ai-evidence.js`: support citation linking from result cards.
- `web/static/css/features/ai.css`: shortcut/process/card styling.
- `web/static/locales/zh-CN/ai.json` and `web/static/locales/en/ai.json`: all new UI copy.

---

## Task 1: Schema And Repository Persistence

**Files:**

- Modify: `internal/repository/schema/schema.go`
- Modify: `internal/repository/ai_conversation_repo.go`
- Modify: `internal/repository/ai_conversation_repo_test.go`

- [ ] **Step 1: Write failing repository tests**

Add tests to `internal/repository/ai_conversation_repo_test.go`:

```go
func TestAIConversationRunArtifactsRoundTrip(t *testing.T) {
	repo := newAIConversationRepoForTest(t)
	convID, err := repo.CreateConversation()
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	userID, err := repo.AddMessage(convID, "user", "帮我查找 ATAC 数据", AIMessageMeta{})
	if err != nil {
		t.Fatalf("AddMessage user: %v", err)
	}
	assistantID, err := repo.AddMessage(convID, "assistant", "找到相关文献", AIMessageMeta{CitationsJSON: `[{"i":1}]`})
	if err != nil {
		t.Fatalf("AddMessage assistant: %v", err)
	}

	runID, err := repo.CreateTurnRun(AITurnRun{
		ConversationID:    convID,
		UserMessageID:     userID,
		AssistantMessageID: assistantID,
		Intent:            "library_search",
		IntentHint:        "library_search",
		ProcessSummaryJSON: `{"stages":[{"label":"全库检索","count":184},{"label":"命中","count":12}]}`,
		Status:            "completed",
	})
	if err != nil {
		t.Fatalf("CreateTurnRun: %v", err)
	}
	if _, err := repo.AddToolCall(AIToolCall{
		TurnRunID:         runID,
		ToolName:          "library_search",
		InputJSON:         `{"query":"ATAC"}`,
		OutputSummaryJSON: `{"scanned":184,"hits":12}`,
		Status:            "completed",
		DurationMS:        17,
	}); err != nil {
		t.Fatalf("AddToolCall: %v", err)
	}
	if _, err := repo.AddResultCard(AIResultCard{
		TurnRunID:   runID,
		CardType:    "paper_hit",
		SortOrder:   1,
		PayloadJSON: `{"paper_id":42,"title":"ATAC Paper"}`,
	}); err != nil {
		t.Fatalf("AddResultCard: %v", err)
	}

	runs, err := repo.ListTurnRuns(convID)
	if err != nil {
		t.Fatalf("ListTurnRuns: %v", err)
	}
	if len(runs) != 1 || runs[0].Intent != "library_search" || runs[0].ProcessSummaryJSON == "" {
		t.Fatalf("runs = %+v", runs)
	}
	calls, err := repo.ListToolCalls(runID)
	if err != nil {
		t.Fatalf("ListToolCalls: %v", err)
	}
	if len(calls) != 1 || calls[0].ToolName != "library_search" || calls[0].DurationMS != 17 {
		t.Fatalf("calls = %+v", calls)
	}
	cards, err := repo.ListResultCards(runID)
	if err != nil {
		t.Fatalf("ListResultCards: %v", err)
	}
	if len(cards) != 1 || cards[0].CardType != "paper_hit" || cards[0].SortOrder != 1 {
		t.Fatalf("cards = %+v", cards)
	}
}

func TestAIConversationRunArtifactsCascadeWithConversation(t *testing.T) {
	repo := newAIConversationRepoForTest(t)
	convID, _ := repo.CreateConversation()
	userID, _ := repo.AddMessage(convID, "user", "q", AIMessageMeta{})
	assistantID, _ := repo.AddMessage(convID, "assistant", "a", AIMessageMeta{})
	runID, err := repo.CreateTurnRun(AITurnRun{
		ConversationID: convID, UserMessageID: userID, AssistantMessageID: assistantID,
		Intent: "library_search", Status: "completed",
	})
	if err != nil {
		t.Fatalf("CreateTurnRun: %v", err)
	}
	_, _ = repo.AddToolCall(AIToolCall{TurnRunID: runID, ToolName: "library_search", Status: "completed"})
	_, _ = repo.AddResultCard(AIResultCard{TurnRunID: runID, CardType: "paper_hit", SortOrder: 1, PayloadJSON: `{}`})

	if err := repo.DeleteConversation(convID); err != nil {
		t.Fatalf("DeleteConversation: %v", err)
	}
	runs, err := repo.ListTurnRuns(convID)
	if err != nil {
		t.Fatalf("ListTurnRuns after delete: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("runs after delete = %+v, want empty", runs)
	}
}
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```bash
go test ./internal/repository -run 'TestAIConversationRunArtifacts' -count=1
```

Expected: compile failure for undefined `AITurnRun`, `AIToolCall`, `AIResultCard`, and repository methods.

- [ ] **Step 3: Add schema**

In `internal/repository/schema/schema.go`, add these tables to `initSchema()` after `ai_conversation_papers`:

```sql
	CREATE TABLE IF NOT EXISTS ai_turn_runs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		conversation_id INTEGER NOT NULL REFERENCES ai_conversations(id) ON DELETE CASCADE,
		user_message_id INTEGER NOT NULL REFERENCES ai_messages(id) ON DELETE CASCADE,
		assistant_message_id INTEGER REFERENCES ai_messages(id) ON DELETE SET NULL,
		intent TEXT NOT NULL DEFAULT '',
		intent_hint TEXT NOT NULL DEFAULT '',
		process_summary_json TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'completed' CHECK(status IN ('running','completed','stopped','failed')),
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS ai_tool_calls (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		turn_run_id INTEGER NOT NULL REFERENCES ai_turn_runs(id) ON DELETE CASCADE,
		tool_name TEXT NOT NULL DEFAULT '',
		input_json TEXT NOT NULL DEFAULT '',
		output_summary_json TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'completed' CHECK(status IN ('running','completed','skipped','failed')),
		duration_ms INTEGER NOT NULL DEFAULT 0,
		error TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS ai_result_cards (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		turn_run_id INTEGER NOT NULL REFERENCES ai_turn_runs(id) ON DELETE CASCADE,
		card_type TEXT NOT NULL DEFAULT '',
		sort_order INTEGER NOT NULL DEFAULT 0,
		payload_json TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
```

Add indexes:

```sql
	CREATE INDEX IF NOT EXISTS idx_ai_turn_runs_conv ON ai_turn_runs(conversation_id, id);
	CREATE INDEX IF NOT EXISTS idx_ai_turn_runs_user ON ai_turn_runs(user_message_id);
	CREATE INDEX IF NOT EXISTS idx_ai_tool_calls_run ON ai_tool_calls(turn_run_id, id);
	CREATE INDEX IF NOT EXISTS idx_ai_result_cards_run ON ai_result_cards(turn_run_id, sort_order, id);
```

In `ensureSchemaColumns()`, call a new helper before `ensureIndexes()`:

```go
	if err := m.ensureAIOrchestrationSchema(); err != nil {
		return err
	}
```

Add the helper:

```go
func (m *Manager) ensureAIOrchestrationSchema() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS ai_turn_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			conversation_id INTEGER NOT NULL REFERENCES ai_conversations(id) ON DELETE CASCADE,
			user_message_id INTEGER NOT NULL REFERENCES ai_messages(id) ON DELETE CASCADE,
			assistant_message_id INTEGER REFERENCES ai_messages(id) ON DELETE SET NULL,
			intent TEXT NOT NULL DEFAULT '',
			intent_hint TEXT NOT NULL DEFAULT '',
			process_summary_json TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'completed' CHECK(status IN ('running','completed','stopped','failed')),
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS ai_tool_calls (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			turn_run_id INTEGER NOT NULL REFERENCES ai_turn_runs(id) ON DELETE CASCADE,
			tool_name TEXT NOT NULL DEFAULT '',
			input_json TEXT NOT NULL DEFAULT '',
			output_summary_json TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'completed' CHECK(status IN ('running','completed','skipped','failed')),
			duration_ms INTEGER NOT NULL DEFAULT 0,
			error TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS ai_result_cards (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			turn_run_id INTEGER NOT NULL REFERENCES ai_turn_runs(id) ON DELETE CASCADE,
			card_type TEXT NOT NULL DEFAULT '',
			sort_order INTEGER NOT NULL DEFAULT 0,
			payload_json TEXT NOT NULL DEFAULT '{}',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ai_turn_runs_conv ON ai_turn_runs(conversation_id, id)`,
		`CREATE INDEX IF NOT EXISTS idx_ai_turn_runs_user ON ai_turn_runs(user_message_id)`,
		`CREATE INDEX IF NOT EXISTS idx_ai_tool_calls_run ON ai_tool_calls(turn_run_id, id)`,
		`CREATE INDEX IF NOT EXISTS idx_ai_result_cards_run ON ai_result_cards(turn_run_id, sort_order, id)`,
	}
	for _, stmt := range stmts {
		if _, err := m.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 4: Add repository models and methods**

In `internal/repository/ai_conversation_repo.go`, add:

```go
type AITurnRun struct {
	ID                 int64
	ConversationID     int64
	UserMessageID      int64
	AssistantMessageID sql.NullInt64
	Intent             string
	IntentHint         string
	ProcessSummaryJSON string
	Status             string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type AIToolCall struct {
	ID                int64
	TurnRunID         int64
	ToolName          string
	InputJSON         string
	OutputSummaryJSON string
	Status            string
	DurationMS        int
	Error             string
	CreatedAt         time.Time
}

type AIResultCard struct {
	ID          int64
	TurnRunID   int64
	CardType    string
	SortOrder   int
	PayloadJSON string
	CreatedAt   time.Time
}
```

Add methods:

```go
func (r *AIConversationRepository) CreateTurnRun(run AITurnRun) (int64, error) {
	status := strings.TrimSpace(run.Status)
	if status == "" {
		status = "completed"
	}
	res, err := r.db.Exec(`
		INSERT INTO ai_turn_runs (
			conversation_id, user_message_id, assistant_message_id,
			intent, intent_hint, process_summary_json, status
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, run.ConversationID, run.UserMessageID, nullableInt64(run.AssistantMessageID),
		run.Intent, run.IntentHint, run.ProcessSummaryJSON, status)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *AIConversationRepository) UpdateTurnRunAssistant(runID, assistantMessageID int64, status string) error {
	if strings.TrimSpace(status) == "" {
		status = "completed"
	}
	_, err := r.db.Exec(`
		UPDATE ai_turn_runs
		SET assistant_message_id = ?, status = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, assistantMessageID, status, runID)
	return err
}

func (r *AIConversationRepository) AddToolCall(call AIToolCall) (int64, error) {
	status := strings.TrimSpace(call.Status)
	if status == "" {
		status = "completed"
	}
	res, err := r.db.Exec(`
		INSERT INTO ai_tool_calls (
			turn_run_id, tool_name, input_json, output_summary_json,
			status, duration_ms, error
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, call.TurnRunID, call.ToolName, call.InputJSON, call.OutputSummaryJSON,
		status, call.DurationMS, call.Error)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *AIConversationRepository) AddResultCard(card AIResultCard) (int64, error) {
	payload := strings.TrimSpace(card.PayloadJSON)
	if payload == "" {
		payload = "{}"
	}
	res, err := r.db.Exec(`
		INSERT INTO ai_result_cards (turn_run_id, card_type, sort_order, payload_json)
		VALUES (?, ?, ?, ?)
	`, card.TurnRunID, card.CardType, card.SortOrder, payload)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *AIConversationRepository) ListTurnRuns(conversationID int64) ([]AITurnRun, error) {
	rows, err := r.db.Query(`
		SELECT id, conversation_id, user_message_id, assistant_message_id,
		       intent, intent_hint, process_summary_json, status, created_at, updated_at
		FROM ai_turn_runs
		WHERE conversation_id = ?
		ORDER BY id ASC
	`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AITurnRun, 0)
	for rows.Next() {
		var run AITurnRun
		if err := rows.Scan(&run.ID, &run.ConversationID, &run.UserMessageID,
			&run.AssistantMessageID, &run.Intent, &run.IntentHint,
			&run.ProcessSummaryJSON, &run.Status, &run.CreatedAt, &run.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

func (r *AIConversationRepository) ListToolCalls(turnRunID int64) ([]AIToolCall, error) {
	rows, err := r.db.Query(`
		SELECT id, turn_run_id, tool_name, input_json, output_summary_json,
		       status, duration_ms, error, created_at
		FROM ai_tool_calls
		WHERE turn_run_id = ?
		ORDER BY id ASC
	`, turnRunID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AIToolCall, 0)
	for rows.Next() {
		var call AIToolCall
		if err := rows.Scan(&call.ID, &call.TurnRunID, &call.ToolName, &call.InputJSON,
			&call.OutputSummaryJSON, &call.Status, &call.DurationMS, &call.Error, &call.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, call)
	}
	return out, rows.Err()
}

func (r *AIConversationRepository) ListResultCards(turnRunID int64) ([]AIResultCard, error) {
	rows, err := r.db.Query(`
		SELECT id, turn_run_id, card_type, sort_order, payload_json, created_at
		FROM ai_result_cards
		WHERE turn_run_id = ?
		ORDER BY sort_order ASC, id ASC
	`, turnRunID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AIResultCard, 0)
	for rows.Next() {
		var card AIResultCard
		if err := rows.Scan(&card.ID, &card.TurnRunID, &card.CardType, &card.SortOrder, &card.PayloadJSON, &card.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, card)
	}
	return out, rows.Err()
}

func nullableInt64(v sql.NullInt64) interface{} {
	if !v.Valid {
		return nil
	}
	return v.Int64
}
```

- [ ] **Step 5: Run repository tests**

Run:

```bash
gofmt -w internal/repository/schema/schema.go internal/repository/ai_conversation_repo.go internal/repository/ai_conversation_repo_test.go
go test ./internal/repository -run 'TestAIConversationRunArtifacts|TestAIConversationRepository' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/repository/schema/schema.go internal/repository/ai_conversation_repo.go internal/repository/ai_conversation_repo_test.go
git commit -m "feat(ai): persist orchestrated turn artifacts"
```

---

## Task 2: Assistant Shared Types And Intent Router

**Files:**

- Create: `internal/service/ai_assistant/types.go`
- Create: `internal/service/ai_assistant/router.go`
- Create: `internal/service/ai_assistant/router_test.go`

- [ ] **Step 1: Write failing router tests**

Create `internal/service/ai_assistant/router_test.go`:

```go
package ai_assistant

import "testing"

func TestRouteIntentHonorsHint(t *testing.T) {
	got := RouteIntent(RouteInput{Content: "普通问题", IntentHint: IntentFigureLookup})
	if got.Intent != IntentFigureLookup || got.Confidence != "hint" {
		t.Fatalf("route = %+v", got)
	}
}

func TestRouteIntentDetectsLibrarySearch(t *testing.T) {
	got := RouteIntent(RouteInput{Content: "帮我查找包括 ATAC 数据的文章"})
	if got.Intent != IntentLibrarySearch {
		t.Fatalf("intent = %q", got.Intent)
	}
}

func TestRouteIntentDetectsExternalSearch(t *testing.T) {
	got := RouteIntent(RouteInput{Content: "查一下外部有没有 single-cell ATAC 综述"})
	if got.Intent != IntentExternalSearch {
		t.Fatalf("intent = %q", got.Intent)
	}
}

func TestRouteIntentDetectsPaperCompare(t *testing.T) {
	got := RouteIntent(RouteInput{Content: "对比这两篇文献的结论差异", Context: RequestContext{PaperIDs: []int64{1, 2}}})
	if got.Intent != IntentPaperRead {
		t.Fatalf("intent = %q", got.Intent)
	}
}

func TestRouteIntentDetectsFigureLookup(t *testing.T) {
	for _, q := range []string{"看图 1", "找所有 ATAC 相关的图"} {
		got := RouteIntent(RouteInput{Content: q})
		if got.Intent != IntentFigureLookup {
			t.Fatalf("RouteIntent(%q) = %q", q, got.Intent)
		}
	}
}
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```bash
go test ./internal/service/ai_assistant -run TestRouteIntent -count=1
```

Expected: package or symbol missing.

- [ ] **Step 3: Add shared types**

Create `internal/service/ai_assistant/types.go`:

```go
package ai_assistant

import "github.com/xuzhougeng/citebox/internal/service/research"

const (
	IntentAuto           = ""
	IntentLibrarySearch  = "library_search"
	IntentExternalSearch = "external_search"
	IntentPaperRead      = "paper_read"
	IntentFigureLookup   = "figure_lookup"
	IntentChat           = "chat"
)

type RequestContext struct {
	Source   string  `json:"source,omitempty"`
	PaperID  int64   `json:"paper_id,omitempty"`
	PaperIDs []int64 `json:"paper_ids,omitempty"`
	FigureID int64   `json:"figure_id,omitempty"`
}

type RouteInput struct {
	Content    string
	IntentHint string
	Context    RequestContext
}

type RouteDecision struct {
	Intent     string
	Confidence string
	Reason     string
}

type ProcessStage struct {
	Label      string `json:"label"`
	Count      int    `json:"count,omitempty"`
	Unit       string `json:"unit,omitempty"`
	Status     string `json:"status,omitempty"`
	DurationMS int   `json:"duration_ms,omitempty"`
}

type ProcessSummary struct {
	Intent string         `json:"intent"`
	Stages []ProcessStage `json:"stages"`
	Note   string         `json:"note,omitempty"`
}

type ResultCard struct {
	Type    string `json:"type"`
	Payload any   `json:"payload"`
}

type ToolCallSummary struct {
	ToolName          string
	InputJSON         string
	OutputSummaryJSON string
	Status            string
	DurationMS        int
	Error             string
}

type Citation struct {
	I          int              `json:"i"`
	PaperID    int64            `json:"paper_id,omitempty"`
	ExternalID string           `json:"external_id,omitempty"`
	S2PaperID  string           `json:"s2_paper_id,omitempty"`
	Title      string           `json:"title,omitempty"`
	Source     string           `json:"source,omitempty"`
	Snippet    research.Snippet `json:"snippet"`
	Score      float64          `json:"score,omitempty"`
}

type ToolResult struct {
	Process       ProcessSummary
	Cards         []ResultCard
	Citations     []Citation
	AnswerContext string
	ToolCalls     []ToolCallSummary
}
```

- [ ] **Step 4: Add router**

Create `internal/service/ai_assistant/router.go`:

```go
package ai_assistant

import "strings"

func RouteIntent(in RouteInput) RouteDecision {
	if isKnownIntent(in.IntentHint) {
		return RouteDecision{Intent: in.IntentHint, Confidence: "hint", Reason: "user selected shortcut"}
	}
	q := strings.ToLower(strings.TrimSpace(in.Content))
	if q == "" {
		return RouteDecision{Intent: IntentChat, Confidence: "low", Reason: "empty content"}
	}
	if containsAny(q, "图", "figure", "fig.", "fig ") {
		return RouteDecision{Intent: IntentFigureLookup, Confidence: "rule", Reason: "figure terms"}
	}
	if containsAny(q, "外部", "semantic scholar", "pubmed", "web", "综述", "review") {
		return RouteDecision{Intent: IntentExternalSearch, Confidence: "rule", Reason: "external search terms"}
	}
	if containsAny(q, "对比", "比较", "compare", "异同", "差异") || len(in.Context.PaperIDs) > 1 {
		return RouteDecision{Intent: IntentPaperRead, Confidence: "rule", Reason: "compare terms or multiple papers"}
	}
	if containsAny(q, "查找", "找", "检索", "哪些文章", "哪些文献", "相关文献", "相关的文章", "articles", "papers") {
		return RouteDecision{Intent: IntentLibrarySearch, Confidence: "rule", Reason: "library search terms"}
	}
	if in.Context.PaperID > 0 {
		return RouteDecision{Intent: IntentPaperRead, Confidence: "rule", Reason: "paper context"}
	}
	return RouteDecision{Intent: IntentChat, Confidence: "low", Reason: "default chat"}
}

func isKnownIntent(intent string) bool {
	switch intent {
	case IntentLibrarySearch, IntentExternalSearch, IntentPaperRead, IntentFigureLookup:
		return true
	default:
		return false
	}
}

func containsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 5: Run tests**

Run:

```bash
gofmt -w internal/service/ai_assistant
go test ./internal/service/ai_assistant -run TestRouteIntent -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/service/ai_assistant
git commit -m "feat(ai): add assistant intent routing types"
```

---

## Task 3: Library Search Tool

**Files:**

- Create: `internal/service/ai_assistant/library_search_tool.go`
- Create: `internal/service/ai_assistant/library_search_tool_test.go`
- Modify: `internal/service/ai_conversation/evidence.go`

- [ ] **Step 1: Write failing tool tests**

Create `internal/service/ai_assistant/library_search_tool_test.go`:

```go
package ai_assistant

import (
	"context"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/model"
)

type stubPaperStore struct {
	papers map[int64]*model.Paper
	ids    []int64
}

func (s stubPaperStore) GetPaperDetail(id int64) (*model.Paper, error) {
	return s.papers[id], nil
}

func (s stubPaperStore) ListEvidenceCandidatePaperIDs(terms []string, limit int) ([]int64, error) {
	return append([]int64(nil), s.ids...), nil
}

func TestLibrarySearchToolReturnsPaperHitCards(t *testing.T) {
	tool := NewLibrarySearchTool(stubPaperStore{
		ids: []int64{1, 2},
		papers: map[int64]*model.Paper{
			1: {ID: 1, Title: "ATAC Atlas", DOI: "10.1/atac", PDFText: "ATAC-seq identifies chromatin accessibility changes."},
			2: {ID: 2, Title: "Unrelated", PDFText: "Protein localization only."},
		},
	})
	res, err := tool.Run(context.Background(), ToolInput{Query: "帮我查找包括 ATAC 数据的文章"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Cards) != 1 || res.Cards[0].Type != "paper_hit" {
		t.Fatalf("cards = %+v", res.Cards)
	}
	if len(res.Citations) != 1 || res.Citations[0].PaperID != 1 {
		t.Fatalf("citations = %+v", res.Citations)
	}
	if !strings.Contains(res.AnswerContext, "ATAC Atlas") || !strings.Contains(res.AnswerContext, "chromatin accessibility") {
		t.Fatalf("answer context = %s", res.AnswerContext)
	}
	if len(res.Process.Stages) == 0 || res.Process.Stages[0].Label != "全库检索" {
		t.Fatalf("process = %+v", res.Process)
	}
}
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```bash
go test ./internal/service/ai_assistant -run TestLibrarySearchToolReturnsPaperHitCards -count=1
```

Expected: undefined `NewLibrarySearchTool` and `ToolInput`.

- [ ] **Step 3: Add tool input type**

Append to `internal/service/ai_assistant/types.go`:

```go
type ToolInput struct {
	Query     string
	Context   RequestContext
	Limit     int
	IntentHint string
}
```

- [ ] **Step 4: Add library search tool**

Create `internal/service/ai_assistant/library_search_tool.go`:

```go
package ai_assistant

import (
	"context"
	"fmt"
	"strings"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/service/research"
)

type PaperGetter interface {
	GetPaperDetail(id int64) (*model.Paper, error)
}

type EvidenceCandidateLister interface {
	ListEvidenceCandidatePaperIDs(terms []string, limit int) ([]int64, error)
}

type LibrarySearchTool struct {
	papers PaperGetter
}

func NewLibrarySearchTool(papers PaperGetter) *LibrarySearchTool {
	return &LibrarySearchTool{papers: papers}
}

func (t *LibrarySearchTool) Run(ctx context.Context, in ToolInput) (ToolResult, error) {
	_ = ctx
	limit := in.Limit
	if limit <= 0 {
		limit = 12
	}
	terms := EvidenceSearchTerms(in.Query)
	ids := candidateIDs(t.papers, terms, 120)
	cards := make([]ResultCard, 0, limit)
	citations := make([]Citation, 0, limit)
	for _, id := range ids {
		if len(cards) >= limit {
			break
		}
		paper, err := t.papers.GetPaperDetail(id)
		if err != nil || paper == nil {
			continue
		}
		matches := FindLocalEvidenceMatches(*paper, terms, 3)
		if len(matches) == 0 {
			continue
		}
		snippets := make([]PaperHitSnippet, 0, len(matches))
		for _, m := range matches {
			citation := Citation{
				I:       len(citations) + 1,
				PaperID: paper.ID,
				Title:   paper.Title,
				Source:  "local",
				Snippet: m.Snippet,
				Score:   m.Score,
			}
			citations = append(citations, citation)
			snippets = append(snippets, PaperHitSnippet{
				CitationIndex: citation.I,
				Location:      m.Location,
				Text:          m.Snippet.Text,
			})
		}
		card := PaperHitCard{
			PaperID: paper.ID,
			Title:   paper.Title,
			DOI:     paper.DOI,
			Year:    paper.PublishedAt,
			Reason:  "命中 " + strings.Join(matchedLocations(snippets), "、"),
			Snippets: snippets,
		}
		cards = append(cards, ResultCard{Type: "paper_hit", Payload: card})
	}
	return ToolResult{
		Process: ProcessSummary{
			Intent: IntentLibrarySearch,
			Stages: []ProcessStage{
				{Label: "全库检索", Count: len(ids), Unit: "篇", Status: "completed"},
				{Label: "命中", Count: len(cards), Unit: "篇", Status: "completed"},
			},
		},
		Cards:         cards,
		Citations:     citations,
		AnswerContext: libraryAnswerContext(cards),
		ToolCalls: []ToolCallSummary{{
			ToolName:          "library_search",
			InputJSON:         fmt.Sprintf(`{"query":%q}`, in.Query),
			OutputSummaryJSON: fmt.Sprintf(`{"candidates":%d,"hits":%d}`, len(ids), len(cards)),
			Status:            "completed",
		}},
	}, nil
}

type LocalEvidenceMatch struct {
	Location string
	Snippet  research.Snippet
	Score    float64
}
```

Also add these payload structs in `types.go`:

```go
type PaperHitCard struct {
	PaperID  int64             `json:"paper_id"`
	Title    string            `json:"title"`
	DOI      string            `json:"doi,omitempty"`
	Year     string            `json:"year,omitempty"`
	Reason   string            `json:"reason"`
	Snippets []PaperHitSnippet `json:"snippets"`
}

type PaperHitSnippet struct {
	CitationIndex int    `json:"citation_index"`
	Location      string `json:"location"`
	Text          string `json:"text"`
}
```

Move or copy the term expansion and local match helpers from `internal/service/ai_conversation/evidence.go` into this package as exported helpers:

```go
func EvidenceSearchTerms(query string) []string
func FindLocalEvidenceMatches(paper model.Paper, terms []string, limit int) []LocalEvidenceMatch
```

The helper implementation must preserve current behavior:

- expand `ATAC 数据` to `ATAC-seq`, `chromatin accessibility`, `scATAC-seq`
- scan title, abstract, notes, paper notes, and PDF text
- return snippets with locations
- never use embeddings

- [ ] **Step 5: Resolve compile issues deliberately**

Run:

```bash
go test ./internal/service/ai_assistant -run TestLibrarySearchToolReturnsPaperHitCards -count=1
```

If the compiler reports missing imports or duplicate helpers, fix only this package boundary. Do not change API or frontend code in this task.

- [ ] **Step 6: Commit**

```bash
gofmt -w internal/service/ai_assistant internal/service/ai_conversation/evidence.go
go test ./internal/service/ai_assistant -run TestLibrarySearchToolReturnsPaperHitCards -count=1
git add internal/service/ai_assistant internal/service/ai_conversation/evidence.go
git commit -m "feat(ai): add library search tool"
```

---

## Task 4: External, Paper Read, And Figure Tools

**Files:**

- Create: `internal/service/ai_assistant/external_search_tool.go`
- Create: `internal/service/ai_assistant/paper_read_tool.go`
- Create: `internal/service/ai_assistant/figure_lookup_tool.go`
- Create: `internal/service/ai_assistant/tools_test.go`

- [ ] **Step 1: Write failing tool tests**

Create `internal/service/ai_assistant/tools_test.go`:

```go
package ai_assistant

import (
	"context"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/service/research"
)

type stubExternalSearch struct {
	search research.PaperList
	snips  research.SnippetList
}

func (s stubExternalSearch) Search(ctx context.Context, query string, opts research.SearchOpts) (research.PaperList, error) {
	return s.search, nil
}

func (s stubExternalSearch) SnippetSearch(ctx context.Context, query string, opts research.SnippetSearchOpts) (research.SnippetList, error) {
	return s.snips, nil
}

func TestExternalSearchToolReturnsExternalPaperCards(t *testing.T) {
	tool := NewExternalSearchTool(stubExternalSearch{
		search: research.PaperList{Items: []research.Paper{{
			PaperID: "s2-1", Title: "ATAC Review", Year: 2024, Venue: "Genome Biology",
			ExternalIDs: research.IDs{DOI: "10.1/ext"}, TLDR: "ATAC review summary.",
		}}},
	})
	res, err := tool.Run(context.Background(), ToolInput{Query: "single-cell ATAC 综述"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Cards) != 1 || res.Cards[0].Type != "external_paper" {
		t.Fatalf("cards = %+v", res.Cards)
	}
	if !strings.Contains(res.AnswerContext, "ATAC Review") {
		t.Fatalf("answer context = %s", res.AnswerContext)
	}
}

func TestPaperReadToolComparesFullText(t *testing.T) {
	store := stubPaperStore{
		ids: []int64{1, 2},
		papers: map[int64]*model.Paper{
			1: {ID: 1, Title: "Paper A", PDFText: "ATAC-seq measures chromatin accessibility."},
			2: {ID: 2, Title: "Paper B", PDFText: "scRNA-seq measures gene expression."},
		},
	}
	tool := NewPaperReadTool(store)
	res, err := tool.Run(context.Background(), ToolInput{
		Query: "对比这两篇文献",
		Context: RequestContext{PaperIDs: []int64{1, 2}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Cards) != 1 || res.Cards[0].Type != "paper_compare" {
		t.Fatalf("cards = %+v", res.Cards)
	}
	if len(res.Citations) == 0 {
		t.Fatalf("citations empty")
	}
}

type stubFigureStore struct {
	figures []FigureRecord
}

func (s stubFigureStore) SearchFigures(query string, paperID int64, limit int) ([]FigureRecord, int, error) {
	return s.figures, len(s.figures), nil
}

func TestFigureLookupToolReturnsFigureCards(t *testing.T) {
	tool := NewFigureLookupTool(stubFigureStore{
		figures: []FigureRecord{{
			FigureID: 7, PaperID: 1, PaperTitle: "ATAC Paper", DisplayLabel: "Fig 1",
			ImageURL: "/api/figures/7/image", Caption: "ATAC-seq overview", NotesText: "Important panel.",
		}},
	})
	res, err := tool.Run(context.Background(), ToolInput{Query: "看图 1", Context: RequestContext{PaperID: 1}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Cards) != 1 || res.Cards[0].Type != "figure_result" {
		t.Fatalf("cards = %+v", res.Cards)
	}
	if !strings.Contains(res.AnswerContext, "ATAC-seq overview") {
		t.Fatalf("answer context = %s", res.AnswerContext)
	}
}
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```bash
go test ./internal/service/ai_assistant -run 'TestExternalSearchTool|TestPaperReadTool|TestFigureLookupTool' -count=1
```

Expected: missing constructors and types.

- [ ] **Step 3: Implement ExternalSearchTool**

Create `internal/service/ai_assistant/external_search_tool.go` with:

```go
package ai_assistant

import (
	"context"
	"fmt"
	"strings"

	"github.com/xuzhougeng/citebox/internal/service/research"
)

type ExternalSearcher interface {
	Search(ctx context.Context, query string, opts research.SearchOpts) (research.PaperList, error)
	SnippetSearch(ctx context.Context, query string, opts research.SnippetSearchOpts) (research.SnippetList, error)
}

type ExternalSearchTool struct {
	searcher ExternalSearcher
}

func NewExternalSearchTool(searcher ExternalSearcher) *ExternalSearchTool {
	return &ExternalSearchTool{searcher: searcher}
}

func (t *ExternalSearchTool) Run(ctx context.Context, in ToolInput) (ToolResult, error) {
	limit := in.Limit
	if limit <= 0 {
		limit = 8
	}
	res, err := t.searcher.Search(ctx, in.Query, research.SearchOpts{Limit: limit})
	if err != nil {
		return ToolResult{
			Process: ProcessSummary{Intent: IntentExternalSearch, Stages: []ProcessStage{{Label: "外部搜索", Status: "failed"}}},
			ToolCalls: []ToolCallSummary{{ToolName: "external_search", InputJSON: fmt.Sprintf(`{"query":%q}`, in.Query), Status: "failed", Error: err.Error()}},
		}, nil
	}
	cards := make([]ResultCard, 0, len(res.Items))
	citations := make([]Citation, 0, len(res.Items))
	for _, p := range res.Items {
		citation := Citation{
			I:          len(citations) + 1,
			S2PaperID:  p.PaperID,
			ExternalID: externalID(p),
			Title:      p.Title,
			Source:     "external",
			Snippet:    research.Snippet{Text: firstNonEmpty(p.TLDR, p.Abstract, p.Title), SnippetKind: "abstract", Section: "Semantic Scholar"},
		}
		citations = append(citations, citation)
		cards = append(cards, ResultCard{Type: "external_paper", Payload: ExternalPaperCard{
			S2PaperID: p.PaperID, Title: p.Title, Year: p.Year, Venue: p.Venue,
			DOI: p.ExternalIDs.DOI, TLDR: p.TLDR, CitationIndex: citation.I,
		}})
	}
	return ToolResult{
		Process: ProcessSummary{Intent: IntentExternalSearch, Stages: []ProcessStage{
			{Label: "外部搜索", Count: len(res.Items), Unit: "条", Status: "completed"},
			{Label: "命中", Count: len(cards), Unit: "条", Status: "completed"},
		}},
		Cards:         cards,
		Citations:     citations,
		AnswerContext: externalAnswerContext(cards),
		ToolCalls: []ToolCallSummary{{ToolName: "external_search", InputJSON: fmt.Sprintf(`{"query":%q}`, in.Query), OutputSummaryJSON: fmt.Sprintf(`{"hits":%d}`, len(cards)), Status: "completed"}},
	}, nil
}
```

Add these helper definitions in the same file:

```go
type ExternalPaperCard struct {
	S2PaperID     string `json:"s2_paper_id"`
	Title         string `json:"title"`
	Year          int    `json:"year,omitempty"`
	Venue         string `json:"venue,omitempty"`
	DOI           string `json:"doi,omitempty"`
	TLDR          string `json:"tldr,omitempty"`
	CitationIndex int    `json:"citation_index,omitempty"`
}

func externalID(p research.Paper) string {
	if p.ExternalIDs.DOI != "" {
		return "DOI:" + p.ExternalIDs.DOI
	}
	if p.ExternalIDs.ArXiv != "" {
		return "ARXIV:" + p.ExternalIDs.ArXiv
	}
	if p.ExternalIDs.PubMed != "" {
		return "PMID:" + p.ExternalIDs.PubMed
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func externalAnswerContext(cards []ResultCard) string {
	var b strings.Builder
	for i, card := range cards {
		p, ok := card.Payload.(ExternalPaperCard)
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "[external %d] %s", i+1, p.Title)
		if p.Year > 0 || p.Venue != "" {
			fmt.Fprintf(&b, " (%s %d)", p.Venue, p.Year)
		}
		if p.TLDR != "" {
			fmt.Fprintf(&b, "\n%s", p.TLDR)
		}
		b.WriteString("\n\n")
	}
	return b.String()
}
```

- [ ] **Step 4: Implement PaperReadTool**

Create `internal/service/ai_assistant/paper_read_tool.go` with:

```go
package ai_assistant

import (
	"context"
	"fmt"
	"strings"
)

type PaperReadTool struct {
	papers PaperGetter
}

func NewPaperReadTool(papers PaperGetter) *PaperReadTool {
	return &PaperReadTool{papers: papers}
}

func (t *PaperReadTool) Run(ctx context.Context, in ToolInput) (ToolResult, error) {
	_ = ctx
	ids := in.Context.PaperIDs
	if len(ids) == 0 && in.Context.PaperID > 0 {
		ids = []int64{in.Context.PaperID}
	}
	if len(ids) == 0 {
		return ToolResult{
			Process: ProcessSummary{Intent: IntentPaperRead, Note: "没有指定文献"},
			ToolCalls: []ToolCallSummary{{ToolName: "paper_read", Status: "skipped", OutputSummaryJSON: `{"reason":"no_papers"}`}},
		}, nil
	}
	terms := EvidenceSearchTerms(in.Query)
	papers := make([]PaperCompareItem, 0, len(ids))
	citations := make([]Citation, 0)
	for _, id := range ids {
		p, err := t.papers.GetPaperDetail(id)
		if err != nil || p == nil {
			continue
		}
		matches := FindLocalEvidenceMatches(*p, terms, 3)
		item := PaperCompareItem{PaperID: p.ID, Title: p.Title}
		for _, m := range matches {
			citation := Citation{I: len(citations) + 1, PaperID: p.ID, Title: p.Title, Source: "local", Snippet: m.Snippet, Score: m.Score}
			citations = append(citations, citation)
			item.Evidence = append(item.Evidence, PaperHitSnippet{CitationIndex: citation.I, Location: m.Location, Text: m.Snippet.Text})
		}
		papers = append(papers, item)
	}
	cardType := "paper_compare"
	if len(papers) == 1 {
		cardType = "paper_read"
	}
	return ToolResult{
		Process: ProcessSummary{Intent: IntentPaperRead, Stages: []ProcessStage{
			{Label: "全文扫描", Count: len(ids), Unit: "篇", Status: "completed"},
			{Label: "命中证据", Count: len(citations), Unit: "段", Status: "completed"},
		}},
		Cards: []ResultCard{{Type: cardType, Payload: PaperCompareCard{
			Query: in.Query, Papers: papers, Note: compareNote(len(ids)),
		}}},
		Citations:     citations,
		AnswerContext: paperCompareAnswerContext(papers),
		ToolCalls: []ToolCallSummary{{ToolName: "paper_read", InputJSON: fmt.Sprintf(`{"paper_ids":%q}`, strings.Trim(strings.Join(strings.Fields(fmt.Sprint(ids)), ","), "[]")), OutputSummaryJSON: fmt.Sprintf(`{"papers":%d,"citations":%d}`, len(papers), len(citations)), Status: "completed"}},
	}, nil
}
```

Add these helper definitions in the same file:

```go
type PaperCompareCard struct {
	Query  string             `json:"query"`
	Papers []PaperCompareItem `json:"papers"`
	Note   string             `json:"note,omitempty"`
}

type PaperCompareItem struct {
	PaperID  int64             `json:"paper_id"`
	Title    string            `json:"title"`
	Evidence []PaperHitSnippet `json:"evidence"`
}

func compareNote(n int) string {
	if n <= 2 {
		return ""
	}
	return "已完成多篇全文证据扫描；请选择 1-2 篇继续深入展开。"
}

func paperCompareAnswerContext(items []PaperCompareItem) string {
	var b strings.Builder
	for _, item := range items {
		fmt.Fprintf(&b, "### %s\n", item.Title)
		for _, ev := range item.Evidence {
			fmt.Fprintf(&b, "- [%d] %s: %s\n", ev.CitationIndex, ev.Location, ev.Text)
		}
		b.WriteString("\n")
	}
	return b.String()
}
```

- [ ] **Step 5: Implement FigureLookupTool**

Create `internal/service/ai_assistant/figure_lookup_tool.go`:

```go
package ai_assistant

import (
	"context"
	"fmt"
)

type FigureRecord struct {
	FigureID     int64
	PaperID      int64
	PaperTitle   string
	DisplayLabel string
	ImageURL     string
	Caption      string
	NotesText    string
}

type FigureSearcher interface {
	SearchFigures(query string, paperID int64, limit int) ([]FigureRecord, int, error)
}

type FigureLookupTool struct {
	figures FigureSearcher
}

func NewFigureLookupTool(figures FigureSearcher) *FigureLookupTool {
	return &FigureLookupTool{figures: figures}
}

func (t *FigureLookupTool) Run(ctx context.Context, in ToolInput) (ToolResult, error) {
	_ = ctx
	limit := in.Limit
	if limit <= 0 {
		limit = 12
	}
	items, total, err := t.figures.SearchFigures(in.Query, in.Context.PaperID, limit)
	if err != nil {
		return ToolResult{
			Process: ProcessSummary{Intent: IntentFigureLookup, Stages: []ProcessStage{{Label: "图文检索", Status: "failed"}}},
			ToolCalls: []ToolCallSummary{{ToolName: "figure_lookup", InputJSON: fmt.Sprintf(`{"query":%q}`, in.Query), Status: "failed", Error: err.Error()}},
		}, nil
	}
	cards := make([]ResultCard, 0, len(items))
	for _, item := range items {
		cards = append(cards, ResultCard{Type: "figure_result", Payload: FigureResultCard(item)})
	}
	return ToolResult{
		Process: ProcessSummary{Intent: IntentFigureLookup, Stages: []ProcessStage{
			{Label: "图文检索", Count: total, Unit: "张图", Status: "completed"},
			{Label: "命中", Count: len(cards), Unit: "张", Status: "completed"},
		}},
		Cards:         cards,
		AnswerContext: figureAnswerContext(items),
		ToolCalls: []ToolCallSummary{{ToolName: "figure_lookup", InputJSON: fmt.Sprintf(`{"query":%q,"paper_id":%d}`, in.Query, in.Context.PaperID), OutputSummaryJSON: fmt.Sprintf(`{"total":%d,"hits":%d}`, total, len(cards)), Status: "completed"}},
	}, nil
}

type FigureResultCard FigureRecord
```

Add `figureAnswerContext` in the same file:

```go
func figureAnswerContext(items []FigureRecord) string {
	var b strings.Builder
	for _, item := range items {
		fmt.Fprintf(&b, "### %s - %s\n", item.PaperTitle, item.DisplayLabel)
		if item.Caption != "" {
			fmt.Fprintf(&b, "Caption: %s\n", item.Caption)
		}
		if item.NotesText != "" {
			fmt.Fprintf(&b, "Notes: %s\n", item.NotesText)
		}
		b.WriteString("\n")
	}
	return b.String()
}
```

- [ ] **Step 6: Add repository adapter used by server wiring**

In `figure_lookup_tool.go`, add an adapter type that wraps the existing figure list surface:

```go
type FigureListProvider interface {
	ListFigures(filter model.FigureFilter) ([]model.FigureListItem, int, error)
}

type RepositoryFigureSearcher struct {
	repo FigureListProvider
}

func NewRepositoryFigureSearcher(repo FigureListProvider) *RepositoryFigureSearcher {
	return &RepositoryFigureSearcher{repo: repo}
}

func (s *RepositoryFigureSearcher) SearchFigures(query string, paperID int64, limit int) ([]FigureRecord, int, error) {
	if limit <= 0 {
		limit = 12
	}
	filter := model.FigureFilter{Keyword: query, Page: 1, PageSize: limit}
	if paperID > 0 {
		filter.PaperID = &paperID
	}
	figures, total, err := s.repo.ListFigures(filter)
	if err != nil {
		return nil, 0, err
	}
	out := make([]FigureRecord, 0, len(figures))
	for _, figure := range figures {
		out = append(out, FigureRecordFromListItem(figure))
	}
	return out, total, nil
}
```

Map `model.FigureListItem` to `FigureRecord` with:

```go
func FigureRecordFromListItem(item model.FigureListItem) FigureRecord {
	return FigureRecord{
		FigureID:     item.ID,
		PaperID:      item.PaperID,
		PaperTitle:   item.PaperTitle,
		DisplayLabel: item.FigureDisplayLabel,
		ImageURL:     item.ImageURL,
		Caption:      item.Caption,
		NotesText:    item.NotesText,
	}
}
```

Use import `github.com/xuzhougeng/citebox/internal/model`. The `figure_lookup_tool.go` import block becomes:

```go
import (
	"context"
	"fmt"
	"strings"

	"github.com/xuzhougeng/citebox/internal/model"
)
```

- [ ] **Step 7: Run tests and commit**

Run:

```bash
gofmt -w internal/service/ai_assistant
go test ./internal/service/ai_assistant -run 'TestExternalSearchTool|TestPaperReadTool|TestFigureLookupTool' -count=1
git add internal/service/ai_assistant
git commit -m "feat(ai): add assistant search and reading tools"
```

Expected: PASS.

---

## Task 5: Orchestrator

**Files:**

- Create: `internal/service/ai_assistant/orchestrator.go`
- Create: `internal/service/ai_assistant/orchestrator_test.go`

- [ ] **Step 1: Write failing orchestrator tests**

Create `internal/service/ai_assistant/orchestrator_test.go`:

```go
package ai_assistant

import (
	"context"
	"strings"
	"testing"
)

type stubTool struct {
	res ToolResult
}

func (s stubTool) Run(ctx context.Context, in ToolInput) (ToolResult, error) {
	return s.res, nil
}

func TestOrchestratorRunsSelectedTool(t *testing.T) {
	orch := NewOrchestrator(ToolSet{
		LibrarySearch: stubTool{res: ToolResult{
			Process: ProcessSummary{Intent: IntentLibrarySearch, Stages: []ProcessStage{{Label: "全库检索", Count: 2}}},
			Cards: []ResultCard{{Type: "paper_hit", Payload: PaperHitCard{PaperID: 1, Title: "ATAC Paper"}}},
			AnswerContext: "ATAC evidence",
		}},
	})
	out, err := orch.Run(context.Background(), RunInput{Content: "帮我查找 ATAC 文章"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.Intent != IntentLibrarySearch || len(out.Cards) != 1 {
		t.Fatalf("out = %+v", out)
	}
	if !strings.Contains(out.AnswerContext, "ATAC evidence") {
		t.Fatalf("answer context = %q", out.AnswerContext)
	}
}
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```bash
go test ./internal/service/ai_assistant -run TestOrchestratorRunsSelectedTool -count=1
```

Expected: missing `NewOrchestrator`, `ToolSet`, and `RunInput`.

- [ ] **Step 3: Implement orchestrator**

Create `internal/service/ai_assistant/orchestrator.go`:

```go
package ai_assistant

import (
	"context"
	"strings"
)

type Tool interface {
	Run(ctx context.Context, in ToolInput) (ToolResult, error)
}

type ToolSet struct {
	LibrarySearch Tool
	ExternalSearch Tool
	PaperRead      Tool
	FigureLookup   Tool
}

type Orchestrator struct {
	tools ToolSet
}

func NewOrchestrator(tools ToolSet) *Orchestrator {
	return &Orchestrator{tools: tools}
}

type RunInput struct {
	Content    string
	IntentHint string
	Context    RequestContext
}

type RunOutput struct {
	Intent        string
	IntentHint    string
	Process       ProcessSummary
	Cards         []ResultCard
	Citations     []Citation
	AnswerContext string
	ToolCalls     []ToolCallSummary
}

func (o *Orchestrator) Run(ctx context.Context, in RunInput) (RunOutput, error) {
	route := RouteIntent(RouteInput{Content: in.Content, IntentHint: in.IntentHint, Context: in.Context})
	tool := o.toolForIntent(route.Intent)
	if tool == nil {
		return RunOutput{
			Intent: route.Intent, IntentHint: in.IntentHint,
			Process: ProcessSummary{Intent: route.Intent, Note: route.Reason},
			AnswerContext: "用户问题：\n" + strings.TrimSpace(in.Content),
		}, nil
	}
	res, err := tool.Run(ctx, ToolInput{Query: in.Content, Context: in.Context, IntentHint: in.IntentHint})
	if err != nil {
		return RunOutput{}, err
	}
	return RunOutput{
		Intent:        route.Intent,
		IntentHint:    in.IntentHint,
		Process:       res.Process,
		Cards:         res.Cards,
		Citations:     res.Citations,
		AnswerContext: buildFinalAnswerContext(in.Content, res),
		ToolCalls:     res.ToolCalls,
	}, nil
}

func (o *Orchestrator) toolForIntent(intent string) Tool {
	switch intent {
	case IntentLibrarySearch:
		return o.tools.LibrarySearch
	case IntentExternalSearch:
		return o.tools.ExternalSearch
	case IntentPaperRead:
		return o.tools.PaperRead
	case IntentFigureLookup:
		return o.tools.FigureLookup
	default:
		return nil
	}
}

func buildFinalAnswerContext(userText string, res ToolResult) string {
	var b strings.Builder
	b.WriteString("你正在基于工具检索结果回答。只使用下列证据和结果卡片支持结论；证据不足时明确说明。\n\n")
	if res.AnswerContext != "" {
		b.WriteString("工具结果：\n")
		b.WriteString(res.AnswerContext)
		b.WriteString("\n\n")
	}
	b.WriteString("用户问题：\n")
	b.WriteString(userText)
	return b.String()
}
```

- [ ] **Step 4: Run tests and commit**

```bash
gofmt -w internal/service/ai_assistant
go test ./internal/service/ai_assistant -run 'TestRouteIntent|TestOrchestrator' -count=1
git add internal/service/ai_assistant
git commit -m "feat(ai): add assistant orchestrator"
```

---

## Task 6: Conversation Service And Handler Integration

**Files:**

- Modify: `internal/service/ai_conversation/types.go`
- Modify: `internal/service/ai_conversation/service.go`
- Modify: `internal/handler/ai_conversation.go`
- Modify: `internal/handler/ai_conversation_test.go`
- Modify: `internal/app/server.go`

- [ ] **Step 1: Write failing handler test**

Add to `internal/handler/ai_conversation_test.go`:

```go
func TestAIConversationPostMessageAcceptsIntentHintAndContext(t *testing.T) {
	stub := &stubAIConversationService{}
	h := NewAIConversationHandler(stub)
	body := strings.NewReader(`{"content":"看图 1","intent_hint":"figure_lookup","context":{"source":"paper","paper_id":7,"figure_id":3}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/conversations/new/messages", body)
	rr := httptest.NewRecorder()

	h.PostMessage(rr, req)

	if stub.sentInput.IntentHint != "figure_lookup" {
		t.Fatalf("IntentHint = %q", stub.sentInput.IntentHint)
	}
	if stub.sentInput.Context.PaperID != 7 || stub.sentInput.Context.FigureID != 3 || stub.sentInput.Context.Source != "paper" {
		t.Fatalf("Context = %+v", stub.sentInput.Context)
	}
}
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```bash
go test ./internal/handler -run TestAIConversationPostMessageAcceptsIntentHintAndContext -count=1
```

Expected: missing fields on `SendMessageInput`.

- [ ] **Step 3: Extend service types**

In `internal/service/ai_conversation/types.go`, import `github.com/xuzhougeng/citebox/internal/service/ai_assistant` and add:

```go
type TurnRun struct {
	ID                 int64        `json:"id"`
	UserMessageID      int64        `json:"user_message_id"`
	AssistantMessageID int64        `json:"assistant_message_id,omitempty"`
	Intent             string       `json:"intent"`
	IntentHint         string       `json:"intent_hint,omitempty"`
	ProcessSummaryJSON string       `json:"process_summary_json,omitempty"`
	Status             string       `json:"status"`
	Cards              []ResultCard `json:"cards,omitempty"`
}

type ResultCard struct {
	ID          int64  `json:"id"`
	TurnRunID   int64  `json:"turn_run_id"`
	CardType    string `json:"card_type"`
	SortOrder   int    `json:"sort_order"`
	PayloadJSON string `json:"payload_json"`
}
```

Extend `Conversation`:

```go
TurnRuns []TurnRun `json:"turn_runs,omitempty"`
```

Extend `SendMessageInput`:

```go
IntentHint string
Context    ai_assistant.RequestContext
```

- [ ] **Step 4: Extend handler request parsing**

In `internal/handler/ai_conversation.go`, add fields to the POST body struct:

```go
IntentHint string                       `json:"intent_hint,omitempty"`
Context    ai_assistant.RequestContext `json:"context,omitempty"`
```

Import:

```go
"github.com/xuzhougeng/citebox/internal/service/ai_assistant"
```

Pass them into `SendMessageInput`:

```go
IntentHint: body.IntentHint,
Context:    body.Context,
```

- [ ] **Step 5: Add service orchestrator dependency**

In `internal/service/ai_conversation/service.go`, add:

```go
orchestrator interface {
	Run(ctx context.Context, in ai_assistant.RunInput) (ai_assistant.RunOutput, error)
}
```

Add field to `Service`:

```go
orchestrator orchestrator
```

Update constructor to accept the orchestrator:

```go
func New(repo *repository.AIConversationRepository, papers *repository.PaperRepository,
	settings AISettingsProvider, caller StreamCaller, searcher ExternalEvidenceSearcher,
	logger *slog.Logger, orchestrator orchestrator) *Service
```

Update all call sites in tests and `internal/app/server.go` in the same task. Tests that do not need orchestration should pass `nil`.

- [ ] **Step 6: Use orchestrator in SendMessage**

In `SendMessage`, after `assembleForTurn` and before legacy evidence injection, add:

```go
var runOut ai_assistant.RunOutput
var runUsed bool
if s.orchestrator != nil {
	out, orchErr := s.orchestrator.Run(ctx, ai_assistant.RunInput{
		Content:    in.Content,
		IntentHint: in.IntentHint,
		Context:    in.Context,
	})
	if orchErr != nil {
		s.logger.Warn("ai_conversation: orchestrator failed", "error", orchErr)
	} else {
		runOut = out
		runUsed = true
		if strings.TrimSpace(out.AnswerContext) != "" {
			suffix := "用户问题：\n" + in.Content
			if strings.HasSuffix(asm.userPrompt, suffix) {
				asm.userPrompt = strings.TrimSuffix(asm.userPrompt, suffix) + out.AnswerContext
			} else {
				asm.userPrompt += "\n\n" + out.AnswerContext
			}
		}
		citationsJSON = marshalAssistantCitations(out.Citations)
	}
}
```

Skip legacy `injectEvidence` when `runUsed` is true. Keep legacy path for compatibility when orchestrator is nil or old switches are used.

Add `marshalAssistantCitations` mapping `ai_assistant.Citation` to JSON.

- [ ] **Step 7: Persist run artifacts after assistant message**

After assistant message is inserted:

```go
if runUsed {
	runID, err := s.repo.CreateTurnRun(repository.AITurnRun{
		ConversationID:     in.ConversationID,
		UserMessageID:      userMsgID,
		AssistantMessageID: sql.NullInt64{Int64: asstID, Valid: true},
		Intent:             runOut.Intent,
		IntentHint:         runOut.IntentHint,
		ProcessSummaryJSON: mustJSON(runOut.Process),
		Status:             modeOrCompleted(mode),
	})
	if err != nil {
		s.logger.Warn("ai_conversation: persist turn run failed", "error", err)
	} else {
		persistToolCalls(s.repo, runID, runOut.ToolCalls, s.logger)
		persistResultCards(s.repo, runID, runOut.Cards, s.logger)
	}
}
```

Implement helper functions in `service.go` or a new `orchestration_persist.go`.

- [ ] **Step 8: Stream process/cards/citations events**

Before final LLM call, if `runUsed`, call `onDelta` is not enough because it only streams `delta`. Extend the service callback model with a typed stream event callback, or minimally update handler to expose process/cards after `SendMessage` completes.

Preferred change:

```go
type StreamEvent struct {
	Type string
	Data any
}
```

Add optional callback field to `SendMessageInput`:

```go
OnEvent func(StreamEvent) error
```

Emit:

```go
_ = in.OnEvent(StreamEvent{Type: "process", Data: runOut.Process})
_ = in.OnEvent(StreamEvent{Type: "cards", Data: runOut.Cards})
_ = in.OnEvent(StreamEvent{Type: "citations", Data: runOut.Citations})
```

Update handler to encode these event types as NDJSON. Keep `onDelta` for text deltas.

- [ ] **Step 9: Wire server**

In `internal/app/server.go`, create tools and orchestrator near the existing `aiConvService` setup:

```go
assistantOrchestrator := ai_assistant.NewOrchestrator(ai_assistant.ToolSet{
	LibrarySearch:  ai_assistant.NewLibrarySearchTool(repo.Paper),
	ExternalSearch: ai_assistant.NewExternalSearchTool(researchSvc),
	PaperRead:      ai_assistant.NewPaperReadTool(repo.Paper),
	FigureLookup:   ai_assistant.NewFigureLookupTool(ai_assistant.NewRepositoryFigureSearcher(repo.Library)),
})
aiConvService := ai_conversation.New(repo.AIConversation, repo.Paper, aiSvc, aiSvc, researchSvc, logger.With("component", "ai_conversation"), assistantOrchestrator)
```

Adjust names to match the actual repository aggregate. If `repo.Library` is not available, create an adapter around `repo.Figure`.

- [ ] **Step 10: Run integration tests and commit**

```bash
gofmt -w internal/service/ai_conversation internal/handler/ai_conversation.go internal/handler/ai_conversation_test.go internal/app/server.go
go test ./internal/handler -run TestAIConversation -count=1
go test ./internal/service/ai_conversation -count=1
git add internal/service/ai_conversation internal/handler/ai_conversation.go internal/handler/ai_conversation_test.go internal/app/server.go
git commit -m "feat(ai): integrate assistant orchestrator with conversations"
```

---

## Task 7: Frontend Composer, Process Strip, And Result Cards

**Files:**

- Create: `web/static/js/ai-composer.js`
- Create: `web/static/js/ai-process-strip.js`
- Create: `web/static/js/ai-result-cards.js`
- Create: `web/static/js/ai-message-list.js`
- Modify: `web/ai.html`
- Modify: `web/static/js/ai-reader.js`
- Modify: `web/static/js/ai-conversation-view.js`
- Modify: `web/static/js/ai-evidence.js`
- Modify: `web/static/css/features/ai.css`
- Modify: `web/static/locales/zh-CN/ai.json`
- Modify: `web/static/locales/en/ai.json`

- [ ] **Step 1: Add composer module**

Create `web/static/js/ai-composer.js`:

```javascript
(function () {
    'use strict';

    const intents = ['library_search', 'external_search', 'paper_read', 'figure_lookup'];

    const Composer = {
        init(opts) {
            this.input = opts.input;
            this.sendBtn = opts.sendBtn;
            this.stopBtn = opts.stopBtn;
            this.shortcutRoot = opts.shortcutRoot;
            this.onSend = opts.onSend || function () {};
            this.intentHint = '';
            this._renderShortcuts();
            this._bind();
        },

        _bind() {
            if (this.sendBtn) this.sendBtn.addEventListener('click', () => this.submit());
            if (this.input) {
                this.input.addEventListener('keydown', (event) => {
                    if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
                        event.preventDefault();
                        this.submit();
                    }
                });
            }
        },

        _renderShortcuts() {
            if (!this.shortcutRoot) return;
            const t = window.CiteBoxI18n?.t || ((key, fallback) => fallback || key);
            const labels = {
                library_search: t('ai.intent_library_search', '查全库'),
                external_search: t('ai.intent_external_search', '查外部'),
                paper_read: t('ai.intent_paper_read', '读文献'),
                figure_lookup: t('ai.intent_figure_lookup', '看图/图文'),
            };
            this.shortcutRoot.innerHTML = intents.map((intent) => (
                `<button class="ai-intent-shortcut" type="button" data-intent="${intent}">${labels[intent]}</button>`
            )).join('');
            this.shortcutRoot.querySelectorAll('[data-intent]').forEach((button) => {
                button.addEventListener('click', () => {
                    this.intentHint = button.dataset.intent || '';
                    this.shortcutRoot.querySelectorAll('.is-active').forEach((el) => el.classList.remove('is-active'));
                    button.classList.add('is-active');
                    this.input?.focus();
                });
            });
        },

        submit() {
            const content = (this.input?.value || '').trim();
            if (!content) return;
            const payload = { content, intent_hint: this.intentHint };
            this.intentHint = '';
            this.shortcutRoot?.querySelectorAll('.is-active').forEach((el) => el.classList.remove('is-active'));
            this.onSend(payload);
        },

        clear() {
            if (this.input) this.input.value = '';
        },
    };

    window.AIReader = window.AIReader || {};
    window.AIReader.composer = Composer;
})();
```

- [ ] **Step 2: Add process strip module**

Create `web/static/js/ai-process-strip.js`:

```javascript
(function () {
    'use strict';

    function escapeHtml(value) {
        return String(value || '').replace(/[&<>"']/g, (ch) => ({
            '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
        }[ch]));
    }

    function render(summary) {
        if (!summary || !Array.isArray(summary.stages) || summary.stages.length === 0) return '';
        const stages = summary.stages.map((stage) => {
            const count = stage.count ? ` ${stage.count}${stage.unit || ''}` : '';
            return `<span class="ai-process-stage">${escapeHtml(stage.label)}${escapeHtml(count)}</span>`;
        }).join('<span class="ai-process-sep">·</span>');
        return `<div class="ai-process-strip">${stages}</div>`;
    }

    window.AIReader = window.AIReader || {};
    window.AIReader.processStrip = { render };
})();
```

- [ ] **Step 3: Add result cards module**

Create `web/static/js/ai-result-cards.js`:

```javascript
(function () {
    'use strict';

    function escapeHtml(value) {
        return String(value || '').replace(/[&<>"']/g, (ch) => ({
            '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
        }[ch]));
    }

    function payload(card) {
        if (!card) return {};
        if (typeof card.payload === 'object') return card.payload || {};
        if (typeof card.payload_json === 'string') {
            try { return JSON.parse(card.payload_json); } catch (e) { return {}; }
        }
        return {};
    }

    function render(cards) {
        if (!Array.isArray(cards) || cards.length === 0) return '';
        return `<div class="ai-result-cards">${cards.map(renderCard).join('')}</div>`;
    }

    function renderCard(card) {
        const p = payload(card);
        switch (card.type || card.card_type) {
        case 'paper_hit':
            return `<article class="ai-result-card">
                <h4>${escapeHtml(p.title)}</h4>
                <p>${escapeHtml(p.reason || '')}</p>
                ${(p.snippets || []).slice(0, 3).map((s) => `<blockquote>${escapeHtml(s.text)} <sup>[${s.citation_index}]</sup></blockquote>`).join('')}
                ${p.paper_id ? `<a class="btn btn-small btn-outline" href="/library?paper=${encodeURIComponent(p.paper_id)}">打开文献</a>` : ''}
            </article>`;
        case 'external_paper':
            return `<article class="ai-result-card">
                <h4>${escapeHtml(p.title)}</h4>
                <p>${escapeHtml([p.venue, p.year].filter(Boolean).join(' · '))}</p>
                <p>${escapeHtml(p.tldr || '')}</p>
            </article>`;
        case 'paper_compare':
            return `<article class="ai-result-card">
                <h4>${escapeHtml(p.query || '文献对比')}</h4>
                ${(p.papers || []).map((paper) => `<section><strong>${escapeHtml(paper.title)}</strong>${(paper.evidence || []).map((s) => `<blockquote>${escapeHtml(s.text)}</blockquote>`).join('')}</section>`).join('')}
            </article>`;
        case 'figure_result':
            return `<article class="ai-result-card ai-figure-result-card">
                ${p.image_url ? `<img src="${escapeHtml(p.image_url)}" alt="${escapeHtml(p.display_label || 'figure')}">` : '<div class="ai-figure-missing">图片不可用</div>'}
                <h4>${escapeHtml(p.display_label || 'Figure')}</h4>
                <p>${escapeHtml(p.caption || '')}</p>
                <p>${escapeHtml(p.notes_text || '')}</p>
            </article>`;
        default:
            return `<article class="ai-result-card"><pre>${escapeHtml(JSON.stringify(p, null, 2))}</pre></article>`;
        }
    }

    window.AIReader = window.AIReader || {};
    window.AIReader.resultCards = { render };
})();
```

- [ ] **Step 4: Add message list module**

Create `web/static/js/ai-message-list.js`:

```javascript
(function () {
    'use strict';

    const MessageList = {
        init(opts) {
            this.container = opts.container;
        },

        appendMessage(message) {
            if (!this.container) return null;
            const bubble = document.createElement('div');
            bubble.className = 'ai-message ai-message-' + (message.role || 'assistant');
            bubble.dataset.messageId = message.id || '';
            const content = document.createElement('div');
            content.className = 'ai-message-content';
            content.textContent = message.content || '';
            bubble.appendChild(content);
            this.container.appendChild(bubble);
            this.scrollToBottom();
            return bubble;
        },

        appendHTML(target, html) {
            if (!target || !html) return;
            target.insertAdjacentHTML('beforeend', html);
            this.scrollToBottom();
        },

        scrollToBottom() {
            if (!this.container) return;
            this.container.scrollTop = this.container.scrollHeight;
        },
    };

    window.AIReader = window.AIReader || {};
    window.AIReader.messageList = MessageList;
})();
```

- [ ] **Step 5: Wire HTML**

In `web/ai.html`, add shortcut root inside `.ai-composer-tools` before role prompt hint:

```html
<div id="aiIntentShortcuts" class="ai-intent-shortcuts" aria-label="AI 任务快捷入口"></div>
```

Remove the primary `.ai-mode-toggles` block from visible UI. If compatibility is needed for existing code during migration, keep the checkboxes hidden:

```html
<div class="ai-mode-toggles" hidden>
```

Add scripts before `ai-conversation-view.js`:

```html
<script src="/static/js/ai-composer.js"></script>
<script src="/static/js/ai-message-list.js"></script>
<script src="/static/js/ai-process-strip.js"></script>
<script src="/static/js/ai-result-cards.js"></script>
```

- [ ] **Step 6: Update view send body and stream handling**

In `web/static/js/ai-conversation-view.js`, change `sendCurrentInput()` to accept a payload from composer:

```javascript
async sendPayload(payload) {
    const content = (payload && payload.content || '').trim();
    if (!content) return;
    const body = { content: content };
    if (payload.intent_hint) body.intent_hint = payload.intent_hint;
    body.context = this._currentContext();
    await this._sendBody(body);
}
```

Refactor existing fetch code into `_sendBody(body)`. In `_consumeNdjson`, add handlers:

```javascript
if (msg.type === 'process') {
    this._appendProcess(msg.process || msg.data);
} else if (msg.type === 'cards') {
    this._appendCards(msg.cards || msg.data || []);
} else if (msg.type === 'citations') {
    this._state.pendingCitations = msg.citations || msg.data || [];
}
```

Add:

```javascript
_appendProcess(summary) {
    const html = window.AIReader.processStrip?.render(summary) || '';
    if (html && this._state.streaming?.assistantBubbleEl) {
        this._state.streaming.assistantBubbleEl.insertAdjacentHTML('beforeend', html);
    }
}

_appendCards(cards) {
    const html = window.AIReader.resultCards?.render(cards) || '';
    if (html && this._state.streaming?.assistantBubbleEl) {
        this._state.streaming.assistantBubbleEl.insertAdjacentHTML('beforeend', html);
    }
}

_currentContext() {
    const paperID = this._state._draftPaperId || 0;
    return paperID ? { source: 'ai', paper_id: paperID } : { source: 'ai' };
}
```

- [ ] **Step 7: Wire composer in reader**

In `web/static/js/ai-reader.js`, after `view.init(...)`, add:

```javascript
if (window.AIReader.composer) {
    window.AIReader.composer.init({
        input: $('aiQuestionInput'),
        sendBtn: $('runAIReaderButton'),
        stopBtn: $('stopAIReaderButton'),
        shortcutRoot: $('aiIntentShortcuts'),
        onSend: (payload) => view.sendPayload(payload),
    });
}
```

Remove direct send button binding in `ai-conversation-view.js` to avoid double-send.

- [ ] **Step 8: Add CSS and locale keys**

In `web/static/css/features/ai.css`, add:

```css
.ai-intent-shortcuts {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
}

.ai-intent-shortcut {
    border: 1px solid var(--border-color);
    background: var(--surface-color);
    color: var(--text-color);
    border-radius: 999px;
    padding: 0.32rem 0.68rem;
    font-size: 0.86rem;
    cursor: pointer;
}

.ai-intent-shortcut.is-active {
    border-color: var(--primary-color);
    color: var(--primary-color);
    background: var(--primary-soft);
}

.ai-process-strip {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.35rem;
    margin: 0.25rem 0 0.65rem;
    color: var(--muted-text);
    font-size: 0.84rem;
}

.ai-process-sep {
    opacity: 0.6;
}

.ai-result-cards {
    display: grid;
    gap: 0.75rem;
    margin-top: 0.75rem;
}

.ai-result-card {
    border: 1px solid var(--border-color);
    border-radius: 8px;
    background: var(--surface-color);
    padding: 0.85rem;
}

.ai-result-card h4 {
    margin: 0 0 0.4rem;
    font-size: 0.98rem;
}

.ai-result-card blockquote {
    margin: 0.45rem 0;
    padding-left: 0.65rem;
    border-left: 3px solid var(--border-color);
    color: var(--muted-text);
}

.ai-figure-result-card img {
    max-width: 100%;
    border-radius: 6px;
    border: 1px solid var(--border-color);
}
```

Add locale keys to both `web/static/locales/zh-CN/ai.json` and `web/static/locales/en/ai.json`:

```json
"ai.intent_library_search": "查全库",
"ai.intent_external_search": "查外部",
"ai.intent_paper_read": "读文献",
"ai.intent_figure_lookup": "看图/图文"
```

English:

```json
"ai.intent_library_search": "Search Library",
"ai.intent_external_search": "Search External",
"ai.intent_paper_read": "Read Papers",
"ai.intent_figure_lookup": "Figures"
```

- [ ] **Step 9: Verify and commit**

```bash
node --check web/static/js/ai-composer.js
node --check web/static/js/ai-message-list.js
node --check web/static/js/ai-process-strip.js
node --check web/static/js/ai-result-cards.js
node --check web/static/js/ai-conversation-view.js
node -e "const fs=require('fs'); for (const dir of ['web/static/locales/zh-CN','web/static/locales/en']) for (const f of fs.readdirSync(dir)) if (f.endsWith('.json')) JSON.parse(fs.readFileSync(dir+'/'+f,'utf8'));"
git add web/ai.html web/static/js web/static/css/features/ai.css web/static/locales/zh-CN/ai.json web/static/locales/en/ai.json
git commit -m "feat(web): add AI assistant command composer and result cards"
```

---

## Task 8: API Docs And Browser Verification

**Files:**

- Modify: `docs/api.md`
- Test with Playwright or browser.

- [ ] **Step 1: Update API docs**

In `docs/api.md`, update the AI conversation message section to include:

```md
- `intent_hint`: optional one-turn routing hint. Supported values: `library_search`, `external_search`, `paper_read`, `figure_lookup`.
- `context`: optional object with `source`, `paper_id`, `paper_ids`, and `figure_id`.

The NDJSON stream may emit:

- `process`: compact process-strip summary.
- `cards`: structured result cards.
- `citations`: citation array used by footnotes and cards.
```

Document that `strict_evidence` and `include_external_evidence` remain compatibility fields, not the primary UI model.

- [ ] **Step 2: Full test suite**

Run:

```bash
go test ./...
node --check web/static/js/ai-composer.js
node --check web/static/js/ai-message-list.js
node --check web/static/js/ai-process-strip.js
node --check web/static/js/ai-result-cards.js
node --check web/static/js/ai-conversation-view.js
node --check web/static/js/ai-reader.js
node -e "const fs=require('fs'); for (const dir of ['web/static/locales/zh-CN','web/static/locales/en']) for (const f of fs.readdirSync(dir)) if (f.endsWith('.json')) JSON.parse(fs.readFileSync(dir+'/'+f,'utf8'));"
git diff --check
```

Expected: all commands exit 0.

- [ ] **Step 3: Manual browser verification**

Start or reuse dev server:

```bash
make dev
```

Verify with Playwright:

- AI page title is `AI 助手 - CiteBox`.
- Shortcut buttons are visible.
- Clicking `查全库` then sending a message sends `intent_hint: "library_search"`.
- A mocked `process` event renders a compact process strip.
- A mocked `cards` event renders at least one `paper_hit` card.
- Delete dialog still uses app-native `Utils.confirm`.

- [ ] **Step 4: Commit**

```bash
git add docs/api.md
git commit -m "docs: update AI assistant orchestration API"
```

---

## Execution Notes

- Keep each task independently buildable.
- Do not remove old `strict_evidence` database fields in this plan.
- Do not use embeddings, vector databases, or a hidden background task runtime.
- Prefer rule-based intent routing in this implementation. Model-based routing can be added after the data flow is stable.
- If a task uncovers a schema or API issue, add a narrow test first, then fix it.

## Final Verification

After all tasks:

```bash
go test ./...
node --check web/static/js/ai-composer.js
node --check web/static/js/ai-message-list.js
node --check web/static/js/ai-process-strip.js
node --check web/static/js/ai-result-cards.js
node --check web/static/js/ai-conversation-view.js
node --check web/static/js/ai-reader.js
node -e "const fs=require('fs'); for (const dir of ['web/static/locales/zh-CN','web/static/locales/en']) for (const f of fs.readdirSync(dir)) if (f.endsWith('.json')) JSON.parse(fs.readFileSync(dir+'/'+f,'utf8'));"
git diff --check
```

Acceptance checks:

- The AI page remains chat-first.
- The composer has four shortcuts.
- The primary UI no longer depends on internal/external search switches.
- `intent_hint` reaches the backend.
- The orchestrator can route library search, external search, paper reading/comparison, and figure lookup.
- Each orchestrated turn can persist and reload process summaries and result cards.
- Result cards render for paper hits, external papers, comparisons, and figures.
- No implementation uses embeddings.

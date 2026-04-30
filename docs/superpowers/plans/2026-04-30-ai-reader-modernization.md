# AI 伴读页面现代化改造 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild `/ai` page from a single in-memory paper-bound chat into a ChatGPT/Claude.ai-style modern AI reader with persisted conversations, multi-paper pinning, sliding-window+summary context management, and strict-evidence mode wired to `/api/research/snippets`.

**Architecture:** New `internal/service/ai_conversation/` package owns conversation lifecycle and reuses the existing `service.AIService.CallProviderStream` LLM primitive. Three new SQLite tables back persistence. Frontend `/ai` page is rebuilt as a 2-pane SPA (sidebar + main) with five focused JS modules. Strict-evidence mode calls into the existing `research.Client.SnippetSearch`.

**Tech Stack:** Go 1.21 + modernc.org/sqlite, vanilla ES2020 JS (no build step), CSS variables for theming, NDJSON server-sent stream events. Tests via `go test` and Playwright smoke.

**Spec:** `docs/superpowers/specs/2026-04-30-ai-reader-modernization-design.md`

---

## File Structure

### New files (Commit 1 — backend)

| Path | Responsibility |
|---|---|
| `internal/repository/ai_conversation_repo.go` | DB CRUD: conversations / messages / pins. Search via LIKE. |
| `internal/repository/ai_conversation_repo_test.go` | Unit tests (file-backed sqlite tempdir). |
| `internal/service/ai_conversation/types.go` | `Conversation`, `Message`, `PinnedPaper`, request/response types. |
| `internal/service/ai_conversation/service.go` | Conversation lifecycle + send-message orchestration (sliding-window only in C1). |
| `internal/service/ai_conversation/service_test.go` | Service unit tests. |
| `internal/service/ai_conversation/title.go` | Async title generation. |
| `internal/service/ai_conversation/export.go` | Markdown export. |
| `internal/handler/ai_conversation.go` | HTTP handlers. |
| `internal/handler/ai_conversation_test.go` | Handler tests. |

### New files (Commit 2 — frontend)

| Path | Responsibility |
|---|---|
| `web/static/js/ai-conversations.js` | Sidebar list / search / CRUD / inline rename. |
| `web/static/js/ai-conversation-view.js` | Main pane: messages, streaming, export, strict-evidence toggle. |
| `web/static/js/ai-pin.js` | Pin chip area + picker + auto-pin logic. |
| `web/static/js/ai-mention.js` | Existing `@` palette extracted from `ai-reader.js`. |
| `web/static/css/features/ai-sidebar.css` | Sidebar + main pane layout + chip styles. |

### Modified files (Commits 1–3)

| Path | Notes |
|---|---|
| `internal/repository/schema/schema.go` | Add 3 new tables + indexes in `initSchema`. |
| `internal/repository/library_repo.go` | Wire `AIConversation *AIConversationRepository`. |
| `internal/model/ai.go` | Add `PinPapersLimit`, `ContextBudgetTokens` to `AISettings` + defaults. |
| `internal/service/ai_service_provider.go` | Export `CallProviderStreamGeneric` for use by new package. |
| `internal/app/server.go` | Register `/api/ai/conversations/*` routes. Wire new service. |
| `internal/handler/settings.go` | Persist new AI settings fields. |
| `web/ai.html` | Rebuild markup for L1 layout. |
| `web/static/js/ai-reader.js` | Slim down to bootstrap + URL parsing + cross-module state. |
| `web/static/css/features/ai.css` | Drop pieces moved to `ai-sidebar.css`. |
| `web/settings.html` | Two new input fields. |
| `web/static/js/settings.js` | Bind new fields. |
| `web/static/js/browser-pages.js`, `paper-viewer.js` | Add "在 AI 中追问 →" actions. |

### New files (Commit 3 — summarizer + evidence)

| Path | Responsibility |
|---|---|
| `internal/service/ai_conversation/summarizer.go` | Compresses old turns when over budget. |
| `internal/service/ai_conversation/summarizer_test.go` | Tests. |
| `internal/service/ai_conversation/evidence.go` | Calls `research.Client.SnippetSearch`, builds evidence block. |
| `internal/service/ai_conversation/evidence_test.go` | Tests. |
| `web/static/js/ai-evidence.js` | Citation `[n]` → `<sup>` + tooltip. |

---

# Commit 1: 后端 schema + service + API 骨架

## Task 1.1: Add 3 new tables to schema

**Files:**
- Modify: `internal/repository/schema/schema.go:30-150` (within `initSchema`)

- [ ] **Step 1: Append the new tables to the schema string**

After the `research_basket_items` block, before the `CREATE INDEX` block:

```go
CREATE TABLE IF NOT EXISTS ai_conversations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL DEFAULT '',
    title_locked INTEGER NOT NULL DEFAULT 0,
    strict_evidence INTEGER NOT NULL DEFAULT 0,
    summary_text TEXT NOT NULL DEFAULT '',
    summary_through_message_id INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS ai_messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    conversation_id INTEGER NOT NULL REFERENCES ai_conversations(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK(role IN ('user','assistant')),
    content TEXT NOT NULL,
    provider TEXT,
    model TEXT,
    mode TEXT,
    included_figures INTEGER,
    citations_json TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS ai_conversation_papers (
    conversation_id INTEGER NOT NULL REFERENCES ai_conversations(id) ON DELETE CASCADE,
    paper_id INTEGER NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    pinned_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (conversation_id, paper_id)
);
```

After the existing index block, append:

```go
CREATE INDEX IF NOT EXISTS idx_ai_messages_conv     ON ai_messages(conversation_id, id);
CREATE INDEX IF NOT EXISTS idx_ai_conv_papers_paper ON ai_conversation_papers(paper_id);
CREATE INDEX IF NOT EXISTS idx_ai_conv_updated      ON ai_conversations(updated_at DESC);
```

- [ ] **Step 2: Verify schema initializes cleanly**

Run: `cd /home/xzg/project/paper_image_db && go test ./internal/repository/... -run TestNewLibraryRepository -count=1`
Expected: PASS (existing test exercises full schema init).

If the existing tests don't have a name like that, just run `go test ./internal/repository/...` and verify no schema errors.

- [ ] **Step 3: Commit**

```bash
git add internal/repository/schema/schema.go
git commit -m "feat(ai): add ai_conversations / ai_messages / ai_conversation_papers tables"
```

---

## Task 1.2: AIConversationRepository — Conversation CRUD + search

**Files:**
- Create: `internal/repository/ai_conversation_repo.go`
- Create: `internal/repository/ai_conversation_repo_test.go`

- [ ] **Step 1: Write the failing test (Conversation CRUD)**

Create `internal/repository/ai_conversation_repo_test.go`:

```go
package repository

import (
	"path/filepath"
	"testing"
)

func newAIConversationRepoForTest(t *testing.T) *AIConversationRepository {
	t.Helper()
	libRepo, err := NewLibraryRepository(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatalf("NewLibraryRepository: %v", err)
	}
	t.Cleanup(func() { _ = libRepo.Close() })
	return libRepo.AIConversation
}

func TestAIConversationCreateAndGet(t *testing.T) {
	repo := newAIConversationRepoForTest(t)

	id, err := repo.CreateConversation()
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if id <= 0 {
		t.Fatalf("id = %d", id)
	}

	conv, err := repo.GetConversation(id)
	if err != nil {
		t.Fatalf("GetConversation: %v", err)
	}
	if conv.ID != id || conv.Title != "" || conv.StrictEvidence {
		t.Fatalf("conv = %+v", conv)
	}
}

func TestAIConversationListOrderByUpdatedAt(t *testing.T) {
	repo := newAIConversationRepoForTest(t)
	id1, _ := repo.CreateConversation()
	id2, _ := repo.CreateConversation()
	// touch id1 to bump updated_at
	if err := repo.TouchConversation(id1); err != nil {
		t.Fatalf("TouchConversation: %v", err)
	}

	list, err := repo.ListConversations("", 50, 0)
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if len(list) != 2 || list[0].ID != id1 || list[1].ID != id2 {
		t.Fatalf("expected [%d,%d] got %+v", id1, id2, list)
	}
}

func TestAIConversationUpdate(t *testing.T) {
	repo := newAIConversationRepoForTest(t)
	id, _ := repo.CreateConversation()

	if err := repo.UpdateTitle(id, "新标题", true); err != nil {
		t.Fatalf("UpdateTitle: %v", err)
	}
	if err := repo.UpdateStrictEvidence(id, true); err != nil {
		t.Fatalf("UpdateStrictEvidence: %v", err)
	}

	conv, _ := repo.GetConversation(id)
	if conv.Title != "新标题" || !conv.TitleLocked || !conv.StrictEvidence {
		t.Fatalf("conv = %+v", conv)
	}
}

func TestAIConversationDeleteCascadesMessages(t *testing.T) {
	repo := newAIConversationRepoForTest(t)
	id, _ := repo.CreateConversation()
	if _, err := repo.AddMessage(id, "user", "hi", AIMessageMeta{}); err != nil {
		t.Fatalf("AddMessage: %v", err)
	}

	if err := repo.DeleteConversation(id); err != nil {
		t.Fatalf("DeleteConversation: %v", err)
	}

	msgs, err := repo.ListMessages(id, 0, 100)
	if err != nil {
		t.Fatalf("ListMessages after delete: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("messages should be cascaded; got %d", len(msgs))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/xzg/project/paper_image_db && go test ./internal/repository/ -run TestAIConversation -count=1`
Expected: FAIL — `AIConversationRepository` undefined.

- [ ] **Step 3: Implement AIConversationRepository (CRUD half)**

Create `internal/repository/ai_conversation_repo.go`:

```go
package repository

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// ErrAIConversationNotFound is returned when a conversation id has no row.
var ErrAIConversationNotFound = errors.New("ai_conversation: not found")

// AIConversation is one persisted chat session.
type AIConversation struct {
	ID                       int64
	Title                    string
	TitleLocked              bool
	StrictEvidence           bool
	SummaryText              string
	SummaryThroughMessageID  sql.NullInt64
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

// AIMessage is one persisted message (user or assistant).
type AIMessage struct {
	ID              int64
	ConversationID  int64
	Role            string // "user" | "assistant"
	Content         string
	Provider        string
	Model           string
	Mode            string
	IncludedFigures int
	CitationsJSON   string
	CreatedAt       time.Time
}

// AIMessageMeta carries assistant-specific fields when persisting a message.
type AIMessageMeta struct {
	Provider        string
	Model           string
	Mode            string
	IncludedFigures int
	CitationsJSON   string
}

// AIPinnedPaper joins ai_conversation_papers and papers for sidebar / pin chips.
type AIPinnedPaper struct {
	PaperID  int64
	Title    string
	DOI      string
	PinnedAt time.Time
}

// AIConversationRepository owns all three new tables.
type AIConversationRepository struct {
	db *sql.DB
}

// NewAIConversationRepository wires the repo around an open db handle.
func NewAIConversationRepository(db *sql.DB) *AIConversationRepository {
	return &AIConversationRepository{db: db}
}

// CreateConversation inserts a blank row and returns its id.
func (r *AIConversationRepository) CreateConversation() (int64, error) {
	res, err := r.db.Exec(`INSERT INTO ai_conversations DEFAULT VALUES`)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetConversation returns a single conversation row.
func (r *AIConversationRepository) GetConversation(id int64) (AIConversation, error) {
	row := r.db.QueryRow(`
		SELECT id, title, title_locked, strict_evidence,
		       summary_text, summary_through_message_id,
		       created_at, updated_at
		FROM ai_conversations WHERE id = ?
	`, id)
	var c AIConversation
	var titleLocked, strict int
	if err := row.Scan(&c.ID, &c.Title, &titleLocked, &strict,
		&c.SummaryText, &c.SummaryThroughMessageID,
		&c.CreatedAt, &c.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AIConversation{}, ErrAIConversationNotFound
		}
		return AIConversation{}, err
	}
	c.TitleLocked = titleLocked != 0
	c.StrictEvidence = strict != 0
	return c, nil
}

// ListConversations returns conversations matching the optional query string,
// sorted by updated_at DESC, with offset / limit pagination.
func (r *AIConversationRepository) ListConversations(q string, limit, offset int) ([]AIConversation, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q = strings.TrimSpace(q)

	var rows *sql.Rows
	var err error
	if q == "" {
		rows, err = r.db.Query(`
			SELECT id, title, title_locked, strict_evidence,
			       summary_text, summary_through_message_id,
			       created_at, updated_at
			FROM ai_conversations
			ORDER BY updated_at DESC, id DESC
			LIMIT ? OFFSET ?
		`, limit, offset)
	} else {
		// Match title OR any message body OR any pinned paper title.
		like := "%" + strings.ToLower(q) + "%"
		rows, err = r.db.Query(`
			SELECT DISTINCT c.id, c.title, c.title_locked, c.strict_evidence,
			                c.summary_text, c.summary_through_message_id,
			                c.created_at, c.updated_at
			FROM ai_conversations c
			LEFT JOIN ai_messages m ON m.conversation_id = c.id
			LEFT JOIN ai_conversation_papers cp ON cp.conversation_id = c.id
			LEFT JOIN papers p ON p.id = cp.paper_id
			WHERE LOWER(c.title) LIKE ?
			   OR LOWER(m.content) LIKE ?
			   OR LOWER(p.title) LIKE ?
			ORDER BY c.updated_at DESC, c.id DESC
			LIMIT ? OFFSET ?
		`, like, like, like, limit, offset)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]AIConversation, 0)
	for rows.Next() {
		var c AIConversation
		var titleLocked, strict int
		if err := rows.Scan(&c.ID, &c.Title, &titleLocked, &strict,
			&c.SummaryText, &c.SummaryThroughMessageID,
			&c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.TitleLocked = titleLocked != 0
		c.StrictEvidence = strict != 0
		out = append(out, c)
	}
	return out, rows.Err()
}

// TouchConversation bumps updated_at to now.
func (r *AIConversationRepository) TouchConversation(id int64) error {
	_, err := r.db.Exec(`UPDATE ai_conversations SET updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	return err
}

// UpdateTitle sets title (and optionally locks it so the auto-titler skips).
func (r *AIConversationRepository) UpdateTitle(id int64, title string, lock bool) error {
	lockVal := 0
	if lock {
		lockVal = 1
	}
	_, err := r.db.Exec(
		`UPDATE ai_conversations SET title = ?, title_locked = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		title, lockVal, id)
	return err
}

// UpdateStrictEvidence flips the strict_evidence boolean.
func (r *AIConversationRepository) UpdateStrictEvidence(id int64, on bool) error {
	v := 0
	if on {
		v = 1
	}
	_, err := r.db.Exec(
		`UPDATE ai_conversations SET strict_evidence = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		v, id)
	return err
}

// UpdateSummary writes the new summary text and the message id it includes.
func (r *AIConversationRepository) UpdateSummary(id int64, summary string, throughMessageID int64) error {
	_, err := r.db.Exec(
		`UPDATE ai_conversations
		 SET summary_text = ?, summary_through_message_id = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		summary, throughMessageID, id)
	return err
}

// DeleteConversation hard-deletes the conversation; cascade removes messages and pins.
func (r *AIConversationRepository) DeleteConversation(id int64) error {
	_, err := r.db.Exec(`DELETE FROM ai_conversations WHERE id = ?`, id)
	return err
}
```

(Message + pin methods come in Task 1.3 — keep this file under 400 lines.)

- [ ] **Step 4: Stub message/pin methods so the test file compiles**

Append to `ai_conversation_repo.go`:

```go
// AddMessage inserts a message; returns its id. Implemented in Task 1.3.
func (r *AIConversationRepository) AddMessage(conversationID int64, role, content string, meta AIMessageMeta) (int64, error) {
	res, err := r.db.Exec(`
		INSERT INTO ai_messages (conversation_id, role, content, provider, model, mode, included_figures, citations_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, conversationID, role, content, nullableString(meta.Provider), nullableString(meta.Model),
		nullableString(meta.Mode), meta.IncludedFigures, nullableString(meta.CitationsJSON))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListMessages returns messages with id > afterID, oldest first, up to limit.
func (r *AIConversationRepository) ListMessages(conversationID int64, afterID int64, limit int) ([]AIMessage, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := r.db.Query(`
		SELECT id, conversation_id, role, content,
		       COALESCE(provider, ''), COALESCE(model, ''), COALESCE(mode, ''),
		       COALESCE(included_figures, 0), COALESCE(citations_json, ''), created_at
		FROM ai_messages
		WHERE conversation_id = ? AND id > ?
		ORDER BY id ASC
		LIMIT ?
	`, conversationID, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AIMessage, 0)
	for rows.Next() {
		var m AIMessage
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content,
			&m.Provider, &m.Model, &m.Mode, &m.IncludedFigures, &m.CitationsJSON, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
```

- [ ] **Step 5: Wire into LibraryRepository**

Edit `internal/repository/library_repo.go`:

In the `LibraryRepository` struct (around line 28), add:
```go
AIConversation *AIConversationRepository
```

In `NewLibraryRepository` (around line 64), after `researchRepo := NewResearchRepository(db)`, add:
```go
aiConversationRepo := NewAIConversationRepository(db)
```

In the `&LibraryRepository{...}` literal (around line 67), add:
```go
AIConversation: aiConversationRepo,
```

- [ ] **Step 6: Run the test**

Run: `go test ./internal/repository/ -run TestAIConversation -count=1`
Expected: PASS for all 4 tests.

- [ ] **Step 7: Commit**

```bash
git add internal/repository/ai_conversation_repo.go internal/repository/ai_conversation_repo_test.go internal/repository/library_repo.go
git commit -m "feat(ai): AIConversationRepository CRUD + search"
```

---

## Task 1.3: AIConversationRepository — Pin/Unpin + message access

**Files:**
- Modify: `internal/repository/ai_conversation_repo.go`
- Modify: `internal/repository/ai_conversation_repo_test.go`

- [ ] **Step 1: Write failing tests for pins**

Append to `ai_conversation_repo_test.go`:

```go
func TestAIConversationPinPaper(t *testing.T) {
	libRepo, err := NewLibraryRepository(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatalf("NewLibraryRepository: %v", err)
	}
	t.Cleanup(func() { _ = libRepo.Close() })
	repo := libRepo.AIConversation

	// Need a real paper row to satisfy the FK.
	// Use the existing paper repo path; minimal create.
	paperID := mustInsertTestPaper(t, libRepo, "Pinned Test", "10.1/abc")

	convID, _ := repo.CreateConversation()
	if err := repo.PinPaper(convID, paperID); err != nil {
		t.Fatalf("PinPaper: %v", err)
	}
	// idempotent: pinning twice does not error
	if err := repo.PinPaper(convID, paperID); err != nil {
		t.Fatalf("PinPaper second time: %v", err)
	}
	pinned, err := repo.ListPinnedPapers(convID)
	if err != nil {
		t.Fatalf("ListPinnedPapers: %v", err)
	}
	if len(pinned) != 1 || pinned[0].PaperID != paperID || pinned[0].Title != "Pinned Test" {
		t.Fatalf("pinned = %+v", pinned)
	}
	if err := repo.UnpinPaper(convID, paperID); err != nil {
		t.Fatalf("UnpinPaper: %v", err)
	}
	pinned, _ = repo.ListPinnedPapers(convID)
	if len(pinned) != 0 {
		t.Fatalf("expected unpin to clear; got %+v", pinned)
	}
}

// mustInsertTestPaper inserts a minimal papers row (FK target for pin test).
func mustInsertTestPaper(t *testing.T, libRepo *LibraryRepository, title, doi string) int64 {
	t.Helper()
	res, err := libRepo.db.Exec(`
		INSERT INTO papers (title, doi, original_filename, stored_pdf_name)
		VALUES (?, ?, 'test.pdf', 'test.pdf')
	`, title, doi)
	if err != nil {
		t.Fatalf("insert paper: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}
```

- [ ] **Step 2: Run, verify FAIL**

Run: `go test ./internal/repository/ -run TestAIConversationPinPaper -count=1`
Expected: FAIL — `PinPaper` / `UnpinPaper` / `ListPinnedPapers` undefined.

- [ ] **Step 3: Implement pin methods**

Append to `ai_conversation_repo.go`:

```go
// PinPaper attaches a paper to a conversation. Idempotent (PRIMARY KEY).
func (r *AIConversationRepository) PinPaper(conversationID, paperID int64) error {
	_, err := r.db.Exec(`
		INSERT INTO ai_conversation_papers (conversation_id, paper_id)
		VALUES (?, ?)
		ON CONFLICT(conversation_id, paper_id) DO NOTHING
	`, conversationID, paperID)
	if err == nil {
		_ = r.TouchConversation(conversationID)
	}
	return err
}

// UnpinPaper removes the pin. Not-found is silent.
func (r *AIConversationRepository) UnpinPaper(conversationID, paperID int64) error {
	_, err := r.db.Exec(
		`DELETE FROM ai_conversation_papers WHERE conversation_id = ? AND paper_id = ?`,
		conversationID, paperID)
	if err == nil {
		_ = r.TouchConversation(conversationID)
	}
	return err
}

// CountPinnedPapers returns the current pin count (for limit checks).
func (r *AIConversationRepository) CountPinnedPapers(conversationID int64) (int, error) {
	row := r.db.QueryRow(`SELECT COUNT(*) FROM ai_conversation_papers WHERE conversation_id = ?`, conversationID)
	var n int
	err := row.Scan(&n)
	return n, err
}

// ListPinnedPapers returns pinned papers (with paper.title / doi) ordered by pin time.
func (r *AIConversationRepository) ListPinnedPapers(conversationID int64) ([]AIPinnedPaper, error) {
	rows, err := r.db.Query(`
		SELECT p.id, p.title, COALESCE(p.doi, ''), cp.pinned_at
		FROM ai_conversation_papers cp
		JOIN papers p ON p.id = cp.paper_id
		WHERE cp.conversation_id = ?
		ORDER BY cp.pinned_at ASC, p.id ASC
	`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AIPinnedPaper, 0)
	for rows.Next() {
		var p AIPinnedPaper
		if err := rows.Scan(&p.PaperID, &p.Title, &p.DOI, &p.PinnedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Run, verify PASS**

Run: `go test ./internal/repository/ -run TestAIConversation -count=1 -v`
Expected: all 5 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/repository/ai_conversation_repo.go internal/repository/ai_conversation_repo_test.go
git commit -m "feat(ai): pin/unpin papers + message accessors"
```

---

## Task 1.4: AISettings — add `PinPapersLimit` and `ContextBudgetTokens`

**Files:**
- Modify: `internal/model/ai.go:76-96` (struct), `188-231` (defaults)

- [ ] **Step 1: Add fields to struct**

Inside `AISettings` (line ~76), append before the closing brace:

```go
PinPapersLimit       int                   `json:"pin_papers_limit"`
ContextBudgetTokens  int                   `json:"context_budget_tokens"`
```

- [ ] **Step 2: Set defaults**

Inside `DefaultAISettings()`, in the returned literal, before `RolePrompts: []AIRolePrompt{}`:

```go
PinPapersLimit:      5,
ContextBudgetTokens: 32000,
```

- [ ] **Step 3: Verify build**

Run: `go build ./...`
Expected: success.

Run: `go test ./internal/model/... ./internal/service/... -count=1` (sanity, may take ~30s)
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/model/ai.go
git commit -m "feat(ai): AISettings adds PinPapersLimit + ContextBudgetTokens"
```

---

## Task 1.5: ai_conversation package — types & service skeleton

**Files:**
- Create: `internal/service/ai_conversation/types.go`
- Create: `internal/service/ai_conversation/service.go`

- [ ] **Step 1: Write types**

Create `internal/service/ai_conversation/types.go`:

```go
// Package ai_conversation implements stateful AI chat conversations:
// CRUD + pin papers + send-message orchestration with sliding-window context
// management. Strict-evidence and summarization live in sibling files.
package ai_conversation

import "time"

// Conversation is the read view returned by GetConversation.
type Conversation struct {
	ID                int64           `json:"id"`
	Title             string          `json:"title"`
	TitleLocked       bool            `json:"title_locked"`
	StrictEvidence    bool            `json:"strict_evidence"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
	PinnedPapers      []PinnedPaper   `json:"pinned_papers"`
	RecentMessages    []Message       `json:"recent_messages"`
}

// PinnedPaper mirrors AIPinnedPaper from the repo layer.
type PinnedPaper struct {
	PaperID  int64     `json:"paper_id"`
	Title    string    `json:"title"`
	DOI      string    `json:"doi,omitempty"`
	PinnedAt time.Time `json:"pinned_at"`
}

// Message is a single user/assistant message.
type Message struct {
	ID              int64     `json:"id"`
	Role            string    `json:"role"`
	Content         string    `json:"content"`
	Provider        string    `json:"provider,omitempty"`
	Model           string    `json:"model,omitempty"`
	Mode            string    `json:"mode,omitempty"`
	IncludedFigures int       `json:"included_figures,omitempty"`
	CitationsJSON   string    `json:"citations_json,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// SendMessageInput is the body for POST .../messages.
type SendMessageInput struct {
	ConversationID int64  // 0 means "create new"
	Content        string
	PaperID        int64  // optional auto-pin
}

// SendMessageResult is the metadata returned to the handler when the stream is done.
type SendMessageResult struct {
	ConversationID  int64
	UserMessage     Message
	AssistantMessage Message
	GeneratedTitle  string // present only when title was just auto-generated
}
```

- [ ] **Step 2: Write service skeleton**

Create `internal/service/ai_conversation/service.go`:

```go
package ai_conversation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

// AISettingsProvider exposes the parts of *service.AIService we depend on.
// It is satisfied by *service.AIService (see ai_service_export.go bridge).
type AISettingsProvider interface {
	GetSettings() (*model.AISettings, error)
	// LoadPaperContext returns prompt-ready text + image inputs for a paper.
	// (added in Task 1.6)
}

// StreamCaller is the minimal LLM streaming primitive we need.
// Satisfied by service.AIService.CallProviderStreamGeneric (added in Task 1.6).
type StreamCaller interface {
	CallProviderStreamGeneric(ctx context.Context, settings model.AISettings, systemPrompt, userPrompt string,
		images []model.AIImageInput, onDelta func(string) error) (string, string, error)
}

// Service is the conversation lifecycle manager.
type Service struct {
	repo     *repository.AIConversationRepository
	papers   *repository.PaperRepository
	settings AISettingsProvider
	caller   StreamCaller
	logger   *slog.Logger
}

// New builds the service. All deps required.
func New(repo *repository.AIConversationRepository, papers *repository.PaperRepository,
	settings AISettingsProvider, caller StreamCaller, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default().With("component", "ai_conversation")
	}
	return &Service{repo: repo, papers: papers, settings: settings, caller: caller, logger: logger}
}

// CreateDraft creates an empty conversation row. Used by handler when client
// posts to /messages with conversation_id=0.
func (s *Service) CreateDraft() (int64, error) {
	return s.repo.CreateConversation()
}

// GetConversation returns the meta + recent messages for the active pane.
func (s *Service) GetConversation(id int64) (Conversation, error) {
	row, err := s.repo.GetConversation(id)
	if err != nil {
		return Conversation{}, mapRepoErr(err)
	}
	pinned, err := s.repo.ListPinnedPapers(id)
	if err != nil {
		return Conversation{}, err
	}
	msgs, err := s.repo.ListMessages(id, 0, 200)
	if err != nil {
		return Conversation{}, err
	}
	return Conversation{
		ID:             row.ID,
		Title:          row.Title,
		TitleLocked:    row.TitleLocked,
		StrictEvidence: row.StrictEvidence,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
		PinnedPapers:   toPinnedPapers(pinned),
		RecentMessages: toMessages(msgs),
	}, nil
}

// ListConversations is a passthrough.
func (s *Service) ListConversations(q string, limit, offset int) ([]Conversation, error) {
	rows, err := s.repo.ListConversations(q, limit, offset)
	if err != nil {
		return nil, err
	}
	out := make([]Conversation, 0, len(rows))
	for _, r := range rows {
		// Sidebar list: include pinned summary but skip messages.
		pinned, _ := s.repo.ListPinnedPapers(r.ID)
		out = append(out, Conversation{
			ID:             r.ID,
			Title:          r.Title,
			TitleLocked:    r.TitleLocked,
			StrictEvidence: r.StrictEvidence,
			CreatedAt:      r.CreatedAt,
			UpdatedAt:      r.UpdatedAt,
			PinnedPapers:   toPinnedPapers(pinned),
		})
	}
	return out, nil
}

// UpdateTitle replaces the title; lock=true marks it user-edited.
func (s *Service) UpdateTitle(id int64, title string, lock bool) error {
	t := strings.TrimSpace(title)
	if t == "" {
		return apperr.New(apperr.CodeInvalidArgument, "标题不能为空")
	}
	if len([]rune(t)) > 50 {
		t = string([]rune(t)[:50])
	}
	if err := s.repo.UpdateTitle(id, t, lock); err != nil {
		return mapRepoErr(err)
	}
	return nil
}

// UpdateStrictEvidence flips the per-conversation flag.
func (s *Service) UpdateStrictEvidence(id int64, on bool) error {
	if err := s.repo.UpdateStrictEvidence(id, on); err != nil {
		return mapRepoErr(err)
	}
	return nil
}

// DeleteConversation hard-deletes (cascade).
func (s *Service) DeleteConversation(id int64) error {
	if err := s.repo.DeleteConversation(id); err != nil {
		return mapRepoErr(err)
	}
	return nil
}

// PinPaper attaches a paper subject to the configured limit.
func (s *Service) PinPaper(conversationID, paperID int64) error {
	settings, err := s.settings.GetSettings()
	if err != nil {
		return err
	}
	limit := settings.PinPapersLimit
	if limit <= 0 {
		limit = 5
	}
	count, err := s.repo.CountPinnedPapers(conversationID)
	if err != nil {
		return err
	}
	if count >= limit {
		// idempotent re-pin should not 422
		pinned, _ := s.repo.ListPinnedPapers(conversationID)
		for _, p := range pinned {
			if p.PaperID == paperID {
				return nil
			}
		}
		return apperr.New(apperr.CodeFailedPrecondition,
			fmt.Sprintf("最多可 pin %d 篇文献", limit))
	}
	return s.repo.PinPaper(conversationID, paperID)
}

// UnpinPaper detaches.
func (s *Service) UnpinPaper(conversationID, paperID int64) error {
	return s.repo.UnpinPaper(conversationID, paperID)
}

// ListMessages returns messages with id > afterID, oldest first, up to limit.
// Used by the GET /messages endpoint for "load older history" pagination.
func (s *Service) ListMessages(id int64, afterID int64, limit int) ([]Message, error) {
	rows, err := s.repo.ListMessages(id, afterID, limit)
	if err != nil {
		return nil, err
	}
	return toMessages(rows), nil
}

func mapRepoErr(err error) error {
	if errors.Is(err, repository.ErrAIConversationNotFound) {
		return apperr.New(apperr.CodeNotFound, "会话不存在")
	}
	return err
}

func toPinnedPapers(rows []repository.AIPinnedPaper) []PinnedPaper {
	out := make([]PinnedPaper, 0, len(rows))
	for _, r := range rows {
		out = append(out, PinnedPaper{PaperID: r.PaperID, Title: r.Title, DOI: r.DOI, PinnedAt: r.PinnedAt})
	}
	return out
}

func toMessages(rows []repository.AIMessage) []Message {
	out := make([]Message, 0, len(rows))
	for _, r := range rows {
		out = append(out, Message{
			ID: r.ID, Role: r.Role, Content: r.Content,
			Provider: r.Provider, Model: r.Model, Mode: r.Mode,
			IncludedFigures: r.IncludedFigures, CitationsJSON: r.CitationsJSON,
			CreatedAt: r.CreatedAt,
		})
	}
	return out
}

// Touch a value so the import doesn't go stale during incremental development.
var _ = time.Now
```

- [ ] **Step 3: Build**

Run: `go build ./...`
Expected: error — `model.AIImageInput` doesn't exist. Fix in Task 1.6.

- [ ] **Step 4: Stub the missing type**

Create `internal/model/ai_input.go` (new file, exporting the previously unexported helper):

```go
package model

// AIImageInput is the public mirror of the package-private aiImageInput in
// internal/service. It exists so the ai_conversation package can pass image
// data to AIService.CallProviderStreamGeneric without importing internal types.
type AIImageInput struct {
	MIMEType string
	Data     string // base64-encoded
}
```

- [ ] **Step 5: Build again**

Run: `go build ./...`
Expected: still fails — `*service.AIService` doesn't have `CallProviderStreamGeneric` yet. We'll add it in Task 1.6.

The package itself must compile in isolation, however:

Run: `go build ./internal/service/ai_conversation/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/service/ai_conversation/types.go internal/service/ai_conversation/service.go internal/model/ai_input.go
git commit -m "feat(ai): conversation service skeleton (CRUD + pin/unpin)"
```

---

## Task 1.6: Bridge AIService → ai_conversation (export streaming primitive)

**Files:**
- Modify: `internal/service/ai_service_provider.go`
- Create: `internal/service/ai_conversation_bridge.go`

The new package needs a way to call the LLM stream without depending on the internal `aiReadPrepared` type. Add a generic streaming method on `*AIService` that takes plain `model.AISettings` + system/user/images.

- [ ] **Step 1: Read the existing private callProviderStream**

Run: `sed -n '79,150p' internal/service/ai_service_provider.go`
Note the signature; we'll wrap it.

- [ ] **Step 2: Add the public bridge**

Create `internal/service/ai_conversation_bridge.go`:

```go
package service

import (
	"context"

	"github.com/xuzhougeng/citebox/internal/model"
)

// CallProviderStreamGeneric is the public LLM streaming primitive used by
// the ai_conversation package. It bypasses the read-paper specific prepare
// step and accepts arbitrary system/user prompts plus optional images.
//
// Returns the full assembled assistant text, the provider mode label
// (e.g. "responses", "chat"), and an error.
func (s *AIService) CallProviderStreamGeneric(ctx context.Context,
	settings model.AISettings,
	systemPrompt, userPrompt string,
	images []model.AIImageInput,
	onDelta func(string) error,
) (string, string, error) {
	internalImages := make([]aiImageInput, 0, len(images))
	for _, im := range images {
		internalImages = append(internalImages, aiImageInput{MIMEType: im.MIMEType, Data: im.Data})
	}
	prepared := &aiReadPrepared{
		settings:     settings,
		action:       model.AIActionPaperQA,
		systemPrompt: systemPrompt,
		userPrompt:   userPrompt,
		images:       internalImages,
	}
	rawText, err := s.callProviderStream(ctx, prepared, onDelta)
	if err != nil {
		return "", "", err
	}
	return rawText, aiProviderMode(settings), nil
}

// CallProviderGeneric is the non-streaming counterpart, used by the title
// generator and the summarizer.
func (s *AIService) CallProviderGeneric(ctx context.Context,
	settings model.AISettings,
	systemPrompt, userPrompt string,
) (string, string, error) {
	prepared := &aiReadPrepared{
		settings:     settings,
		action:       model.AIActionPaperQA,
		systemPrompt: systemPrompt,
		userPrompt:   userPrompt,
	}
	rawText, mode, err := s.callProvider(ctx, prepared)
	if err != nil {
		return "", "", err
	}
	return rawText, mode, nil
}
```

- [ ] **Step 3: Build**

Run: `go build ./...`
Expected: PASS.

- [ ] **Step 4: Verify existing AI tests still pass**

Run: `go test ./internal/service/ -count=1 -timeout=120s`
Expected: PASS — we didn't change behaviour, only added two methods.

- [ ] **Step 5: Commit**

```bash
git add internal/service/ai_conversation_bridge.go
git commit -m "feat(ai): export CallProviderStreamGeneric for conversation service"
```

---

## Task 1.7: ai_conversation.SendMessage — full happy-path with sliding window

**Files:**
- Modify: `internal/service/ai_conversation/service.go`
- Create: `internal/service/ai_conversation/context_assembler.go`
- Create: `internal/service/ai_conversation/service_test.go`

This is the big one. Implement the send-message flow without summarization or strict-evidence; those slot in later via dedicated files.

- [ ] **Step 1: Write the failing test**

Create `internal/service/ai_conversation/service_test.go`:

```go
package ai_conversation

import (
	"context"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

type stubSettingsProvider struct {
	settings model.AISettings
}

func (s *stubSettingsProvider) GetSettings() (*model.AISettings, error) {
	return &s.settings, nil
}

type stubStreamCaller struct {
	calls       int32
	systemSeen  string
	userSeen    string
	staticReply string
}

func (s *stubStreamCaller) CallProviderStreamGeneric(ctx context.Context, settings model.AISettings,
	systemPrompt, userPrompt string, images []model.AIImageInput, onDelta func(string) error) (string, string, error) {
	atomic.AddInt32(&s.calls, 1)
	s.systemSeen = systemPrompt
	s.userSeen = userPrompt
	if err := onDelta(s.staticReply); err != nil {
		return "", "", err
	}
	return s.staticReply, "test", nil
}

func newServiceForTest(t *testing.T) (*Service, *repository.LibraryRepository, *stubStreamCaller) {
	t.Helper()
	libRepo, err := repository.NewLibraryRepository(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatalf("NewLibraryRepository: %v", err)
	}
	t.Cleanup(func() { _ = libRepo.Close() })

	settings := model.DefaultAISettings()
	settings.APIKey = "fake"
	settings.PinPapersLimit = 5
	settings.ContextBudgetTokens = 32000

	caller := &stubStreamCaller{staticReply: "AI 回答正文"}
	svc := New(libRepo.AIConversation, libRepo.Paper,
		&stubSettingsProvider{settings: settings},
		caller, nil)
	return svc, libRepo, caller
}

func TestSendMessageCreatesConversationAndPersists(t *testing.T) {
	svc, libRepo, caller := newServiceForTest(t)

	convID, err := svc.CreateDraft()
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}

	deltas := []string{}
	res, err := svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: convID,
		Content:        "你好",
	}, func(delta string) error {
		deltas = append(deltas, delta)
		return nil
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if caller.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", caller.calls)
	}
	if res.UserMessage.Content != "你好" || res.AssistantMessage.Content != "AI 回答正文" {
		t.Fatalf("messages = %+v", res)
	}
	if !strings.Contains(strings.Join(deltas, ""), "AI 回答正文") {
		t.Fatalf("deltas = %v", deltas)
	}
	// History persisted
	msgs, _ := libRepo.AIConversation.ListMessages(convID, 0, 100)
	if len(msgs) != 2 || msgs[0].Role != "user" || msgs[1].Role != "assistant" {
		t.Fatalf("persisted msgs = %+v", msgs)
	}
}

func TestSendMessageAutoPinsPaper(t *testing.T) {
	svc, libRepo, _ := newServiceForTest(t)
	paperID := mustInsertPaperForTest(t, libRepo, "Auto Pin Paper", "10.1/auto")
	convID, _ := svc.CreateDraft()

	_, err := svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: convID,
		Content:        "关于这篇论文",
		PaperID:        paperID,
	}, func(string) error { return nil })
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	pinned, _ := libRepo.AIConversation.ListPinnedPapers(convID)
	if len(pinned) != 1 || pinned[0].PaperID != paperID {
		t.Fatalf("expected paper auto-pinned, got %+v", pinned)
	}
}

func TestSendMessagePinLimit(t *testing.T) {
	svc, libRepo, _ := newServiceForTest(t)
	convID, _ := svc.CreateDraft()
	// Pin 5 to fill quota.
	for i := 0; i < 5; i++ {
		pid := mustInsertPaperForTest(t, libRepo, "P", "10.1/x")
		if err := svc.PinPaper(convID, pid); err != nil {
			t.Fatalf("pin %d: %v", i, err)
		}
	}
	// 6th via auto-pin must reject the message.
	pid6 := mustInsertPaperForTest(t, libRepo, "P6", "10.1/six")
	_, err := svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: convID,
		Content:        "再加一篇",
		PaperID:        pid6,
	}, func(string) error { return nil })
	if err == nil {
		t.Fatalf("expected pin-limit rejection")
	}
	if !strings.Contains(err.Error(), "最多") {
		t.Fatalf("error message = %v", err)
	}
}

func mustInsertPaperForTest(t *testing.T, libRepo *repository.LibraryRepository, title, doi string) int64 {
	t.Helper()
	res, err := libRepo.DB().Exec(`INSERT INTO papers (title, doi, original_filename, stored_pdf_name) VALUES (?, ?, 'x.pdf', 'x.pdf')`, title, doi)
	if err != nil {
		t.Fatalf("insert paper: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}
```

- [ ] **Step 2: Run, verify FAIL (compile error: SendMessage undefined; libRepo.DB() undefined)**

Run: `go test ./internal/service/ai_conversation/ -count=1`
Expected: FAIL.

- [ ] **Step 3: Expose `libRepo.DB()` for tests**

Edit `internal/repository/library_repo.go`. After `Close()` method, add:

```go
// DB returns the underlying *sql.DB. Test-only helper; production code must
// use the typed repository methods.
func (r *LibraryRepository) DB() *sql.DB { return r.db }
```

- [ ] **Step 4: Implement context assembler**

Create `internal/service/ai_conversation/context_assembler.go`:

```go
package ai_conversation

import (
	"fmt"
	"strings"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

// assembleContext builds the system + user prompt for a turn. Sliding-window
// truncation only — summarization & evidence injection happen in sibling files
// later.
type assembledContext struct {
	systemPrompt string
	userPrompt   string
	images       []model.AIImageInput
}

// assembleForTurn returns prompts ready for the LLM call. Pinned papers'
// abstract + first ~6 KB of pdf_text are included as context. Recent messages
// are concatenated; oldest are dropped if estimated tokens > budget.
func (s *Service) assembleForTurn(conv repository.AIConversation,
	pinned []repository.AIPinnedPaper, history []repository.AIMessage,
	userText string, settings model.AISettings) (assembledContext, error) {

	var paperBlocks []string
	for _, pp := range pinned {
		paper, err := s.papers.GetPaperDetail(pp.PaperID)
		if err != nil {
			s.logger.Warn("ai_conversation: pinned paper missing", "paper_id", pp.PaperID, "error", err)
			continue
		}
		body := truncateRunes(paper.PDFText, 6000)
		paperBlocks = append(paperBlocks, fmt.Sprintf(
			"### %s\nDOI: %s\n摘要: %s\n正文片段:\n%s",
			paper.Title, paper.DOI,
			truncateRunes(paper.AbstractText, 800),
			body))
	}
	pinnedBlock := ""
	if len(paperBlocks) > 0 {
		pinnedBlock = "已钉文献：\n\n" + strings.Join(paperBlocks, "\n\n---\n\n") + "\n\n"
	}

	// Sliding-window: keep newest history while estimated total stays within budget.
	budget := settings.ContextBudgetTokens
	if budget <= 0 {
		budget = 32000
	}
	systemPrompt := strings.TrimSpace(settings.SystemPrompt)

	var historyLines []string
	staticBudget := estimateTokens(systemPrompt) + estimateTokens(pinnedBlock) + estimateTokens(userText) + 200
	available := budget - staticBudget
	if available < 0 {
		available = 0
	}
	cumulative := 0
	for i := len(history) - 1; i >= 0; i-- {
		m := history[i]
		line := fmt.Sprintf("%s: %s", m.Role, m.Content)
		cost := estimateTokens(line)
		if cumulative+cost > available {
			break
		}
		historyLines = append([]string{line}, historyLines...)
		cumulative += cost
	}

	userPrompt := pinnedBlock
	if conv.SummaryText != "" {
		userPrompt += "对话摘要（更早的内容）：\n" + conv.SummaryText + "\n\n"
	}
	if len(historyLines) > 0 {
		userPrompt += "近期对话：\n" + strings.Join(historyLines, "\n") + "\n\n"
	}
	userPrompt += "用户问题：\n" + userText

	return assembledContext{
		systemPrompt: systemPrompt,
		userPrompt:   userPrompt,
	}, nil
}

// estimateTokens returns a heuristic token count: ASCII chars/4, CJK chars/2.
func estimateTokens(s string) int {
	cjk, ascii := 0, 0
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			cjk++
		} else {
			ascii++
		}
	}
	return cjk/2 + ascii/4
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
```

- [ ] **Step 5: Implement SendMessage on the Service**

Append to `internal/service/ai_conversation/service.go`:

```go
// SendMessage runs one turn: optional auto-pin → INSERT user message → assemble
// prompt → stream LLM → INSERT assistant message → bump updated_at → optional
// async title generation. The onDelta callback receives streamed chunks; the
// caller is responsible for forwarding them to the HTTP client.
func (s *Service) SendMessage(ctx context.Context, in SendMessageInput, onDelta func(string) error) (SendMessageResult, error) {
	if in.ConversationID <= 0 {
		return SendMessageResult{}, apperr.New(apperr.CodeInvalidArgument, "conversation_id 无效")
	}
	if strings.TrimSpace(in.Content) == "" {
		return SendMessageResult{}, apperr.New(apperr.CodeInvalidArgument, "消息内容为空")
	}

	conv, err := s.repo.GetConversation(in.ConversationID)
	if err != nil {
		return SendMessageResult{}, mapRepoErr(err)
	}

	// Auto-pin (β/γ flow). Pin-limit may reject here.
	if in.PaperID > 0 {
		if err := s.PinPaper(in.ConversationID, in.PaperID); err != nil {
			return SendMessageResult{}, err
		}
	}

	// Persist user message immediately so it survives provider failures.
	userMsgID, err := s.repo.AddMessage(in.ConversationID, "user", in.Content, repository.AIMessageMeta{})
	if err != nil {
		return SendMessageResult{}, err
	}

	settings, err := s.settings.GetSettings()
	if err != nil {
		return SendMessageResult{}, err
	}
	pinned, err := s.repo.ListPinnedPapers(in.ConversationID)
	if err != nil {
		return SendMessageResult{}, err
	}
	history, err := s.repo.ListMessages(in.ConversationID, conv.SummaryThroughMessageID.Int64, 1000)
	if err != nil {
		return SendMessageResult{}, err
	}
	// Drop the just-inserted user msg from history (we're about to append it explicitly).
	history = history[:len(history)-1]

	asm, err := s.assembleForTurn(conv, pinned, history, in.Content, *settings)
	if err != nil {
		return SendMessageResult{}, err
	}

	rawText, mode, err := s.caller.CallProviderStreamGeneric(ctx, *settings, asm.systemPrompt, asm.userPrompt, asm.images, onDelta)
	if err != nil {
		// User-cancelled stream: persist whatever was already streamed with mode="stopped".
		// The caller already received those deltas via onDelta; the partial text comes
		// back via rawText (callProviderStream returns accumulated text on cancel).
		if errors.Is(err, context.Canceled) && rawText != "" {
			_, persistErr := s.repo.AddMessage(in.ConversationID, "assistant", rawText, repository.AIMessageMeta{
				Provider: string(settings.Provider),
				Model:    settings.Model,
				Mode:     "stopped",
			})
			if persistErr != nil {
				s.logger.Warn("ai_conversation: persist stopped message failed", "error", persistErr)
			}
			_ = s.repo.TouchConversation(in.ConversationID)
		}
		return SendMessageResult{}, err
	}

	asstID, err := s.repo.AddMessage(in.ConversationID, "assistant", rawText, repository.AIMessageMeta{
		Provider: string(settings.Provider),
		Model:    settings.Model,
		Mode:     mode,
	})
	if err != nil {
		return SendMessageResult{}, err
	}
	_ = s.repo.TouchConversation(in.ConversationID)

	res := SendMessageResult{
		ConversationID:   in.ConversationID,
		UserMessage:      Message{ID: userMsgID, Role: "user", Content: in.Content},
		AssistantMessage: Message{ID: asstID, Role: "assistant", Content: rawText, Provider: string(settings.Provider), Model: settings.Model, Mode: mode},
	}
	return res, nil
}
```

- [ ] **Step 6: Run, verify all 3 tests PASS**

Run: `go test ./internal/service/ai_conversation/ -count=1 -v`
Expected: 3 tests PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/service/ai_conversation/ internal/repository/library_repo.go
git commit -m "feat(ai): SendMessage with sliding-window context assembly"
```

---

## Task 1.8: Async title generation

**Files:**
- Create: `internal/service/ai_conversation/title.go`
- Modify: `internal/service/ai_conversation/service.go` (add hook)
- Modify: `internal/service/ai_conversation/service_test.go`

- [ ] **Step 1: Write the test**

Append to `service_test.go`:

```go
func TestTitleGeneratedOnFirstTurn(t *testing.T) {
	svc, libRepo, caller := newServiceForTest(t)
	convID, _ := svc.CreateDraft()

	// Make the streaming caller return a fixed reply, but set up a non-streaming
	// caller that will be invoked for title generation.
	caller.staticReply = "回答内容"
	titleCaller := &stubNonStreamCaller{staticReply: "  对话标题  "}
	svc.titleCaller = titleCaller

	_, err := svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: convID,
		Content:        "第一条消息",
	}, func(string) error { return nil })
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	// title generation is async; wait briefly
	for i := 0; i < 50; i++ {
		c, _ := libRepo.AIConversation.GetConversation(convID)
		if c.Title != "" {
			if c.Title != "对话标题" {
				t.Fatalf("title = %q, want trimmed", c.Title)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("title was not generated within 1s")
}

type stubNonStreamCaller struct {
	staticReply string
}

func (s *stubNonStreamCaller) CallProviderGeneric(ctx context.Context, settings model.AISettings,
	systemPrompt, userPrompt string) (string, string, error) {
	return s.staticReply, "test", nil
}
```

(Add `"time"` to the imports if not already.)

- [ ] **Step 2: Implement titler**

Create `internal/service/ai_conversation/title.go`:

```go
package ai_conversation

import (
	"context"
	"strings"

	"github.com/xuzhougeng/citebox/internal/model"
)

// NonStreamCaller is the non-streaming counterpart of StreamCaller.
type NonStreamCaller interface {
	CallProviderGeneric(ctx context.Context, settings model.AISettings,
		systemPrompt, userPrompt string) (string, string, error)
}

const titleSystemPrompt = "你是一个会话标题生成器。给出一个简洁、不带引号、不超过 20 字的中文标题，概括下面这段对话的主题。只返回标题文本，不要任何前后缀。"

// generateTitle calls the LLM once with the first user message + first assistant
// reply. Returns the trimmed title text or "" if the call failed.
func generateTitle(ctx context.Context, caller NonStreamCaller, settings model.AISettings,
	userText, assistantText string) string {

	user := "用户：" + userText + "\n\nAI：" + assistantText
	out, _, err := caller.CallProviderGeneric(ctx, settings, titleSystemPrompt, user)
	if err != nil {
		return ""
	}
	t := strings.TrimSpace(out)
	t = strings.Trim(t, "「」\"'`")
	if len([]rune(t)) > 20 {
		t = string([]rune(t)[:20])
	}
	return t
}
```

- [ ] **Step 3: Wire into Service**

In `service.go`, add field to Service:

```go
titleCaller NonStreamCaller
```

In the `New(...)` constructor body, before `return`:
```go
if tc, ok := caller.(NonStreamCaller); ok {
	s.titleCaller = tc
}
```

(`*service.AIService` will satisfy both interfaces.)

Inside `SendMessage`, after the assistant message INSERT and `TouchConversation`, before `return`:

```go
if conv.Title == "" && !conv.TitleLocked && s.titleCaller != nil {
	go func(convID int64, settings model.AISettings, userText, asstText string) {
		bg, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		title := generateTitle(bg, s.titleCaller, settings, userText, asstText)
		if title == "" {
			return
		}
		if err := s.repo.UpdateTitle(convID, title, false); err != nil {
			s.logger.Warn("ai_conversation: title persist failed", "error", err)
		}
	}(in.ConversationID, *settings, in.Content, rawText)
}
```

(Add `"time"` to imports if missing.)

Also fix the `var _ = time.Now` placeholder line — remove it now that `time` is genuinely used.

Then on the test side, the New() ctor returned by `newServiceForTest` won't have `titleCaller` set unless the stub satisfies `NonStreamCaller`. The test sets `svc.titleCaller` directly — make it accept that by changing field visibility. Since the field stays unexported and the test is in the same package, direct assignment works.

- [ ] **Step 4: Run**

Run: `go test ./internal/service/ai_conversation/ -count=1 -v -run TestTitle`
Expected: PASS.

- [ ] **Step 5: Run all package tests**

Run: `go test ./internal/service/ai_conversation/ -count=1 -v`
Expected: 4 tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/service/ai_conversation/title.go internal/service/ai_conversation/service.go internal/service/ai_conversation/service_test.go
git commit -m "feat(ai): async conversation title generation"
```

---

## Task 1.9: Markdown export

**Files:**
- Create: `internal/service/ai_conversation/export.go`
- Modify: `internal/service/ai_conversation/service.go`

- [ ] **Step 1: Write the export function**

Create `internal/service/ai_conversation/export.go`:

```go
package ai_conversation

import (
	"fmt"
	"strings"
	"time"
)

// ExportMarkdown returns conversation content as a single Markdown string.
func (s *Service) ExportMarkdown(id int64) (string, string, error) {
	conv, err := s.repo.GetConversation(id)
	if err != nil {
		return "", "", mapRepoErr(err)
	}
	pinned, err := s.repo.ListPinnedPapers(id)
	if err != nil {
		return "", "", err
	}
	msgs, err := s.repo.ListMessages(id, 0, 1000)
	if err != nil {
		return "", "", err
	}

	var b strings.Builder
	title := conv.Title
	if title == "" {
		title = fmt.Sprintf("会话 #%d", id)
	}
	fmt.Fprintf(&b, "# %s\n\n", title)
	fmt.Fprintf(&b, "_创建于 %s_\n\n", conv.CreatedAt.Format(time.RFC3339))
	if len(pinned) > 0 {
		b.WriteString("**已钉文献**:\n")
		for _, p := range pinned {
			fmt.Fprintf(&b, "- %s", p.Title)
			if p.DOI != "" {
				fmt.Fprintf(&b, " · DOI: %s", p.DOI)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	for _, m := range msgs {
		who := "用户"
		if m.Role == "assistant" {
			who = fmt.Sprintf("AI (%s/%s)", m.Provider, m.Model)
		}
		fmt.Fprintf(&b, "## %s\n\n%s\n\n", who, m.Content)
	}
	filename := safeFilename(title) + ".md"
	return b.String(), filename, nil
}

func safeFilename(title string) string {
	r := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	out := r.Replace(strings.TrimSpace(title))
	if out == "" {
		out = "conversation"
	}
	if len([]rune(out)) > 60 {
		out = string([]rune(out)[:60])
	}
	return out
}
```

- [ ] **Step 2: Build**

Run: `go build ./...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/service/ai_conversation/export.go
git commit -m "feat(ai): conversation markdown export"
```

---

## Task 1.10: HTTP handler

**Files:**
- Create: `internal/handler/ai_conversation.go`
- Create: `internal/handler/ai_conversation_test.go`

- [ ] **Step 1: Write a handler test for List**

Create `internal/handler/ai_conversation_test.go`:

```go
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/service/ai_conversation"
)

type stubAIConversationService struct {
	listResult []ai_conversation.Conversation
	getResult  ai_conversation.Conversation
	getErr     error
	deleted    int64
}

func (s *stubAIConversationService) ListConversations(q string, limit, offset int) ([]ai_conversation.Conversation, error) {
	return s.listResult, nil
}
func (s *stubAIConversationService) GetConversation(id int64) (ai_conversation.Conversation, error) {
	return s.getResult, s.getErr
}
func (s *stubAIConversationService) ListMessages(id, after int64, limit int) ([]ai_conversation.Message, error) {
	return nil, nil
}
func (s *stubAIConversationService) CreateDraft() (int64, error) { return 99, nil }
func (s *stubAIConversationService) UpdateTitle(id int64, title string, lock bool) error {
	return nil
}
func (s *stubAIConversationService) UpdateStrictEvidence(id int64, on bool) error { return nil }
func (s *stubAIConversationService) DeleteConversation(id int64) error            { s.deleted = id; return nil }
func (s *stubAIConversationService) PinPaper(c, p int64) error                    { return nil }
func (s *stubAIConversationService) UnpinPaper(c, p int64) error                  { return nil }
func (s *stubAIConversationService) SendMessage(ctx context.Context, in ai_conversation.SendMessageInput, onDelta func(string) error) (ai_conversation.SendMessageResult, error) {
	_ = onDelta("hi")
	return ai_conversation.SendMessageResult{
		ConversationID:   in.ConversationID,
		UserMessage:      ai_conversation.Message{ID: 1, Role: "user", Content: in.Content},
		AssistantMessage: ai_conversation.Message{ID: 2, Role: "assistant", Content: "hi"},
	}, nil
}
func (s *stubAIConversationService) ExportMarkdown(id int64) (string, string, error) {
	return "# md\n", "x.md", nil
}

func TestAIConversationListEndpoint(t *testing.T) {
	stub := &stubAIConversationService{
		listResult: []ai_conversation.Conversation{{ID: 1, Title: "Test"}},
	}
	h := NewAIConversationHandler(stub)
	req := httptest.NewRequest(http.MethodGet, "/api/ai/conversations", nil)
	rec := httptest.NewRecorder()
	h.List(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Items []ai_conversation.Conversation `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].Title != "Test" {
		t.Fatalf("body = %+v", body)
	}
}

func TestAIConversationDeleteEndpoint(t *testing.T) {
	stub := &stubAIConversationService{}
	h := NewAIConversationHandler(stub)
	req := httptest.NewRequest(http.MethodDelete, "/api/ai/conversations/42", nil)
	rec := httptest.NewRecorder()
	h.Detail(rec, req)
	if rec.Code != 204 {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if stub.deleted != 42 {
		t.Fatalf("deleted id = %d", stub.deleted)
	}
}

func TestAIConversationSendMessageStreams(t *testing.T) {
	stub := &stubAIConversationService{}
	h := NewAIConversationHandler(stub)
	body := strings.NewReader(`{"content":"hi"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/conversations/7/messages", body)
	rec := httptest.NewRecorder()
	h.PostMessage(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "\"delta\":\"hi\"") {
		t.Fatalf("expected NDJSON delta, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "\"type\":\"final\"") {
		t.Fatalf("expected final event, got %s", rec.Body.String())
	}
}
```

- [ ] **Step 2: Run, verify FAIL**

Run: `go test ./internal/handler/ -run TestAIConversation -count=1`
Expected: FAIL — `NewAIConversationHandler` undefined.

- [ ] **Step 3: Implement handler**

Create `internal/handler/ai_conversation.go`:

```go
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/service/ai_conversation"
)

// AIConversationService is the surface AIConversationHandler depends on.
type AIConversationService interface {
	ListConversations(q string, limit, offset int) ([]ai_conversation.Conversation, error)
	GetConversation(id int64) (ai_conversation.Conversation, error)
	ListMessages(id int64, afterID int64, limit int) ([]ai_conversation.Message, error)
	CreateDraft() (int64, error)
	UpdateTitle(id int64, title string, lock bool) error
	UpdateStrictEvidence(id int64, on bool) error
	DeleteConversation(id int64) error
	PinPaper(conversationID, paperID int64) error
	UnpinPaper(conversationID, paperID int64) error
	SendMessage(ctx context.Context, in ai_conversation.SendMessageInput, onDelta func(string) error) (ai_conversation.SendMessageResult, error)
	ExportMarkdown(id int64) (string, string, error)
}

// AIConversationHandler implements /api/ai/conversations/* routes.
type AIConversationHandler struct {
	svc AIConversationService
}

// NewAIConversationHandler returns the handler.
func NewAIConversationHandler(svc AIConversationService) *AIConversationHandler {
	return &AIConversationHandler{svc: svc}
}

// List → GET /api/ai/conversations?q=&limit=&offset=
func (h *AIConversationHandler) List(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, err := h.svc.ListConversations(q, limit, offset)
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}

// Detail → GET/PATCH/DELETE /api/ai/conversations/:id
func (h *AIConversationHandler) Detail(w http.ResponseWriter, r *http.Request) {
	id, err := h.parseConversationID(r.URL.Path)
	if err != nil {
		sendError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		conv, err := h.svc.GetConversation(id)
		if err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, http.StatusOK, conv)
	case http.MethodPatch:
		var body struct {
			Title          *string `json:"title,omitempty"`
			StrictEvidence *bool   `json:"strict_evidence,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
			return
		}
		if body.Title != nil {
			if err := h.svc.UpdateTitle(id, *body.Title, true); err != nil {
				sendError(w, err)
				return
			}
		}
		if body.StrictEvidence != nil {
			if err := h.svc.UpdateStrictEvidence(id, *body.StrictEvidence); err != nil {
				sendError(w, err)
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		if err := h.svc.DeleteConversation(id); err != nil {
			sendError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// PostMessage → POST /api/ai/conversations/:id/messages with NDJSON streaming reply.
func (h *AIConversationHandler) PostMessage(w http.ResponseWriter, r *http.Request) {
	idPart := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/ai/conversations/"), "/messages")
	var conversationID int64
	if idPart == "new" {
		newID, err := h.svc.CreateDraft()
		if err != nil {
			sendError(w, err)
			return
		}
		conversationID = newID
	} else {
		n, err := strconv.ParseInt(idPart, 10, 64)
		if err != nil {
			sendError(w, apperr.New(apperr.CodeInvalidArgument, "conversation id 无效"))
			return
		}
		conversationID = n
	}

	var body struct {
		Content string `json:"content"`
		PaperID int64  `json:"paper_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	encoder := json.NewEncoder(w)
	controller := http.NewResponseController(w)
	send := func(payload map[string]interface{}) error {
		if err := encoder.Encode(payload); err != nil {
			return err
		}
		return controller.Flush()
	}

	// emit conversation id immediately so the client can update the URL
	if err := send(map[string]interface{}{"type": "meta", "conversation_id": conversationID}); err != nil {
		return
	}

	res, err := h.svc.SendMessage(r.Context(), ai_conversation.SendMessageInput{
		ConversationID: conversationID,
		Content:        body.Content,
		PaperID:        body.PaperID,
	}, func(delta string) error {
		return send(map[string]interface{}{"type": "delta", "delta": delta})
	})
	if err != nil {
		_ = send(map[string]interface{}{
			"type":  "error",
			"error": apperr.Message(err),
			"code":  string(apperr.CodeOf(err)),
		})
		return
	}
	_ = send(map[string]interface{}{
		"type":              "final",
		"user_message":      res.UserMessage,
		"assistant_message": res.AssistantMessage,
	})
}

// Messages → GET /api/ai/conversations/:id/messages?after=&limit=
func (h *AIConversationHandler) Messages(w http.ResponseWriter, r *http.Request) {
	id, err := h.parseConversationID(strings.TrimSuffix(r.URL.Path, "/messages"))
	if err != nil {
		sendError(w, err)
		return
	}
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.svc.ListMessages(id, after, limit)
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}

// Pin → POST /api/ai/conversations/:id/papers ; Unpin → DELETE .../papers/:pid
func (h *AIConversationHandler) Pins(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/ai/conversations/"), "/")
	if len(parts) < 2 || parts[1] != "papers" {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "路径错误"))
		return
	}
	convID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "conversation id 无效"))
		return
	}
	switch r.Method {
	case http.MethodPost:
		var body struct {
			PaperID int64 `json:"paper_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			sendError(w, apperr.New(apperr.CodeInvalidArgument, "请求体格式错误"))
			return
		}
		if err := h.svc.PinPaper(convID, body.PaperID); err != nil {
			sendError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		if len(parts) < 3 {
			sendError(w, apperr.New(apperr.CodeInvalidArgument, "paper id 缺失"))
			return
		}
		paperID, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			sendError(w, apperr.New(apperr.CodeInvalidArgument, "paper id 无效"))
			return
		}
		if err := h.svc.UnpinPaper(convID, paperID); err != nil {
			sendError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// Export → GET /api/ai/conversations/:id/export
func (h *AIConversationHandler) Export(w http.ResponseWriter, r *http.Request) {
	id, err := h.parseConversationID(strings.TrimSuffix(r.URL.Path, "/export"))
	if err != nil {
		sendError(w, err)
		return
	}
	md, filename, err := h.svc.ExportMarkdown(id)
	if err != nil {
		sendError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	_, _ = w.Write([]byte(md))
}

func (h *AIConversationHandler) parseConversationID(path string) (int64, error) {
	rest := strings.TrimPrefix(path, "/api/ai/conversations/")
	if idx := strings.Index(rest, "/"); idx >= 0 {
		rest = rest[:idx]
	}
	id, err := strconv.ParseInt(rest, 10, 64)
	if err != nil || id <= 0 {
		return 0, apperr.New(apperr.CodeInvalidArgument, "conversation id 无效")
	}
	return id, nil
}

// Ensure errors.Is etc. are available (kept for future use).
var _ = errors.Is
```

- [ ] **Step 4: Run handler tests**

Run: `go test ./internal/handler/ -run TestAIConversation -count=1 -v`
Expected: 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/handler/ai_conversation.go internal/handler/ai_conversation_test.go
git commit -m "feat(ai): /api/ai/conversations/* HTTP handler"
```

---

## Task 1.11: Wire routes into server.go

**Files:**
- Modify: `internal/app/server.go`

- [ ] **Step 1: Construct service + handler in `buildHandler`**

Edit `internal/app/server.go`. Find the existing `buildHandler` function (around line 204) and locate where `aiHandler` is constructed (`aiHandler := handler.NewAIHandler(aiSvc)`). Right after, add:

```go
aiConvService := ai_conversation.New(repo.AIConversation, repo.Paper, aiSvc, aiSvc, logger.With("component", "ai_conversation"))
aiConversationHandler := handler.NewAIConversationHandler(aiConvService)
```

(Imports: at the top of the file, add `"github.com/xuzhougeng/citebox/internal/service/ai_conversation"` to the import block.)

`aiSvc` (a `*service.AIService`) satisfies both `ai_conversation.AISettingsProvider` and `ai_conversation.StreamCaller` because of Task 1.6's bridge methods.

- [ ] **Step 2: Register the routes**

Find the existing `/api/ai/...` route registrations (around line 414). After the last existing AI route (e.g., the `check-model` route ending around line 480), add:

```go
mux.HandleFunc("/api/ai/conversations", func(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }
    aiConversationHandler.List(w, r)
})

mux.HandleFunc("/api/ai/conversations/", func(w http.ResponseWriter, r *http.Request) {
    p := r.URL.Path
    switch {
    case strings.HasSuffix(p, "/messages"):
        switch r.Method {
        case http.MethodGet:
            aiConversationHandler.Messages(w, r)
        case http.MethodPost:
            aiConversationHandler.PostMessage(w, r)
        default:
            http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        }
    case strings.HasSuffix(p, "/export"):
        if r.Method != http.MethodGet {
            http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
            return
        }
        aiConversationHandler.Export(w, r)
    case strings.Contains(p, "/papers"):
        aiConversationHandler.Pins(w, r)
    default:
        aiConversationHandler.Detail(w, r)
    }
})
```

- [ ] **Step 3: Verify build**

Run: `go build ./...`
Expected: PASS.

- [ ] **Step 4: Run all tests**

Run: `go test ./... -count=1 -timeout=240s`
Expected: All packages PASS, including the new ones.

- [ ] **Step 5: Commit**

```bash
git add internal/app/server.go
git commit -m "feat(ai): register /api/ai/conversations routes"
```

---

## Task 1.12 (Commit 1 close-out): integration smoke

**Files:** none

- [ ] **Step 1: Run server, hit conversation endpoints**

```bash
SMOKE=/tmp/citebox-c1
rm -rf "$SMOKE"; mkdir -p "$SMOKE"
go build -o /tmp/citebox-c1-srv ./cmd/server
DISABLE_AUTH=true STORAGE_DIR="$SMOKE/library" UPLOAD_DIR="$SMOKE/uploads" \
  DATABASE_PATH="$SMOKE/library.db" SERVER_PORT=18092 \
  /tmp/citebox-c1-srv > "$SMOKE/srv.log" 2>&1 &
PID=$!
sleep 1

# create + send (conversation_id="new") — should return NDJSON
curl -sN -X POST -H "Content-Type: application/json" \
  -d '{"content":"hello"}' \
  http://127.0.0.1:18092/api/ai/conversations/new/messages | head

# list
curl -s http://127.0.0.1:18092/api/ai/conversations | head -c 400

kill $PID
```

The send will likely fail with "API key not configured" (no LLM key in smoke env) — that's fine. We're verifying the routes are wired and respond. The `meta` event with `conversation_id` should still come through.

- [ ] **Step 2: Done. The Commit 1 chain is in place.**

No additional commit; this task is just verification.

---

# Commit 2: 前端重构 + 入口注入

> Frontend tasks describe the public surface of each module + the elements / class names / events they touch. Implementations follow the existing project style (vanilla ES2020, no JSX, IIFE wrappers, `window.AIReader.<module>` namespace). Each module gets a brief module-header comment summarizing its responsibility.

## Task 2.1: Refactor /ai HTML to L1 two-pane layout

**Files:**
- Modify: `web/ai.html`

- [ ] **Step 1: Rewrite the `<main>` block**

Replace the current `<main class="page-shell">...</main>` with:

```html
<main class="page-shell ai-page-shell">
    <aside id="aiSidebar" class="ai-sidebar">
        <div class="ai-sidebar-head">
            <button id="aiNewConversation" class="btn btn-primary btn-block" type="button" data-i18n="ai.btn_new_conversation">＋ 新对话</button>
            <input id="aiConversationSearch" class="form-input ai-sidebar-search" type="text" data-i18n-placeholder="ai.search_placeholder" placeholder="搜索对话…">
        </div>
        <ul id="aiConversationList" class="ai-conversation-list" role="list"></ul>
        <div id="aiSidebarEmpty" class="ai-sidebar-empty" hidden>
            <p data-i18n="ai.sidebar_empty">还没有对话。点 + 新对话开始。</p>
        </div>
    </aside>

    <section class="ai-main">
        <header class="ai-main-head">
            <h2 id="aiActiveTitle" class="ai-active-title" data-i18n="ai.active_title_default">新对话</h2>
            <div class="ai-main-actions">
                <label class="ai-strict-toggle">
                    <input id="aiStrictEvidenceToggle" type="checkbox">
                    <span data-i18n="ai.strict_evidence">严格证据</span>
                </label>
                <button id="aiExportConversation" class="btn btn-outline btn-small" type="button" data-i18n="ai.btn_export_conversation">对话导出</button>
                <button id="aiDeleteConversation" class="btn btn-outline btn-small" type="button" data-i18n="ai.btn_delete_conversation">删除</button>
            </div>
        </header>

        <div id="aiPinChips" class="ai-pin-chips"></div>

        <div id="aiConversation" class="ai-conversation"></div>

        <label class="field ai-question-field">
            <span class="ai-question-label" data-i18n="ai.question_label">想让 AI 帮你什么？</span>
            <div class="ai-question-input-wrap">
                <div id="aiQuestionMirror" class="ai-question-mirror" aria-hidden="true"></div>
                <textarea id="aiQuestionInput" class="form-textarea ai-question-input" rows="4" data-i18n-placeholder="ai.question_placeholder" placeholder="输入 @ 切换文献或调用角色…" spellcheck="false"></textarea>
                <div id="aiMentionPopover" class="ai-mention-popover" role="listbox" aria-label="@ 选项" hidden></div>
            </div>
            <div class="ai-role-prompt-bar">
                <p id="aiRolePromptHint" class="ai-role-prompt-hint" data-i18n="ai.role_prompt_hint">输入 @ 唤起选择面板。</p>
                <div id="aiRolePromptQuickList" class="ai-role-prompt-list"></div>
            </div>
        </label>
        <div class="submit-row">
            <button id="runAIReaderButton" class="btn btn-primary" type="button" data-i18n="ai.btn_send">发送问题</button>
            <button id="stopAIReaderButton" class="btn btn-outline" type="button" hidden data-i18n="ai.btn_stop">停止生成</button>
            <div id="aiModelSummary" class="ai-model-summary"></div>
        </div>
    </section>
</main>
```

- [ ] **Step 2: Update script tags**

Replace the existing script block (bottom of `web/ai.html`) with:

```html
<script src="/static/js/theme.js"></script>
<script src="/static/js/i18n.js"></script>
<script src="/static/js/utils.js"></script>
<script src="/static/js/api.js"></script>
<script src="/static/js/paper-viewer.js"></script>
<script src="/static/js/figure-viewer.js"></script>
<script src="/static/js/note-viewer.js"></script>
<script src="/static/js/main.js"></script>
<script src="/static/js/ai-mention.js"></script>
<script src="/static/js/ai-pin.js"></script>
<script src="/static/js/ai-conversations.js"></script>
<script src="/static/js/ai-conversation-view.js"></script>
<script src="/static/js/ai-reader.js"></script>
<script src="/static/js/translate.js"></script>
```

- [ ] **Step 3: Verify HTML parses**

Run: `node -e "new (require('linkedom').DOMParser)().parseFromString(require('fs').readFileSync('/home/xzg/project/paper_image_db/web/ai.html','utf8'),'text/html')" 2>&1 | head` — if `linkedom` not installed, skip and rely on browser smoke.

Or simpler: just visually open the file and confirm it's valid HTML5.

- [ ] **Step 4: Commit**

```bash
git add web/ai.html
git commit -m "refactor(web): rebuild /ai markup as L1 two-pane layout"
```

---

## Task 2.2: Sidebar + main pane CSS

**Files:**
- Create: `web/static/css/features/ai-sidebar.css`
- Modify: `web/static/css/style.css` (import the new file)

- [ ] **Step 1: Find how style.css imports feature CSS**

Run: `grep -n 'features/ai' /home/xzg/project/paper_image_db/web/static/css/style.css`
Expected: shows current `@import` for `features/ai.css`.

- [ ] **Step 2: Add the new file's import below it**

In `style.css`, after the existing `@import url("features/ai.css");` (or equivalent), add:
```css
@import url("features/ai-sidebar.css");
```

- [ ] **Step 3: Write the sidebar CSS**

Create `web/static/css/features/ai-sidebar.css` with the following structure (full content; abbreviated here for plan readability — write all rules listed):

Required selectors and behaviors:

- `.ai-page-shell` — `display: grid; grid-template-columns: 280px 1fr; gap: 1.2rem; height: calc(100vh - var(--nav-height, 64px));`
- `.ai-sidebar` — `display: flex; flex-direction: column; border-right: 1px solid rgba(var(--ink-rgb), 0.08); padding: 1rem 0.75rem; gap: 0.5rem; overflow-y: auto;`
- `.ai-sidebar-head` — vertical stack, gap 0.5rem
- `.btn-block` — full width
- `.ai-sidebar-search` — full width input
- `.ai-conversation-list` — `list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: 0.25rem;`
- `.ai-conversation-row` — `padding: 0.5rem 0.65rem; border-radius: var(--radius-sm); cursor: pointer; color: var(--ink); font-size: 0.92rem;`
- `.ai-conversation-row:hover` — `background: var(--accent-soft);`
- `.ai-conversation-row.is-active` — `background: var(--accent-soft); color: var(--accent-deep); font-weight: 600;`
- `.ai-conversation-row-meta` — `font-size: 0.78rem; color: var(--muted); margin-top: 0.15rem;`
- `.ai-conversation-row-actions` — hover-revealed inline action buttons (rename ✎ / delete ×)
- `.ai-main` — `display: flex; flex-direction: column; padding: 1rem; gap: 1rem; overflow: hidden;`
- `.ai-main-head` — flex row: title left, actions right
- `.ai-active-title` — large heading, `cursor: text` for inline rename
- `.ai-strict-toggle` — inline label + checkbox
- `.ai-pin-chips` — flex row, `gap: 0.4rem; flex-wrap: wrap;`
- `.ai-pin-chip` — `padding: 0.25rem 0.6rem; border-radius: 999px; background: var(--accent-soft); color: var(--accent-deep); font-size: 0.78rem; display: inline-flex; align-items: center; gap: 0.3rem;`
- `.ai-pin-chip-remove` — small × button inside chip
- `.ai-pin-chip.add` — dashed border, no background, "+" label
- All sizes consistent with existing tokens in `variables.css`

- [ ] **Step 4: Hard-refresh /ai in a browser, confirm layout looks right**

(No commit yet — coupled with Task 2.3+.)

- [ ] **Step 5: Commit**

```bash
git add web/static/css/features/ai-sidebar.css web/static/css/style.css
git commit -m "feat(web): /ai sidebar + main pane styles"
```

---

## Task 2.3: Extract `@` palette into `ai-mention.js`

**Files:**
- Create: `web/static/js/ai-mention.js`
- Modify: `web/static/js/ai-reader.js`

- [ ] **Step 1: Identify the mention palette block in ai-reader.js**

Run: `grep -n '_mentionState\|mention\|MentionPopover\|@ palette' /home/xzg/project/paper_image_db/web/static/js/ai-reader.js | head -15`

The mention palette logic spans approximately lines 1070–1280 of the current file (functions `triggerMentionFromButton`, `handleQuestionInput`, `renderMentionPopover`, `_mentionState`, etc.).

- [ ] **Step 2: Move the mention palette into a new module**

Create `web/static/js/ai-mention.js`. Pattern:

```js
// ai-mention.js — @ palette logic. Owns the popover, keyboard nav, and the
// callback that the question input fires when the user types `@`.
// Exposes: window.AIReader.mention.
(function () {
    'use strict';

    const Mention = {
        _state: { active: false, query: '', anchor: null, items: [] },
        // public API:
        attach(input, popover, providers) { /* binds input events */ },
        dismiss() { /* hides popover */ },
        // internal helpers (renderItems, navigate, etc.) — copy verbatim from
        // the old ai-reader.js implementation
    };

    if (typeof window !== 'undefined') {
        window.AIReader = window.AIReader || {};
        window.AIReader.mention = Mention;
    }
})();
```

`providers` is `{ getPapers: () => Paper[], getRolePrompts: () => RolePrompt[], onPickPaper: fn, onPickRole: fn }`. The new conversation-view module wires it.

The exact code to copy lives in `ai-reader.js`. Move every `_mention*` field and method onto the new `Mention` object. Leave `ai-reader.js` referencing `window.AIReader.mention.attach(...)` from its bootstrap.

- [ ] **Step 3: node --check syntax**

Run: `node --check web/static/js/ai-mention.js && echo OK`
Expected: OK.

- [ ] **Step 4: Commit (interim — frontend isn't wired yet)**

```bash
git add web/static/js/ai-mention.js
git commit -m "refactor(web): extract @ palette into ai-mention module"
```

---

## Task 2.4: `ai-conversations.js` — sidebar list + search + CRUD

**Files:**
- Create: `web/static/js/ai-conversations.js`

- [ ] **Step 1: Author the module**

Create `web/static/js/ai-conversations.js`. Required public surface:

```js
window.AIReader.conversations = {
    init(container, options),                  // container = #aiConversationList
    refresh(),                                  // GET /api/ai/conversations + render
    setActive(id),
    setSearchQuery(q),                          // debounced 250ms → refresh
    openInlineRename(rowEl),                    // turns title into <input>
};
```

Behaviors:

- `init` binds delegated click handlers: row click → `options.onSelect(id)`. Hover-revealed ✎/× buttons → rename / delete.
- Renders rows with conversation title (or "新对话" placeholder), the joined pin paper titles in muted small text, and the `is-active` class on `state.activeId`.
- Empty state: when `items.length === 0`, hide the list, show `#aiSidebarEmpty`.
- Search: `setSearchQuery` debounces 250 ms then calls refresh with `?q=...`.
- Delete: `confirm("确认删除？")` → `DELETE /api/ai/conversations/:id` → refresh + emit `ai-reader:conversation-changed` if active was deleted.
- Rename: replace title span with input, blur or Enter commits via `PATCH /api/ai/conversations/:id` `{title}`.

The implementation should follow the same vanilla-JS event-delegation style as `web/static/js/research.js`. Skip duplicating all 250 lines here — write straightforward code.

- [ ] **Step 2: node --check**

Run: `node --check web/static/js/ai-conversations.js && echo OK`

- [ ] **Step 3: Commit**

```bash
git add web/static/js/ai-conversations.js
git commit -m "feat(web): ai-conversations sidebar module"
```

---

## Task 2.5: `ai-conversation-view.js` — main pane rendering + streaming

**Files:**
- Create: `web/static/js/ai-conversation-view.js`

- [ ] **Step 1: Author the module**

Required public surface:

```js
window.AIReader.view = {
    init(elements),                             // {conversation, title, pinChips, strictEvidence, runBtn, stopBtn, exportBtn, deleteBtn, questionInput}
    load(conversationId),                       // GET /api/ai/conversations/:id → render
    loadDraft(prefilledPaperId),                // create empty draft view; conversationId stays null until first send
    sendCurrentInput(),                         // POST .../messages, stream NDJSON, append messages
    stop(),                                     // abort current AbortController
    setStrictEvidence(on),                      // PATCH conversation
    rename(newTitle),                           // PATCH conversation
};
```

Streaming protocol (matches Task 1.10's NDJSON):
- `{type:"meta", conversation_id}` → if currently in draft, persist conversationId, replace URL via `history.replaceState`, refresh sidebar
- `{type:"delta", delta}` → append text to the streaming assistant bubble
- `{type:"final", user_message, assistant_message}` → freeze bubble; render both with citation markers if any
- `{type:"error", code, message}` → toast + leave user message visible

Rendering helpers:
- `renderMessage(m)` — Markdown via existing `Utils.renderMarkdown` if available (check `web/static/js/utils.js`)
- For assistant messages, after render, look at `m.citations_json`; if non-empty, dispatch `ai-reader:message-rendered` so `ai-evidence.js` (Commit 3) can hydrate footnotes
- Empty state: when no messages, render an onboarding card

Wire interactions:
- `runBtn` click / Ctrl+Enter on textarea → `sendCurrentInput()`
- `stopBtn` click → `stop()`
- `strictEvidence` change → `setStrictEvidence(checked)`
- `exportBtn` → `window.location = '/api/ai/conversations/<id>/export'`
- `deleteBtn` → confirm → DELETE → `window.AIReader.conversations.refresh()` + clear pane

- [ ] **Step 2: node --check**

Run: `node --check web/static/js/ai-conversation-view.js && echo OK`

- [ ] **Step 3: Commit**

```bash
git add web/static/js/ai-conversation-view.js
git commit -m "feat(web): ai-conversation-view module (main pane + streaming)"
```

---

## Task 2.6: `ai-pin.js` — pin chip area + picker + auto-pin

**Files:**
- Create: `web/static/js/ai-pin.js`

- [ ] **Step 1: Author the module**

Required public surface:

```js
window.AIReader.pin = {
    init(container, options),                  // container = #aiPinChips
    setPinned(papers),                          // []{paper_id, title, doi}
    refresh(),                                  // GET pinned via /api/ai/conversations/:id
    pin(paperID),                               // POST .../papers
    unpin(paperID),                             // DELETE .../papers/:pid
    openPicker(),                               // overlay with paper search; calls pin() on select
};
```

Behaviors:
- Each chip = `<span class="ai-pin-chip">📌 <em>title</em> <button class="ai-pin-chip-remove">×</button></span>`
- "+" chip at end opens picker overlay with `<input>` that searches `/api/papers?q=...` (existing endpoint) — paginate with limit 20
- Picker uses keyboard nav (↑↓Enter Esc) similar to research autocomplete
- On pick: call `pin()` then close
- pin failure (422 pin_limit) → toast `最多 pin N 篇`, refresh

- [ ] **Step 2: node --check**

Run: `node --check web/static/js/ai-pin.js && echo OK`

- [ ] **Step 3: Commit**

```bash
git add web/static/js/ai-pin.js
git commit -m "feat(web): ai-pin module (chip area + picker)"
```

---

## Task 2.7: Slim down `ai-reader.js` to bootstrap

**Files:**
- Modify: `web/static/js/ai-reader.js`

- [ ] **Step 1: Replace contents with a thin bootstrap**

The new `ai-reader.js` (target ≤200 lines) should:

1. Read URL params on `DOMContentLoaded`:
   - `?conversation=Y` → `window.AIReader.view.load(Y)` + `window.AIReader.conversations.setActive(Y)`
   - `?paper_id=X` → `window.AIReader.view.loadDraft(X)`
   - else → load first conversation from sidebar; if empty, show onboarding
2. `window.AIReader.conversations.init` with `onSelect: id => window.AIReader.view.load(id)`
3. `window.AIReader.view.init` with all the DOM element refs
4. `window.AIReader.pin.init` with the chip container
5. `window.AIReader.mention.attach(textarea, popover, providers)` where providers expose pin papers + role prompts from current AISettings
6. Subscribe to `ai-reader:conversation-changed`, `:pin-updated` events for cross-module refresh
7. Load AI settings once (`GET /api/ai/settings`) and cache in `window.AIReader.settings`

Drop:
- The entire `state.sessions` in-memory map
- The 5-turn round badge logic
- The "current paper bar" — replaced by pin chips
- All in-line conversation rendering

Keep:
- The shortcut help summary modal (`<details class="ai-shortcut-help">` interaction)
- The settings summary panel re-render when settings change

- [ ] **Step 2: node --check**

Run: `node --check web/static/js/ai-reader.js && echo OK`

- [ ] **Step 3: Manual hard-refresh in browser, sanity check page renders**

(Sidebar will be empty — that's OK. We're checking layout + JS doesn't error.)

Run: smoke server (same pattern as Task 1.12) and open `http://localhost:18092/ai`. Open devtools console. No JS errors expected.

- [ ] **Step 4: Commit**

```bash
git add web/static/js/ai-reader.js
git commit -m "refactor(web): slim ai-reader.js to bootstrap; persistence now server-side"
```

---

## Task 2.8: Settings page — `pin_papers_limit` and `context_budget_tokens`

**Files:**
- Modify: `web/settings.html`
- Modify: `web/static/js/settings.js`
- Modify: `internal/handler/settings.go` (if it filters known fields)

- [ ] **Step 1: Find the existing AI settings form section in settings.html**

Run: `grep -n 'aiTemperature\|aiMaxFigures' web/settings.html | head`
Expected: shows the AI section.

- [ ] **Step 2: Add the two new fields in the AI section**

Near `aiMaxFiguresInput`, insert:

```html
<div class="form-row">
    <label class="form-label" for="aiPinPapersLimitInput">
        <span data-i18n="settings.ai.pin_papers_limit_label">单会话最多 pin 文献数</span>
    </label>
    <input id="aiPinPapersLimitInput" class="form-input" type="number" min="1" max="20" step="1">
</div>

<div class="form-row">
    <label class="form-label" for="aiContextBudgetTokensInput">
        <span data-i18n="settings.ai.context_budget_tokens_label">上下文 token 预算</span>
    </label>
    <input id="aiContextBudgetTokensInput" class="form-input" type="number" min="2000" step="1000" data-i18n-placeholder="settings.ai.context_budget_tokens_hint" placeholder="默认 32000">
</div>
```

- [ ] **Step 3: Bind in settings.js**

Find the AI settings load + save logic. Run: `grep -n 'aiMaxFiguresInput' web/static/js/settings.js | head`

In the load path: after `aiMaxFiguresInput.value = settings.max_figures`, add:
```js
document.getElementById('aiPinPapersLimitInput').value = settings.pin_papers_limit || 5;
document.getElementById('aiContextBudgetTokensInput').value = settings.context_budget_tokens || 32000;
```

In the save path: after the `max_figures` field is read into the payload, add:
```js
payload.pin_papers_limit = parseInt(document.getElementById('aiPinPapersLimitInput').value, 10) || 5;
payload.context_budget_tokens = parseInt(document.getElementById('aiContextBudgetTokensInput').value, 10) || 32000;
```

- [ ] **Step 4: Verify the handler persists them**

Run: `grep -n 'pin_papers_limit\|PinPapersLimit\|max_figures' internal/handler/settings.go | head`

If the handler explicitly enumerates known fields, add `pin_papers_limit` and `context_budget_tokens` to that list (mirroring how `max_figures` is handled). If it just JSON-decodes the whole AISettings struct, no change needed (the model fields added in Task 1.4 already make this work).

- [ ] **Step 5: Smoke test**

Open settings page, set values, save, refresh, verify they persist.

- [ ] **Step 6: Commit**

```bash
git add web/settings.html web/static/js/settings.js internal/handler/settings.go
git commit -m "feat(web): settings UI for pin_papers_limit + context_budget_tokens"
```

---

## Task 2.9: Add "在 AI 中追问 →" entries on paper cards

**Files:**
- Modify: `web/static/js/browser-pages.js` (library/figures/notes share this rendering)
- Modify: `web/static/js/paper-viewer.js` (paper detail modal)

- [ ] **Step 1: Find where paper card actions render**

Run: `grep -n 'data-action=' web/static/js/browser-pages.js | head -20`

Identify the paper card action menu / button row.

- [ ] **Step 2: Insert the action**

Add a button/link with `data-action="ask-ai"` and `data-paper-id="<id>"`:

```js
`<button class="paper-action-btn" type="button" data-action="ask-ai" data-paper-id="${paper.id}" data-i18n="paper.btn_ask_ai">在 AI 中追问 →</button>`
```

- [ ] **Step 3: Bind handler**

In the click delegation block:

```js
if (action === 'ask-ai') {
    const id = el.dataset.paperId;
    if (id) window.location.href = '/ai?paper_id=' + encodeURIComponent(id);
    return;
}
```

- [ ] **Step 4: Repeat for paper-viewer.js (modal)**

Find the modal action toolbar (e.g., near the existing "下载 PDF" or "查看引用" buttons) and add the same button + handler.

- [ ] **Step 5: node --check + manual verify**

Run: `node --check web/static/js/browser-pages.js web/static/js/paper-viewer.js`
Expected: OK on both.

Open library; click 在 AI 中追问 on a paper → should navigate to `/ai?paper_id=N`.

- [ ] **Step 6: Commit**

```bash
git add web/static/js/browser-pages.js web/static/js/paper-viewer.js
git commit -m "feat(web): paper cards add 在 AI 中追问 entry → /ai?paper_id"
```

---

## Task 2.10: Playwright smoke for /ai page

**Files:** none (manual smoke)

- [ ] **Step 1: Run smoke server**

```bash
SMOKE=/tmp/citebox-c2
rm -rf "$SMOKE"; mkdir -p "$SMOKE"
go build -o /tmp/citebox-c2-srv ./cmd/server
DISABLE_AUTH=true STORAGE_DIR="$SMOKE/library" UPLOAD_DIR="$SMOKE/uploads" \
  DATABASE_PATH="$SMOKE/library.db" SERVER_PORT=18092 \
  /tmp/citebox-c2-srv > "$SMOKE/srv.log" 2>&1 &
PID=$!; sleep 1
```

- [ ] **Step 2: Navigate to /ai in playwright**

Use the existing playwright tools:
1. `mcp__plugin_playwright_playwright__browser_navigate` → `http://127.0.0.1:18092/ai`
2. `mcp__plugin_playwright_playwright__browser_take_screenshot` → confirms 2-pane layout renders
3. Click `+ 新对话`, type "测试", submit → confirm streaming shows "API key not configured" error toast (no LLM key in smoke env), but conversation row appears in sidebar with "新对话" label
4. Reload page → conversation row still in sidebar (persisted)

- [ ] **Step 3: Stop server, clean up**

```bash
kill $PID; rm -rf /tmp/citebox-c2 /tmp/citebox-c2-srv
```

- [ ] **Step 4: Commit if any small polish issues found and fixed**

(Otherwise no commit; this is verification.)

---

# Commit 3: 摘要器 + 严格证据模式

## Task 3.1: Summarizer

**Files:**
- Create: `internal/service/ai_conversation/summarizer.go`
- Create: `internal/service/ai_conversation/summarizer_test.go`

- [ ] **Step 1: Write the failing test**

Create `summarizer_test.go`:

```go
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
```

- [ ] **Step 2: Run, FAIL**

`go test ./internal/service/ai_conversation/ -run TestSummarizer -count=1`

- [ ] **Step 3: Implement**

Create `summarizer.go`:

```go
package ai_conversation

import (
	"context"
	"fmt"
	"strings"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

const summarizerSystemPrompt = "你是对话摘要器。把下面的对话片段压缩成 ≤300 字的中文摘要，保留关键问题、结论、引用。如果有已有摘要，请把它合并进新摘要中。只返回摘要文本。"

// summarize compresses the oldest half of msgs together with the existing summary.
// Returns the new summary text and the largest message id included.
func summarize(ctx context.Context, caller NonStreamCaller, settings model.AISettings,
	existing string, msgs []repository.AIMessage) (string, int64, error) {
	if len(msgs) == 0 {
		return existing, 0, nil
	}
	half := len(msgs) / 2
	if half == 0 {
		half = 1
	}
	chunk := msgs[:half]
	through := chunk[len(chunk)-1].ID

	var buf strings.Builder
	if existing != "" {
		fmt.Fprintf(&buf, "已有摘要：\n%s\n\n新对话：\n", existing)
	}
	for _, m := range chunk {
		fmt.Fprintf(&buf, "%s: %s\n", m.Role, m.Content)
	}
	out, _, err := caller.CallProviderGeneric(ctx, settings, summarizerSystemPrompt, buf.String())
	if err != nil {
		return "", 0, err
	}
	return strings.TrimSpace(out), through, nil
}
```

- [ ] **Step 4: PASS**

`go test ./internal/service/ai_conversation/ -run TestSummarizer -count=1 -v`

- [ ] **Step 5: Commit**

```bash
git add internal/service/ai_conversation/summarizer.go internal/service/ai_conversation/summarizer_test.go
git commit -m "feat(ai): conversation history summarizer"
```

---

## Task 3.2: Wire summarizer into SendMessage

**Files:**
- Modify: `internal/service/ai_conversation/service.go`
- Modify: `internal/service/ai_conversation/service_test.go`

- [ ] **Step 1: Add a test that forces summary trigger**

Append to `service_test.go`:

```go
func TestSendMessageTriggersSummaryWhenOverBudget(t *testing.T) {
	svc, libRepo, caller := newServiceForTest(t)
	titleCaller := &stubNonStreamCaller{staticReply: "压缩 / 标题"}
	svc.titleCaller = titleCaller
	svc.summaryCaller = titleCaller

	convID, _ := svc.CreateDraft()
	// Seed many messages so we exceed budget.
	bigText := strings.Repeat("一段很长的内容。", 800)
	for i := 0; i < 6; i++ {
		_, _ = libRepo.AIConversation.AddMessage(convID, "user", bigText, repository.AIMessageMeta{})
		_, _ = libRepo.AIConversation.AddMessage(convID, "assistant", bigText, repository.AIMessageMeta{})
	}

	// Force a tiny budget so summarization fires.
	svc.settings = &stubSettingsProvider{settings: func() model.AISettings {
		s := model.DefaultAISettings()
		s.APIKey = "fake"
		s.PinPapersLimit = 5
		s.ContextBudgetTokens = 500 // very small
		return s
	}()}

	caller.staticReply = "答案"
	_, err := svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: convID,
		Content:        "新问题",
	}, func(string) error { return nil })
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	conv, _ := libRepo.AIConversation.GetConversation(convID)
	if conv.SummaryText == "" {
		t.Fatalf("expected summary to be persisted")
	}
}
```

(May need to nudge `stubSettingsProvider` to pass directly the `model.AISettings`; adjust as needed.)

- [ ] **Step 2: Add summaryCaller field + wire in New**

In `service.go`, in the `Service` struct, add:
```go
summaryCaller NonStreamCaller
```

In `New(...)`:
```go
if sc, ok := caller.(NonStreamCaller); ok {
    s.summaryCaller = sc
}
```

(Same instance fine — `*service.AIService` satisfies both.)

- [ ] **Step 3: Inject summarization in `assembleForTurn`**

Refactor `assembleForTurn` so it takes the conversation by pointer, mutates `conv.SummaryText` and `conv.SummaryThroughMessageID`, and persists via repo when summarization runs.

```go
// new helper
func (s *Service) maybeSummarize(ctx context.Context, conv *repository.AIConversation,
	history *[]repository.AIMessage, settings model.AISettings) error {

	if s.summaryCaller == nil {
		return nil
	}
	for attempt := 0; attempt < 3; attempt++ {
		// estimate full prompt size (rough)
		total := estimateTokens(conv.SummaryText)
		for _, m := range *history {
			total += estimateTokens(m.Content)
		}
		if total <= settings.ContextBudgetTokens {
			return nil
		}
		newSummary, through, err := summarize(ctx, s.summaryCaller, settings, conv.SummaryText, *history)
		if err != nil {
			s.logger.Warn("ai_conversation: summarize failed; falling back to truncation", "error", err)
			return nil
		}
		if through == 0 {
			return nil
		}
		if err := s.repo.UpdateSummary(conv.ID, newSummary, through); err != nil {
			return err
		}
		conv.SummaryText = newSummary
		conv.SummaryThroughMessageID.Int64 = through
		conv.SummaryThroughMessageID.Valid = true
		// drop summarized messages from history
		filtered := (*history)[:0]
		for _, m := range *history {
			if m.ID > through {
				filtered = append(filtered, m)
			}
		}
		*history = filtered
	}
	return nil
}
```

In `SendMessage`, after loading `history`, call `s.maybeSummarize(ctx, &conv, &history, *settings)` before `assembleForTurn`.

- [ ] **Step 4: Run all tests**

`go test ./internal/service/ai_conversation/ -count=1 -v`
Expected: previous 4 tests + new summary test PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/ai_conversation/service.go internal/service/ai_conversation/service_test.go
git commit -m "feat(ai): trigger summarizer when prompt exceeds context budget"
```

---

## Task 3.3: Strict-evidence module

**Files:**
- Create: `internal/service/ai_conversation/evidence.go`
- Create: `internal/service/ai_conversation/evidence_test.go`

- [ ] **Step 1: Write the failing test**

Create `evidence_test.go`:

```go
package ai_conversation

import (
	"context"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service/research"
)

type stubSnippetSearcher struct {
	last research.SnippetSearchOpts
	res  research.SnippetList
	err  error
}

func (s *stubSnippetSearcher) SnippetSearch(ctx context.Context, query string, opts research.SnippetSearchOpts) (research.SnippetList, error) {
	s.last = opts
	if s.err != nil {
		return research.SnippetList{}, s.err
	}
	return s.res, nil
}

func TestEvidenceInjectAddsBlockAndCitations(t *testing.T) {
	searcher := &stubSnippetSearcher{
		res: research.SnippetList{
			Items: []research.SnippetMatch{
				{PaperID: "abc", Snippet: research.Snippet{Text: "snippet text", SnippetKind: "body", Section: "Intro"}, Score: 0.9},
			},
		},
	}
	pinned := []repository.AIPinnedPaper{
		{PaperID: 42, Title: "Pinned", DOI: "10.1/abc"},
	}
	prompt, citations, err := injectEvidence(context.Background(), searcher, "用户问题", pinned)
	if err != nil {
		t.Fatalf("injectEvidence: %v", err)
	}
	if !strings.Contains(prompt, "[1]") || !strings.Contains(prompt, "snippet text") {
		t.Fatalf("prompt missing evidence: %s", prompt)
	}
	if len(citations) != 1 || citations[0].PaperID != 42 || citations[0].ExternalID != "DOI:10.1/abc" {
		t.Fatalf("citations = %+v", citations)
	}
	if searcher.last.PaperIDs[0] != "DOI:10.1/abc" {
		t.Fatalf("paperIDs sent to S2 = %v", searcher.last.PaperIDs)
	}
}

func TestEvidenceFallsBackOnSearcherError(t *testing.T) {
	searcher := &stubSnippetSearcher{err: research.ErrRateLimited}
	pinned := []repository.AIPinnedPaper{{PaperID: 1, Title: "x", DOI: "10.1/x"}}
	prompt, _, err := injectEvidence(context.Background(), searcher, "Q", pinned)
	if err == nil {
		t.Fatalf("expected error; caller falls back")
	}
	_ = prompt
}

func TestEvidenceSkipsWhenNoExternalIDs(t *testing.T) {
	searcher := &stubSnippetSearcher{}
	pinned := []repository.AIPinnedPaper{{PaperID: 1, Title: "x", DOI: ""}} // no doi
	_, _, err := injectEvidence(context.Background(), searcher, "Q", pinned)
	if err != ErrNoExternalIDs {
		t.Fatalf("err = %v, want ErrNoExternalIDs", err)
	}
}
```

- [ ] **Step 2: FAIL**

`go test ./internal/service/ai_conversation/ -run TestEvidence -count=1`

- [ ] **Step 3: Implement**

Create `evidence.go`:

```go
package ai_conversation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service/research"
)

// ErrNoExternalIDs signals that none of the pinned papers had a DOI/arXiv id we
// could pass to /snippet/search.
var ErrNoExternalIDs = errors.New("ai_conversation: no usable external ids")

// SnippetSearcher is the surface we depend on (satisfied by *research.Service or *research.Client).
type SnippetSearcher interface {
	SnippetSearch(ctx context.Context, query string, opts research.SnippetSearchOpts) (research.SnippetList, error)
}

// Citation is one entry in the persisted citations_json array.
type Citation struct {
	I          int                  `json:"i"`
	PaperID    int64                `json:"paper_id"`
	ExternalID string               `json:"external_id"`
	S2PaperID  string               `json:"s2_paper_id,omitempty"`
	Snippet    research.Snippet     `json:"snippet"`
	Score      float64              `json:"score"`
}

// injectEvidence runs SnippetSearch over the pinned papers' external ids and
// returns (evidence-block prompt fragment, citations array, err).
func injectEvidence(ctx context.Context, searcher SnippetSearcher,
	userText string, pinned []repository.AIPinnedPaper) (string, []Citation, error) {

	idMap := map[string]int64{}
	idList := make([]string, 0, len(pinned))
	for _, p := range pinned {
		ext := externalIDFor(p)
		if ext == "" {
			continue
		}
		idMap[ext] = p.PaperID
		idList = append(idList, ext)
	}
	if len(idList) == 0 {
		return "", nil, ErrNoExternalIDs
	}

	q := userText
	if len([]rune(q)) > 200 {
		q = string([]rune(q)[:200])
	}
	res, err := searcher.SnippetSearch(ctx, q, research.SnippetSearchOpts{
		PaperIDs: idList,
		Limit:    8,
	})
	if err != nil {
		return "", nil, err
	}

	citations := make([]Citation, 0, len(res.Items))
	var b strings.Builder
	b.WriteString("你必须基于以下从已钉文献中检索到的证据片段回答。每个论断后用 [n] 标注引用。如果证据不足以支撑回答，请明确说明\"证据不足\"。\n\n证据：\n")
	for i, m := range res.Items {
		idx := i + 1
		ext := ""
		var paperID int64
		// match back to pinned paper by S2 paperId or fall back to the order of idList
		// (S2 returns paper.PaperID; we don't have a clean DOI map, so try ExternalIDs.DOI)
		if m.Paper.ExternalIDs.DOI != "" {
			ext = "DOI:" + m.Paper.ExternalIDs.DOI
		} else if m.Paper.ExternalIDs.ArXiv != "" {
			ext = "ARXIV:" + m.Paper.ExternalIDs.ArXiv
		}
		if id, ok := idMap[ext]; ok {
			paperID = id
		}
		citations = append(citations, Citation{
			I: idx, PaperID: paperID, ExternalID: ext, S2PaperID: m.PaperID,
			Snippet: m.Snippet, Score: m.Score,
		})
		section := m.Snippet.Section
		if section == "" {
			section = m.Snippet.SnippetKind
		}
		fmt.Fprintf(&b, "[%d] (%s) %s\n", idx, section, m.Snippet.Text)
	}
	b.WriteString("\n用户问题：\n")
	b.WriteString(userText)
	return b.String(), citations, nil
}

func externalIDFor(p repository.AIPinnedPaper) string {
	if p.DOI != "" {
		return "DOI:" + p.DOI
	}
	return ""
}

// MarshalCitations is a tiny convenience wrapper.
func MarshalCitations(c []Citation) string {
	if len(c) == 0 {
		return ""
	}
	buf, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	return string(buf)
}
```

- [ ] **Step 4: PASS**

`go test ./internal/service/ai_conversation/ -run TestEvidence -count=1 -v`

- [ ] **Step 5: Commit**

```bash
git add internal/service/ai_conversation/evidence.go internal/service/ai_conversation/evidence_test.go
git commit -m "feat(ai): strict-evidence injector + citation builder"
```

---

## Task 3.4: Wire evidence into SendMessage

**Files:**
- Modify: `internal/service/ai_conversation/service.go`
- Modify: `internal/app/server.go`

- [ ] **Step 1: Add the searcher dep to Service**

In `Service`:
```go
searcher SnippetSearcher
```

Update `New(...)` signature to accept it:
```go
func New(repo *repository.AIConversationRepository, papers *repository.PaperRepository,
    settings AISettingsProvider, caller StreamCaller, searcher SnippetSearcher,
    logger *slog.Logger) *Service {
```

- [ ] **Step 2: In SendMessage, when conv.StrictEvidence is true, replace user prompt**

After `asm, err := s.assembleForTurn(...)`:

```go
var citations []Citation
if conv.StrictEvidence && s.searcher != nil {
    enrichedUser, cites, evErr := injectEvidence(ctx, s.searcher, in.Content, pinned)
    if evErr != nil {
        if !errors.Is(evErr, ErrNoExternalIDs) {
            s.logger.Warn("ai_conversation: evidence search failed", "error", evErr)
        }
        // fall back: leave asm as-is. emit a warning event up to caller via onDelta
        _ = onDelta("\n\n_(证据检索失败或无外部标识，本次按普通模式作答)_\n\n")
    } else {
        // replace the "用户问题：..." trailing block by re-assembling with enriched text
        asm.userPrompt = strings.TrimSuffix(asm.userPrompt, "用户问题：\n"+in.Content) + enrichedUser
        citations = cites
    }
}
```

(Make sure `errors` and `strings` are imported.)

- [ ] **Step 3: After assistant message INSERT, write citations_json**

In `SendMessage`, change the assistant insert to include citations:

```go
asstID, err := s.repo.AddMessage(in.ConversationID, "assistant", rawText, repository.AIMessageMeta{
    Provider: string(settings.Provider),
    Model:    settings.Model,
    Mode:     mode,
    CitationsJSON: MarshalCitations(citations),
})
```

- [ ] **Step 4: Wire searcher in server.go**

In `buildHandler`, the `aiConvService := ai_conversation.New(...)` line needs the searcher.

We already construct `researchSvc := research.NewService(s2Client, ...)` earlier. `*research.Service` satisfies `SnippetSearcher` (it has the `SnippetSearch` method).

Update the call:
```go
aiConvService := ai_conversation.New(repo.AIConversation, repo.Paper, aiSvc, aiSvc, researchSvc,
    logger.With("component", "ai_conversation"))
```

- [ ] **Step 5: Build + tests**

```bash
go build ./...
go test ./internal/service/ai_conversation/ -count=1
```

- [ ] **Step 6: Commit**

```bash
git add internal/service/ai_conversation/service.go internal/app/server.go
git commit -m "feat(ai): inject strict-evidence into SendMessage"
```

---

## Task 3.5: Frontend citation rendering

**Files:**
- Create: `web/static/js/ai-evidence.js`
- Modify: `web/ai.html` (add script tag)
- Modify: `web/static/css/features/ai-sidebar.css` (citation styles)

- [ ] **Step 1: Create the module**

Create `web/static/js/ai-evidence.js`. Required public surface:

```js
window.AIReader.evidence = {
    init() {
        document.addEventListener('ai-reader:message-rendered', (e) => {
            this.hydrate(e.detail.element, e.detail.citations);
        });
    },
    hydrate(messageEl, citationsJSON) { /* parse JSON, replace [n] tokens, attach hover tooltip */ },
};
```

Behaviour:

- Parse `citationsJSON`. Bail if empty.
- Within `messageEl`, replace text nodes containing `\[(\d+)\]` patterns with `<sup class="ai-citation" data-cite="$1">[$1]</sup>`.
- On `mouseenter` / `focus`, render a tooltip near the `<sup>`. Tooltip layout:
  - Header: paper title (resolve via citation.paper_id → look up active conversation's pinned papers)
  - Section label
  - Snippet text (truncate at 400 chars + "…")
  - "在原文中查看 →" link → if paper exists in library, opens paper-viewer modal at the section; otherwise external link to S2 (`https://www.semanticscholar.org/paper/<s2_paper_id>`)
- Single shared tooltip element appended to body, repositioned per hover.

- [ ] **Step 2: Add citation styles**

Append to `web/static/css/features/ai-sidebar.css`:

```css
.ai-citation {
    color: var(--accent);
    cursor: pointer;
    margin: 0 0.05rem;
    font-size: 0.78em;
    font-weight: 600;
}
.ai-citation:hover { color: var(--accent-deep); text-decoration: underline; }

.ai-citation-tooltip {
    position: absolute;
    z-index: 60;
    max-width: 360px;
    padding: 0.7rem 0.85rem;
    background: var(--panel-strong);
    border: 1px solid rgba(var(--ink-rgb), 0.1);
    border-radius: var(--radius-sm);
    box-shadow: var(--shadow);
    color: var(--ink);
    font-size: 0.85rem;
    line-height: 1.5;
}
.ai-citation-tooltip[hidden] { display: none; }
.ai-citation-tooltip h4 { margin: 0 0 0.3rem; font-size: 0.9rem; }
.ai-citation-tooltip .meta { color: var(--muted); font-size: 0.78rem; margin-bottom: 0.4rem; }
.ai-citation-tooltip .src { display: block; margin-top: 0.5rem; color: var(--accent); font-size: 0.8rem; }
```

- [ ] **Step 3: Hook in conversation-view**

In `ai-conversation-view.js`'s `renderMessage` (Task 2.5), after rendering an assistant message, dispatch:

```js
if (message.citations_json) {
    document.dispatchEvent(new CustomEvent('ai-reader:message-rendered', {
        detail: { element: bubbleEl, citations: message.citations_json }
    }));
}
```

- [ ] **Step 4: Add script tag**

In `web/ai.html`, before `ai-reader.js`:

```html
<script src="/static/js/ai-evidence.js"></script>
```

- [ ] **Step 5: node --check**

`node --check web/static/js/ai-evidence.js`

- [ ] **Step 6: Commit**

```bash
git add web/static/js/ai-evidence.js web/static/js/ai-conversation-view.js web/static/css/features/ai-sidebar.css web/ai.html
git commit -m "feat(web): citation footnote rendering with hover tooltip"
```

---

## Task 3.6: Integration smoke + final QA

**Files:** none

- [ ] **Step 1: Run full test suite**

```bash
go test ./... -count=1 -timeout=240s
```
Expected: all packages PASS.

- [ ] **Step 2: Smoke server, full Playwright walkthrough**

Steps:
1. Open `/library`, click 在 AI 中追问 on a paper → land on `/ai?paper_id=N` → paper auto-pinned
2. Type "请总结这篇" → submit → streaming reply visible (will probably fail due to no API key in smoke; that's a known smoke limitation)
3. Open settings → set context budget to 1000 → return to /ai → send → confirm summarize fires (check server log)
4. Toggle 严格证据 on → submit a follow-up → confirm `[1]` appears (if S2 has the DOI)
5. Refresh page → conversation persists in sidebar

- [ ] **Step 3: Push branch, open PR**

```bash
git push -u origin feat/ai-reader-modernization
gh pr create --title "feat(ai): modernize /ai with persistent conversations + multi-pin + strict-evidence" \
  --body-file docs/superpowers/specs/2026-04-30-ai-reader-modernization-design.md
```

(Or hand-write the PR body summarizing the three commits.)

- [ ] **Step 4: Done.**

---

## Out of Scope Reminder

Per spec § 12, the following are **not** in this plan:

- Regenerate last assistant response
- Edit user message + re-run / branch tree
- Star / archive / folders
- Mobile responsive layout
- AI tool-use auto-evidence (γ approach)
- FTS5 search backend
- Multi-account / shared conversations

These should land in subsequent specs/plans, not this one.

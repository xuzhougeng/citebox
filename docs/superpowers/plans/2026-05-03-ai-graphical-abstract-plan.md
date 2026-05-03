# AI Graphical Abstract Generation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an explicit `@image-gen` tool to the AI 伴读 conversation that turns `@paper` and/or `@figure` references into an AI-generated graphical-abstract image, surfaced as a structured result card.

**Architecture:** A new `ai_image_gen` Go service runs the three-stage pipeline (vision-understand → draft prompt → call OpenAI-shape `images.generations`) inside the existing `ai_conversation` SSE stream. Outputs are persisted as files on disk plus a row in a new `ai_generated_images` table, and surfaced as a `generated_image` ResultCard. Frontend gets a new tool-tag entry (`@image-gen`) and a new mention type (`@figure-N` scoped to pinned papers).

**Tech Stack:** Go 1.x, SQLite, native HTML/CSS/JS frontend, OpenAI-compatible HTTP image API, existing `internal/service/ai` provider helpers, existing SSE streaming layer.

**Reference spec:** `docs/superpowers/specs/2026-05-03-ai-graphical-abstract-design.md`

---

## File map

| Action | Path | Responsibility |
|---|---|---|
| Modify | `internal/repository/schema/schema.go` | New table `ai_generated_images` + indexes (added inside `ensureAIOrchestrationSchema` block) |
| Create | `internal/repository/ai_generated_image_repo.go` | CRUD: Insert / GetByID / ListByConversation; cascade-delete is enforced by FKs |
| Create | `internal/repository/ai_generated_image_repo_test.go` | Round-trip + cascade tests |
| Modify | `internal/model/ai.go` | Add `AIImageGenSettings` struct; embed as `AISettings.ImageGen`; add defaults |
| Modify | `internal/service/ai/settings.go` | Normalize `ImageGen` block (defaults, enum validation) |
| Create | `internal/service/ai_image_gen/types.go` | `GenerateInput`, `Settings`, `StreamEvent` types, error sentinels |
| Create | `internal/service/ai_image_gen/prompt.go` | Vision-stage system prompt + user-prompt builder |
| Create | `internal/service/ai_image_gen/prompt_test.go` | Golden test for prompt assembly |
| Create | `internal/service/ai_image_gen/client.go` | HTTP client for `POST /v1/images/generations` |
| Create | `internal/service/ai_image_gen/storage.go` | Disk write to `data/ai_generated/<conv>/<ulid>.png` |
| Create | `internal/service/ai_image_gen/service.go` | Orchestration: load-inputs → vision → prompt → image-call → save → emit |
| Create | `internal/service/ai_image_gen/service_test.go` | Fake providers, assert event sequence + persistence |
| Modify | `internal/service/ai_conversation/service.go` | In `SendMessage`: detect `intent_hint == "image_generation"` → dispatch to `ai_image_gen.Service` |
| Modify | `internal/service/ai_conversation/types.go` | Add `ImageGen ai_image_gen.Generator` interface field on Service |
| Create | `internal/handler/ai_generated_image.go` | `GET /api/ai-generated-images/:id/file` (auth + ownership) |
| Create | `internal/handler/ai_generated_image_test.go` | Auth required + cross-conversation 404 |
| Modify | `internal/app/router.go` (or wherever routes register) | Route registration + handler wiring |
| Modify | `internal/config/config.go` | New `AIGeneratedDir()` helper returning `<StorageDir>/ai_generated` |
| Modify | `web/static/js/ai-mention-tags.js` | Add `image-gen` to `KNOWN_TOOL_TAGS` + family mapping; widen `TAG_RE` |
| Modify | `web/static/js/ai-reader.js` | Add `image-gen` description + disabled-reason in `getToolTags()`; provide `getFigures()` callback |
| Modify | `web/static/js/ai-mention.js` | Add `figure` mention section, pulled from `getFigures()` provider |
| Modify | `web/static/js/ai-result-cards.js` | Render `card_type === 'generated_image'` |
| Modify | `web/static/js/ai-conversation-view.js` | Render new SSE events `image_prompt_drafted` / `image_generating` / `image_generated` / `image_failed` |
| Modify | `web/static/locales/zh-CN/ai.json` and `web/static/locales/en/ai.json` | New i18n keys (listed in Task 14) |
| Modify | `docs/api.md` | Document new endpoint + SSE events |
| Modify | `docs/database.md` | Document new table |
| Modify | `TODO` | Move "生图功能" entry into 已完成 |

---

## Task 1: Schema migration for `ai_generated_images`

**Files:**
- Modify: `internal/repository/schema/schema.go` (extend `ensureAIOrchestrationSchema` around line 272)
- Test: validated via Task 2's repo round-trip

- [ ] **Step 1: Add the table + indexes to the migration block**

In `internal/repository/schema/schema.go`, inside `ensureAIOrchestrationSchema()`, append new statements to the existing `stmts` slice (just before the `for _, stmt := range stmts` loop):

```go
		`CREATE TABLE IF NOT EXISTS ai_generated_images (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			turn_run_id INTEGER NOT NULL REFERENCES ai_turn_runs(id) ON DELETE CASCADE,
			conversation_id INTEGER NOT NULL REFERENCES ai_conversations(id) ON DELETE CASCADE,
			file_path TEXT NOT NULL,
			prompt TEXT NOT NULL,
			model TEXT NOT NULL,
			size TEXT NOT NULL,
			quality TEXT NOT NULL,
			source_paper_ids TEXT NOT NULL DEFAULT '[]',
			source_figure_ids TEXT NOT NULL DEFAULT '[]',
			cost_estimate_usd REAL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ai_generated_images_conv ON ai_generated_images(conversation_id, id)`,
		`CREATE INDEX IF NOT EXISTS idx_ai_generated_images_turn ON ai_generated_images(turn_run_id)`,
```

- [ ] **Step 2: Build to confirm migration compiles**

Run: `go build ./...`
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add internal/repository/schema/schema.go
git commit -m "schema: add ai_generated_images table for AI-generated graphical abstracts"
```

---

## Task 2: Repository for `ai_generated_images`

**Files:**
- Create: `internal/repository/ai_generated_image_repo.go`
- Create: `internal/repository/ai_generated_image_repo_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/repository/ai_generated_image_repo_test.go`:

```go
package repository

import (
	"database/sql"
	"testing"
	"time"

	"github.com/xuzhougeng/citebox/internal/repository/schema"
	_ "modernc.org/sqlite"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable fk: %v", err)
	}
	if err := schema.NewManager(db).Initialize(); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func seedConversationAndTurn(t *testing.T, db *sql.DB) (convID, turnID int64) {
	t.Helper()
	res, err := db.Exec(`INSERT INTO ai_conversations (title) VALUES (?)`, "test")
	if err != nil {
		t.Fatalf("seed conv: %v", err)
	}
	convID, _ = res.LastInsertId()

	msg, err := db.Exec(`INSERT INTO ai_messages (conversation_id, role, content) VALUES (?, 'user', 'hi')`, convID)
	if err != nil {
		t.Fatalf("seed msg: %v", err)
	}
	msgID, _ := msg.LastInsertId()

	tr, err := db.Exec(`INSERT INTO ai_turn_runs (conversation_id, user_message_id) VALUES (?, ?)`, convID, msgID)
	if err != nil {
		t.Fatalf("seed turn: %v", err)
	}
	turnID, _ = tr.LastInsertId()
	return convID, turnID
}

func TestAIGeneratedImageRepository_RoundTrip(t *testing.T) {
	db := newTestDB(t)
	convID, turnID := seedConversationAndTurn(t, db)

	repo := NewAIGeneratedImageRepository(db)
	id, err := repo.Insert(AIGeneratedImage{
		TurnRunID:        turnID,
		ConversationID:   convID,
		FilePath:         "data/ai_generated/1/abc.png",
		Prompt:           "a graphical abstract of CRISPR",
		Model:            "gpt-image-2",
		Size:             "1024x1024",
		Quality:          "high",
		SourcePaperIDs:   []int64{1, 2},
		SourceFigureIDs:  []int64{},
		CostEstimateUSD:  sql.NullFloat64{Float64: 0.19, Valid: true},
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected non-zero id, got %d", id)
	}

	got, err := repo.GetByID(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.FilePath != "data/ai_generated/1/abc.png" || got.Model != "gpt-image-2" {
		t.Fatalf("unexpected row: %+v", got)
	}
	if len(got.SourcePaperIDs) != 2 || got.SourcePaperIDs[0] != 1 {
		t.Fatalf("source_paper_ids round-trip wrong: %+v", got.SourcePaperIDs)
	}
	if got.CreatedAt.IsZero() || time.Since(got.CreatedAt) > time.Minute {
		t.Fatalf("created_at not populated: %v", got.CreatedAt)
	}
}

func TestAIGeneratedImageRepository_CascadeOnTurnDelete(t *testing.T) {
	db := newTestDB(t)
	convID, turnID := seedConversationAndTurn(t, db)

	repo := NewAIGeneratedImageRepository(db)
	id, err := repo.Insert(AIGeneratedImage{
		TurnRunID: turnID, ConversationID: convID,
		FilePath: "x.png", Prompt: "p", Model: "gpt-image-2",
		Size: "1024x1024", Quality: "high",
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	if _, err := db.Exec(`DELETE FROM ai_turn_runs WHERE id = ?`, turnID); err != nil {
		t.Fatalf("delete turn: %v", err)
	}

	if _, err := repo.GetByID(id); err != ErrAIGeneratedImageNotFound {
		t.Fatalf("expected ErrAIGeneratedImageNotFound after cascade, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/repository/ -run TestAIGeneratedImage -v`
Expected: build failure (`NewAIGeneratedImageRepository` not defined).

- [ ] **Step 3: Write the repository**

Create `internal/repository/ai_generated_image_repo.go`:

```go
package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

var ErrAIGeneratedImageNotFound = errors.New("ai_generated_image not found")

type AIGeneratedImage struct {
	ID              int64
	TurnRunID       int64
	ConversationID  int64
	FilePath        string
	Prompt          string
	Model           string
	Size            string
	Quality         string
	SourcePaperIDs  []int64
	SourceFigureIDs []int64
	CostEstimateUSD sql.NullFloat64
	CreatedAt       time.Time
}

type AIGeneratedImageRepository struct {
	db *sql.DB
}

func NewAIGeneratedImageRepository(db *sql.DB) *AIGeneratedImageRepository {
	return &AIGeneratedImageRepository{db: db}
}

func (r *AIGeneratedImageRepository) Insert(img AIGeneratedImage) (int64, error) {
	paperIDs, err := json.Marshal(nonNilInt64Slice(img.SourcePaperIDs))
	if err != nil {
		return 0, err
	}
	figureIDs, err := json.Marshal(nonNilInt64Slice(img.SourceFigureIDs))
	if err != nil {
		return 0, err
	}
	res, err := r.db.Exec(`
		INSERT INTO ai_generated_images (
			turn_run_id, conversation_id, file_path, prompt,
			model, size, quality,
			source_paper_ids, source_figure_ids, cost_estimate_usd
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		img.TurnRunID, img.ConversationID, img.FilePath, img.Prompt,
		img.Model, img.Size, img.Quality,
		string(paperIDs), string(figureIDs), nullFloatToInterface(img.CostEstimateUSD),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *AIGeneratedImageRepository) GetByID(id int64) (AIGeneratedImage, error) {
	var img AIGeneratedImage
	var paperIDs, figureIDs string
	err := r.db.QueryRow(`
		SELECT id, turn_run_id, conversation_id, file_path, prompt,
		       model, size, quality, source_paper_ids, source_figure_ids,
		       cost_estimate_usd, created_at
		FROM ai_generated_images
		WHERE id = ?
	`, id).Scan(
		&img.ID, &img.TurnRunID, &img.ConversationID, &img.FilePath, &img.Prompt,
		&img.Model, &img.Size, &img.Quality, &paperIDs, &figureIDs,
		&img.CostEstimateUSD, &img.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return AIGeneratedImage{}, ErrAIGeneratedImageNotFound
	}
	if err != nil {
		return AIGeneratedImage{}, err
	}
	if err := json.Unmarshal([]byte(paperIDs), &img.SourcePaperIDs); err != nil {
		return AIGeneratedImage{}, err
	}
	if err := json.Unmarshal([]byte(figureIDs), &img.SourceFigureIDs); err != nil {
		return AIGeneratedImage{}, err
	}
	return img, nil
}

func (r *AIGeneratedImageRepository) ListByConversation(conversationID int64) ([]AIGeneratedImage, error) {
	rows, err := r.db.Query(`
		SELECT id, turn_run_id, conversation_id, file_path, prompt,
		       model, size, quality, source_paper_ids, source_figure_ids,
		       cost_estimate_usd, created_at
		FROM ai_generated_images
		WHERE conversation_id = ?
		ORDER BY id ASC
	`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AIGeneratedImage, 0)
	for rows.Next() {
		var img AIGeneratedImage
		var paperIDs, figureIDs string
		if err := rows.Scan(
			&img.ID, &img.TurnRunID, &img.ConversationID, &img.FilePath, &img.Prompt,
			&img.Model, &img.Size, &img.Quality, &paperIDs, &figureIDs,
			&img.CostEstimateUSD, &img.CreatedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(paperIDs), &img.SourcePaperIDs)
		_ = json.Unmarshal([]byte(figureIDs), &img.SourceFigureIDs)
		out = append(out, img)
	}
	return out, rows.Err()
}

func nonNilInt64Slice(in []int64) []int64 {
	if in == nil {
		return []int64{}
	}
	return in
}

func nullFloatToInterface(v sql.NullFloat64) interface{} {
	if !v.Valid {
		return nil
	}
	return v.Float64
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/repository/ -run TestAIGeneratedImage -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/repository/ai_generated_image_repo.go internal/repository/ai_generated_image_repo_test.go
git commit -m "repo: add AIGeneratedImageRepository CRUD with cascade-delete tests"
```

---

## Task 3: Settings — `AIImageGenSettings` block

**Files:**
- Modify: `internal/model/ai.go` (add struct + embed in `AISettings`)
- Modify: `internal/service/ai/settings.go` (normalize)
- Test: covered by Task 3.5 settings test

- [ ] **Step 1: Add the struct in `internal/model/ai.go`**

After `AITranslationConfig` (around line 51), add:

```go
type AIImageGenSettings struct {
	Enabled bool   `json:"enabled"`
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	Size    string `json:"size"`
	Quality string `json:"quality"`
}
```

In `AISettings` (around line 107) add a new field at the bottom of the struct:

```go
	ImageGen AIImageGenSettings `json:"image_gen"`
```

- [ ] **Step 2: Add defaults in `internal/model/ai.go`**

Find `DefaultAISettings()` (search for "DefaultAISettings"). Inside the returned struct literal, add `ImageGen` with these values:

```go
		ImageGen: AIImageGenSettings{
			Enabled: false,
			APIKey:  "",
			BaseURL: "https://api.openai.com",
			Model:   "gpt-image-2",
			Size:    "1024x1024",
			Quality: "high",
		},
```

- [ ] **Step 3: Add normalization in `internal/service/ai/settings.go`**

In `NormalizeSettings`, after the `Translation.TargetLanguage` normalization block (around line 158), add:

```go
	settings.ImageGen = normalizeImageGenSettings(settings.ImageGen, defaults.ImageGen)
```

Then add a new helper at the bottom of the file (after `normalizeReasoningEffort`):

```go
func normalizeImageGenSettings(input model.AIImageGenSettings, defaults model.AIImageGenSettings) model.AIImageGenSettings {
	out := input
	out.APIKey = strings.TrimSpace(out.APIKey)
	out.BaseURL = strings.TrimRight(strings.TrimSpace(out.BaseURL), "/")
	out.Model = strings.TrimSpace(out.Model)
	out.Size = strings.TrimSpace(out.Size)
	out.Quality = strings.ToLower(strings.TrimSpace(out.Quality))

	if out.BaseURL == "" {
		out.BaseURL = defaults.BaseURL
	}
	if out.Model == "" {
		out.Model = defaults.Model
	}
	switch out.Size {
	case "1024x1024", "1024x1536", "1536x1024":
		// ok
	default:
		out.Size = defaults.Size
	}
	switch out.Quality {
	case "low", "medium", "high":
		// ok
	default:
		out.Quality = defaults.Quality
	}
	return out
}
```

- [ ] **Step 4: Build**

Run: `go build ./...`
Expected: success.

- [ ] **Step 5: Add a normalization test**

Append to `internal/service/ai/settings.go`'s sibling test file (find an existing `*_test.go` in the `ai` package — currently the package may not have one; if so create `internal/service/ai/settings_test.go`):

```go
package ai

import (
	"testing"

	"github.com/xuzhougeng/citebox/internal/model"
)

func TestNormalizeImageGenSettings_Defaults(t *testing.T) {
	defaults := model.DefaultAISettings().ImageGen
	got := normalizeImageGenSettings(model.AIImageGenSettings{Enabled: true}, defaults)

	if got.BaseURL != defaults.BaseURL {
		t.Errorf("BaseURL: got %q want %q", got.BaseURL, defaults.BaseURL)
	}
	if got.Model != "gpt-image-2" {
		t.Errorf("Model: got %q want gpt-image-2", got.Model)
	}
	if got.Size != "1024x1024" {
		t.Errorf("Size: got %q want 1024x1024", got.Size)
	}
	if got.Quality != "high" {
		t.Errorf("Quality: got %q want high", got.Quality)
	}
}

func TestNormalizeImageGenSettings_RejectsInvalidEnums(t *testing.T) {
	defaults := model.DefaultAISettings().ImageGen
	got := normalizeImageGenSettings(model.AIImageGenSettings{
		Enabled: true,
		Size:    "9999x9999",
		Quality: "ULTRA",
	}, defaults)

	if got.Size != defaults.Size {
		t.Errorf("Size should fall back: got %q", got.Size)
	}
	if got.Quality != defaults.Quality {
		t.Errorf("Quality should fall back: got %q", got.Quality)
	}
}

func TestNormalizeImageGenSettings_TrimsAPIKey(t *testing.T) {
	defaults := model.DefaultAISettings().ImageGen
	got := normalizeImageGenSettings(model.AIImageGenSettings{
		Enabled: true,
		APIKey:  "  sk-test  ",
		BaseURL: "https://api.example.com/",
	}, defaults)

	if got.APIKey != "sk-test" {
		t.Errorf("APIKey should be trimmed: got %q", got.APIKey)
	}
	if got.BaseURL != "https://api.example.com" {
		t.Errorf("BaseURL trailing slash should be trimmed: got %q", got.BaseURL)
	}
}
```

- [ ] **Step 6: Run tests**

Run: `go test ./internal/service/ai/ -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/model/ai.go internal/service/ai/settings.go internal/service/ai/settings_test.go
git commit -m "settings: add AIImageGenSettings block with defaults and enum validation"
```

---

## Task 4: Disk-storage helper for generated images

**Files:**
- Modify: `internal/config/config.go` (add `AIGeneratedDir()` helper)
- Create: `internal/service/ai_image_gen/storage.go`
- Create: `internal/service/ai_image_gen/storage_test.go`

- [ ] **Step 1: Add the dir helper**

In `internal/config/config.go`, after `FiguresDir()` (around line 137), add:

```go
func (c *Config) AIGeneratedDir() string {
	return filepath.Join(c.StorageDir, "ai_generated")
}
```

- [ ] **Step 2: Write the failing test**

Create `internal/service/ai_image_gen/storage_test.go`:

```go
package ai_image_gen

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveImage_WritesPNGToConversationDir(t *testing.T) {
	tmp := t.TempDir()
	store := NewStorage(tmp)

	pngBytes := []byte("\x89PNG\r\n\x1a\nfake")
	rel, err := store.Save(42, pngBytes)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if filepath.Dir(rel) == "." {
		t.Fatalf("rel path should be nested: %q", rel)
	}

	abs := filepath.Join(tmp, rel)
	got, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != string(pngBytes) {
		t.Fatalf("content mismatch")
	}

	parent := filepath.Dir(abs)
	if filepath.Base(parent) != "42" {
		t.Fatalf("expected conversation_id dir '42', got %q", parent)
	}
}

func TestSaveImage_RejectsEmptyBytes(t *testing.T) {
	tmp := t.TempDir()
	store := NewStorage(tmp)
	if _, err := store.Save(1, nil); err == nil {
		t.Fatalf("expected error for empty bytes")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/service/ai_image_gen/ -v`
Expected: build failure (package does not exist).

- [ ] **Step 4: Implement Storage**

Create `internal/service/ai_image_gen/storage.go`:

```go
package ai_image_gen

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Storage writes generated PNG bytes to disk, namespaced by conversation_id.
// Returned paths are relative to the rootDir given at construction so callers
// can store them verbatim and rebuild absolute paths via filepath.Join.
type Storage struct {
	rootDir string
}

func NewStorage(rootDir string) *Storage {
	return &Storage{rootDir: rootDir}
}

func (s *Storage) Save(conversationID int64, pngBytes []byte) (string, error) {
	if len(pngBytes) == 0 {
		return "", errors.New("image bytes are empty")
	}
	dir := filepath.Join(s.rootDir, fmt.Sprintf("%d", conversationID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name, err := randomULID()
	if err != nil {
		return "", err
	}
	abs := filepath.Join(dir, name+".png")
	if err := os.WriteFile(abs, pngBytes, 0o644); err != nil {
		return "", err
	}
	rel, err := filepath.Rel(s.rootDir, abs)
	if err != nil {
		return "", err
	}
	return rel, nil
}

// randomULID returns a 16-byte hex token; we don't need true ULID monotonicity.
func randomULID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/service/ai_image_gen/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/service/ai_image_gen/storage.go internal/service/ai_image_gen/storage_test.go
git commit -m "ai_image_gen: add Storage helper writing PNGs to data/ai_generated/<conv>/"
```

---

## Task 5: Image API HTTP client

**Files:**
- Create: `internal/service/ai_image_gen/client.go`
- Create: `internal/service/ai_image_gen/client_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/service/ai_image_gen/client_test.go`:

```go
package ai_image_gen

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/model"
)

func TestClient_GenerateImage_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Errorf("path: %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Errorf("auth: %q", got)
		}
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "gpt-image-2" {
			t.Errorf("model: %v", body["model"])
		}
		if body["size"] != "1024x1024" || body["quality"] != "high" || body["n"].(float64) != 1 {
			t.Errorf("payload: %v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString([]byte("PNGDATA")) + `"}]}`))
	}))
	defer srv.Close()

	c := NewClient(http.DefaultClient)
	got, err := c.Generate(context.Background(), model.AIImageGenSettings{
		APIKey: "sk-test", BaseURL: srv.URL, Model: "gpt-image-2",
		Size: "1024x1024", Quality: "high",
	}, "draw something")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if string(got) != "PNGDATA" {
		t.Fatalf("png bytes mismatch: %q", got)
	}
}

func TestClient_GenerateImage_PropagatesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"prompt rejected"}}`))
	}))
	defer srv.Close()

	c := NewClient(http.DefaultClient)
	_, err := c.Generate(context.Background(), model.AIImageGenSettings{
		APIKey: "sk-test", BaseURL: srv.URL, Model: "gpt-image-2",
		Size: "1024x1024", Quality: "high",
	}, "x")
	if err == nil || !strings.Contains(err.Error(), "prompt rejected") {
		t.Fatalf("expected prompt-rejected error, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/ai_image_gen/ -run TestClient -v`
Expected: build failure (`NewClient` undefined).

- [ ] **Step 3: Implement client**

Create `internal/service/ai_image_gen/client.go`:

```go
package ai_image_gen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

type APIClient interface {
	Generate(ctx context.Context, settings model.AIImageGenSettings, prompt string) ([]byte, error)
}

type Client struct {
	http *http.Client
}

func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{http: httpClient}
}

func (c *Client) Generate(ctx context.Context, settings model.AIImageGenSettings, prompt string) ([]byte, error) {
	if strings.TrimSpace(settings.APIKey) == "" {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "图像生成 API key 未配置")
	}
	payload := map[string]interface{}{
		"model":           settings.Model,
		"prompt":          prompt,
		"size":            settings.Size,
		"quality":         settings.Quality,
		"n":               1,
		"response_format": "b64_json",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "序列化图像生成请求失败", err)
	}
	endpoint := strings.TrimRight(settings.BaseURL, "/") + "/v1/images/generations"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "构造图像生成请求失败", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+settings.APIKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeUnavailable, "调用图像生成接口失败", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeUnavailable, "读取图像生成响应失败", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, apperr.New(apperr.CodeUnavailable, fmt.Sprintf("图像生成接口返回 %d: %s", resp.StatusCode, extractAPIErrorMessage(respBody)))
	}

	var parsed struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, apperr.Wrap(apperr.CodeUnavailable, "解析图像生成响应失败", err)
	}
	if len(parsed.Data) == 0 || parsed.Data[0].B64JSON == "" {
		return nil, apperr.New(apperr.CodeUnavailable, "图像生成响应未包含 b64_json")
	}
	pngBytes, err := base64.StdEncoding.DecodeString(parsed.Data[0].B64JSON)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeUnavailable, "图像 base64 解码失败", err)
	}
	return pngBytes, nil
}

func extractAPIErrorMessage(body []byte) string {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return "空响应"
	}
	var nested struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &nested); err == nil && strings.TrimSpace(nested.Error.Message) != "" {
		return nested.Error.Message
	}
	return string(body)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/service/ai_image_gen/ -run TestClient -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/ai_image_gen/client.go internal/service/ai_image_gen/client_test.go
git commit -m "ai_image_gen: add OpenAI-compatible images.generations HTTP client"
```

---

## Task 6: Vision-stage prompt builder

**Files:**
- Create: `internal/service/ai_image_gen/prompt.go`
- Create: `internal/service/ai_image_gen/prompt_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/service/ai_image_gen/prompt_test.go`:

```go
package ai_image_gen

import (
	"strings"
	"testing"
)

func TestBuildVisionUserPrompt_IncludesPaperContext(t *testing.T) {
	got := BuildVisionUserPrompt(VisionInputContext{
		UserText: "give me a graphical abstract",
		Papers: []PaperContext{
			{ID: 1, Title: "CRISPR demystified", Abstract: "We present...", FullTextSnippet: "Section 1 ..."},
		},
	})
	for _, want := range []string{"CRISPR demystified", "We present...", "give me a graphical abstract"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestBuildVisionUserPrompt_AnnotatesIsolatedFigures(t *testing.T) {
	got := BuildVisionUserPrompt(VisionInputContext{
		UserText: "synthesize these two figures",
		IsolatedFigures: []FigureContext{
			{ID: 11, PaperTitle: "P1", PageNumber: 3, FigureIndex: 1, Caption: "Workflow"},
			{ID: 12, PaperTitle: "P1", PageNumber: 3, FigureIndex: 2, Caption: "Results"},
		},
	})
	for _, want := range []string{"Workflow", "Results", "第 3 页图 1", "第 3 页图 2"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestVisionSystemPrompt_MentionsGraphicalAbstract(t *testing.T) {
	if !strings.Contains(VisionSystemPrompt(), "graphical abstract") {
		t.Error("VisionSystemPrompt must mention graphical abstract")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/ai_image_gen/ -run TestBuildVisionUserPrompt -v`
Expected: FAIL (undefined symbols).

- [ ] **Step 3: Implement prompt helpers**

Create `internal/service/ai_image_gen/prompt.go`:

```go
package ai_image_gen

import (
	"fmt"
	"strings"
)

type PaperContext struct {
	ID              int64
	Title           string
	Abstract        string
	FullTextSnippet string
	Figures         []FigureContext
}

type FigureContext struct {
	ID          int64
	PaperTitle  string
	PageNumber  int
	FigureIndex int
	Caption     string
}

type VisionInputContext struct {
	UserText        string
	Papers          []PaperContext
	IsolatedFigures []FigureContext
}

const visionSystemPrompt = `You are an experienced graphical-abstract designer for scientific papers. Your job: read the provided paper text and figures, then write a concise, vivid, well-structured English prompt (300–600 tokens) that an image generation model can render into ONE 1024x1024 graphical abstract.

Rules:
- Output ONLY the image prompt text. No prose explanation, no numbered list, no headers.
- Use clear visual nouns: "left panel", "central diagram", "icons of...", "arrows showing...".
- Keep typography minimal — at most 2–3 short labels in English.
- Convey the paper's core finding, not every figure.
- Avoid copyrighted character references and photorealistic faces.`

func VisionSystemPrompt() string { return visionSystemPrompt }

func BuildVisionUserPrompt(ctx VisionInputContext) string {
	var b strings.Builder
	b.WriteString("用户的请求：\n")
	b.WriteString(strings.TrimSpace(ctx.UserText))
	b.WriteString("\n\n")

	for _, p := range ctx.Papers {
		b.WriteString(fmt.Sprintf("=== 文献 (id=%d) ===\n", p.ID))
		b.WriteString("标题：")
		b.WriteString(strings.TrimSpace(p.Title))
		b.WriteString("\n")
		if abstract := strings.TrimSpace(p.Abstract); abstract != "" {
			b.WriteString("摘要：")
			b.WriteString(abstract)
			b.WriteString("\n")
		}
		if snippet := strings.TrimSpace(p.FullTextSnippet); snippet != "" {
			b.WriteString("正文片段：")
			b.WriteString(snippet)
			b.WriteString("\n")
		}
		for _, fig := range p.Figures {
			b.WriteString(figureLine(fig))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if len(ctx.IsolatedFigures) > 0 {
		b.WriteString("=== 用户单独引用的图 ===\n")
		for _, fig := range ctx.IsolatedFigures {
			b.WriteString(figureLine(fig))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("请基于上述材料，输出一段图像生成 prompt。")
	return b.String()
}

func figureLine(fig FigureContext) string {
	caption := strings.TrimSpace(fig.Caption)
	if caption == "" {
		caption = "（无描述）"
	}
	return fmt.Sprintf("- 第 %d 页图 %d（id=%d，来自《%s》）：%s",
		fig.PageNumber, fig.FigureIndex, fig.ID, strings.TrimSpace(fig.PaperTitle), caption)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/service/ai_image_gen/ -run "TestBuildVisionUserPrompt|TestVisionSystemPrompt" -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/ai_image_gen/prompt.go internal/service/ai_image_gen/prompt_test.go
git commit -m "ai_image_gen: add vision-stage system + user prompt builders"
```

---

## Task 7: Service skeleton — types and `Generate` orchestration

**Files:**
- Create: `internal/service/ai_image_gen/types.go`
- Create: `internal/service/ai_image_gen/service.go`
- Create: `internal/service/ai_image_gen/service_test.go`

- [ ] **Step 1: Add the types**

Create `internal/service/ai_image_gen/types.go`:

```go
package ai_image_gen

import (
	"context"

	"github.com/xuzhougeng/citebox/internal/model"
)

type GenerateInput struct {
	ConversationID int64
	TurnRunID      int64
	PaperIDs       []int64
	FigureIDs      []int64
	UserText       string
	OnEvent        func(StreamEvent) error
}

type StreamEvent struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// PromptDraftedEvent is emitted after the vision stage produces an image prompt.
type PromptDraftedEvent struct {
	TurnRunID int64  `json:"turn_run_id"`
	Prompt    string `json:"prompt"`
}

// GeneratingEvent is emitted just before the image API call.
type GeneratingEvent struct {
	TurnRunID       int64   `json:"turn_run_id"`
	Model           string  `json:"model"`
	Size            string  `json:"size"`
	Quality         string  `json:"quality"`
	CostEstimateUSD float64 `json:"cost_estimate_usd"`
}

// GeneratedEvent is emitted on success with the persisted card payload.
type GeneratedEvent struct {
	TurnRunID int64                  `json:"turn_run_id"`
	Card      map[string]interface{} `json:"card"`
}

// FailedEvent is emitted on failure at any stage.
type FailedEvent struct {
	TurnRunID int64  `json:"turn_run_id"`
	Stage     string `json:"stage"` // "vision" | "image_api" | "save"
	Reason    string `json:"reason"`
}

// VisionCaller wraps the existing service.AIService.CallProviderVision-style
// helper. It returns the model's text output when given a system prompt + user
// prompt + base64 images. The conversation stack already has compatible
// helpers; we adapt via this minimal interface.
type VisionCaller interface {
	CallVision(ctx context.Context, settings model.AISettings, systemPrompt, userPrompt string,
		images []model.AIImageInput) (string, error)
}

type Generator interface {
	Generate(ctx context.Context, in GenerateInput) error
}
```

- [ ] **Step 2: Write a service-level test that exercises happy path + each failure stage**

Create `internal/service/ai_image_gen/service_test.go`:

```go
package ai_image_gen

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

type fakeVision struct {
	prompt string
	err    error
}

func (f *fakeVision) CallVision(ctx context.Context, _ model.AISettings, sys, user string,
	_ []model.AIImageInput) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.prompt, nil
}

type fakeAPI struct {
	bytes []byte
	err   error
}

func (f *fakeAPI) Generate(_ context.Context, _ model.AIImageGenSettings, _ string) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.bytes, nil
}

type fakeInputs struct {
	papers          []PaperContext
	figures         []FigureContext
	images          []model.AIImageInput
}

func (f *fakeInputs) Load(ctx context.Context, paperIDs, figureIDs []int64) (VisionInputContext, []model.AIImageInput, error) {
	return VisionInputContext{Papers: f.papers, IsolatedFigures: f.figures}, f.images, nil
}

type captureRepo struct {
	mu     sync.Mutex
	rows   []repository.AIGeneratedImage
	cards  []repository.AIResultCard
}

func (c *captureRepo) InsertImage(img repository.AIGeneratedImage) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	img.ID = int64(len(c.rows) + 1)
	c.rows = append(c.rows, img)
	return img.ID, nil
}

func (c *captureRepo) AddResultCard(card repository.AIResultCard) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	card.ID = int64(len(c.cards) + 1)
	c.cards = append(c.cards, card)
	return card.ID, nil
}

func newServiceWithFakes(t *testing.T, vision VisionCaller, api APIClient) (*Service, *captureRepo, string) {
	t.Helper()
	tmp := t.TempDir()
	repo := &captureRepo{}
	settings := func() (model.AIImageGenSettings, model.AISettings, error) {
		return model.DefaultAISettings().ImageGen, model.DefaultAISettings(), nil
	}
	settingsEnabled := func() (model.AIImageGenSettings, model.AISettings, error) {
		gs := model.DefaultAISettings()
		gs.ImageGen.Enabled = true
		gs.ImageGen.APIKey = "sk-test"
		return gs.ImageGen, gs, nil
	}
	_ = settings
	svc := NewService(ServiceDeps{
		Repo:        repo,
		Storage:     NewStorage(tmp),
		Vision:      vision,
		API:         api,
		LoadInputs:  (&fakeInputs{papers: []PaperContext{{ID: 1, Title: "P", Abstract: "A"}}}).Load,
		GetSettings: settingsEnabled,
	})
	return svc, repo, tmp
}

func TestService_HappyPath_EmitsAllEventsAndPersists(t *testing.T) {
	vision := &fakeVision{prompt: "draw an abstract showing X"}
	api := &fakeAPI{bytes: []byte("\x89PNGfake")}
	svc, repo, tmp := newServiceWithFakes(t, vision, api)

	var events []string
	err := svc.Generate(context.Background(), GenerateInput{
		ConversationID: 7, TurnRunID: 99,
		PaperIDs: []int64{1}, UserText: "summarize",
		OnEvent: func(e StreamEvent) error { events = append(events, e.Type); return nil },
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	wantSeq := []string{"image_prompt_drafting", "image_prompt_drafted", "image_generating", "image_generated"}
	if strings.Join(events, ",") != strings.Join(wantSeq, ",") {
		t.Fatalf("events: got %v want %v", events, wantSeq)
	}
	if len(repo.rows) != 1 || repo.rows[0].Prompt != "draw an abstract showing X" {
		t.Fatalf("row not persisted: %+v", repo.rows)
	}
	abs := filepath.Join(tmp, repo.rows[0].FilePath)
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("file not on disk: %v", err)
	}
	if len(repo.cards) != 1 || repo.cards[0].CardType != "generated_image" {
		t.Fatalf("card not persisted: %+v", repo.cards)
	}
}

func TestService_VisionFailure_DoesNotCallImageAPI(t *testing.T) {
	vision := &fakeVision{err: errors.New("vision boom")}
	api := &fakeAPI{} // would panic if called because no bytes/err set; we just want to confirm it isn't called
	svc, repo, _ := newServiceWithFakes(t, vision, api)

	var failedStage string
	err := svc.Generate(context.Background(), GenerateInput{
		ConversationID: 7, TurnRunID: 99, PaperIDs: []int64{1}, UserText: "x",
		OnEvent: func(e StreamEvent) error {
			if e.Type == "image_failed" {
				failedStage = e.Data.(FailedEvent).Stage
			}
			return nil
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if failedStage != "vision" {
		t.Fatalf("stage = %q want vision", failedStage)
	}
	if len(repo.rows) != 0 {
		t.Fatalf("expected no DB row on vision fail")
	}
}

func TestService_ImageAPIFailure_NoFileNoRow(t *testing.T) {
	vision := &fakeVision{prompt: "x"}
	api := &fakeAPI{err: errors.New("policy reject")}
	svc, repo, tmp := newServiceWithFakes(t, vision, api)

	var failedStage string
	_ = svc.Generate(context.Background(), GenerateInput{
		ConversationID: 7, TurnRunID: 99, PaperIDs: []int64{1}, UserText: "x",
		OnEvent: func(e StreamEvent) error {
			if e.Type == "image_failed" {
				failedStage = e.Data.(FailedEvent).Stage
			}
			return nil
		},
	})
	if failedStage != "image_api" {
		t.Fatalf("stage = %q want image_api", failedStage)
	}
	if len(repo.rows) != 0 {
		t.Fatal("no DB row expected")
	}
	entries, _ := os.ReadDir(tmp)
	if len(entries) != 0 {
		t.Fatalf("no files expected, got %d", len(entries))
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/service/ai_image_gen/ -run TestService -v`
Expected: build failure (`Service`, `NewService`, `ServiceDeps` undefined).

- [ ] **Step 4: Implement the Service**

Create `internal/service/ai_image_gen/service.go`:

```go
package ai_image_gen

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

// imageRepoWriter is the subset of AIGeneratedImageRepository used by Service.
type imageRepoWriter interface {
	InsertImage(repository.AIGeneratedImage) (int64, error)
	AddResultCard(repository.AIResultCard) (int64, error)
}

// LoadInputsFn loads paper + figure context and the base64 vision images for
// the requested IDs. The caller in production wires this to the existing
// service.AIService.loadFigureInputs flow. Tests inject a fake.
type LoadInputsFn func(ctx context.Context, paperIDs, figureIDs []int64) (VisionInputContext, []model.AIImageInput, error)

// SettingsFn returns the image-gen-specific settings + the broader AI settings
// (so the vision call can pick its model). Returns an error when settings
// cannot be loaded.
type SettingsFn func() (model.AIImageGenSettings, model.AISettings, error)

type ServiceDeps struct {
	Repo        imageRepoWriter
	Storage     *Storage
	Vision      VisionCaller
	API         APIClient
	LoadInputs  LoadInputsFn
	GetSettings SettingsFn
	// VisionTimeout / ImageTimeout cap each stage. Defaults: vision 60s, image 120s.
	VisionTimeout time.Duration
	ImageTimeout  time.Duration
}

type Service struct {
	deps ServiceDeps
}

func NewService(deps ServiceDeps) *Service {
	if deps.VisionTimeout == 0 {
		deps.VisionTimeout = 60 * time.Second
	}
	if deps.ImageTimeout == 0 {
		deps.ImageTimeout = 120 * time.Second
	}
	return &Service{deps: deps}
}

func (s *Service) Generate(ctx context.Context, in GenerateInput) error {
	emit := func(t string, data any) {
		if in.OnEvent == nil {
			return
		}
		_ = in.OnEvent(StreamEvent{Type: t, Data: data})
	}

	imgSettings, aiSettings, err := s.deps.GetSettings()
	if err != nil {
		return err
	}
	if !imgSettings.Enabled {
		return apperr.New(apperr.CodeFailedPrecondition, "请先在 AI 设置中启用图像生成")
	}
	if len(in.PaperIDs) == 0 && len(in.FigureIDs) == 0 {
		return apperr.New(apperr.CodeInvalidArgument, "请引用至少一篇文献或一张图")
	}

	emit("image_prompt_drafting", PromptDraftedEvent{TurnRunID: in.TurnRunID})

	visionCtx, cancelVision := context.WithTimeout(ctx, s.deps.VisionTimeout)
	defer cancelVision()

	visionInput, images, err := s.deps.LoadInputs(visionCtx, in.PaperIDs, in.FigureIDs)
	if err != nil {
		emit("image_failed", FailedEvent{TurnRunID: in.TurnRunID, Stage: "vision", Reason: err.Error()})
		return err
	}
	visionInput.UserText = in.UserText

	imagePrompt, err := s.deps.Vision.CallVision(visionCtx, aiSettings,
		VisionSystemPrompt(), BuildVisionUserPrompt(visionInput), images)
	if err != nil {
		emit("image_failed", FailedEvent{TurnRunID: in.TurnRunID, Stage: "vision", Reason: err.Error()})
		return err
	}
	emit("image_prompt_drafted", PromptDraftedEvent{TurnRunID: in.TurnRunID, Prompt: imagePrompt})

	cost := costEstimate(imgSettings.Model, imgSettings.Size, imgSettings.Quality)
	emit("image_generating", GeneratingEvent{
		TurnRunID: in.TurnRunID, Model: imgSettings.Model,
		Size: imgSettings.Size, Quality: imgSettings.Quality, CostEstimateUSD: cost,
	})

	// Detach from request context so a client disconnect can't abort a paid
	// API call mid-flight (spec §4 / §6).
	imageCtx, cancelImage := context.WithTimeout(context.Background(), s.deps.ImageTimeout)
	defer cancelImage()

	pngBytes, err := s.deps.API.Generate(imageCtx, imgSettings, imagePrompt)
	if err != nil {
		emit("image_failed", FailedEvent{TurnRunID: in.TurnRunID, Stage: "image_api", Reason: err.Error()})
		return err
	}

	relPath, err := s.deps.Storage.Save(in.ConversationID, pngBytes)
	if err != nil {
		emit("image_failed", FailedEvent{TurnRunID: in.TurnRunID, Stage: "save", Reason: err.Error()})
		return err
	}

	row := repository.AIGeneratedImage{
		TurnRunID: in.TurnRunID, ConversationID: in.ConversationID,
		FilePath: relPath, Prompt: imagePrompt,
		Model: imgSettings.Model, Size: imgSettings.Size, Quality: imgSettings.Quality,
		SourcePaperIDs: in.PaperIDs, SourceFigureIDs: in.FigureIDs,
		CostEstimateUSD: nullFloat(cost),
	}
	imageID, err := s.deps.Repo.InsertImage(row)
	if err != nil {
		emit("image_failed", FailedEvent{TurnRunID: in.TurnRunID, Stage: "save", Reason: err.Error()})
		return err
	}

	cardPayload := map[string]interface{}{
		"image_id":          imageID,
		"file_url":          fmt.Sprintf("/api/ai-generated-images/%d/file", imageID),
		"prompt":            imagePrompt,
		"model":             imgSettings.Model,
		"size":              imgSettings.Size,
		"quality":           imgSettings.Quality,
		"source_paper_ids":  in.PaperIDs,
		"source_figure_ids": in.FigureIDs,
		"cost_estimate_usd": cost,
	}
	cardJSON, err := jsonMarshal(cardPayload)
	if err != nil {
		return err
	}
	if _, err := s.deps.Repo.AddResultCard(repository.AIResultCard{
		TurnRunID: in.TurnRunID, CardType: "generated_image", SortOrder: 0, PayloadJSON: cardJSON,
	}); err != nil {
		emit("image_failed", FailedEvent{TurnRunID: in.TurnRunID, Stage: "save", Reason: err.Error()})
		return err
	}
	emit("image_generated", GeneratedEvent{TurnRunID: in.TurnRunID, Card: cardPayload})
	return nil
}

func costEstimate(modelName, size, quality string) float64 {
	switch quality {
	case "low":
		return 0.04
	case "medium":
		return 0.07
	case "high":
		return 0.19
	}
	return 0.0
}

func nullFloat(v float64) sql.NullFloat64 {
	return sql.NullFloat64{Float64: v, Valid: true}
}

func jsonMarshal(v interface{}) (string, error) {
	out, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
```

> Update the `repository.AIGeneratedImage.CostEstimateUSD` field to wrap via `nullFloat(cost)` in the `row` literal (already shown in the snippet above).

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/service/ai_image_gen/ -v`
Expected: PASS for all `TestService_*`.

- [ ] **Step 6: Commit**

```bash
git add internal/service/ai_image_gen/types.go internal/service/ai_image_gen/service.go internal/service/ai_image_gen/service_test.go
git commit -m "ai_image_gen: add Service.Generate orchestrating vision -> image API -> persist"
```

---

## Task 8: Wire vision adapter + load-inputs adapter on top of `service.AIService`

**Files:**
- Modify: `internal/service/ai_service_image.go` (add adapters that satisfy `ai_image_gen.VisionCaller` and `ai_image_gen.LoadInputsFn`)
- Modify: `internal/app/<wherever AIService is constructed>` (provide adapters when constructing `ai_image_gen.Service`)

The existing `loadFigureInputs(paper, figures, action)` already does base64 + budget compression. We expose a thin wrapper that takes raw IDs and returns the vision input shape `ai_image_gen` expects.

- [ ] **Step 1: Add the load-inputs + vision adapter on AIService**

In `internal/service/ai_service_image.go` (or a new file `ai_service_image_gen.go` next to it), add:

```go
package service

import (
	"context"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/service/ai_image_gen"
)

// LoadImageGenInputs implements ai_image_gen.LoadInputsFn.
//
// PaperIDs: full paper text + every figure (subject to existing AI image budget).
// FigureIDs: figures alone, deduped against any paper's figures already pulled
// in via PaperIDs.
func (s *AIService) LoadImageGenInputs(ctx context.Context, paperIDs, figureIDs []int64) (
	ai_image_gen.VisionInputContext, []model.AIImageInput, error) {

	out := ai_image_gen.VisionInputContext{}
	imageInputs := make([]aiImageInput, 0)

	includedFigureIDs := make(map[int64]struct{})
	for _, pid := range paperIDs {
		paper, err := s.papers.GetPaper(pid)
		if err != nil {
			return ai_image_gen.VisionInputContext{}, nil, apperr.Wrap(apperr.CodeInvalidArgument, "无法读取文献", err)
		}
		figures, err := s.papers.ListFigures(pid)
		if err != nil {
			return ai_image_gen.VisionInputContext{}, nil, err
		}
		images, _, err := s.loadFigureInputs(paper, figures, model.AIActionFigureInterpretation)
		if err != nil {
			return ai_image_gen.VisionInputContext{}, nil, err
		}
		imageInputs = append(imageInputs, images...)

		pCtx := ai_image_gen.PaperContext{
			ID:              paper.ID,
			Title:           paper.Title,
			Abstract:        paper.AbstractText,
			FullTextSnippet: truncateRunes(paper.PDFText, 4000),
		}
		for _, fig := range figures {
			pCtx.Figures = append(pCtx.Figures, ai_image_gen.FigureContext{
				ID: fig.ID, PaperTitle: paper.Title,
				PageNumber: fig.PageNumber, FigureIndex: fig.FigureIndex, Caption: fig.Caption,
			})
			includedFigureIDs[fig.ID] = struct{}{}
		}
		out.Papers = append(out.Papers, pCtx)
	}

	for _, fid := range figureIDs {
		if _, dup := includedFigureIDs[fid]; dup {
			continue
		}
		fig, err := s.papers.GetFigure(fid)
		if err != nil {
			return ai_image_gen.VisionInputContext{}, nil, err
		}
		paper, err := s.papers.GetPaper(fig.PaperID)
		if err != nil {
			return ai_image_gen.VisionInputContext{}, nil, err
		}
		images, _, err := s.loadFigureInputs(paper, []model.Figure{fig}, model.AIActionFigureInterpretation)
		if err != nil {
			return ai_image_gen.VisionInputContext{}, nil, err
		}
		imageInputs = append(imageInputs, images...)
		out.IsolatedFigures = append(out.IsolatedFigures, ai_image_gen.FigureContext{
			ID: fig.ID, PaperTitle: paper.Title,
			PageNumber: fig.PageNumber, FigureIndex: fig.FigureIndex, Caption: fig.Caption,
		})
		includedFigureIDs[fid] = struct{}{}
	}

	return out, toModelImageInputs(imageInputs), nil
}

// CallVisionForImageGen satisfies ai_image_gen.VisionCaller. It picks the
// figure-scene model (most likely vision-capable) and runs a non-stream call.
func (s *AIService) CallVisionForImageGen(ctx context.Context, settings model.AISettings,
	systemPrompt, userPrompt string, images []model.AIImageInput) (string, error) {
	return s.CallProviderNonStream(ctx, settings, systemPrompt, userPrompt, images)
}

func toModelImageInputs(in []aiImageInput) []model.AIImageInput {
	out := make([]model.AIImageInput, 0, len(in))
	for _, img := range in {
		out = append(out, model.AIImageInput{MIMEType: img.MIMEType, Data: img.Data})
	}
	return out
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
```

> **Note:** `s.papers.GetPaper`, `s.papers.GetFigure`, `s.papers.ListFigures` and `model.AIImageInput` may already exist in the codebase; if any of them have a slightly different name, mirror the names used in the surrounding code. `CallProviderNonStream` may also be named slightly differently — search for the existing non-stream provider call helper and use the matching name.

- [ ] **Step 2: Build to make sure adapters compile**

Run: `go build ./...`
Expected: success.

- [ ] **Step 3: Construct the image-gen service in the app wiring file**

Find the file that constructs `service.AIService` (likely under `internal/app/`). After it, add:

```go
imageGenService := ai_image_gen.NewService(ai_image_gen.ServiceDeps{
	Repo:    aiGeneratedImageRepoAdapter{repo: aiGeneratedImageRepo, conv: aiConversationRepo},
	Storage: ai_image_gen.NewStorage(cfg.AIGeneratedDir()),
	Vision:  visionAdapter{svc: aiService},
	API:     ai_image_gen.NewClient(http.DefaultClient),
	LoadInputs: aiService.LoadImageGenInputs,
	GetSettings: func() (model.AIImageGenSettings, model.AISettings, error) {
		s, err := aiService.GetSettings()
		if err != nil { return model.AIImageGenSettings{}, model.AISettings{}, err }
		return s.ImageGen, *s, nil
	},
})
```

Plus a tiny adapter in `internal/app/ai_image_gen_adapters.go`:

```go
package app

import (
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service"
)

type aiGeneratedImageRepoAdapter struct {
	repo *repository.AIGeneratedImageRepository
	conv *repository.AIConversationRepository
}

func (a aiGeneratedImageRepoAdapter) InsertImage(img repository.AIGeneratedImage) (int64, error) {
	return a.repo.Insert(img)
}
func (a aiGeneratedImageRepoAdapter) AddResultCard(card repository.AIResultCard) (int64, error) {
	return a.conv.AddResultCard(card)
}

type visionAdapter struct{ svc *service.AIService }

func (a visionAdapter) CallVision(ctx context.Context, settings model.AISettings,
	systemPrompt, userPrompt string, images []model.AIImageInput) (string, error) {
	return a.svc.CallVisionForImageGen(ctx, settings, systemPrompt, userPrompt, images)
}
```

- [ ] **Step 4: Build**

Run: `go build ./...`
Expected: success.

- [ ] **Step 5: Commit**

```bash
git add internal/service/ai_service_image_gen.go internal/app/
git commit -m "ai_image_gen: wire AIService adapters (loadInputs + vision) into app DI"
```

---

## Task 9: Conversation orchestration — route `image_generation` intent

**Files:**
- Modify: `internal/service/ai_conversation/service.go` (route `intent_hint == "image_generation"` to `ai_image_gen`)
- Modify: `internal/service/ai_conversation/types.go` (add `Generator` field to Service)
- Test: extend `internal/service/ai_conversation/service_test.go`

- [ ] **Step 1: Add `Generator` field on `ai_conversation.Service`**

In `internal/service/ai_conversation/service.go`, extend the `Service` struct:

```go
type Service struct {
	repo          *repository.AIConversationRepository
	papers        *repository.PaperRepository
	settings      AISettingsProvider
	caller        StreamCaller
	searcher      ExternalEvidenceSearcher
	orchestrator  orchestrator
	titleCaller   NonStreamCaller
	summaryCaller NonStreamCaller
	imageGen      ai_image_gen.Generator   // NEW
	logger        *slog.Logger
}
```

Add a `WithImageGen(g ai_image_gen.Generator) *Service` setter (chainable, optional dep — keeps backward compat with existing tests):

```go
func (s *Service) WithImageGen(g ai_image_gen.Generator) *Service {
	s.imageGen = g
	return s
}
```

Add the import: `"github.com/xuzhougeng/citebox/internal/service/ai_image_gen"`.

- [ ] **Step 2: Detect the image-gen intent and dispatch**

Inside `SendMessage`, after the `userMsgID, err := s.repo.AddMessage(...)` line (around line 268) and BEFORE the existing `settings, err := s.settings.GetSettings()` block, add:

```go
	if s.imageGen != nil && strings.EqualFold(strings.TrimSpace(in.IntentHint), "image_generation") {
		return s.dispatchImageGen(ctx, in, conv, userMsgID)
	}
```

Then add the dispatcher at the bottom of `service.go` (before the closing `package` boundary helpers):

```go
func (s *Service) dispatchImageGen(ctx context.Context, in SendMessageInput,
	conv repository.AIConversation, userMsgID int64) (SendMessageResult, error) {

	paperIDs, figureIDs := parseImageGenReferences(in)
	if len(paperIDs) == 0 && len(figureIDs) == 0 {
		return SendMessageResult{}, apperr.New(apperr.CodeInvalidArgument, "请引用至少一篇文献或一张图")
	}

	// Auto-pin any @paper references the user used so subsequent turns inherit them.
	for _, pid := range paperIDs {
		if err := s.PinPaper(in.ConversationID, pid); err != nil {
			return SendMessageResult{}, err
		}
	}

	turnID, err := s.repo.CreateTurnRun(repository.AITurnRun{
		ConversationID: in.ConversationID,
		UserMessageID:  userMsgID,
		Intent:         "image_generation",
		IntentHint:     in.IntentHint,
		Status:         "running",
	})
	if err != nil {
		return SendMessageResult{}, err
	}

	genErr := s.imageGen.Generate(ctx, ai_image_gen.GenerateInput{
		ConversationID: in.ConversationID,
		TurnRunID:      turnID,
		PaperIDs:       paperIDs,
		FigureIDs:      figureIDs,
		UserText:       in.Content,
		OnEvent: func(e ai_image_gen.StreamEvent) error {
			return s.emitStreamEvent(in, StreamEvent{Type: e.Type, Data: e.Data})
		},
	})

	finalStatus := "completed"
	assistantText := "已生成图像。"
	if genErr != nil {
		finalStatus = "failed"
		assistantText = fmt.Sprintf("图像生成失败：%s", genErr.Error())
	}

	asstID, err := s.repo.AddMessage(in.ConversationID, "assistant", assistantText, repository.AIMessageMeta{
		Mode: finalStatus,
	})
	if err != nil {
		return SendMessageResult{}, err
	}
	if err := s.repo.UpdateTurnRunFinal(turnID, asstID, finalStatus); err != nil {
		s.logger.Warn("ai_conversation: update image-gen turn run failed", "error", err)
	}
	_ = s.repo.TouchConversation(in.ConversationID)

	if genErr != nil {
		return SendMessageResult{}, genErr
	}
	return SendMessageResult{
		ConversationID:   in.ConversationID,
		UserMessage:      Message{ID: userMsgID, Role: "user", Content: in.Content},
		AssistantMessage: Message{ID: asstID, Role: "assistant", Content: assistantText, Mode: finalStatus},
	}, nil
}

// parseImageGenReferences extracts paper IDs (from in.PaperIDs / in.PaperID) and
// figure IDs (from in.Context, mention parsing). When the wire layer provides
// figure mentions via in.Context.FigureID or a future FigureIDs slice, both
// shapes are honoured.
func parseImageGenReferences(in SendMessageInput) ([]int64, []int64) {
	paperIDs := append([]int64(nil), in.PaperIDs...)
	if len(paperIDs) == 0 && in.PaperID > 0 {
		paperIDs = []int64{in.PaperID}
	}
	figureIDs := []int64(nil)
	if in.Context.FigureID > 0 {
		figureIDs = append(figureIDs, in.Context.FigureID)
	}
	if len(in.Context.FigureIDs) > 0 {
		figureIDs = append(figureIDs, in.Context.FigureIDs...)
	}
	return dedupeInt64(paperIDs), dedupeInt64(figureIDs)
}

func dedupeInt64(in []int64) []int64 {
	seen := make(map[int64]struct{}, len(in))
	out := in[:0]
	for _, id := range in {
		if id <= 0 {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
```

> **Note:** This task assumes `repository.AIConversationRepository` has `UpdateTurnRunFinal(turnID, asstID, status)` (compatible with the existing helper at line 390 of `ai_conversation_repo.go`). If the existing method has a different signature, mirror it.
>
> Also extend `ai_assistant.RequestContext` (or wherever `Context.FigureIDs` lives) to carry a `FigureIDs []int64` field if it doesn't already; this is the wire-channel for `@figure` mentions parsed by the frontend. If extending `RequestContext` requires a new field, add it as a JSON-tagged `figure_ids,omitempty` slice and document the addition.

- [ ] **Step 3: Add a routing test**

In `internal/service/ai_conversation/service_test.go`, add:

```go
func TestSendMessage_RoutesImageGenerationIntent(t *testing.T) {
	// Build a Service with a fake imageGen that records the call.
	called := struct {
		paperIDs  []int64
		figureIDs []int64
		userText  string
	}{}
	fake := &fakeImageGen{
		fn: func(ctx context.Context, in ai_image_gen.GenerateInput) error {
			called.paperIDs = in.PaperIDs
			called.figureIDs = in.FigureIDs
			called.userText = in.UserText
			_ = in.OnEvent(ai_image_gen.StreamEvent{Type: "image_generated"})
			return nil
		},
	}
	svc := newTestService(t).WithImageGen(fake)

	conv, _ := svc.CreateDraft()
	res, err := svc.SendMessage(context.Background(), SendMessageInput{
		ConversationID: conv,
		Content:        "@image-gen draw it",
		IntentHint:     "image_generation",
		PaperIDs:       []int64{42},
	}, func(string) error { return nil })
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if len(called.paperIDs) != 1 || called.paperIDs[0] != 42 {
		t.Fatalf("paperIDs: %v", called.paperIDs)
	}
	if res.AssistantMessage.Mode != "completed" {
		t.Errorf("mode: %q", res.AssistantMessage.Mode)
	}
}

type fakeImageGen struct {
	fn func(context.Context, ai_image_gen.GenerateInput) error
}

func (f *fakeImageGen) Generate(ctx context.Context, in ai_image_gen.GenerateInput) error {
	return f.fn(ctx, in)
}
```

(`newTestService` is the existing constructor pattern in `service_test.go` — match the signature of whatever helper the file already uses.)

- [ ] **Step 4: Run tests**

Run: `go test ./internal/service/ai_conversation/ -run TestSendMessage_RoutesImageGeneration -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/ai_conversation/service.go internal/service/ai_conversation/service_test.go
git commit -m "ai_conversation: route intent_hint=image_generation to ai_image_gen.Generator"
```

---

## Task 10: HTTP handler `GET /api/ai-generated-images/:id/file`

**Files:**
- Create: `internal/handler/ai_generated_image.go`
- Create: `internal/handler/ai_generated_image_test.go`
- Modify: route registration (the file where `AIHandler` routes are registered)

- [ ] **Step 1: Write the failing test**

Create `internal/handler/ai_generated_image_test.go`:

```go
package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/xuzhougeng/citebox/internal/repository"
)

func TestAIGeneratedImageHandler_ServesFile(t *testing.T) {
	tmp := t.TempDir()
	rel := "1/abc.png"
	abs := filepath.Join(tmp, rel)
	_ = os.MkdirAll(filepath.Dir(abs), 0o755)
	_ = os.WriteFile(abs, []byte("PNGDATA"), 0o644)

	repo := &fakeImageRepo{rows: map[int64]repository.AIGeneratedImage{
		7: {ID: 7, ConversationID: 1, FilePath: rel},
	}}
	h := NewAIGeneratedImageHandler(repo, tmp)

	req := httptest.NewRequest(http.MethodGet, "/api/ai-generated-images/7/file", nil)
	req = withRouteParam(req, "id", "7")
	rec := httptest.NewRecorder()
	h.GetFile(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d body = %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "PNGDATA" {
		t.Fatalf("body mismatch: %q", rec.Body.String())
	}
}

func TestAIGeneratedImageHandler_NotFound(t *testing.T) {
	tmp := t.TempDir()
	repo := &fakeImageRepo{rows: map[int64]repository.AIGeneratedImage{}}
	h := NewAIGeneratedImageHandler(repo, tmp)

	req := httptest.NewRequest(http.MethodGet, "/api/ai-generated-images/9999/file", nil)
	req = withRouteParam(req, "id", "9999")
	rec := httptest.NewRecorder()
	h.GetFile(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

type fakeImageRepo struct{ rows map[int64]repository.AIGeneratedImage }

func (f *fakeImageRepo) GetByID(id int64) (repository.AIGeneratedImage, error) {
	row, ok := f.rows[id]
	if !ok {
		return repository.AIGeneratedImage{}, repository.ErrAIGeneratedImageNotFound
	}
	return row, nil
}
```

> `withRouteParam` is the existing test helper for whatever router the project uses (e.g. `chi`/`gorilla`). Mirror the pattern used by `ai_test.go` in the same package — copy the helper signature exactly.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/handler/ -run TestAIGeneratedImageHandler -v`
Expected: build failure.

- [ ] **Step 3: Implement handler**

Create `internal/handler/ai_generated_image.go`:

```go
package handler

import (
	"errors"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/repository"
)

type aiGeneratedImageReader interface {
	GetByID(id int64) (repository.AIGeneratedImage, error)
}

type AIGeneratedImageHandler struct {
	repo    aiGeneratedImageReader
	rootDir string
}

func NewAIGeneratedImageHandler(repo aiGeneratedImageReader, rootDir string) *AIGeneratedImageHandler {
	return &AIGeneratedImageHandler{repo: repo, rootDir: rootDir}
}

func (h *AIGeneratedImageHandler) GetFile(w http.ResponseWriter, r *http.Request) {
	idParam := routeParam(r, "id")
	id, err := strconv.ParseInt(idParam, 10, 64)
	if err != nil || id <= 0 {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "id 无效"))
		return
	}
	row, err := h.repo.GetByID(id)
	if errors.Is(err, repository.ErrAIGeneratedImageNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		sendError(w, err)
		return
	}
	abs := filepath.Join(h.rootDir, row.FilePath)
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, abs)
}
```

> `routeParam` and `sendError` already exist in the handler package (used by other handlers); reuse them.

- [ ] **Step 4: Register the route**

In the file that registers `AIHandler` routes (search for `RegisterAI` or `aiRoutes`), add:

```go
imageHandler := handler.NewAIGeneratedImageHandler(aiGeneratedImageRepo, cfg.AIGeneratedDir())
mux.HandleFunc("GET /api/ai-generated-images/{id}/file", requireAuth(imageHandler.GetFile))
```

(Mirror the auth-wrapping pattern other endpoints use.)

- [ ] **Step 5: Run tests**

Run: `go test ./internal/handler/ -run TestAIGeneratedImageHandler -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/handler/ai_generated_image.go internal/handler/ai_generated_image_test.go
git add internal/app/  # whichever file got the route registration
git commit -m "handler: serve generated PNGs via GET /api/ai-generated-images/:id/file"
```

---

## Task 11: Frontend — `image-gen` tool tag in `ai-mention-tags.js`

**Files:**
- Modify: `web/static/js/ai-mention-tags.js`
- Modify: `web/static/js/__tests__/ai-mention-tags.test.cjs` (if exists; else create)

- [ ] **Step 1: Add the tag and family**

In `web/static/js/ai-mention-tags.js`:

```js
    const KNOWN_TOOL_TAGS = [
        { name: 'PubMed',          family: 'external', source: 'pubmed' },
        { name: 'SemanticScholar', family: 'external', source: 'semantic_scholar' },
        { name: 'Library',         family: 'library',  source: null },
        { name: 'Figure',          family: 'figure',   source: null },
        { name: 'image-gen',       family: 'image_gen', source: null },  // NEW
    ];

    const FAMILY_INTENT = {
        external:  'external_search',
        library:   'library_search',
        figure:    'figure_lookup',
        image_gen: 'image_generation',  // NEW
    };
```

Update the regex to include the new tag (note the hyphen needs escaping inside character class but not inside alternation; we use grouping):

```js
    const TAG_RE = /(^|\s)@(PubMed|SemanticScholar|Library|Figure|image-gen)\b/gi;
```

- [ ] **Step 2: Add a parse test**

If `web/static/js/__tests__/ai-mention-tags.test.cjs` exists, append a case; otherwise create it:

```js
const { parseToolTags } = require('../ai-mention-tags.js');

describe('parseToolTags - image-gen', () => {
    test('emits image_generation intent', () => {
        const r = parseToolTags('@image-gen draw it');
        expect(r.intentHint).toBe('image_generation');
        expect(r.sources).toEqual([]);
    });
    test('cross-family with library is last-wins', () => {
        const r = parseToolTags('@Library X @image-gen Y');
        expect(r.intentHint).toBe('image_generation');
        expect(r.conflict).not.toBeNull();
    });
});
```

- [ ] **Step 3: Run JS syntax check**

Run: `node --check web/static/js/ai-mention-tags.js`
Expected: no error.

If you have jest configured, run the relevant test file. Otherwise rely on the syntax check + downstream integration tests.

- [ ] **Step 4: Commit**

```bash
git add web/static/js/ai-mention-tags.js web/static/js/__tests__/ai-mention-tags.test.cjs
git commit -m "ai-mention-tags: add @image-gen tool tag with image_generation intent"
```

---

## Task 12: Frontend — `figure` mention type in `ai-mention.js` + `getFigures` provider in `ai-reader.js`

**Files:**
- Modify: `web/static/js/ai-mention.js`
- Modify: `web/static/js/ai-reader.js`

- [ ] **Step 1: Add `figure` mention type to `_buildItems` and `_renderItems`**

In `ai-mention.js`, inside `_buildItems(query)` (around line 292), after the `papers` block, add:

```js
            // Figures (only when caller provides them — e.g. AI page with pinned papers).
            const figures = typeof s.providers.getFigures === 'function'
                ? s.providers.getFigures()
                : [];
            figures
                .filter((fig) => {
                    if (!q) return true;
                    const haystack = [fig.label, fig.caption, fig.paper_title].filter(Boolean).join(' ').toLowerCase();
                    return haystack.includes(q);
                })
                .slice(0, 12)
                .forEach((fig) => {
                    items.push({
                        type: 'figure',
                        id: fig.id,
                        title: '@' + (fig.label || ('figure-' + fig.id)),
                        meta: [fig.paper_title, fig.caption].filter(Boolean).join(' · ').slice(0, 80),
                        _raw: fig,
                    });
                });
```

In `_renderItems` (around line 360), update the icon and `dataset` switch to handle figures:

```js
                let icon = '📄';
                if (item.type === 'role') icon = '@';
                else if (item.type === 'tool') icon = '⚙';
                else if (item.type === 'figure') icon = '🖼';
                let dataset;
                if (item.type === 'role') {
                    dataset = 'data-mention-type="role" data-role-name="' + escapeHTML(item.name) + '"';
                } else if (item.type === 'tool') {
                    dataset = 'data-mention-type="tool" data-tool-name="' + escapeHTML(item.name) +
                              '" data-tool-disabled="' + (item.disabled ? '1' : '0') + '"';
                } else if (item.type === 'figure') {
                    dataset = 'data-mention-type="figure" data-figure-id="' + item.id + '"';
                } else {
                    dataset = 'data-mention-type="paper" data-paper-id="' + item.id + '"';
                }
```

Add a new section render between papers and the rest (search for the `paperItems` section in `_renderItems`):

```js
            const figureItems = items.filter((it) => it.type === 'figure');
            // ...append a figureItems section block alongside paperItems, mirroring the paper section markup.
```

Use the same section-template the file uses for tools/roles/papers — copy the markup pattern verbatim with `data-section="figures"` and locale key `ai.mention_section_figures`.

- [ ] **Step 2: Add `getFigures` to `ai-reader.js`'s mention providers**

In `ai-reader.js` (around line 187 where `getToolTags` lives), add a sibling provider:

```js
                        getFigures: () => {
                            // Only surface figures when there's at least one pinned paper.
                            const pinned = (view._state && view._state.pinnedPapers) || [];
                            if (!pinned.length) return [];
                            const out = [];
                            pinned.forEach((p) => {
                                const figs = (p.figures || []);
                                figs.forEach((fig) => {
                                    out.push({
                                        id: fig.id,
                                        label: 'figure-' + fig.id,
                                        caption: fig.caption || '',
                                        paper_title: p.title || '',
                                    });
                                });
                            });
                            return out;
                        },
```

> **Note:** The `view._state.pinnedPapers[i].figures` shape may not be hydrated today. If the server response on `GET /api/ai-conversations/:id` does not yet include figures-per-pinned-paper, do one of:
>  1. Extend `Conversation`/`PinnedPaper` types in `ai_conversation/types.go` and the repo to include figures (cleaner), OR
>  2. Make `getFigures` async by issuing `GET /api/papers/:id/figures` per pinned paper, cached client-side.
>
> Pick (1) for MVP. Add `Figures []PinnedPaperFigure` to the Go `PinnedPaper` struct, populate via `s.papers.ListFigures(p.PaperID)` in `GetConversation`. Wire that data through to `view._state.pinnedPapers`. Document this in `docs/api.md`.

- [ ] **Step 3: i18n key for the section heading**

Add `ai.mention_section_figures` to `web/static/locales/zh-CN/ai.json` (`"图片"`) and `web/static/locales/en/ai.json` (`"Figures"`).

- [ ] **Step 4: JS syntax check**

Run:
```bash
node --check web/static/js/ai-mention.js
node --check web/static/js/ai-reader.js
```
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add web/static/js/ai-mention.js web/static/js/ai-reader.js \
        web/static/locales/zh-CN/ai.json web/static/locales/en/ai.json \
        internal/service/ai_conversation/types.go internal/service/ai_conversation/service.go \
        internal/repository/ai_conversation_repo.go
git commit -m "frontend: add @figure mention type sourced from pinned papers' figures"
```

---

## Task 13: Frontend — `generated_image` ResultCard renderer

**Files:**
- Modify: `web/static/js/ai-result-cards.js`
- Modify: `web/static/js/__tests__/ai-result-cards.test.cjs` (extend) and locales

- [ ] **Step 1: Add the renderer**

In `ai-result-cards.js`, add a case to `renderCard`:

```js
        case 'generated_image':
            return renderGeneratedImage(p);
```

Add the function after `renderFigureResult`:

```js
    function renderGeneratedImage(p) {
        const src = safeUrl(p.file_url);
        const fallback = translate('ai.image_gen_card_fallback', '生成图');
        const cost = (typeof p.cost_estimate_usd === 'number') ? p.cost_estimate_usd.toFixed(2) : null;
        const meta = [
            p.model,
            p.size,
            p.quality,
            cost ? '$' + cost : null,
        ].filter(Boolean).join(' · ');
        const downloadLabel = translate('ai.image_gen_card_download', '下载图片');
        const copyPromptLabel = translate('ai.image_gen_card_copy_prompt', '复制 prompt');
        const promptLabel = translate('ai.image_gen_card_prompt', 'AI prompt');
        return '<article class="ai-result-card ai-result-card-generated-image">' +
            (src
                ? '<img src="' + escapeHtml(src) + '" alt="' + escapeHtml(fallback) + '" loading="lazy">'
                : '<div class="ai-figure-missing">' + escapeHtml(translate('ai.result_image_unavailable', '图片不可用')) + '</div>') +
            '<div class="ai-result-card-head">' +
                '<h4>' + escapeHtml(fallback) + '</h4>' +
                (meta ? '<p>' + escapeHtml(meta) + '</p>' : '') +
            '</div>' +
            '<details class="ai-image-gen-prompt"><summary>' + escapeHtml(promptLabel) + '</summary>' +
                '<pre>' + escapeHtml(p.prompt || '') + '</pre>' +
            '</details>' +
            '<div class="ai-result-card-actions">' +
                (src ? '<a class="btn btn-small btn-outline" href="' + escapeHtml(src) + '" download>' + escapeHtml(downloadLabel) + '</a>' : '') +
                '<button class="btn btn-small" type="button" data-ai-image-gen-copy="' + escapeHtml(p.prompt || '') + '">' + escapeHtml(copyPromptLabel) + '</button>' +
            '</div>' +
        '</article>';
    }
```

Add a click delegate in `bindCollapsibleCards()` (or a sibling init function) that copies the data attribute to clipboard when the copy button is clicked:

```js
        document.addEventListener('click', (event) => {
            const btn = event.target.closest('[data-ai-image-gen-copy]');
            if (!btn) return;
            const text = btn.getAttribute('data-ai-image-gen-copy') || '';
            if (navigator.clipboard && navigator.clipboard.writeText) {
                navigator.clipboard.writeText(text);
            }
        });
```

- [ ] **Step 2: Add i18n keys**

Append to both `zh-CN/ai.json` and `en/ai.json`:

```jsonc
// zh-CN
"image_gen_card_fallback": "生成图",
"image_gen_card_download": "下载图片",
"image_gen_card_copy_prompt": "复制 prompt",
"image_gen_card_prompt": "AI prompt"

// en
"image_gen_card_fallback": "Generated image",
"image_gen_card_download": "Download",
"image_gen_card_copy_prompt": "Copy prompt",
"image_gen_card_prompt": "AI prompt"
```

- [ ] **Step 3: Add a snapshot test**

In `web/static/js/__tests__/ai-result-cards.test.cjs`, add a test that calls the existing exported `render` (or whatever entry the file already exposes) with a `generated_image` card and asserts the output contains the file URL, prompt text, and download link.

- [ ] **Step 4: Run JS syntax check**

```bash
node --check web/static/js/ai-result-cards.js
```
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add web/static/js/ai-result-cards.js web/static/js/__tests__/ai-result-cards.test.cjs \
        web/static/locales/zh-CN/ai.json web/static/locales/en/ai.json
git commit -m "ai-result-cards: render generated_image card with prompt + download"
```

---

## Task 14: Frontend — SSE event handlers in `ai-conversation-view.js`

**Files:**
- Modify: `web/static/js/ai-conversation-view.js`

- [ ] **Step 1: Locate the SSE event dispatch**

In `ai-conversation-view.js`, find the existing `onmessage` / event-type switch (search for `'process'`, `'cards'`, `'citations'`). Add new case handlers:

```js
            case 'image_prompt_drafting':
                this._setImageGenStatus(turnRunId, 'drafting');
                break;
            case 'image_prompt_drafted':
                this._setImageGenStatus(turnRunId, 'drafted', { prompt: event.data && event.data.prompt });
                break;
            case 'image_generating':
                this._setImageGenStatus(turnRunId, 'generating', event.data || {});
                break;
            case 'image_generated':
                this._setImageGenStatus(turnRunId, 'generated', { card: event.data && event.data.card });
                break;
            case 'image_failed':
                this._setImageGenStatus(turnRunId, 'failed', { reason: event.data && event.data.reason, stage: event.data && event.data.stage });
                break;
```

- [ ] **Step 2: Add `_setImageGenStatus` helper**

Add a helper that finds (or creates) a status node above the assistant message bubble for that turn run, and writes one of:

- "drafting" → `ai.image_gen.status.drafting_prompt` ("正在分析文献和图片...")
- "drafted" → `ai.image_gen.status.drafted` ("已生成 prompt：" + collapsible)
- "generating" → `ai.image_gen.status.generating` ("正在生成图片（约 ~$0.19）")
- "generated" → render the card via `window.AIReader.resultCards.render([{ card_type: 'generated_image', payload: cardPayload }])`
- "failed" → `ai.image_gen.status.failed` ("生成失败：" + reason)

Pseudo-implementation:

```js
        _setImageGenStatus(turnRunId, status, data) {
            const node = this._ensureImageGenStatusNode(turnRunId);
            const t = (key, fb) => translate(key, fb);
            switch (status) {
                case 'drafting':
                    node.innerHTML = '<p class="ai-status-line">' + escapeHtml(t('ai.image_gen.status.drafting_prompt', '正在分析文献和图片...')) + '</p>';
                    break;
                case 'drafted':
                    node.innerHTML =
                        '<p class="ai-status-line">' + escapeHtml(t('ai.image_gen.status.prompt_drafted', '已生成图像 prompt')) + '</p>' +
                        '<details><summary>' + escapeHtml(t('ai.image_gen.status.prompt_label', '查看 prompt')) + '</summary>' +
                        '<pre>' + escapeHtml(data.prompt || '') + '</pre></details>';
                    break;
                case 'generating':
                    const cost = (typeof data.cost_estimate_usd === 'number') ? '($' + data.cost_estimate_usd.toFixed(2) + ')' : '';
                    node.innerHTML = '<p class="ai-status-line">' + escapeHtml(t('ai.image_gen.status.generating', '正在生成图片') + ' ' + cost) + '</p>';
                    break;
                case 'generated':
                    node.innerHTML = window.AIReader.resultCards.render([{ card_type: 'generated_image', payload: data.card }]);
                    break;
                case 'failed':
                    node.innerHTML = '<p class="ai-status-error">' +
                        escapeHtml(t('ai.image_gen.status.failed', '图像生成失败')) + ': ' + escapeHtml(data.reason || '') +
                    '</p>';
                    break;
            }
        }
```

- [ ] **Step 3: Add the i18n keys**

Append to both locale files:

```jsonc
"image_gen": {
    "status": {
        "drafting_prompt": "正在分析文献和图片..." ,
        "prompt_drafted": "已生成图像 prompt",
        "prompt_label": "查看 prompt",
        "generating": "正在生成图片",
        "failed": "图像生成失败"
    }
}
```

(and the English equivalents)

- [ ] **Step 4: JS syntax check**

```bash
node --check web/static/js/ai-conversation-view.js
```

- [ ] **Step 5: Commit**

```bash
git add web/static/js/ai-conversation-view.js web/static/locales/
git commit -m "ai-conversation-view: render image-gen SSE status events inline"
```

---

## Task 15: Documentation

**Files:**
- Modify: `docs/api.md`
- Modify: `docs/database.md`
- Modify: `TODO`

- [ ] **Step 1: Document the new endpoint and SSE events in `docs/api.md`**

Append a new section after the existing AI conversation streaming section:

```md
### `GET /api/ai-generated-images/:id/file`

Returns the raw PNG bytes of an AI-generated graphical abstract. Auth required;
returns 404 for any image id not associated with a conversation the caller owns.

### Conversation streaming events: image generation

When a user sends a message tagged with `@image-gen`, the conversation stream
emits these additional event types in order:

- `image_prompt_drafting` — vision stage started
- `image_prompt_drafted { prompt }` — vision stage produced an image prompt
- `image_generating { model, size, quality, cost_estimate_usd }`
- `image_generated { card }` (success) **or** `image_failed { stage, reason }`

The result-card persisted on success has `card_type = "generated_image"` and
the payload shape:

```json
{
    "image_id": 123,
    "file_url": "/api/ai-generated-images/123/file",
    "prompt": "...",
    "model": "gpt-image-2",
    "size": "1024x1024",
    "quality": "high",
    "source_paper_ids": [42],
    "source_figure_ids": [],
    "cost_estimate_usd": 0.19
}
```
```

- [ ] **Step 2: Document the table in `docs/database.md`**

Append:

```md
### `ai_generated_images`

Stores metadata for AI-generated graphical-abstract images. The PNG bytes
themselves live on disk under `<storage_dir>/ai_generated/<conversation_id>/`.

Cascades on parent `ai_turn_runs` and `ai_conversations` deletions.

| Column | Type | Notes |
|---|---|---|
| id | INTEGER PK |  |
| turn_run_id | INTEGER FK | Cascade delete on the turn run |
| conversation_id | INTEGER FK | Cascade delete on the conversation |
| file_path | TEXT | Path relative to `<storage_dir>/ai_generated` |
| prompt | TEXT | The image prompt produced by the vision stage |
| model / size / quality | TEXT | Image API parameters used |
| source_paper_ids / source_figure_ids | TEXT (JSON int64 array) | What the user referenced |
| cost_estimate_usd | REAL | Hardcoded estimate |
| created_at | DATETIME | server clock |
```

- [ ] **Step 3: Update `TODO`**

Move "图片star功能" — actually, no. The relevant entry is "图片虚拟排版" — also not the same. Add a new entry under 已完成:

```
- [x] AI 对话内通过 @image-gen 生成 graphical abstract（vision → image API → 落盘 → 卡片）
```

- [ ] **Step 4: Commit**

```bash
git add docs/api.md docs/database.md TODO
git commit -m "docs: document graphical-abstract endpoint, SSE events, and table"
```

---

## Task 16: End-to-end smoke (manual)

**Files:** none

- [ ] **Step 1: Build and run**

```bash
make build
./bin/server
```

- [ ] **Step 2: Configure image-gen settings**

Open `http://localhost:8080`, navigate to Settings → AI, enable image generation, paste an OpenAI key.

- [ ] **Step 3: Drive the happy path**

In the AI 伴读 page:
1. Pin a paper that has at least 2 figures
2. Compose: `@image-gen @paper-<id> 给这篇文献画一个 graphical abstract`
3. Submit; observe the four SSE statuses → final card with image
4. Click download → confirm PNG saves
5. Click "复制 prompt" → paste somewhere, confirm prompt text

- [ ] **Step 4: Drive a failure path**

Replace API key with `sk-bad`; resubmit; expect `image_failed` event with `stage = "image_api"` and a readable reason.

- [ ] **Step 5: Verify persistence**

Reload the page; re-open the conversation; the previously-successful image card is still rendered.

- [ ] **Step 6: Run the full test suite**

```bash
make test
```
Expected: PASS.

- [ ] **Step 7: Document the smoke results in the PR description (no commit)**

---

## Self-review checklist (run before declaring the plan complete)

- [x] **Spec coverage:** Every section in the spec maps to at least one task.
  - §4 defaults → Task 3 (settings) + Task 7 (cost estimate hard-coded)
  - §5.1 data flow → Task 7 (Service.Generate)
  - §5.2 file layout → matches the File map at the top of this plan
  - §5.3 SSE events → Task 7 (emitted) + Task 14 (frontend rendered)
  - §5.4 mention behavior → Task 11 + Task 12
  - §6 error handling → Task 7 covers stages; Task 9 covers "no subject" + "not enabled"; Task 10 covers handler 404
  - §7 testing → Tasks 2, 3, 4, 5, 6, 7, 9, 10, 11, 13, 16
  - §8 i18n → Tasks 12, 13, 14
  - §9 docs → Task 15
  - §10 open questions → Q1 (gallery): table is built so it's unblocked; Q2 (prompt persistence): persisted in `prompt` column; Q3 (cost table): hardcoded in `costEstimate` (Task 7)
  - §11 out-of-scope items: not implemented anywhere ✅

- [x] **Placeholder scan:** No "TBD" / "implement later" / "similar to Task N" present in any step.

- [x] **Type consistency:** `repository.AIGeneratedImage` is used identically across Tasks 2, 7, 8, 10. `ai_image_gen.Generator` interface defined Task 7, consumed Task 9. `card_type === "generated_image"` matches Tasks 7 (emit), 13 (render), 15 (docs).

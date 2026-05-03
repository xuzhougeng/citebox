# AI Graphical Abstract Generation — Design

**Status:** Draft
**Author:** Brainstormed via superpowers:brainstorming on 2026-05-03
**Scope:** Add an explicit `@image-gen` tool to the AI conversation that turns an `@paper` or one-or-more `@figure` references into an AI-generated graphical-abstract image, rendered as a structured result card in the conversation timeline.

---

## 1. Goal

Let users in the AI 伴读 page generate a single AI-rendered graphical abstract for either:

1. an entire pinned paper (uses paper text + every figure as vision input), or
2. one or more specific figures from pinned papers (uses just those figures).

Generation is triggered explicitly via the `@image-gen` tool tag in the message composer. The output is one 1024×1024 image stored on disk and surfaced as a result card on the assistant turn.

This is the **MVP**. Out-of-scope items are listed in §11.

## 2. User stories

- As a researcher, I select `@image-gen` from the tool palette and add `@paper-42 给这篇文献画一个 graphical abstract` so the assistant produces a shareable summary image.
- As a researcher, I write `@image-gen @figure-101 @figure-103 把这两张图合成一张总结图` to fuse two existing figures into one new abstract.
- As a researcher, I see live progress: prompt drafted → image generating → image rendered, with the underlying prompt visible (read-only) for transparency.
- As a researcher, I can download the generated PNG and copy the prompt that produced it.

## 3. Non-goals (deferred)

- Multi-paper review images
- Editing the prompt and re-running from the same card (user re-sends with edited message instead)
- Image generation on Wolai / WeChat surfaces
- Per-user / per-conversation quota or rate limits
- `images.edit` reference-image mode (the "visual reuse" path)
- Generated-image gallery / library page (table is created so this is unblocked later)

## 4. Defaults & configuration

| Decision | Default | Notes |
|---|---|---|
| Image model | `gpt-image-2` | Model name is configurable in settings; user is the source of truth on naming |
| Size | `1024x1024` | Square graphical-abstract format |
| Quality | `high` | gpt-image-* quality tiers: `low` / `medium` / `high` |
| API base URL | `https://api.openai.com` | Configurable to support OpenAI-compatible providers |
| API key | Independent `image_gen` settings block (not reused from chat models) | Image API is OpenAI-shape only; isolating settings avoids forcing all chat traffic through OpenAI |
| Prompt-drafting model | Reuses the conversation's current QA scene model (must be vision-capable) | Avoids adding a new model selector slot at MVP |
| Storage | `data/ai_generated/<conversation_id>/<ulid>.png` + `ai_generated_images` table | Disk file + lightweight metadata row |
| Cost guard | Cost estimate shown in the generating-state UI; no hard quota | Single-tenant local app |
| Cancellation | The vision stage runs under the request context. Once the image API call starts, the service detaches to a fresh `context.Background()` with a hard timeout (default 120s) so that an HTTP disconnect during generation cannot abort a paid call mid-flight. | Avoids wasting a \$0.19 call when the user closes the tab |
| Retry | "Retry" re-runs the entire turn (existing turn_run retry semantics); no card-level regenerate | Smaller surface for MVP |

## 5. Architecture (Approach 3: SSE multi-stage events)

The feature plugs into the existing `ai_conversation` orchestration. When the user message carries an `image-gen` tool tag, the orchestrator routes the turn to a new `ai_image_gen` service instead of the default LLM-text path. The service streams three SSE events (`image_prompt_drafted`, `image_generating`, `image_generated` / `image_failed`) over the same `OnEvent` callback that conversations already use.

The vision-understanding stage uses the same `CallOpenAIResponses` (or equivalent provider-specific call) and `loadFigureInputs` helpers that figure interpretation already uses. The image-generation stage is a single synchronous HTTP POST to `/v1/images/generations` that returns base64 PNG bytes; we decode and persist them.

### 5.1 Data flow

```
User submits message containing the @image-gen tool tag
    ↓
ai_conversation.Service.SendMessage detects tool tag = "image-gen"
    ↓
ai_image_gen.Service.Generate(ctx, GenerateInput{
    ConversationID, TurnRunID,
    PaperIDs:  [...],   // from @paper mentions
    FigureIDs: [...],   // from @figure mentions
    UserText:  "...",
    OnEvent:   func(StreamEvent) error,
})
    ↓ emit image_prompt_drafting
    ↓
1. Load inputs
   - For each PaperID: paper title, abstract, full-text chunks, every figure (loadFigureInputs)
   - For each FigureID (without a covering @paper): just that figure + caption + parent paper title
    ↓
2. Vision call — provider's existing chat/responses endpoint
   - System prompt: "You are a graphical-abstract designer..."
   - User prompt: assembled paper context + user request
   - Images: base64-compressed figures (existing budget rules apply)
   → returns the image-generation prompt (text)
    ↓ emit image_prompt_drafted { prompt }
    ↓ emit image_generating
    ↓
3. Image API — POST {base_url}/v1/images/generations
   { model, prompt, size: "1024x1024", quality: "high", n: 1, response_format: "b64_json" }
    ↓
4. Decode b64_json → write to data/ai_generated/<conversation_id>/<ulid>.png
    ↓
5. Insert ai_generated_images row, get id
    ↓
6. AddResultCard(turn_run_id, card_type="generated_image",
                 payload={image_id, file_url, prompt, model, size, quality,
                          source_paper_ids, source_figure_ids, cost_estimate_usd})
    ↓ emit image_generated { card }
```

Failures at steps 2 / 3 / 4 / 5 emit `image_failed` with a user-readable reason and write the turn_run as `failed`. Step 2 failures abort before the paid step 3.

### 5.2 Component layout

**New packages / files**

```
internal/service/ai_image_gen/
    service.go          — Generate() orchestration; emits SSE events
    prompt.go           — system prompt for the vision stage; render helpers
    client.go           — OpenAI-compatible images.generations HTTP client
    storage.go          — disk write + path conventions
    types.go            — GenerateInput / Settings / errors
    service_test.go     — fake imageAPIClient + fake vision provider
    prompt_test.go      — golden test on prompt assembly

internal/repository/
    ai_generated_image_repo.go         — Insert / GetByID / ListByConversation
    ai_generated_image_repo_test.go    — cascade delete with turn_run; round-trip

internal/handler/
    ai_generated_image.go              — GET /api/ai-generated-images/:id/file (auth required, owner check)
    ai_generated_image_test.go         — auth + ownership tests
```

**Schema migration (additive)** — `internal/repository/schema/schema.go`

```sql
CREATE TABLE ai_generated_images (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    turn_run_id INTEGER NOT NULL REFERENCES ai_turn_runs(id) ON DELETE CASCADE,
    conversation_id INTEGER NOT NULL REFERENCES ai_conversations(id) ON DELETE CASCADE,
    file_path TEXT NOT NULL,
    prompt TEXT NOT NULL,
    model TEXT NOT NULL,
    size TEXT NOT NULL,
    quality TEXT NOT NULL,
    source_paper_ids TEXT NOT NULL,    -- JSON array of int64
    source_figure_ids TEXT NOT NULL,   -- JSON array of int64
    cost_estimate_usd REAL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_ai_generated_images_conversation ON ai_generated_images(conversation_id);
CREATE INDEX idx_ai_generated_images_turn_run ON ai_generated_images(turn_run_id);
```

**Settings** — `internal/model/ai.go` adds:

```go
type AIImageGenSettings struct {
    Enabled  bool   `json:"enabled"`
    APIKey   string `json:"api_key"`
    BaseURL  string `json:"base_url"`     // default "https://api.openai.com"
    Model    string `json:"model"`        // default "gpt-image-2"
    Size     string `json:"size"`         // default "1024x1024"
    Quality  string `json:"quality"`      // default "high"
}
```

This is added as `AISettings.ImageGen` and round-tripped via the existing `ai/settings.go` JSON layer. `NormalizeSettings` fills in defaults when `Enabled` is true and validates the size / quality enums.

**Modified files**

- `internal/service/ai_conversation/orchestration.go` — branch on `image-gen` tool tag → route to `ai_image_gen.Service`. Keeps the rest of the turn-run / result-card persistence pipeline intact.
- `internal/service/ai_conversation/service.go` — pass image-gen tool detections through to orchestration.
- `internal/handler/ai.go` (or wherever tool-tag handling lives) — register `image-gen` in the tool tag family.
- `web/static/js/ai-mention.js` — add `figure` mention type (palette source = pinned papers' figures; ranks by paper then figure index); register `image-gen` tool with disabled-reason fallback when settings.enabled is false.
- `web/static/js/ai-result-cards.js` — render `card_type === 'generated_image'`: image (clickable to enlarge), prompt (collapsible), download / copy-prompt buttons, cost label.
- `web/static/js/ai-conversation-view.js` — render the three new SSE event types as inline progress states on the assistant turn.
- `web/static/locales/zh-CN/ai.json` and `web/static/locales/en/ai.json` — keys for the tool palette entry, mention section, status strings, card buttons.
- `docs/api.md` — document `GET /api/ai-generated-images/:id/file` and the new SSE event types.
- `docs/database.md` — document the new table and column semantics.

### 5.3 SSE event payloads

All events flow over the existing message stream. Payload shapes:

```jsonc
// event: image_prompt_drafting
{ "turn_run_id": 12345 }

// event: image_prompt_drafted
{ "turn_run_id": 12345, "prompt": "..." }

// event: image_generating
{ "turn_run_id": 12345, "model": "gpt-image-2", "size": "1024x1024", "quality": "high",
  "cost_estimate_usd": 0.19 }

// event: image_generated
{ "turn_run_id": 12345, "card": { /* ResultCard JSON */ } }

// event: image_failed
{ "turn_run_id": 12345, "reason": "user-readable Chinese/English message",
  "stage": "vision" | "image_api" | "save" }
```

### 5.4 Frontend mention behaviour

- The `@image-gen` tool entry is shown in the existing `ai_conversation` mention popover under `工具/数据源`. When `AISettings.ImageGen.Enabled` is false, the entry is `disabled` with reason "未启用，前往设置 → 图像生成"; the existing `disabledReason` rendering handles this.
- `@figure` mentions: a new mention section labeled `图片`. Source = figures of every paper currently pinned in the conversation. Sort by paper, then figure index. Empty state when no papers are pinned: "请先 pin 一篇文献"。
- Composer chips display as `@figureN` (figure index within paper) so the rendered mention matches the user's example phrasing `@image1 @image2`. The actual stored token references the figure id.

## 6. Error handling

| Failure | Behaviour |
|---|---|
| Message has `@image-gen` but no `@paper` and no `@figure` | Reject before any model call: `apperr.CodeInvalidArgument` "请引用至少一篇文献或一张图" |
| `AISettings.ImageGen.Enabled` false | `apperr.CodeFailedPrecondition` "请先在 AI 设置中启用图像生成" |
| Vision call fails | Skip image API (saves money). Emit `image_failed{stage:"vision"}`. Mark turn_run failed |
| Image API returns 4xx (content policy, invalid prompt, auth, etc.) | Emit `image_failed{stage:"image_api", reason: parsed message}`. Mark turn_run failed. No file written |
| Image API returns 5xx / network | Same as 4xx but reason describes transient failure; user can retry the turn |
| Decode / write to disk fails | Roll back: do not insert DB row. Emit `image_failed{stage:"save"}` |
| Client disconnects mid-generation | Once the image API call starts, the work runs under a detached `context.Background()` (hard 120s timeout); save + DB insert + result card all complete. Next time the conversation is opened the card is visible |
| User opens `GET /api/ai-generated-images/:id/file` for an image they don't own | 404 (ownership inferred via conversation) |

## 7. Testing

**Go unit / integration**
- `ai_image_gen/service_test.go` — fake vision provider + fake image API client; assert event sequence, error propagation per stage, file written on success, no file written on failure.
- `ai_image_gen/prompt_test.go` — golden test the assembled vision system+user prompt for a fixture paper with two figures.
- `repository/ai_generated_image_repo_test.go` — round-trip insert/get; cascade delete when parent turn_run is deleted; cascade when conversation is deleted.
- `handler/ai_generated_image_test.go` — auth required; cross-user access returns 404.

**Frontend**
- `web/static/js/__tests__/ai-result-cards.test.cjs` — snapshot for `generated_image` card render.
- `web/static/js/__tests__/ai-mention.test.cjs` (extend or add) — `figure` mention section appears only when papers are pinned; `image-gen` tool entry shows `disabledReason` correctly.
- JS syntax check for every touched file (`node --check`).

**Manual E2E** (not in CI)
- Real OpenAI key + a paper with figures; verify event order and final card.
- Failure injection: invalid API key → expect `image_failed{stage:"image_api"}`.

## 8. i18n keys

All user-visible new strings must live in both `zh-CN/ai.json` and `en/ai.json`. Minimum new keys:

- `ai.tool.image_gen.title`, `ai.tool.image_gen.description`
- `ai.tool.image_gen.disabled_reason`
- `ai.mention_section_figures`
- `ai.image_gen.status.drafting_prompt`, `ai.image_gen.status.generating`, `ai.image_gen.status.cost_estimate`
- `ai.image_gen.card.download`, `ai.image_gen.card.copy_prompt`
- `ai.image_gen.errors.no_subject`, `ai.image_gen.errors.not_enabled`, `ai.image_gen.errors.vision_failed`, `ai.image_gen.errors.image_api_failed`, `ai.image_gen.errors.save_failed`

## 9. Documentation updates (in same PR)

- `docs/api.md` — new `GET /api/ai-generated-images/:id/file` endpoint; new SSE event types under the conversation streaming section.
- `docs/database.md` — `ai_generated_images` table description and FK semantics.
- `TODO` — move "生图功能" out of pending and into 已完成 once the implementation merges.

## 10. Open questions for implementation phase

- Path: should the conversation list page surface a "n images" indicator? Out of scope for this spec — the table makes it doable later.
- Should `image_prompt_drafted` events also be persisted (so reopening a conversation shows the prompt history)? Spec answer: yes — store the prompt in `ai_generated_images.prompt`, no separate event log needed.
- Cost estimate calculation: hardcoded table for `(model, size, quality) → USD` for MVP, since OpenAI does not expose pricing in the API response.

## 11. Out of scope (explicit)

- Reference-image visual reuse via `images.edit` (Approach A from brainstorm)
- Editing the drafted prompt before generation (Approach B from brainstorm — we picked C)
- Implicit intent detection (we picked explicit tool tag)
- Surfaces other than the web AI 伴读 page
- Multi-image batches (`n > 1`)
- Per-user quota / rate limiting
- Card-level "regenerate" button

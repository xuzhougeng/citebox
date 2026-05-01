# `@` 提及驱动的工具/数据源显式调用 — 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让用户在 AI 助手对话框里通过 `@PubMed`、`@SemanticScholar`、`@Library`、`@Figure` 显式选择检索工具/数据源，覆盖 auto 路由；不打任何工具标签时保留现有自动路由。

**Architecture:** 显式覆盖层。前端在 `ai-mention.js` 弹层加一段「工具/数据源」选项；提交前 `parseToolTags(text)` 解析为 `{intentHint, sources}` 并随请求体发出。后端 `RunInput` / `ToolInput` 增加 `Sources []string`，`ExternalSearchTool` 据此把执行集合收窄到「用户显式指定 ∩ 已启用」，未启用但被显式指定的源回退为 `ErrSourceDisabled` 失败卡片。`LibrarySearchTool` 在显式 `IntentHint=library_search` 且有 `Context.PaperIDs` 时把候选集裁剪到这些 paper。

**Tech Stack:** Go 后端（`internal/service/ai_assistant`、`internal/service/ai_external`、`internal/service/ai_conversation`、`internal/handler`）；vanilla JS 前端（`web/static/js/ai-mention.js`、`ai-reader.js`、`ai-conversation-view.js`）；Node 22 内建 `node --test` 跑前端纯函数单测。

**Spec：** `docs/superpowers/specs/2026-05-01-at-mention-search-tools-design.md`

---

## File Structure

**新增：**

- `web/static/js/ai-mention-tags.js` — 纯函数模块，导出 `parseToolTags`、`commitToolTag`、`KNOWN_TOOL_TAGS`、`familyOf`。UMD 风格：CommonJS（Node 测试用） + `window.AIReader.toolTags`（浏览器用）。
- `web/static/js/__tests__/ai-mention-tags.test.cjs` — Node `node:test` + `node:assert` 跑的单测。

**修改：**

- `web/ai.html:108-119` — 在 `ai-mention.js` 之前 `<script>` 引入 `ai-mention-tags.js`。
- `web/static/js/ai-mention.js` — 新增「工具/数据源」段、`commitToolTag` 调用、置灰逻辑。providers 契约扩展 `getToolTags()` / `onPickToolTag()` / `getToolTagDisabled()`.
- `web/static/js/ai-reader.js` — 启动时再 fetch 一次 `/api/settings/ai-external-search`，把启用集合缓存到 `Reader.externalSourcesEnabled`。`mention.attach` 的 providers 加新方法。
- `web/static/js/ai-conversation-view.js:240-285` — `_sendBody` 里调用 `AIReader.toolTags.parseToolTags`，把 `intent_hint` / `sources` 写进 body。
- `web/settings.html:163-185` — 给「AI 外部搜索源」section 加 `id="settings-external-sources"` 锚点。
- `internal/service/ai_assistant/types.go:21-32` — `RunInput` / `ToolInput` 加 `Sources []string`.
- `internal/service/ai_assistant/orchestrator.go:43-69` — 透传 `Sources`.
- `internal/service/ai_assistant/external_search_tool.go` — 新增 `ErrSourceDisabled`，`Run` 增加 user-override 分支。
- `internal/service/ai_assistant/library_search_tool.go:73-128` — `IntentHint == library_search && len(PaperIDs)>0` 时裁剪 `candidateIDs` 输出到 `Context.PaperIDs` 集合内。
- `internal/service/ai_conversation/types.go:71-79` — `SendMessageInput` 加 `Sources []string`.
- `internal/service/ai_conversation/service.go:301-307` — 把 `in.Sources` 传给 `RunInput`.
- `internal/handler/ai_conversation.go:141-149` — 请求体 struct 加 `Sources []string`，传给 `SendMessageInput`.
- `docs/api.md` — 在 POST `/api/ai/conversations/:id/messages` 文档里补 `sources` 字段说明。

---

## Task 1: 前端纯函数 `parseToolTags` 与 `commitToolTag` (TDD)

**Files:**
- Create: `web/static/js/ai-mention-tags.js`
- Test: `web/static/js/__tests__/ai-mention-tags.test.cjs`

- [ ] **Step 1: 写失败测试 — 创建测试文件**

```bash
mkdir -p web/static/js/__tests__
```

写入 `web/static/js/__tests__/ai-mention-tags.test.cjs`:

```js
'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const path = require('node:path');

const mod = require(path.join('..', 'ai-mention-tags.js'));
const { parseToolTags, commitToolTag, KNOWN_TOOL_TAGS, familyOf } = mod;

test('KNOWN_TOOL_TAGS lists the four MVP tags in canonical case', () => {
    assert.deepEqual(
        KNOWN_TOOL_TAGS.map((t) => t.name),
        ['PubMed', 'SemanticScholar', 'Library', 'Figure']
    );
});

test('familyOf maps each tag to its routing family', () => {
    assert.equal(familyOf('PubMed'), 'external');
    assert.equal(familyOf('SemanticScholar'), 'external');
    assert.equal(familyOf('Library'), 'library');
    assert.equal(familyOf('Figure'), 'figure');
    assert.equal(familyOf('Unknown'), null);
});

test('parseToolTags returns empty result for plain text', () => {
    const r = parseToolTags('帮我查找 ATAC 文章');
    assert.equal(r.intentHint, '');
    assert.deepEqual(r.sources, []);
    assert.equal(r.conflict, null);
});

test('parseToolTags treats a single @PubMed as external_search + pubmed', () => {
    const r = parseToolTags('@PubMed 找综述');
    assert.equal(r.intentHint, 'external_search');
    assert.deepEqual(r.sources, ['pubmed']);
    assert.equal(r.conflict, null);
});

test('parseToolTags is case-insensitive', () => {
    const r = parseToolTags('@pubmed @SEMANTICSCHOLAR foo');
    assert.equal(r.intentHint, 'external_search');
    assert.deepEqual(r.sources, ['pubmed', 'semantic_scholar']);
});

test('parseToolTags stacks same-family tags and dedupes', () => {
    const r = parseToolTags('@PubMed @SemanticScholar @PubMed foo');
    assert.equal(r.intentHint, 'external_search');
    assert.deepEqual(r.sources.sort(), ['pubmed', 'semantic_scholar']);
});

test('parseToolTags applies cross-family last-wins (figure beats earlier external)', () => {
    const r = parseToolTags('@PubMed @Figure 找细胞图');
    assert.equal(r.intentHint, 'figure_lookup');
    assert.deepEqual(r.sources, []);
    assert.deepEqual(r.conflict, { dropped: 'external', kept: 'figure' });
});

test('parseToolTags applies cross-family last-wins (library beats earlier figure)', () => {
    const r = parseToolTags('@Figure @Library foo');
    assert.equal(r.intentHint, 'library_search');
    assert.deepEqual(r.sources, []);
    assert.deepEqual(r.conflict, { dropped: 'figure', kept: 'library' });
});

test('parseToolTags requires a word boundary so @PubMedTutorial is NOT a match', () => {
    const r = parseToolTags('@PubMedTutorial 教程');
    assert.equal(r.intentHint, '');
    assert.deepEqual(r.sources, []);
});

test('parseToolTags requires whitespace or BOL before @ so emails do not trip', () => {
    const r = parseToolTags('contact me at user@PubMed.com');
    assert.equal(r.intentHint, '');
    assert.deepEqual(r.sources, []);
});

test('parseToolTags ignores unknown @Foo tags', () => {
    const r = parseToolTags('@Foo @PubMed bar');
    assert.equal(r.intentHint, 'external_search');
    assert.deepEqual(r.sources, ['pubmed']);
});

test('parseToolTags preserves user-typed text including the tags', () => {
    // Caller does not rely on parseToolTags to strip — it only reports.
    const r = parseToolTags('@Library please summarize');
    assert.equal(r.intentHint, 'library_search');
});

test('commitToolTag with empty input inserts the tag at caret', () => {
    const r = commitToolTag({ value: '', selectionStart: 0, atIndex: 0, query: '' }, 'PubMed');
    assert.equal(r.value, '@PubMed ');
    assert.equal(r.caret, '@PubMed '.length);
});

test('commitToolTag replaces partial query after @ with full tag', () => {
    // input simulates user typing "@pub" with caret right after, atIndex=0, query="pub"
    const r = commitToolTag(
        { value: '@pub', selectionStart: 4, atIndex: 0, query: 'pub' },
        'PubMed'
    );
    assert.equal(r.value, '@PubMed ');
    assert.equal(r.caret, '@PubMed '.length);
});

test('commitToolTag stacking same-family preserves earlier @PubMed when adding @SemanticScholar', () => {
    const r = commitToolTag(
        { value: '@PubMed @sem', selectionStart: '@PubMed @sem'.length, atIndex: 8, query: 'sem' },
        'SemanticScholar'
    );
    assert.equal(r.value, '@PubMed @SemanticScholar ');
});

test('commitToolTag cross-family removes existing tags from a different family', () => {
    // user previously committed @PubMed; now picks @Library — @PubMed must be removed first
    const r = commitToolTag(
        { value: '@PubMed @lib', selectionStart: '@PubMed @lib'.length, atIndex: 8, query: 'lib' },
        'Library'
    );
    assert.equal(r.value, '@Library ');
    assert.equal(r.caret, '@Library '.length);
});

test('commitToolTag cross-family preserves non-tool text and @paper mentions', () => {
    const r = commitToolTag(
        {
            value: '@PubMed @AlphaFold 这篇 @lib',
            selectionStart: '@PubMed @AlphaFold 这篇 @lib'.length,
            atIndex: '@PubMed @AlphaFold 这篇 '.length,
            query: 'lib',
        },
        'Library'
    );
    // Only the @PubMed tool tag is removed; @AlphaFold (a paper mention) is preserved.
    assert.equal(r.value, '@AlphaFold 这篇 @Library ');
});
```

- [ ] **Step 2: 跑测试看它失败**

```bash
node --test web/static/js/__tests__/ai-mention-tags.test.cjs
```

预期：`Cannot find module '../ai-mention-tags.js'` 报错全部 fail。

- [ ] **Step 3: 写 `ai-mention-tags.js` 让测试通过**

写入 `web/static/js/ai-mention-tags.js`:

```js
// ai-mention-tags.js — pure logic for the @ tool/source palette.
//
// Exposes parseToolTags(text) and commitToolTag(state, name) so they can be
// unit-tested under Node and reused inside the browser-only ai-mention.js.
//
// UMD-ish: under Node it sets module.exports; in the browser it attaches to
// window.AIReader.toolTags.

(function (root, factory) {
    if (typeof module !== 'undefined' && module.exports) {
        module.exports = factory();
    } else {
        root.AIReader = root.AIReader || {};
        root.AIReader.toolTags = factory();
    }
}(typeof window !== 'undefined' ? window : globalThis, function () {
    'use strict';

    // Canonical tag list. Order matters — the popover renders tools in this
    // order so users see the same layout regardless of session.
    const KNOWN_TOOL_TAGS = [
        { name: 'PubMed',          family: 'external', source: 'pubmed' },
        { name: 'SemanticScholar', family: 'external', source: 'semantic_scholar' },
        { name: 'Library',         family: 'library',  source: null },
        { name: 'Figure',          family: 'figure',   source: null },
    ];

    const FAMILY_INTENT = {
        external: 'external_search',
        library:  'library_search',
        figure:   'figure_lookup',
    };

    const NAME_LOOKUP = {};
    KNOWN_TOOL_TAGS.forEach((t) => { NAME_LOOKUP[t.name.toLowerCase()] = t; });

    function familyOf(name) {
        const t = NAME_LOOKUP[String(name || '').toLowerCase()];
        return t ? t.family : null;
    }

    // \b breaks at the boundary between word/non-word chars; we require either
    // start-of-string or whitespace before @ so emails ("foo@bar") never match.
    const TAG_RE = /(^|\s)@(PubMed|SemanticScholar|Library|Figure)\b/gi;

    function scanTags(text) {
        // Returns array of { name, family, source, start, end } in order of
        // appearance. Names are normalized to canonical case.
        const out = [];
        const value = String(text || '');
        TAG_RE.lastIndex = 0;
        let m;
        while ((m = TAG_RE.exec(value)) !== null) {
            const lead = m[1] || '';
            const raw = m[2];
            const t = NAME_LOOKUP[raw.toLowerCase()];
            if (!t) continue;
            const start = m.index + lead.length;            // position of '@'
            const end = start + 1 + raw.length;             // after the name
            out.push({ name: t.name, family: t.family, source: t.source, start, end });
        }
        return out;
    }

    function parseToolTags(text) {
        const tags = scanTags(text);
        if (tags.length === 0) {
            return { intentHint: '', sources: [], conflict: null };
        }

        // Cross-family last-wins. Walk in order; whenever a tag's family
        // differs from the running family, drop everything and reset.
        let keptFamily = tags[0].family;
        let kept = [tags[0]];
        let conflict = null;
        for (let i = 1; i < tags.length; i++) {
            const tag = tags[i];
            if (tag.family !== keptFamily) {
                if (!conflict) conflict = { dropped: keptFamily, kept: tag.family };
                else conflict = { dropped: keptFamily, kept: tag.family };
                keptFamily = tag.family;
                kept = [tag];
            } else {
                kept.push(tag);
            }
        }

        const sources = [];
        const seen = {};
        kept.forEach((tag) => {
            if (!tag.source) return;
            if (seen[tag.source]) return;
            seen[tag.source] = true;
            sources.push(tag.source);
        });

        return {
            intentHint: FAMILY_INTENT[keptFamily] || '',
            sources: sources,
            conflict: conflict,
        };
    }

    // commitToolTag is invoked when the popover commits a tool tag. It returns
    // { value, caret } and is pure — the caller writes back into the textarea.
    //
    // state shape: { value, selectionStart, atIndex, query }
    //   - atIndex: byte index of the '@' that triggered the popover
    //   - query:   the chars typed after '@' so far
    function commitToolTag(state, newTagName) {
        const value = String(state && state.value || '');
        const atIndex = Number.isFinite(state && state.atIndex) ? state.atIndex : value.length;
        const query = String(state && state.query || '');

        // Splice the typed-so-far "@<query>" out: caret currently sits right
        // after that. After splice we reinsert "@NewTag " at the same spot.
        const beforeAt = value.slice(0, atIndex);
        const afterTriggerEnd = atIndex + 1 + query.length;
        const tail = value.slice(afterTriggerEnd);

        const newTag = canonicalNameOf(newTagName);
        const newFamily = familyOf(newTag);

        // Strip *other-family* tool tags from beforeAt + tail. We must rescan
        // them on the joined string and remove only those that don't match the
        // new family. Same-family tags are preserved.
        const composed = beforeAt + tail;
        const composedTags = scanTags(composed);
        const removals = composedTags.filter((t) => t.family !== newFamily);

        let cleaned = composed;
        // Walk removals back-to-front so earlier indices stay valid.
        for (let i = removals.length - 1; i >= 0; i--) {
            const r = removals[i];
            // Trim a trailing space if the tag was followed by " " — keeps the
            // text tidy when we rip it out of the middle of the string.
            let endCut = r.end;
            if (cleaned[endCut] === ' ') endCut++;
            cleaned = cleaned.slice(0, r.start) + cleaned.slice(endCut);
        }

        // Recompute the splice point after removals: every removed tag that
        // sat *before* atIndex shifted atIndex left by its byte length.
        let adjustedAt = atIndex;
        removals.forEach((r) => {
            if (r.start < atIndex) {
                let cutLen = r.end - r.start;
                if (composed[r.end] === ' ') cutLen++;
                adjustedAt -= cutLen;
            }
        });
        if (adjustedAt < 0) adjustedAt = 0;
        if (adjustedAt > cleaned.length) adjustedAt = cleaned.length;

        const before = cleaned.slice(0, adjustedAt);
        const rest = cleaned.slice(adjustedAt);
        const insertion = '@' + newTag + ' ';
        const finalValue = before + insertion + rest;
        return { value: finalValue, caret: before.length + insertion.length };
    }

    function canonicalNameOf(raw) {
        const t = NAME_LOOKUP[String(raw || '').toLowerCase()];
        return t ? t.name : String(raw || '');
    }

    return {
        KNOWN_TOOL_TAGS: KNOWN_TOOL_TAGS,
        familyOf: familyOf,
        parseToolTags: parseToolTags,
        commitToolTag: commitToolTag,
    };
}));
```

- [ ] **Step 4: 跑测试，确认全部通过**

```bash
node --test web/static/js/__tests__/ai-mention-tags.test.cjs
```

预期：所有 `pass`，`# fail 0`.

- [ ] **Step 5: 提交**

```bash
git add web/static/js/ai-mention-tags.js web/static/js/__tests__/ai-mention-tags.test.cjs
git commit -m "$(cat <<'EOF'
Add parseToolTags / commitToolTag pure logic for @ palette

Pure JS module + node:test covering same-family stacking, cross-family
last-wins, case insensitivity, word boundary, and commit-time rewrite
of cross-family tags. UMD-style export so the browser ai-mention.js can
reuse it without a bundler.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: 后端 `RunInput` / `ToolInput` 加 `Sources` 字段

**Files:**
- Modify: `internal/service/ai_assistant/types.go:21-32`
- Modify: `internal/service/ai_assistant/orchestrator.go:43-69`
- Test: `internal/service/ai_assistant/orchestrator_test.go`（追加用例）

- [ ] **Step 1: 写失败测试 — 验证 Orchestrator 把 Sources 透传到 ToolInput**

打开 `internal/service/ai_assistant/orchestrator_test.go`，在文件末尾追加：

```go
func TestOrchestratorPropagatesSourcesToToolInput(t *testing.T) {
	tool := &capturingTool{res: ToolResult{
		Process: ProcessSummary{Intent: IntentExternalSearch},
	}}
	orch := NewOrchestrator(ToolSet{
		ExternalSearch: tool,
	})

	_, err := orch.Run(context.Background(), RunInput{
		Content:    "找一下相关研究",
		IntentHint: IntentExternalSearch,
		Sources:    []string{"pubmed"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := ToolInput{
		Query:      "找一下相关研究",
		IntentHint: IntentExternalSearch,
		Sources:    []string{"pubmed"},
	}
	if !reflect.DeepEqual(tool.in, want) {
		t.Fatalf("tool input = %+v, want %+v", tool.in, want)
	}
}
```

- [ ] **Step 2: 跑测试，确认它编译失败**

```bash
go test ./internal/service/ai_assistant/ -run TestOrchestratorPropagatesSourcesToToolInput
```

预期：编译失败 — `RunInput` / `ToolInput` 没有 `Sources` 字段。

- [ ] **Step 3: 给 `RunInput` / `ToolInput` 加字段**

编辑 `internal/service/ai_assistant/types.go`，把 `RunInput` 和 `ToolInput` 改为：

```go
type RouteInput struct {
	Content    string         `json:"content"`
	IntentHint string         `json:"intent_hint,omitempty"`
	Context    RequestContext `json:"context,omitempty"`
}

type ToolInput struct {
	Query      string         `json:"query"`
	Context    RequestContext `json:"context,omitempty"`
	Limit      int            `json:"limit,omitempty"`
	IntentHint string         `json:"intent_hint,omitempty"`
	Sources    []string       `json:"sources,omitempty"`
}
```

注意：`RouteInput` 是路由专用结构，**不**加 `Sources`（router 不读 sources）。

然后编辑 `internal/service/ai_assistant/orchestrator.go`，把 `RunInput` 和 `Run` 改成：

```go
type RunInput struct {
	Content    string
	IntentHint string
	Sources    []string
	Context    RequestContext
}
```

并把 `Run` 里构造 `ToolInput` 的那一行改为：

```go
res, err := tool.Run(ctx, ToolInput{
	Query:      in.Content,
	Context:    in.Context,
	IntentHint: in.IntentHint,
	Sources:    in.Sources,
})
```

- [ ] **Step 4: 跑测试，确认通过；同时跑全包**

```bash
go test ./internal/service/ai_assistant/...
```

预期：所有用例 PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/service/ai_assistant/types.go internal/service/ai_assistant/orchestrator.go internal/service/ai_assistant/orchestrator_test.go
git commit -m "$(cat <<'EOF'
Add Sources field to RunInput / ToolInput

Threads a Sources slice from RunInput through Orchestrator into ToolInput
so external-search-family tools can scope to a user-specified subset.
RouteInput is intentionally unchanged — the router operates on intent
hints and content alone.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: `ai_conversation.SendMessageInput` 加 `Sources` 并透传到 Orchestrator

**Files:**
- Modify: `internal/service/ai_conversation/types.go:71-79`
- Modify: `internal/service/ai_conversation/service.go:301-307`

- [ ] **Step 1: 编辑 `types.go`**

`internal/service/ai_conversation/types.go` 把 `SendMessageInput` 改为：

```go
// SendMessageInput is the body for POST .../messages.
type SendMessageInput struct {
	ConversationID          int64 // 0 means "create new"
	Content                 string
	PaperID                 int64 // optional auto-pin
	IncludeExternalEvidence bool
	IntentHint              string
	Sources                 []string
	Context                 ai_assistant.RequestContext
	OnEvent                 func(StreamEvent) error
}
```

- [ ] **Step 2: 编辑 `service.go`，把 Sources 传到 RunInput**

`internal/service/ai_conversation/service.go`，找到现在构造 `RunInput` 的位置（约第 303 行）：

```go
out, orchErr := s.orchestrator.Run(ctx, ai_assistant.RunInput{
	Content:    in.Content,
	IntentHint: in.IntentHint,
	Context:    in.Context,
})
```

改为：

```go
out, orchErr := s.orchestrator.Run(ctx, ai_assistant.RunInput{
	Content:    in.Content,
	IntentHint: in.IntentHint,
	Sources:    in.Sources,
	Context:    in.Context,
})
```

同时，找到第 301 行附近的 `explicitAssistantRequest` 判定：

```go
explicitAssistantRequest := strings.TrimSpace(in.IntentHint) != "" || !requestContextEmpty(in.Context)
```

改为：

```go
explicitAssistantRequest := strings.TrimSpace(in.IntentHint) != "" ||
	!requestContextEmpty(in.Context) ||
	len(in.Sources) > 0
```

理由：用户显式打了 `@PubMed` 但没打其它意图标签的 corner case 下，`IntentHint` 是 `external_search`（非空），所以这一行实际不会触发；但加上 `len(Sources)>0` 是防御性的——确保即便上层传 sources 而 IntentHint 漏填，也走 orchestrator 路径。

- [ ] **Step 3: 编译检查**

```bash
go build ./...
```

预期：成功，无报错。

- [ ] **Step 4: 跑全包测试，确认无回归**

```bash
go test ./internal/service/ai_conversation/...
```

预期：PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/service/ai_conversation/types.go internal/service/ai_conversation/service.go
git commit -m "$(cat <<'EOF'
Pipe Sources from SendMessageInput into orchestrator

ai_conversation now accepts an optional Sources slice and forwards it to
ai_assistant.RunInput, so the external_search tool can scope to a
user-chosen subset. explicitAssistantRequest also treats a non-empty
Sources as a signal to take the orchestrator path.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: HTTP handler 解码 `sources` 字段

**Files:**
- Modify: `internal/handler/ai_conversation.go:141-202`
- Test: `internal/handler/ai_conversation_test.go`（追加用例）

- [ ] **Step 1: 写失败测试**

打开 `internal/handler/ai_conversation_test.go`，在合适位置（参照已有的 `figure_lookup` 测试，约第 110 行）追加：

```go
func TestPostMessageDecodesSources(t *testing.T) {
	svc := &fakeAIConversationService{
		sendMessageFn: func(ctx context.Context, in ai_conversation.SendMessageInput, _ func(string) error) (ai_conversation.SendMessageResult, error) {
			if in.IntentHint != "external_search" {
				t.Fatalf("intent_hint = %q, want external_search", in.IntentHint)
			}
			if !reflect.DeepEqual(in.Sources, []string{"pubmed"}) {
				t.Fatalf("sources = %+v, want [pubmed]", in.Sources)
			}
			return ai_conversation.SendMessageResult{}, nil
		},
	}
	h := NewAIConversationHandler(svc)

	body := strings.NewReader(`{"content":"找综述","intent_hint":"external_search","sources":["pubmed"]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/conversations/new/messages", body)
	rec := httptest.NewRecorder()
	h.PostMessage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}
```

如有需要 import：

```go
"reflect"
```

注：如果 `fakeAIConversationService` 当前没有 `sendMessageFn` 这个 hook，参考已有相似测试（约第 100-130 行）的 stub 模式。**如果现有 fake 是直接 panic stub**，则需要给它加一个可注入的 `sendMessageFn` 字段——参考最小改动：把现有 `SendMessage` 方法改为：

```go
func (f *fakeAIConversationService) SendMessage(ctx context.Context, in ai_conversation.SendMessageInput, deltaFn func(string) error) (ai_conversation.SendMessageResult, error) {
	if f.sendMessageFn != nil {
		return f.sendMessageFn(ctx, in, deltaFn)
	}
	return ai_conversation.SendMessageResult{}, nil
}
```

- [ ] **Step 2: 跑测试看它失败**

```bash
go test ./internal/handler/ -run TestPostMessageDecodesSources
```

预期：FAIL（`sources` 解析为空）。

- [ ] **Step 3: 改 handler，解码 `sources`**

`internal/handler/ai_conversation.go` 第 141-149 行 struct 改为：

```go
var body struct {
	Content                 string                      `json:"content"`
	PaperID                 int64                       `json:"paper_id,omitempty"`
	StrictEvidence          *bool                       `json:"strict_evidence,omitempty"`
	IncludeExternalEvidence bool                        `json:"include_external_evidence,omitempty"`
	IntentHint              string                      `json:"intent_hint,omitempty"`
	Sources                 []string                    `json:"sources,omitempty"`
	ReplaceLast             bool                        `json:"replace_last,omitempty"`
	Context                 ai_assistant.RequestContext `json:"context,omitempty"`
}
```

第 190-199 行 `h.svc.SendMessage(...)` 调用改为：

```go
res, err := h.svc.SendMessage(r.Context(), ai_conversation.SendMessageInput{
	ConversationID:          conversationID,
	Content:                 body.Content,
	PaperID:                 body.PaperID,
	IncludeExternalEvidence: body.IncludeExternalEvidence,
	IntentHint:              body.IntentHint,
	Sources:                 body.Sources,
	Context:                 body.Context,
	OnEvent: func(event ai_conversation.StreamEvent) error {
		return send(map[string]interface{}{"type": event.Type, "data": event.Data})
	},
}, func(delta string) error {
	return send(map[string]interface{}{"type": "delta", "delta": delta})
})
```

- [ ] **Step 4: 跑测试确认通过 + 跑整个 handler 包**

```bash
go test ./internal/handler/...
```

预期：PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/handler/ai_conversation.go internal/handler/ai_conversation_test.go
git commit -m "$(cat <<'EOF'
Decode sources field on POST /api/ai/conversations/:id/messages

Adds an optional sources array to the request body and forwards it to
SendMessageInput. The field is ignored when absent, so existing clients
keep their behavior.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: `ExternalSearchTool` 用户层覆盖 + `ErrSourceDisabled`

**Files:**
- Modify: `internal/service/ai_assistant/external_search_tool.go`
- Test: `internal/service/ai_assistant/tools_test.go` 或新建 `external_search_tool_user_override_test.go`

这一步是核心。逻辑：当 `in.Sources` 非空，把 `enabledSources` 与 `in.Sources` 做交集；对在 `in.Sources` 中但未启用的源，加一条 `SourceFailure{Source, Err: ErrSourceDisabled}`。

- [ ] **Step 1: 找现有的 enabled-sources 处理位置**

打开 `internal/service/ai_assistant/external_search_tool.go`，定位第 115-126 行的：

```go
if enabledSources, ok, err := enabledExternalSearchSources(ctx, t); err != nil {
	// ... existing error path
} else if ok {
	sourceQueries = filterExternalSourceQueries(sourceQueries, enabledSources)
}
```

这个分支就是要扩展的地方：在 `ok` 为 true 后、`filterExternalSourceQueries` 之前，应用 `in.Sources` 限制。

- [ ] **Step 2: 写失败测试 — 在 tools_test.go 末尾追加**

打开 `internal/service/ai_assistant/tools_test.go`，先扫一下里面 `enabledSourceQueryCapturingExternalSearch` 的定义（line ~97），它实现了 `Search` 和 `EnabledExternalSources`。我们要复用类似 stub。在文件末尾追加：

```go
func TestExternalSearchToolHonoursUserSourcesOverride(t *testing.T) {
	stub := &enabledSourceQueryCapturingExternalSearch{
		enabled: []ai_external.SourceID{ai_external.SourcePubMed, ai_external.SourceSemanticScholar},
		result: ai_external.SearchResult{
			Sources: []ai_external.SourceID{ai_external.SourcePubMed},
			Papers:  []ai_external.Paper{{Source: ai_external.SourcePubMed, Title: "PMA"}},
		},
	}
	tool := NewExternalSearchTool(stub)
	res, err := tool.Run(context.Background(), ToolInput{
		Query:   "找一下",
		Sources: []string{"pubmed"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Verify the searcher was called with PubMed-only queries.
	if _, ok := stub.lastQueries[ai_external.SourcePubMed]; !ok {
		t.Fatalf("expected PubMed in queries, got %+v", stub.lastQueries)
	}
	if _, ok := stub.lastQueries[ai_external.SourceSemanticScholar]; ok {
		t.Fatalf("Semantic Scholar should be excluded, got %+v", stub.lastQueries)
	}
	_ = res
}

func TestExternalSearchToolReportsDisabledSourceWhenExplicitlyRequested(t *testing.T) {
	stub := &enabledSourceQueryCapturingExternalSearch{
		enabled: []ai_external.SourceID{ai_external.SourcePubMed}, // S2 NOT enabled
		result: ai_external.SearchResult{
			Sources: []ai_external.SourceID{ai_external.SourcePubMed},
			Papers:  []ai_external.Paper{{Source: ai_external.SourcePubMed, Title: "PMA"}},
		},
	}
	tool := NewExternalSearchTool(stub)
	res, err := tool.Run(context.Background(), ToolInput{
		Query:   "找一下",
		Sources: []string{"pubmed", "semantic_scholar"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Process note must mention that Semantic Scholar was disabled.
	if !strings.Contains(res.Process.Note, "Semantic Scholar") || !strings.Contains(res.Process.Note, "未启用") {
		t.Fatalf("expected note to mention disabled S2, got %q", res.Process.Note)
	}
}

func TestExternalSearchToolWithEmptySourcesIsUnchanged(t *testing.T) {
	stub := &enabledSourceQueryCapturingExternalSearch{
		enabled: []ai_external.SourceID{ai_external.SourcePubMed, ai_external.SourceSemanticScholar},
		result: ai_external.SearchResult{
			Sources: []ai_external.SourceID{ai_external.SourcePubMed, ai_external.SourceSemanticScholar},
			Papers:  []ai_external.Paper{{Source: ai_external.SourcePubMed, Title: "PMA"}},
		},
	}
	tool := NewExternalSearchTool(stub)
	_, err := tool.Run(context.Background(), ToolInput{Query: "找一下"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Both sources should appear in queries.
	if _, ok := stub.lastQueries[ai_external.SourcePubMed]; !ok {
		t.Fatalf("expected PubMed in queries")
	}
	if _, ok := stub.lastQueries[ai_external.SourceSemanticScholar]; !ok {
		t.Fatalf("expected Semantic Scholar in queries")
	}
}
```

如有需要：在 `import` 块加 `"strings"` 和 `"github.com/xuzhougeng/citebox/internal/service/ai_external"`（如未已经引入）。

注：测试假设 `enabledSourceQueryCapturingExternalSearch` 已有 `enabled` / `result` / `lastQueries` 字段。先打开 `tools_test.go` 第 97 行附近确认。如果它的字段名不同，按它的实际签名调整测试即可。

- [ ] **Step 3: 跑测试看它失败**

```bash
go test ./internal/service/ai_assistant/ -run TestExternalSearchTool
```

预期：第一个测试 FAIL（S2 仍在 queries 里），第三个 PASS（保持原行为），第二个 FAIL（note 里没有 disabled 提示）。

- [ ] **Step 4: 实现 `ErrSourceDisabled` + sources 交集逻辑**

`internal/service/ai_assistant/external_search_tool.go` 文件顶部 `import` 后新增：

```go
// ErrSourceDisabled signals that the user explicitly named an external source
// (e.g. via @PubMed) that is not enabled in the user's settings.
var ErrSourceDisabled = errors.New("ai_assistant: external source is not enabled in settings")
```

然后在 `Run` 里的 enabled-sources 分支（约 115-126 行）改为：

```go
if enabledSources, ok, err := enabledExternalSearchSources(ctx, t); err != nil {
	searchQueries := flattenExternalSourceQueries(sourceQueries)
	inputJSON, _ := json.Marshal(struct {
		Query           string              `json:"query"`
		SearchQueries   []string            `json:"search_queries,omitempty"`
		QueriesBySource map[string][]string `json:"queries_by_source,omitempty"`
		Limit           int                 `json:"limit"`
	}{Query: in.Query, SearchQueries: searchQueries, QueriesBySource: sourceQueriesForJSON(sourceQueries), Limit: limit})
	return externalSearchFailedResult(inputJSON, err, externalPlanningStages(t, sourceQueries, plan, planErr), searchQueries), nil
} else if ok {
	// User-explicit override: if in.Sources is non-empty, narrow execution
	// further to the intersection. Disabled-but-requested sources are reported
	// as ErrSourceDisabled below so the user sees a clear error card.
	executionSources := enabledSources
	disabledRequested := []ai_external.SourceID(nil)
	if len(in.Sources) > 0 {
		executionSources, disabledRequested = intersectUserSources(in.Sources, enabledSources)
	}
	sourceQueries = filterExternalSourceQueries(sourceQueries, executionSources)
	if len(disabledRequested) > 0 {
		// Stash for the post-search merge: we synthesise SourceFailure entries
		// after the searcher returns so the existing failure-merge code path
		// handles UI rendering uniformly.
		ctx = withDisabledRequestedSources(ctx, disabledRequested)
	}
}
```

接下来，**在文件末尾或合适位置**追加两个 helper：

```go
// intersectUserSources returns (intersection, disabled).
// disabled = sources the user requested but that are not in enabled.
// Unknown source names are silently dropped — they're handled later by the
// "source not configured" branch.
func intersectUserSources(requested []string, enabled []ai_external.SourceID) ([]ai_external.SourceID, []ai_external.SourceID) {
	enabledSet := make(map[ai_external.SourceID]bool, len(enabled))
	for _, s := range enabled {
		enabledSet[s] = true
	}
	intersection := make([]ai_external.SourceID, 0, len(requested))
	disabled := make([]ai_external.SourceID, 0)
	seen := map[ai_external.SourceID]bool{}
	for _, raw := range requested {
		s := ai_external.SourceID(strings.TrimSpace(strings.ToLower(raw)))
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		if enabledSet[s] {
			intersection = append(intersection, s)
		} else {
			disabled = append(disabled, s)
		}
	}
	return intersection, disabled
}

type disabledRequestedKey struct{}

func withDisabledRequestedSources(ctx context.Context, sources []ai_external.SourceID) context.Context {
	return context.WithValue(ctx, disabledRequestedKey{}, sources)
}

func disabledRequestedSourcesFrom(ctx context.Context) []ai_external.SourceID {
	v := ctx.Value(disabledRequestedKey{})
	if v == nil {
		return nil
	}
	if s, ok := v.([]ai_external.SourceID); ok {
		return s
	}
	return nil
}
```

最后，**把 disabled 信息合入失败列表**——找到 `Run` 里 `searchErrs := externalSearchFailures(searchRes, searchErr)`（约 146 行）后面，**在它之后**插入：

```go
if disabled := disabledRequestedSourcesFrom(ctx); len(disabled) > 0 {
	for _, s := range disabled {
		searchErrs = append(searchErrs, fmt.Errorf("%s: %w", externalSourceLabel(s), ErrSourceDisabled))
	}
}
```

并且在 `noteParts` 构造时（约 244 行附近）追加一条对 disabled 的提示。找到：

```go
if len(searchErrs) > 0 && len(candidates) > 0 {
	noteParts = append(noteParts, fmt.Sprintf("外部学术搜索部分失败 %d 个: %s。", len(searchErrs), combineExternalSearchErrors(searchErrs).Error()))
}
```

把它替换为：

```go
if len(searchErrs) > 0 {
	disabledNote := disabledSourcesNote(searchErrs)
	if disabledNote != "" {
		noteParts = append(noteParts, disabledNote)
	}
	if len(candidates) > 0 {
		noteParts = append(noteParts, fmt.Sprintf("外部学术搜索部分失败 %d 个: %s。", len(searchErrs), combineExternalSearchErrors(searchErrs).Error()))
	}
}
```

以及在 helper 区追加：

```go
func disabledSourcesNote(errs []error) string {
	parts := make([]string, 0, len(errs))
	for _, err := range errs {
		if errors.Is(err, ErrSourceDisabled) {
			parts = append(parts, err.Error())
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "用户显式指定但未启用的源: " + strings.Join(parts, "; ") + "（请前往设置页启用）"
}
```

注意：`fmt.Errorf("%s: %w", label, ErrSourceDisabled)` 的字符串里就已经包含「Semantic Scholar: ...」，所以测试用的 "Semantic Scholar" + "未启用" 检查能命中（"未启用" 来自 `disabledSourcesNote` 拼出的 "用户显式指定但未启用的源"）。

- [ ] **Step 5: 跑测试，确认全部通过 + 跑整个包**

```bash
go test ./internal/service/ai_assistant/...
```

预期：所有用例 PASS。如果某个旧测试失败（说明它依赖了 disabled-source 的旧行为），评估其断言并按新语义调整。

- [ ] **Step 6: 提交**

```bash
git add internal/service/ai_assistant/external_search_tool.go internal/service/ai_assistant/tools_test.go
git commit -m "$(cat <<'EOF'
Honour user-supplied Sources in ExternalSearchTool

When ToolInput.Sources is non-empty, narrow execution to the intersection
of user-requested and enabled sources. Sources requested but disabled in
settings surface as ErrSourceDisabled in the failure list and as a clear
process note ("未启用，请前往设置页启用"), instead of being silently
ignored or auto-enabled. Empty Sources keeps existing behaviour.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: `LibrarySearchTool` 在显式 hint 时按 PaperIDs 限定

**Files:**
- Modify: `internal/service/ai_assistant/library_search_tool.go:73-145`
- Test: `internal/service/ai_assistant/tools_test.go`（或同包测试文件）追加用例

- [ ] **Step 1: 写失败测试**

打开 `internal/service/ai_assistant/library_search_tool_test.go`（如不存在则新建），追加：

```go
package ai_assistant

import (
	"context"
	"testing"

	"github.com/xuzhougeng/citebox/internal/model"
)

type stubPaperGetter struct {
	papers map[int64]*model.Paper
	listFn func(terms []string, limit int) ([]int64, error)
}

func (s *stubPaperGetter) GetPaperDetail(id int64) (*model.Paper, error) {
	return s.papers[id], nil
}

func (s *stubPaperGetter) ListEvidenceCandidatePaperIDs(terms []string, limit int) ([]int64, error) {
	if s.listFn != nil {
		return s.listFn(terms, limit)
	}
	return nil, nil
}

func TestLibrarySearchScopesToContextPaperIDsWhenExplicit(t *testing.T) {
	getter := &stubPaperGetter{
		papers: map[int64]*model.Paper{
			11: {ID: 11, Title: "Paper 11", AbstractText: "ATAC-seq study"},
			12: {ID: 12, Title: "Paper 12", AbstractText: "ATAC-seq study"},
			13: {ID: 13, Title: "Paper 13", AbstractText: "ATAC-seq study"},
		},
		listFn: func(terms []string, limit int) ([]int64, error) {
			return []int64{11, 12, 13}, nil
		},
	}
	tool := NewLibrarySearchTool(getter)

	res, err := tool.Run(context.Background(), ToolInput{
		Query:      "找 ATAC",
		IntentHint: IntentLibrarySearch,
		Context:    RequestContext{PaperIDs: []int64{11, 13}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Cards should include 11 and 13, but NOT 12.
	got := map[int64]bool{}
	for _, card := range res.Cards {
		if hit, ok := card.Payload.(PaperHitCard); ok {
			got[hit.PaperID] = true
		}
	}
	if !got[11] || !got[13] {
		t.Fatalf("expected papers 11 and 13, got %+v", got)
	}
	if got[12] {
		t.Fatalf("paper 12 should be excluded by PaperIDs scoping")
	}
}

func TestLibrarySearchIgnoresPaperIDsWhenIntentHintIsEmpty(t *testing.T) {
	getter := &stubPaperGetter{
		papers: map[int64]*model.Paper{
			11: {ID: 11, Title: "Paper 11", AbstractText: "ATAC-seq study"},
			12: {ID: 12, Title: "Paper 12", AbstractText: "ATAC-seq study"},
		},
		listFn: func(terms []string, limit int) ([]int64, error) { return []int64{11, 12}, nil },
	}
	tool := NewLibrarySearchTool(getter)

	res, err := tool.Run(context.Background(), ToolInput{
		Query:   "找 ATAC",
		Context: RequestContext{PaperIDs: []int64{11}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Without explicit IntentHint=library_search, scoping should NOT apply.
	got := map[int64]bool{}
	for _, card := range res.Cards {
		if hit, ok := card.Payload.(PaperHitCard); ok {
			got[hit.PaperID] = true
		}
	}
	if !got[11] || !got[12] {
		t.Fatalf("expected both 11 and 12 without scoping, got %+v", got)
	}
}
```

- [ ] **Step 2: 跑测试看它失败**

```bash
go test ./internal/service/ai_assistant/ -run TestLibrarySearchScopes
```

预期：第一个 FAIL（paper 12 出现在结果里），第二个 PASS（同今天行为）。

- [ ] **Step 3: 在 LibrarySearchTool.Run 里加 scoping**

打开 `internal/service/ai_assistant/library_search_tool.go`，第 110 行附近：

```go
ids, candidateErr := candidateIDs(t.papers, terms, 120)
```

在它**之后**插入：

```go
if in.IntentHint == IntentLibrarySearch && len(in.Context.PaperIDs) > 0 {
	ids = scopePaperIDs(ids, in.Context.PaperIDs)
}
```

并在文件末尾（紧贴最后一个 helper，约第 755 行附近）追加：

```go
// scopePaperIDs intersects ids with allowed in input order. Used when the
// user explicitly asked @Library while @-mentioning specific papers.
func scopePaperIDs(ids []int64, allowed []int64) []int64 {
	if len(allowed) == 0 {
		return ids
	}
	allowedSet := make(map[int64]bool, len(allowed))
	for _, id := range allowed {
		if id > 0 {
			allowedSet[id] = true
		}
	}
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if allowedSet[id] {
			out = append(out, id)
		}
	}
	return out
}
```

- [ ] **Step 4: 跑测试，确认通过 + 跑整个包**

```bash
go test ./internal/service/ai_assistant/...
```

预期：所有用例 PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/service/ai_assistant/library_search_tool.go internal/service/ai_assistant/library_search_tool_test.go
git commit -m "$(cat <<'EOF'
Scope LibrarySearchTool to Context.PaperIDs when explicitly hinted

When IntentHint == library_search and Context.PaperIDs is non-empty, the
candidate id list is filtered to the user-supplied set. This supports
"@Library @<paper>" composition where the user wants to search only inside
specific papers. The default (auto-routed) path is unchanged.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: 设置页加 `id="settings-external-sources"` 锚点

**Files:**
- Modify: `web/settings.html:163-185`

- [ ] **Step 1: 编辑 `web/settings.html`**

找到第 164 行附近的：

```html
<section class="prompt-preset-panel">
    <div class="prompt-preset-head">
        <div>
            <h4 data-i18n="settings.ai.external_search.title">AI 外部搜索源</h4>
```

把 `<section>` 的开标签改为：

```html
<section class="prompt-preset-panel" id="settings-external-sources">
```

- [ ] **Step 2: 浏览器手动验证**

```bash
make dev   # 或者你常用的本地开发命令；如果不确定，请先看 Makefile
```

打开 `/settings#settings-external-sources`，确认页面跳到「AI 外部搜索源」section。如果默认开发命令未知，至少 `grep -n 'settings-external-sources' web/settings.html` 确认 id 已写入。

- [ ] **Step 3: 提交**

```bash
git add web/settings.html
git commit -m "$(cat <<'EOF'
Add id anchor for AI external search settings section

Lets the @ palette deep-link to the external sources panel when the user
clicks a disabled source row.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: ai-reader.js 缓存启用的外部源

**Files:**
- Modify: `web/static/js/ai-reader.js:17-37`

- [ ] **Step 1: 在 init() 里追加一次 GET**

`web/static/js/ai-reader.js` 第 17-37 行 `init()` 改为：

```js
async init() {
    // Cache settings (best-effort)
    try {
        const res = await fetch('/api/ai/settings');
        if (res.ok) {
            const body = await res.json();
            this.settings = body || null;
        }
    } catch (e) { /* offline / fresh install — leave settings null */ }
    // Also cache the enabled external-search sources so the @ palette can
    // gray out disabled rows. Best-effort: failure leaves the set null and
    // the palette will treat all known sources as enabled.
    try {
        const res = await fetch('/api/settings/ai-external-search');
        if (res.ok) {
            const body = await res.json();
            this.externalSourcesEnabled = Array.isArray(body && body.sources)
                ? body.sources.map((s) => String(s || '').toLowerCase())
                : null;
        }
    } catch (e) { /* offline — leave null */ }
    window.AIReader = window.AIReader || {};
    window.AIReader.settings = this.settings;
    window.AIReader.externalSourcesEnabled = this.externalSourcesEnabled;

    // Fire-and-forget: cache the library so the @ palette can offer
    // every paper, not just the ones already pinned.
    this._loadAllPapers();

    this._initModules();
    this._bindTitleRename();
    this._bindQuestionMirror();
    await this._dispatchEntry();
},
```

并在 `Reader` 对象的字段处（约第 13-15 行）加：

```js
const Reader = {
    settings: null,
    externalSourcesEnabled: null,
    _allPapers: [],
    // ...
};
```

- [ ] **Step 2: 浏览器手动验证（可选）**

启动 dev server，打开 `/ai`，DevTools console 跑：

```js
window.AIReader.externalSourcesEnabled
```

应当是 `["pubmed"]` 或 `["pubmed","semantic_scholar"]` 或 `null`。

- [ ] **Step 3: 提交**

```bash
git add web/static/js/ai-reader.js
git commit -m "$(cat <<'EOF'
Cache enabled external sources for @ palette

Best-effort GET to /api/settings/ai-external-search at page boot. Used by
the upcoming tool/source palette section to render disabled sources in a
grayed-out row that links to settings.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: 扩展 `ai-mention.js` 的 providers 契约 + 渲染「工具/数据源」段

**Files:**
- Modify: `web/static/js/ai-mention.js`
- Modify: `web/ai.html:108-119`

这是最大的前端改动。注意把所有改动控制在 `ai-mention.js` 内，避免改动其它模块。

- [ ] **Step 1: 在 `ai.html` 引入新模块**

`web/ai.html` 第 110 行附近，把：

```html
<script src="/static/js/ai-mention.js?v=20260501-p3"></script>
```

替换为：

```html
<script src="/static/js/ai-mention-tags.js?v=20260501-p4"></script>
<script src="/static/js/ai-mention.js?v=20260501-p4"></script>
```

注意：`ai-mention-tags.js` 必须在 `ai-mention.js` 之前加载，因为后者会读取 `window.AIReader.toolTags`。

- [ ] **Step 2: 扩展 providers 契约 (ai-mention.js)**

打开 `web/static/js/ai-mention.js`，找到第 12-17 行的 providers 契约注释。在末尾追加：

```js
//   - getToolTags()        => ToolTagItem[]    (the four MVP tool tags;
//                              each {name, family, source, description, disabled, disabledReason})
//   - onPickToolTag(tag)   => void             (called when user picks a tool tag)
//   - onPickDisabledTag(tag) => void           (called when user clicks a disabled tag row;
//                              implementations should navigate to settings)
```

- [ ] **Step 3: 在 `_buildItems` 之前插入 tool tags**

找到 `_buildItems(query)` (第 237 行附近)，在「Role prompts first.」注释**之前**插入工具/源的注入逻辑：

```js
_buildItems(query) {
    const s = this._state;
    if (!s.providers) return [];
    const q = String(query || '').trim().toLowerCase();
    const items = [];

    // Tool/source tags first — they change routing, so they outrank roles
    // and papers visually.
    const toolTags = typeof s.providers.getToolTags === 'function'
        ? s.providers.getToolTags()
        : [];
    toolTags
        .filter((tag) => {
            if (!q) return true;
            const haystack = [tag.name, tag.description].filter(Boolean).join(' ').toLowerCase();
            return haystack.includes(q);
        })
        .forEach((tag) => {
            items.push({
                type: 'tool',
                name: tag.name,
                title: '@' + tag.name,
                meta: tag.disabled
                    ? (tag.disabledReason || tr('ai.mention_tool_disabled', '未启用，前往设置 →'))
                    : (tag.description || ''),
                disabled: !!tag.disabled,
                _raw: tag,
            });
        });

    // Role prompts second.
    const roles = typeof s.providers.getRolePrompts === 'function'
        ? s.providers.getRolePrompts()
        : [];
    // (existing role logic remains as-is)
```

把已有的 `// Role prompts first.` 注释更新为 `// Role prompts second.`，其余不变。同样 papers 的注释里 "Papers second" 改为 "Papers third"。

- [ ] **Step 4: 在 `_renderItems` 里渲染新分段**

找到 `_renderItems()` (约第 304 行)。里面有 `roleItems = items.filter(...)` 和 `paperItems = items.filter(...)`。在它之前加：

```js
const toolItems = items.filter((it) => it.type === 'tool');
```

并在 `sections.push(...)` 列表的最前面（即在 `roleItems.length` 那段之前）插入：

```js
if (toolItems.length) {
    const toolHTML = toolItems.map((item) => renderItem(item, cursor++)).join('');
    sections.push(
        '<div class="ai-mention-section" data-section="tools">' +
        '<div class="ai-mention-section-title">' +
        escapeHTML(tr('ai.mention_section_tools', '工具/数据源')) +
        '</div>' +
        '<ul class="ai-mention-list" role="presentation">' + toolHTML + '</ul>' +
        '</div>'
    );
}
```

并在 `renderItem` 里给 disabled 行加 class：找到 `const current = item.isCurrent ? ' is-current' : '';` 那一行，**之后**追加：

```js
const disabled = item.disabled ? ' is-disabled' : '';
```

并把后面的 `<li class="ai-mention-item' + active + current +` 改为 `<li class="ai-mention-item' + active + current + disabled +`。

`icon` 那行做小调整：

```js
let icon = '@';
if (item.type === 'paper') icon = '📄';
else if (item.type === 'tool') icon = '⚙';
```

`dataset` 部分加 tool 分支：

```js
let dataset;
if (item.type === 'role') {
    dataset = 'data-mention-type="role" data-role-name="' + escapeHTML(item.name) + '"';
} else if (item.type === 'tool') {
    dataset = 'data-mention-type="tool" data-tool-name="' + escapeHTML(item.name) +
              '" data-tool-disabled="' + (item.disabled ? '1' : '0') + '"';
} else {
    dataset = 'data-mention-type="paper" data-paper-id="' + item.id + '"';
}
```

- [ ] **Step 5: 扩展 `_commitChoice` 处理 tool 选项**

找到 `_commitChoice(input, item)` (约第 407 行)。在 `if (item.type === 'role')` 块**之前**加：

```js
if (item.type === 'tool') {
    if (item.disabled) {
        if (s.providers && typeof s.providers.onPickDisabledTag === 'function') {
            s.providers.onPickDisabledTag(item._raw);
        }
        // Don't insert into textarea — keep the popover dismissed silently.
        this.dismiss();
        input.focus();
        return;
    }
    // Use shared commitToolTag for cross-family rewrite logic.
    const commit = (window.AIReader && window.AIReader.toolTags && window.AIReader.toolTags.commitToolTag)
        || null;
    if (!commit) {
        // Fallback: behave like a plain mention insert, no rewrite.
        const mention = '@' + item.name + ' ';
        const beforeAt = Number.isFinite(s.atIndex) && s.atIndex >= 0
            ? input.value.slice(0, s.atIndex)
            : input.value.slice(0, input.selectionStart);
        const after = input.value.slice(input.selectionStart);
        input.value = beforeAt + mention + after;
        const newCaret = beforeAt.length + mention.length;
        input.setSelectionRange(newCaret, newCaret);
    } else {
        const result = commit(
            { value: input.value, selectionStart: input.selectionStart, atIndex: s.atIndex, query: s.query },
            item.name
        );
        input.value = result.value;
        input.setSelectionRange(result.caret, result.caret);
    }
    if (s.providers && typeof s.providers.onPickToolTag === 'function') {
        s.providers.onPickToolTag(item._raw);
    }
    this.dismiss();
    input.focus();
    input.dispatchEvent(new Event('input', { bubbles: true }));
    return;
}
```

- [ ] **Step 6: 在 ai-reader.js 的 mention.attach 里实现新的 providers 方法**

打开 `web/static/js/ai-reader.js` 第 143 行附近。在传给 `mention.attach` 的 providers 对象里，**在 `getRolePrompts` 后** 追加：

```js
getToolTags: () => {
    const enabled = (window.AIReader && Array.isArray(window.AIReader.externalSourcesEnabled))
        ? new Set(window.AIReader.externalSourcesEnabled)
        : null; // null = "settings unknown, treat all as enabled"
    const known = (window.AIReader && window.AIReader.toolTags && window.AIReader.toolTags.KNOWN_TOOL_TAGS) || [];
    return known.map((t) => {
        const isExternal = t.family === 'external';
        const isDisabled = isExternal && enabled !== null && !enabled.has(t.source);
        let description;
        if (t.name === 'PubMed') description = '外部源 · PubMed';
        else if (t.name === 'SemanticScholar') description = '外部源 · Semantic Scholar';
        else if (t.name === 'Library') description = '本地文本检索（不含图）';
        else if (t.name === 'Figure') description = '本地图片检索';
        return {
            name: t.name,
            family: t.family,
            source: t.source,
            description: description,
            disabled: isDisabled,
            disabledReason: isDisabled ? '未启用，前往设置 →' : '',
        };
    });
},
onPickToolTag: () => { /* nothing — value is read on submit */ },
onPickDisabledTag: () => {
    window.location.href = '/settings#settings-external-sources';
},
```

- [ ] **Step 7: 加最小 CSS — 让 `.is-disabled` 行视觉灰掉**

打开 `web/static/css/style.css`（如果用的是其它样式入口，根据 `<link rel="stylesheet">` 找——`web/ai.html:9` 写 `style.css`）。直接 grep 已有的 `.ai-mention-item`：

```bash
grep -n '\.ai-mention-item' web/static/css/style.css | head -5
```

定位到现有 `.ai-mention-item.active { ... }` 之后，追加：

```css
.ai-mention-item.is-disabled {
    opacity: 0.55;
    cursor: pointer;
}
.ai-mention-item.is-disabled .ai-mention-item-meta {
    color: var(--text-secondary, #9aa0a6);
}
```

- [ ] **Step 8: 浏览器手动验证**

启动 dev server，访问 `/ai`，在文本框打 `@`：

- 弹层第一段是「工具/数据源」，包含 4 项
- 第二段是「角色 Prompt」，第三段是「文献」
- 在设置页关掉 PubMed 后再回到 `/ai`，弹层里 `@PubMed` 行应当置灰；点击它跳到 `/settings#settings-external-sources`
- 点 `@PubMed`（启用状态下）后，文本框出现 `@PubMed `；继续点 `@Library`，textarea 应当变成 `@Library `（@PubMed 被自动删除）
- 点 `@PubMed` 后再点 `@SemanticScholar`，textarea 应当是 `@PubMed @SemanticScholar `（同族叠加）
- 输入 `@AlphaFold` 这样的 paper mention 后再点 `@Library`，paper mention **不应**被删除

- [ ] **Step 9: 提交**

```bash
git add web/ai.html web/static/js/ai-mention.js web/static/js/ai-reader.js web/static/css/style.css
git commit -m "$(cat <<'EOF'
Render tool/source section in @ palette

Adds a new "工具/数据源" section above roles and papers with the four
MVP tags. Disabled external sources render grayed and route the user to
the settings anchor on click. Cross-family commits delegate to
commitToolTag so existing tool tags from other families are stripped
from the textarea before the new tag is inserted.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: 在 `_sendBody` 里调用 `parseToolTags` 并随请求发出

**Files:**
- Modify: `web/static/js/ai-conversation-view.js:240-285`

- [ ] **Step 1: 编辑 `_sendBody`**

打开 `web/static/js/ai-conversation-view.js`，定位 `sendPayload`（约第 240 行）：

```js
async sendPayload(payload) {
    const content = (payload && payload.content || '').trim();
    if (!content) return;
    const body = { content: content, context: this._currentContext() };
    if (payload && payload.intent_hint) body.intent_hint = payload.intent_hint;
    if (this._state.rewriteLast && this._state.conversationId) body.replace_last = true;
    await this._sendBody(body);
},
```

改为：

```js
async sendPayload(payload) {
    const content = (payload && payload.content || '').trim();
    if (!content) return;
    const body = { content: content, context: this._currentContext() };

    // Parse @ tool tags out of the content. The text is left intact in
    // body.content (so the model sees the user's literal input); the parsed
    // intent + sources ride alongside as routing hints.
    const tt = (window.AIReader && window.AIReader.toolTags && window.AIReader.toolTags.parseToolTags)
        ? window.AIReader.toolTags.parseToolTags(content)
        : { intentHint: '', sources: [], conflict: null };
    if (tt.intentHint) body.intent_hint = tt.intentHint;
    if (Array.isArray(tt.sources) && tt.sources.length > 0) body.sources = tt.sources;
    if (tt.conflict) {
        console.warn('[mention] dropped tool tag family=' + tt.conflict.dropped +
            ' due to family=' + tt.conflict.kept);
    }

    // Caller-supplied intent_hint (e.g., the shortcut buttons) overrides
    // anything we parsed — keep the existing precedence.
    if (payload && payload.intent_hint) body.intent_hint = payload.intent_hint;
    if (this._state.rewriteLast && this._state.conversationId) body.replace_last = true;
    await this._sendBody(body);
},
```

- [ ] **Step 2: 浏览器端到端验证**

启动 dev server，在 `/ai` 的对话框输入：

```
@PubMed 帮我找最近的单细胞测序综述
```

提交后用 DevTools Network 面板看 `/api/ai/conversations/.../messages` 的请求体，应当包含：

```json
{ "content": "@PubMed 帮我找最近的单细胞测序综述",
  "intent_hint": "external_search",
  "sources": ["pubmed"],
  ... }
```

并在响应面板里看到 process 阶段为「外部搜索」相关，且只用 PubMed。

再试：

```
@SemanticScholar 但只 S2 启用 → @PubMed @SemanticScholar foo
```

应当返回 PubMed 命中卡片 + 一条 disabled 提示卡片或 process note 提到 Semantic Scholar 未启用。

再试：

```
@Library @AlphaFold 总结这篇
```

（先 @ 一篇 paper），应当只在该 paper 范围内做本地全文检索。

- [ ] **Step 3: 提交**

```bash
git add web/static/js/ai-conversation-view.js
git commit -m "$(cat <<'EOF'
Send parsed tool tags as intent_hint + sources on submit

Before posting a turn, parseToolTags is run against the textarea content;
its intentHint and sources are attached to the request body. The original
content is left intact so the model sees the user's literal input.
Existing IntentHint precedence (shortcut buttons) is preserved.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: 文档更新

**Files:**
- Modify: `docs/api.md`

- [ ] **Step 1: 找到 POST `/api/ai/conversations/:id/messages` 章节**

```bash
grep -n 'POST.*conversations\|/messages' docs/api.md | head -10
```

- [ ] **Step 2: 在请求体字段表里追加 `sources`**

在该章节的请求体描述里，按现有风格追加一行（参考 `intent_hint` 那一行的格式）：

```markdown
- `sources` (array of strings, optional) — Explicit list of external search
  sources to use (`pubmed`, `semantic_scholar`). Only consulted when
  `intent_hint` is `external_search`. Sources requested but disabled in
  user settings are reported as failures (`ErrSourceDisabled`); empty or
  omitted means "use all enabled sources" (current default behaviour).
```

如果文档使用其它结构（例如表格），则按表格格式追加一行。

- [ ] **Step 3: 提交**

```bash
git add docs/api.md
git commit -m "$(cat <<'EOF'
Document sources field on AI conversation messages endpoint

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 12: 全量回归 + 手动验收清单

- [ ] **Step 1: 跑后端全部测试**

```bash
go test ./...
```

预期：全部 PASS。

- [ ] **Step 2: 跑前端纯函数测试**

```bash
node --test web/static/js/__tests__/ai-mention-tags.test.cjs
```

预期：全部 PASS。

- [ ] **Step 3: 手动浏览器 smoke test 清单**

启动 dev server（按项目惯例，例如 `make dev` 或 `go run ./cmd/...`），打开 `/ai`，逐项验证：

- [ ] 文本框打 `@`，弹层第一段是「工具/数据源」，4 项
- [ ] 在设置页禁用 PubMed 后回 `/ai`，`@PubMed` 行置灰
- [ ] 点击灰掉的 `@PubMed`，浏览器跳转到 `/settings#settings-external-sources`，且页面滚到对应 section
- [ ] 选 `@PubMed` → 选 `@SemanticScholar`：textarea 显示 `@PubMed @SemanticScholar `（同族叠加）
- [ ] 选 `@PubMed` → 选 `@Library`：textarea 显示 `@Library `（跨族 last-wins，旧的被擦掉）
- [ ] 选某文献 `@<paper>` → 选 `@Library`：textarea 仍保留 `@<paper>`，新增 `@Library`
- [ ] `@PubMed 找单细胞综述` 提交：DevTools 看请求体 `intent_hint=external_search`、`sources=["pubmed"]`，结果卡片只来自 PubMed
- [ ] `@Library @<某 paper> 总结` 提交：结果卡片仅来自该 paper
- [ ] `@Figure @<某 paper> 找图` 提交：图卡片限定在该 paper 内
- [ ] 不打任何 `@` 工具标签的普通问题：行为完全与今天一致（auto 路由）

如有任何失败项，**不要** mark 此任务完成；回到对应 Task 修复。

- [ ] **Step 4: 总结并合并**

如果全部通过，按项目惯例发起 PR / 合并。

---

## Self-Review

### Spec coverage check

| Spec section | 实现位置 |
|---|---|
| 工具标签清单（4 项） | Task 1 (`KNOWN_TOOL_TAGS`) |
| 同族叠加 / 跨族 last-wins | Task 1 (`parseToolTags`、`commitToolTag`) |
| 工具标签 + `@<文献>` 组合 | Task 6（Library 域限定）+ existing figure_lookup PaperID + Task 5（external 不强制读 PaperIDs，符合"工具自决"） |
| 弹层 3 段（工具最前 / 角色 / 文献） | Task 9 |
| 置灰禁用源 + 跳设置 | Task 7（锚点）+ Task 8（缓存启用集合）+ Task 9（渲染 + 跳转） |
| 前端解析（白名单 + 词边界 + 大小写） | Task 1 |
| `RunInput`/`ToolInput` 加 `Sources` | Task 2 |
| handler / SendMessageInput 透传 | Task 3 + Task 4 |
| `ExternalSearchTool` 用户层覆盖 + `ErrSourceDisabled` | Task 5 |
| `LibrarySearchTool` PaperIDs 限定 | Task 6 |
| `docs/api.md` 字段说明 | Task 11 |
| 测试策略 | Task 1（前端单测）/ Task 2,5,6（后端测试） |

无 spec 要求未覆盖。

### Placeholder scan

- 所有"实现……"、"补……"步骤均给出具体代码或具体检查命令。
- 无 TBD / TODO。
- 浏览器手动测试步骤逐条列出，可勾选。

### Type consistency

- `parseToolTags` 在 Task 1 定义返回 `{ intentHint, sources, conflict }`；Task 10 消费时使用同样字段名。
- `commitToolTag` 输入 `{ value, selectionStart, atIndex, query }`、输出 `{ value, caret }`，Task 1 测试与 Task 9 调用点一致。
- `KNOWN_TOOL_TAGS` 元素为 `{ name, family, source }`，Task 8 providers 消费时使用同样字段名。
- 后端 `Sources []string` 类型在 RunInput / ToolInput / SendMessageInput / handler request body 全链路一致。
- `ErrSourceDisabled` sentinel 定义于 `ai_assistant`，仅本包内使用，无跨包引用问题。

无类型不一致。

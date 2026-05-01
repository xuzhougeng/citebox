# `@` 提及驱动的工具/数据源显式调用 — 设计文档

- **日期**：2026-05-01
- **作者**：xuzhougeng + Claude
- **状态**：待 writing-plans
- **关联**：`docs/superpowers/specs/2026-05-01-ai-external-search-sources-design.md`（前置：外部源开关）

## 背景

AI 助手已经有两类 `@` 提及——「角色 Prompt」和「具体文献」（见 `web/static/js/ai-mention.js`）。后端 `internal/service/ai_assistant/orchestrator.go` 用关键词路由（`router.go`）把请求分到 5 个 intent：`library_search` / `external_search` / `paper_read` / `figure_lookup` / `chat`。

外部检索（`internal/service/ai_external/service.go`）支持 PubMed、Semantic Scholar 两源，但当 `external_search` 命中时，会把**所有启用的源**一起跑，用户没有"只用其中一个"的显式入口。本地文本检索（`library_search`）和图片检索（`figure_lookup`）目前也只能依赖关键词路由触发，不能强制指定。

## 目标

让用户用 `@` 显式选择走哪个**工具/数据源**，覆盖 auto 路由：

- `@PubMed`、`@SemanticScholar` → 仅走该外部源
- `@Library` → 强制走本地文本检索
- `@Figure` → 强制走本地图片检索
- 不打任何工具标签 → 完全保留当前 auto 路由行为

## 非目标

- 不做跨族真并行执行（如 `@Library + @PubMed` 同时跑两个工具，合并结果）。
- 不做 chip / 胶囊视觉，沿用现有 plain-text 提及。
- 不做"自动启用源"的隐式行为。
- 不实现 `@arXiv` / `@Web` 等当前后端尚无对应 source 的标签。
- 不引入 `@<具体 figure>` 这种实例级图片提及。MVP 里 `@Figure` 是工具/动作，不是实例。
- 不改 `RouteIntent` 关键词路由本身。

## 整体架构

```
┌─ 文本框 (#aiQuestionInput) ─────────────────────────────────────┐
│  @PubMed @AlphaFold 这篇有相关进展吗？                            │
└─────────────────────────────────────────────────────────────────┘
            │
            │ ① 用户敲 @ 触发弹层
            ▼
┌─ Mention Popover (#aiMentionPopover) ──────────────────────────┐
│  ▸ 工具/数据源        ← 新增段                                  │
│  ▸ 角色 Prompt         ← 现有                                   │
│  ▸ 文献                ← 现有                                   │
└─────────────────────────────────────────────────────────────────┘
            │
            │ ② 提交时前端 parseToolTags(text)
            ▼
┌─ Orchestrator.Run(RunInput) ──────────────────────────────────┐
│  Content     = 原始文本（含 @ 标签）                             │
│  IntentHint  = 来自工具标签                                      │
│  Sources     = 新增字段，仅外部源族用                            │
│  Context     = 含 PaperIDs（来自 @<paper>）                      │
└────────────────────────────────────────────────────────────────┘
            │
            ▼
   按 IntentHint 选工具 → 工具读 Sources / Context 决定行为
   IntentHint 为空时走现有 RouteIntent
```

整个改动是**显式覆盖层**：用户不打工具标签时，`IntentHint=""`，路由路径与今天一字不差。

## 工具标签清单

| 标签 | 后端 IntentHint | 额外字段 | 检索内容 |
|---|---|---|---|
| `@PubMed` | `external_search` | `Sources=["pubmed"]` | 文本（标题/摘要） |
| `@SemanticScholar` | `external_search` | `Sources=["semantic_scholar"]` | 文本（标题/摘要/snippet） |
| `@Library` | `library_search` | — | 本地文献的标题/摘要/全文/笔记，**不含图** |
| `@Figure` | `figure_lookup` | — | 本地图片，**不含纯文本论文** |

`@Library` 与 `@Figure` 是**正交**通道（一个走文字、一个走图）。

## 多标签语义

- **同族叠加**（外部源族）：`@PubMed @SemanticScholar` → `Sources=["pubmed","semantic_scholar"]`，等价于今天 `external_search` 跑两源。
- **跨族 last-wins**：弹层 commit 时若新选的标签跨族，**先把已有的工具标签从 textarea 删除再插入新的**。textarea 始终所见即所得。手动打字造成的跨族冲突由 `parseToolTags` 在提交时按相同规则归并（保留最后一个族），并在 console 给一条 `dropped/kept` 警告。
- **没打工具标签** → `IntentHint=""`，走 `RouteIntent` auto。

## 工具标签 + `@<文献>` 的组合规则

统一规则：**工具标签决定 intent；`@<文献>` 只是 PaperIDs 上下文，由工具自己决定是否使用**。

| 组合 | 行为 |
|---|---|
| `@PubMed + @<paper>` | external_search，paper 的 title/DOI 作为 query 增强（"find related on PubMed"） |
| `@SemanticScholar + @<paper>` | 同上，走 S2 |
| `@Library + @<paper>` | library_search，paper 作为 PaperIDs 上下文，工具将检索范围限定在这些 paper 内 |
| `@Figure + @<paper>` | figure_lookup 限定该 paper 的图集（与现有 `Context.PaperID > 0` 的过滤逻辑一致） |
| `@<paper1> @<paper2>`（无工具标签） | 走 auto：`len(PaperIDs) > 1` → `paper_read` 对比模式（现有行为） |
| `@<paper>`（无工具标签） | 走 auto：`paper_read`（现有行为） |

## 弹层结构

新弹层 = **3 段**，从顶到底：

1. **工具/数据源**（新）— 4 项常驻：`@PubMed` / `@SemanticScholar` / `@Library` / `@Figure`
   - 每条副标题写一行说明：
     - `@PubMed` → "外部源 · PubMed"
     - `@SemanticScholar` → "外部源 · Semantic Scholar"
     - `@Library` → "本地文本检索（不含图）"
     - `@Figure` → "本地图片检索"
   - **置灰条件**：在设置里被禁用的外部源。副标题改为「未启用，前往设置 →」。点击不 commit 标签，而是导航到 `/settings#settings-external-sources`。该锚点当前在 `web/settings.html` 里**不存在**，实施时需在「AI 外部搜索源」section 上加 `id="settings-external-sources"`（见实施分块 §2）。
2. **角色 Prompt**（现有）
3. **文献**（现有）

排序理由：工具标签会**改变路由**，比角色/文献更"重"，应当最先被看到；常驻 4 项不长，不会挤压下面的列表。

模糊匹配（`q`）逻辑沿用现有 `_buildItems`：对工具名/副标题做 `includes(q)`。

## 前端解析与状态

**单一事实源 = textarea 内容本身**。不引入额外的 `activeTags` 状态对象，避免双向同步 bug。

提交时（`runAIReader` 调用前）做一次纯函数解析：

```js
parseToolTags(text) -> {
  intentHint: 'external_search' | 'library_search' | 'figure_lookup' | '',
  sources:    string[],   // 0..2 项，仅 external_search 时非空
  conflict:   null | { dropped: 'figure'|'library'|'external', kept: 'figure'|'library'|'external' },
}
```

规则：

- 严格词边界正则、有限白名单：`/(^|\s)@(PubMed|SemanticScholar|Library|Figure)\b/gi`
- 大小写不敏感（`@pubmed` = `@PubMed`）
- 同族 → 收集 sources
- 跨族 → last-wins，前面族的标签全部丢弃，记入 `conflict`
- 工具标签**不从 Content 里剥掉**——保留原样发给模型，让模型也能看到用户的显式意图

弹层 commit（`_commitChoice`）的"同族重写"逻辑：

```js
function commitToolTag(input, newTag) {
  const family = familyOf(newTag);            // 'external' | 'library' | 'figure'
  const existing = scanExistingToolTags(input.value);
  const toRemove = existing.filter(t => familyOf(t) !== family);
  // 在跨族情况下，从 textarea 里把 toRemove 全部抠掉
  // 在同族情况下，仅插入新标签（不去重——因为同族叠加是合法的）
  applyTextEdits(input, toRemove, newTag);
}
```

这样 textarea 显示的永远是当前生效的标签集合，所见即所得。

## 后端契约改动

### `internal/service/ai_assistant/types.go`

```go
type RunInput struct {
    Content    string
    IntentHint string
    Sources    []string       // 新增；仅 external_search 工具会读
    Context    RequestContext
}

type ToolInput struct {
    Query      string
    Context    RequestContext
    Limit      int
    IntentHint string
    Sources    []string       // 新增
}
```

`Orchestrator.Run` 将 `RunInput.Sources` 透传到 `ToolInput.Sources`。

### `internal/service/ai_assistant/external_search_tool.go`

`ExternalSearchTool.Run` 增加分支：

- 当 `in.Sources` 非空，作为「用户层覆盖」：
  - 实际**执行集合** = `in.Sources` ∩ 启用集合。
  - 在 `in.Sources` 中但未启用的源，**不静默自动启用**，而是在结果里加一条 `SourceFailure{Source, Err: ErrSourceDisabled}`，让前端能展示明确错误卡片。
  - 在 `in.Sources` 中但**未注册** searcher 的源（如未来打错名字），按现有"source not configured"分支处理，不与 disabled 混淆。
- 当 `in.Sources` 为空（`nil` 或长度 0），行为完全不变（跑所有启用的源）。

新增 sentinel：

```go
var ErrSourceDisabled = errors.New("ai_external: source is not enabled in settings")
```

`@Library` / `@Figure` 不需要改后端：它们已分别对应 `library_search` / `figure_lookup` 两个现成 intent，前端只是把 `IntentHint` 显式填了。

### `internal/service/ai_assistant/library_search_tool.go`（小调整）

当 `IntentHint == "library_search"` 且 `len(Context.PaperIDs) > 0` 时，把这些 paper 作为检索的范围限定（首版：在已有的搜索逻辑前先过滤 paper 域）。本调整足以支持 `@Library + @<paper>` 组合，不引入新接口。

`@Figure + @<paper>` 走的 `figure_lookup` 已经支持按 `Context.PaperID` 过滤，无需改动。

### HTTP / handler 层

请求体新增 `sources?: string[]` 与 `intent_hint?: string`（如已存在则复用），`handler` 层透传到 `RunInput.Sources` / `RunInput.IntentHint`。客户端在没有有效工具标签时不传这两个字段。后端缺省视为 `nil` / `""`，行为与今天一致。

## 错误处理 / 边界

| 情况 | 处理 |
|---|---|
| 用户打了 `@SemanticScholar` 但 S2 在设置里被禁用 | 弹层里就置灰，无法 commit。手动打字进去的由后端按 `ErrSourceDisabled` 返回，前端结果卡片显示「S2 未启用，去设置」 |
| `@PubMed @SemanticScholar` 但只有 PubMed 启用 | 后端实际执行 `{pubmed, s2} ∩ {pubmed} = {pubmed}`；同时附一条 `Failure{S2, ErrSourceDisabled}`，前端展示 S2 错误卡片 |
| 用户打了 `@Foo`（未知标签） | 解析阶段当普通文字处理，不影响 intent；走 auto-route |
| `@PubMed` 但用户在设置里把所有外部源都关了 | 同 `ErrNoSourcesEnabled`，前端在弹层把整组置灰；硬打字时后端返回相同错误 |
| `@Library` + 没有任何已收录文献 | `library_search` 工具按现有路径返回空结果卡片 |
| 同族叠加 `@PubMed @PubMed`（手动重复打） | sources 去重为 `["pubmed"]` |
| 跨族 `@Figure @Library`（手动） | `parseToolTags` last-wins 取 `library`，前端 `console.warn` 一条 `[mention] dropped tool tag family=figure due to family=library` 提示，不阻断提交 |

## 测试策略

- **`ai-mention.js`**：补一份纯函数测试 `parseToolTags`，覆盖：
  - 单标签、同族叠加、跨族 last-wins
  - 大小写、词边界（`@PubMedTutorial` 不应匹配）
  - 与现有 `@<paper>` / `@<role>` 共存
- **commit 行为**：补 DOM 级集成测试覆盖弹层 commit 时的"同族重写"逻辑（如仓库无前端集成测试栈，至少补 `commitToolTag` 的纯函数版本测试）。
- **`router_test.go`**：不动（`IntentHint=""` 路径不变）。
- **`orchestrator_test.go`**：补用例：
  - `IntentHint=external_search + Sources=["pubmed"]` 时仅 PubMed 被调用
  - `Sources=["semantic_scholar"]` 但 S2 未启用 → 返回 `SourceFailure{S2, ErrSourceDisabled}` 且其他源不受影响
  - `Sources=nil` 时与今天行为完全一致
- **`external_search_tool` 集成测试**：补一例验证用户层覆盖与启用集合的交集语义。

## 影响面 / 兼容性

- 无 DB schema 变更。
- 无 API 破坏：`sources` 是新增可选字段，旧客户端继续工作。
- 现有 `RouteIntent` 关键词路由不变，没打工具标签的请求行为完全相同。
- 设置页 `/settings#settings-external-sources` 锚点：当前不存在；实施时在「AI 外部搜索源」section 加 `id="settings-external-sources"`。

## 实施分块（粗）

1. 前端 `parseToolTags` 纯函数 + 测试
2. `web/settings.html` 加 `id="settings-external-sources"` 锚点
3. 弹层加「工具/数据源」段 + 置灰逻辑（含 disabled 项跳转锚点）
4. 弹层 commit 时跨族重写（`commitToolTag`）
5. 提交流程接入解析结果，扩展 API 请求体（`sources`、`intent_hint`）
6. 后端 `RunInput`/`ToolInput` 加 `Sources` 字段，Orchestrator 透传
7. `ExternalSearchTool` 用户层覆盖分支 + `ErrSourceDisabled`
8. `LibrarySearchTool` 接入 PaperIDs 域限定（仅 `IntentHint == "library_search"` 场景生效）
9. 后端测试 + 集成验证
10. 文档：在 `docs/api.md` 补 `sources` / `intent_hint` 字段说明

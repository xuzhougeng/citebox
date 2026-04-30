# AI 伴读页面现代化改造设计

**日期**: 2026-04-30
**作者**: brainstorm session
**状态**: 待实现

## 背景与目标

CiteBox 当前 `/ai` 页（`web/ai.html` + `web/static/js/ai-reader.js` ~1289 行）已有 `@`-mention palette、内联 mention 高亮、流式输出。但产品上有几个明显缺口：

- 会话状态全在浏览器内存里 (`AIReaderPage.state.sessions = { [paperID]: turns[] }`)，浏览器关了就丢
- 单篇文献最多 5 轮，超出要清空对话；没有跨会话历史，没有搜索
- 一个会话强绑定一篇 paper，不能同时围绕几篇 paper 提问
- 没有引用证据可视化，即使有了 `/api/research/snippets` 也没接入

**目标**: 把 `/ai` 重做成 paper-aware 的现代 AI 阅读伴侣 —— ChatGPT/Claude.ai 风格的会话列表 + 服务端持久化 + 多文献 Pin + 严格证据模式。

**显式不做（v1）**: 重新生成、消息编辑/分支、完整分支树、star/归档、文件夹分类、移动端响应式适配、跨账号共享。

## 范围决策摘要

| 决策 | 选择 | 原因 |
| --- | --- | --- |
| 会话模型 | B 独立对象，与 paper 解耦 | 现代 AI 界面标准；单 paper 强绑定限制太大 |
| paper 挂载 | β Pinned + 内联 `@`-mention | 既能长会话有清晰主题，又保留临时引用灵活性 |
| 持久化 | 服务端 SQLite | 与现有 schema 风格一致；跨设备/搜索/导出都更顺 |
| v1 功能盘 | 自动标题 / 重命名删除 / 跨会话搜索 / 严格证据 / 多文献 Pin / 取消 5 轮上限 / 整会话导出 | 围绕"现代会话基础设施 + paper-aware 差异化"，分支编辑 star 等留 v2 |
| 主布局 | L1 二栏（左 sidebar + 右主栏） | 最简洁、最像 ChatGPT/Claude.ai；后续可再加右侧详情 |
| pin 触发 | δ：从其它页面入口自动 pin（γ）+ 第一次 `@` 自动 pin（β）+ 手动兜底 | 用户基本不用操心，需要时点 chip × 取消 |
| pin 上限 | 配置项 `ai.pin_papers_limit`，默认 5 | settings 可调，不进 schema |
| 严格证据模式 | α 每会话级 toggle | 简单可控、cost 可预期；v2 再考虑 AI 自主决定 |
| 引用展示 | 脚注样式 `[n]` + hover/点击 tooltip | 不破坏现有 markdown 渲染管道 |
| 上下文管理 | 滑动窗口 + 摘要器（上限触发时压缩老消息） | 取消 5 轮限制后必须；摘要器复用主 LLM |
| 标题生成 | 第一轮回答完成后异步生成，可手动覆盖 | `title_locked` 字段保护用户改名 |
| 搜索 | 顶部框 + 250 ms 防抖 + LIKE 全文匹配 title/正文/pin paper title | 不上 FTS5；嫌慢再加 |
| 创建流程 | 客户端构造草稿，发送时事务里 INSERT conversation + message | 空对话不污染 sidebar |
| 旧 `/api/ai/read-stream` | **不保留**；research 页将来另设计 AI 集成 | /ai 完全切到新 stateful 流程 |
| 实现方式 | 单 PR，内部分 3 个阶段性 commit | 沿用上一次 PR 的风格 |

## 1. 架构概览

```
┌─────────────────────────────────────────────────────────┐
│  /ai 页（L1 二栏 SPA）                                   │
│  ┌────────────────────┬───────────────────────────────┐  │
│  │ ai-conversations   │ ai-conversation-view          │  │
│  │ sidebar            │ ai-pin / ai-evidence /        │  │
│  │                    │ ai-mention                    │  │
│  └────────────────────┴───────────────────────────────┘  │
│                          ▲ ai-reader.js (bootstrap)      │
└──────────────────────────┼───────────────────────────────┘
                           │ /api/ai/conversations/*
                           ▼
┌─────────────────────────────────────────────────────────┐
│ internal/handler/ai_conversation.go                      │
│         ↓                                                │
│ internal/service/ai_conversation/                        │
│   ├─ service.go        会话 CRUD、发消息流程编排          │
│   ├─ summarizer.go     上下文超限时压缩老消息             │
│   ├─ evidence.go       严格证据：调用 SnippetSearch       │
│   └─ titler.go         异步生成标题                       │
│         ↓                                                │
│ internal/repository/ai_conversation_repo.go              │
│         ↓                                                │
│ SQLite：ai_conversations / ai_messages /                 │
│        ai_conversation_papers                            │
└─────────────────────────────────────────────────────────┘
```

`internal/service/ai_conversation/` 是新独立包，与现有 `internal/service.AIService`（无状态的 read / translate / detect）平级解耦。复用 `AIService` 的 LLM 调用底层 + `research.Client.SnippetSearch`，但生命周期与状态完全独立。

## 2. 数据模型

3 张新表 + 索引：

```sql
CREATE TABLE ai_conversations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL DEFAULT '',
    title_locked INTEGER NOT NULL DEFAULT 0,
    strict_evidence INTEGER NOT NULL DEFAULT 0,
    summary_text TEXT NOT NULL DEFAULT '',
    summary_through_message_id INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE ai_messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    conversation_id INTEGER NOT NULL REFERENCES ai_conversations(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK(role IN ('user','assistant')),
    content TEXT NOT NULL,
    provider TEXT,
    model TEXT,
    mode TEXT,                                  -- normal / strict_evidence / stopped
    included_figures INTEGER,
    citations_json TEXT,                         -- 严格证据 snippet 引用列表
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE ai_conversation_papers (
    conversation_id INTEGER NOT NULL REFERENCES ai_conversations(id) ON DELETE CASCADE,
    paper_id INTEGER NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    pinned_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (conversation_id, paper_id)
);

CREATE INDEX idx_ai_messages_conv     ON ai_messages(conversation_id, id);
CREATE INDEX idx_ai_conv_papers_paper ON ai_conversation_papers(paper_id);
CREATE INDEX idx_ai_conv_updated      ON ai_conversations(updated_at DESC);
```

**字段语义**:
- `title_locked` —— 用户手动改过标题之后置 1，自动标题生成器跳过
- `summary_text` + `summary_through_message_id` —— 摘要状态。assemble prompt 时：summary + (id > summary_through_message_id 的所有 messages)
- `mode` —— 记录每条 assistant 消息的生成形态，便于历史回看时显示徽标
- `citations_json` —— JSON 数组，严格证据模式下保存 snippet 列表（结构见 § 5）

**应用层校验**（不入 schema）:
- pin 数量 ≤ `ai.pin_papers_limit`（默认 5）
- conversation title 长度 ≤ 50
- message content 长度 ≤ 8 KB

## 3. API

| Method | Path | 说明 |
| --- | --- | --- |
| GET | `/api/ai/conversations?q=&limit=&offset=` | 列表 + 搜索（`q` 命中 title/正文/pin paper title） |
| GET | `/api/ai/conversations/:id` | 元数据 + pinned papers + 最近 K 条消息 |
| PATCH | `/api/ai/conversations/:id` | 改 title / strict_evidence |
| DELETE | `/api/ai/conversations/:id` | 硬删（cascade） |
| POST | `/api/ai/conversations/:id/papers` | pin（body `{paper_id}`） |
| DELETE | `/api/ai/conversations/:id/papers/:pid` | unpin |
| GET | `/api/ai/conversations/:id/messages?after=:id&limit=` | 分页加载历史 |
| POST | `/api/ai/conversations/:id/messages` | 发消息（SSE 流式回包）；`:id == "new"` 表示同事务创建会话 |
| GET | `/api/ai/conversations/:id/export` | 整会话 markdown |

**响应壳与现有 `/api/research/*` 一致**（`{items: [...]}` 风格），便于前端复用。

**`POST .../messages` body**:
```json
{
  "content": "用户问题文本",
  "paper_id": 123                         // 可选：自动 pin 这篇（β/γ 入口）
}
```

**SSE 流回包格式** 沿用现有 `/api/ai/read-stream`：
```
event: token
data: {"text":"..."}

event: done
data: {"message_id":456,"citations":[...],"title":"..."}    // title 仅首轮包含

event: error
data: {"code":"...", "message":"..."}
```

## 4. 发消息生命周期

```
POST /api/ai/conversations/:id/messages
  body: {content, paper_id?}
   │
   ├─ 1. (transaction) 若 :id == "new"：INSERT conversation 拿 id
   │
   ├─ 2. (optional) auto-pin paper_id：
   │      不在 pinned 中 → INSERT ai_conversation_papers
   │      已 pin 数 ≥ limit → 422 {code:"pin_limit"}（不创建 message）
   │
   ├─ 3. INSERT user message
   │
   ├─ 4. 加载 prompt 上下文：
   │      - system_prompt（settings.ai.system_prompt）
   │      - pinned papers 全文/图（按 ai.max_figures 截）
   │      - summary_text + (id > summary_through_message_id 的 messages)
   │      - 当前 user message
   │
   ├─ 5. token 估算 (chars/4 启发式，中文混合 chars/2)；
   │      若 > ai.context_budget_tokens：
   │        → summarizer.Summarize(currentSummary, oldestHalfOfRecent)
   │        → UPDATE ai_conversations.summary_text & summary_through_message_id
   │        → 重估，最多 3 轮
   │
   ├─ 6. 若 strict_evidence = 1：evidence.Inject (见 § 5)
   │      失败 → SSE 推 evidence_warning 事件 + fallback 普通模式
   │
   ├─ 7. 流式调 LLM，逐 token SSE 推前端
   │
   ├─ 8. on done：
   │      INSERT assistant message (含 citations_json + mode)
   │      UPDATE ai_conversations.updated_at
   │      若 title == '' && !title_locked：goroutine 触发 titler.Generate（异步）
   │      SSE event: done
   │
   └─ 9. 错误：
         - LLM 5xx/超时：保留 user message，不写 assistant，SSE event: error
         - DB 错误：500 + toast "保存失败"
         - context cancelled (用户停止)：写入已生成片段，mode='stopped'
```

**摘要器** (`summarizer.go`):
- 输入：`current_summary` + 待压缩消息序列
- prompt: `请把下面的对话压缩成 ≤300 字摘要，保留关键问题、结论、引用。已有摘要：{current_summary}\n\n新增对话：\n{messages}`
- 复用主 LLM 模型，不单独配置
- 失败 → fallback 到截断（保留最近 K 轮，丢老的）

**Token 预算来源**: `settings.ai.context_budget_tokens`（默认 32000，settings 页可调）。v1 不做单会话覆盖。

## 5. 严格证据模式

**触发**：`strict_evidence = 1` 的会话，每轮 user message 都会先走 evidence 流程。

**步骤**:
1. `research.Client.SnippetSearch(ctx, query=content[:200], opts={PaperIDs: pinnedExternalIDs, Limit: 8})`
2. `pinnedExternalIDs` 构造规则（按可用性优先级，首个非空即可）：
   - `papers.doi` 非空 → `"DOI:" + doi`
   - 否则 `papers.arxiv_id`（若将来加） → `"ARXIV:" + ...`
   - 否则跳过这篇 paper
   - 一篇都没法构造出标识 → 跳过 evidence，toast 提示"已 pin 文献无外部标识，本次按普通模式作答"

   注：S2 `/snippet/search` 的 `paperIds` 参数支持 `DOI: / ARXIV: / PMID:` 等前缀（与 `/paper/{id}` 一致）。如果实测发现该端点不支持前缀，则在调 SnippetSearch 前加一步 `/paper/batch` 解析为原生 S2 paperId（一次额外请求，可缓存）。
3. snippets 注入 prompt 顶部：

```
你必须基于以下从已钉文献中检索到的证据片段回答。
每个论断后用 [n] 标注引用。如果证据不足以支撑回答，明确说明"证据不足"。

证据：
[1] (Smith 2020, Section: Introduction)
    "We profiled 1.2 million single cells..."
[2] (Lee 2021, abstract)
    "Here we present an integrated analysis..."

用户问题：
{user_message}
```

4. on done 时把 snippets 序列化进 `citations_json`：

```json
[
  {
    "i": 1,
    "paper_id": 42,                         // 本地 papers.id
    "external_id": "DOI:10.1038/...",       // 调 SnippetSearch 时实际使用的标识
    "s2_paper_id": "649def34...",           // S2 返回的 paperId（如果有）
    "snippet": {
      "text": "We profiled 1.2 million single cells...",
      "section": "Introduction",
      "kind": "body",
      "offset": {"start": 1023, "end": 2150}
    },
    "score": 0.87
  }
]
```

**前端渲染** (`ai-evidence.js`):
- 加载 message 时如果 `citations_json` 非空，把 markdown 中匹配 `\[(\d+)\]` 的 token 替换成 `<sup class="ai-citation" data-cite="1">[1]</sup>`
- hover/点击 → tooltip 显示 `paper title + section + snippet text`，带"在原文中查看 →"链接（跳到 paper detail modal）
- 引用编号在同一条 message 内全局，不跨 message

**失败处理**:
- `SnippetSearch` 404/超时/限流 → toast `证据检索失败，本次按普通模式作答`，prompt 里去掉证据块继续走
- pinned 中没有 S2 paper → 跳过 evidence，toast 提示

## 6. 前端模块拆分

| 模块 | 职责 | 估算行数 |
| --- | --- | --- |
| `ai-reader.js` | 入口、bootstrap、URL 参数解析、跨模块状态共享 | ~200 |
| `ai-conversations.js` | sidebar 列表/搜索/CRUD/inline rename | ~250 |
| `ai-conversation-view.js` | 主栏消息列表/流式渲染/导出按钮/strict_evidence toggle | ~350 |
| `ai-pin.js` | pin chip / picker / unpin / auto-pin 逻辑 | ~150 |
| `ai-evidence.js` | citation token → sup → tooltip | ~120 |
| `ai-mention.js` | 现有 `@` palette（拆出来独立） | ~250 |

**共享 state**:
```js
window.AIReader = {
    settings: { aiSettings, extractorSettings, pinLimit, contextBudget },
    sidebar: {
        conversations: [],   // 当前可见列表
        query: '',
        loading: false,
    },
    active: {
        id: null,            // null = 草稿
        meta: null,          // {title, strict_evidence, ...}
        pinnedPapers: [],
        messages: [],
        pendingTurn: null,   // 流式生成中
    },
    mention: { ... },        // 现有 @ palette state
};
```

**事件总线**: 自定义事件 `ai-reader:conversation-changed` / `:message-streamed` / `:pin-updated` / `:strict-evidence-changed`，让模块解耦但能联动 sidebar 数字徽标等次级 UI。

## 7. 入口注入

`library.html / figures.html / notes.html` 三个页面的 paper 卡片菜单加 "在 AI 中追问 →" action：

- 在已有 paper 列表渲染处（`browser-pages.js`、`paper-viewer.js`）加 `<button data-action="ask-ai" data-paper-id="...">`
- 点击 → `window.location = '/ai?paper_id=' + id`

`/ai` 页 init 时检查 URL（按优先级）：
- `?conversation=Y` 存在 → 加载该会话；其它参数（含 `paper_id`）忽略
- `?paper_id=X` → 客户端构造草稿会话，预填 pin paper X，光标停在输入框
- 都没有 → 默认进 sidebar 第一条；空 sidebar → onboarding 卡片

## 8. 错误处理矩阵

| 场景 | 后端 | 前端 |
| --- | --- | --- |
| LLM provider 超时/5xx | user message 已存，assistant 不存；SSE event: error | toast 错误；message 后显示"重试" |
| pin 超过 limit | 422 + `{code:"pin_limit"}` | toast `最多 pin N 篇` |
| 摘要器调用失败 | 退化到截断（保留最近 K 轮） | 不打扰用户，server 日志记录 |
| `/snippet/search` 失败 | 退化到普通 prompt | toast `证据检索失败，本次按普通模式作答` |
| pinned 中无外部标识（DOI/arXiv） | 跳过 evidence | toast `已 pin 文献无外部标识，本次按普通模式作答` |
| DB 错误 | 500 | toast `保存失败`，sidebar 不刷新避免丢已加载内容 |
| stream 中断（用户停止 / 网络断） | 已生成片段写 assistant message，`mode='stopped'` | 显示 `已停止` 标签，可重发 |
| conversation 不存在/已删 | 404 | sidebar 自动刷新；切到第一条 |

## 9. Settings 改动

`internal/service/ai_service_settings.go` 加几项：

```go
type AISettings struct {
    // ... 现有字段
    PinPapersLimit       int `json:"pin_papers_limit"`        // 默认 5
    ContextBudgetTokens  int `json:"context_budget_tokens"`   // 默认 32000
}
```

settings 页 (`web/settings.html`) AI 配置区加对应表单字段。

## 10. 测试

**后端**:
- `internal/repository/ai_conversation_repo_test.go` —— CRUD、cascade delete、search LIKE、按 updated_at 倒序
- `internal/service/ai_conversation/service_test.go` —— 发消息、auto-pin、超过 pin limit 拒绝、context 超 budget 触发摘要、stream 中断写 stopped message
- `internal/service/ai_conversation/evidence_test.go` —— mock SnippetSearch；snippet 注入；失败 fallback；pinned 无 S2 跳过
- `internal/service/ai_conversation/summarizer_test.go` —— 多轮压缩；失败 fallback 到截断
- `internal/handler/ai_conversation_test.go` —— HTTP 路由、SSE 流式（用 httptest.ResponseRecorder hijacker）

**前端**:
- 没有现成的 jest 设置；走 `node --check` 语法 + playwright smoke 跑：
  - 创建会话发首条消息 → sidebar 出现 → 标题异步刷新
  - `?paper_id=X` 自动 pin
  - 严格证据 toggle on → 回答里有 `[1]` `[2]` 引用
  - 搜索框过滤命中
  - 刷新页面会话还在

## 11. 实现分阶段（单 PR，3 个 commit）

沿用上一次 PR 的风格：单 PR、内部分阶段 commit。每个 commit 通过 `go build` + `go test` 即视为可中间合并状态。

**Commit 1 — 后端 schema + 服务 + API 骨架**
- migration 在 `internal/repository/schema/schema.go` 加 3 张表
- `internal/service/ai_conversation/` 新包：service.go + repo adapter
- `internal/handler/ai_conversation.go` + 路由注册
- 暂不接 strict_evidence、暂不做摘要（直接滑动窗口截断）
- 单元测试覆盖 CRUD + 普通发消息流程
- 旧 `/ai` 页面继续工作（仍走旧 in-memory）

**Commit 2 — 前端重构 /ai 页面 + 入口注入**
- HTML 改成 L1 二栏；CSS 加 sidebar；JS 拆 5 模块
- 接 Commit 1 的新 API；删除旧 `state.sessions` 内存逻辑
- settings 加 `pin_papers_limit / context_budget_tokens`
- `library/figures/notes` 卡片菜单加 "在 AI 中追问 →"
- playwright smoke 跑核心路径

**Commit 3 — 摘要器 + 严格证据模式**
- 后端：`summarizer.go` + `evidence.go`，service 主流程接进
- 前端：`ai-evidence.js`、`ai-citation` 样式、tooltip
- 集成测试：摘要触发、evidence 注入、降级路径

## 12. 显式不做（v1）

- 重新生成最后回答（v2）
- 编辑历史 user message 重跑（v2）
- 完整分支树多版本切换（v2+）
- Star / 归档 / 文件夹分类（v2）
- 移动端响应式适配（v2）
- 跨设备通知 / 实时同步（暂无需求）
- AI tool-use 自主决定调 snippet_search（v2，γ 方案）
- 全文搜索升级到 FTS5（按需）
- 多账号 / 共享会话（暂无需求）

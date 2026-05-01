# Semantic Scholar 调研 Panel 设计

**日期**: 2026-04-29
**作者**: brainstorm session
**状态**: 核心已完成（2026-05-01 审核）

## 当前完成状态

- **已完成**: `/research` 页面、Semantic Scholar client/service/repository、S2 cache、调研篮持久化、references/citations/recommendations、导入文献库、markdown 导出、settings API key、导航入口、i18n、README/API 文档和主要后端测试。
- **仍需人工验收**: 下面的手动验收 checklist 依赖真实浏览器和 Semantic Scholar 匿名/API-key 环境，本次状态更新只做代码与文档结构核对，没有重新跑真实外部服务。
- **文档属性**: 本文件保留为实现前设计文档；实际实现已包含后续给 AI 助手复用的 `/api/research/snippets` 能力。

## 背景与目标

CiteBox 已经集成 Crossref（DOI 元数据）和 Unpaywall（OA PDF），但缺少"基于已有论文向外拓展"的能力——也就是看一篇论文引用了谁、被谁引用、有哪些相似论文。本设计在 CiteBox 中新增一个独立的"调研"页面，基于 Semantic Scholar Graph API，实现"种子论文 → 邻居网络 → 调研篮 → 落入文献库"的完整闭环。

**核心场景**: 已经有种子论文（来自文献库，或 DOI/arXiv 粘贴），围绕它做单跳拓展，把感兴趣的论文收进调研篮，最终一键导入文献库或导出 markdown 清单。

**显式不做**: 关系图可视化、作者维度调研、PDF 全文抓取（沿用 Unpaywall）、LLM 自动总结。

## 范围决策摘要

| 决策 | 选择 | 原因 |
| --- | --- | --- |
| 主用例 | B 拓展为主 + A 关键词搜索作入口 | S2 在 references / citations / recommendations 上能力最独特 |
| Panel 落地 | 新顶层页 `/research` + paper-viewer 加一个"在调研中打开"按钮 | 空间充裕，不破坏现有页面主用例 |
| 网络模型 | A+C 混合：单跳列表 + 调研篮（无 graph view） | 任务导向、依赖干净、与 vanilla JS 风格一致 |
| 三类邻居呈现 | 单面板内 3 个 tab | 避免页面分裂，保持种子上下文 |
| 调研篮 | DB 持久化 | 跨会话保留，与 library-centric 风格一致 |
| 缓存策略 | 轻量带 TTL 的响应缓存（仅 `paper/{id}` 元数据） | 避开 1 req/s 配额，又不引入数据陈旧问题 |

## 1. 架构概览

- 新增页面 `web/research.html` + 模块 `web/static/js/research.js`
- 导航: 在"AI伴读"前插入"调研"，所有页面的 `<nav>` 同步更新
- 后端新增独立 service 包 `internal/service/research/`（与 `library_service` 平级），原因：S2 数据是外部、可缓存、只读的，独立 package 边界清晰，方便测试和缓存淘汰
- Handler 层新增 `internal/handler/research.go`，路由前缀 `/api/research/*`
- DB 层新增 2 张表（migration 在 `internal/repository/`）

## 2. 后端设计

### 2.1 S2 客户端 (`internal/service/research/s2_client.go`)

封装单一 HTTP client，方法对齐 S2 端点：

```go
type Client interface {
    Search(ctx, query string, opts SearchOpts) (SearchResult, error)
    Get(ctx, id string, fields []string) (Paper, error)
    References(ctx, id string, page Pagination) (PaperList, error)
    Citations(ctx, id string, page Pagination, opts CitationOpts) (PaperList, error)
    Recommendations(ctx, id string) ([]Paper, error)
    RecommendationsForList(ctx, positives, negatives []string) ([]Paper, error)
}
```

- 复用 `http.Client`，沿用 `library_service` 的 `getJSON` helper 风格
- 全局速率限制（channel-based ticker）：默认 1 req/s（匿名），有 API key 时 5 req/s
- 错误分类：429 → 自动 backoff 一次再吐错；404 → 返回特定 `ErrPaperNotFound` 让前端区分"未找到"vs"接口挂了"
- API key 从 settings 读取（同 Unpaywall email 的位置）；未填写时使用匿名档位

ID 灵活: 接受 DOI、arXiv id、S2 paperId、ACL、PubMed 等所有 S2 支持的前缀格式，client 不做自行解析，直接透传给 `paper/{id}`。

### 2.2 缓存策略

**新表 `s2_paper_cache`**：

```sql
CREATE TABLE IF NOT EXISTS s2_paper_cache (
  cache_key  TEXT PRIMARY KEY,                 -- 形如 "paper:DOI:..." 或 "rec:..."
  payload    TEXT NOT NULL,                    -- JSON
  fetched_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_s2_cache_fetched_at ON s2_paper_cache(fetched_at);
```

- 只缓存 `Get(id)` 返回的元数据；TTL 7 天
- Recommendations 缓存 1 天（用 `paper_id` + `:rec` 后缀作为 key 复用同一张表）
- References / citations 列表**不缓存**：带分页 + 翻页时活跃，无收益
- 命中 → 反序列化返回；过期 → refetch 后写回；同 paper 并发请求用 singleflight 合并

理由：导航过程中同一论文 id 会反复触达——它先是种子卡，再是列表里的"已在库中"判定，再是返回历史栈。`paper/{id}` 是唯一的瓶颈点。

### 2.3 调研篮持久化

**新表 `research_basket_items`**：

```sql
CREATE TABLE IF NOT EXISTS research_basket_items (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  s2_paper_id  TEXT    NOT NULL UNIQUE,
  notes        TEXT    DEFAULT '',
  added_at     DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_basket_added_at ON research_basket_items(added_at DESC);
```

CiteBox 是单用户应用（无 `users` 表，`papers` 也无 `user_id` 列），所以篮子也无需 `user_id` 隔离。不在 `papers` 表上加字段：篮子是"未承诺加入文献库"的临时收集状态，独立表语义更清晰。

**导入文献库流程**：从 cache 取 S2 元数据 → 若有 DOI 先用 `findDuplicateByDOI` 去重 → 不重复时调 `repo.CreatePaper(PaperUpsertInput{...})` 直接落库（元数据-only，无 PDF），`source="semantic_scholar"` → 篮子里清掉对应项。

现有 `LibraryService.ImportPaperByDOI` (in `library_service_oa.go`) 走 Unpaywall **要求拿到 PDF**，否则报错；本 panel 是"先收集元数据，PDF 之后再补"的场景，所以**新增一个独立方法 `LibraryService.ImportPaperFromS2(ctx, paper research.S2Paper) (*model.Paper, error)`**（放在新文件 `library_service_research.go`），它：
1. 用 paper.DOI（如有）查 `findDuplicateByDOI`，已有则返回该条；
2. 调 `repo.CreatePaper` 创建元数据-only 记录（`stored_pdf_name=""`，`extraction_status="manual_pending"`）；
3. 异步触发现有的 `lookupOpenAccessCandidates` 流程把 PDF 拉回来（不阻塞导入）。

### 2.4 API 路由

```
GET    /api/research/search?q=...&year=...&fields_of_study=...&limit=...&offset=...
GET    /api/research/paper/:id
GET    /api/research/paper/:id/references?page=...&limit=...
GET    /api/research/paper/:id/citations?page=...&limit=...&influential_only=...
GET    /api/research/paper/:id/recommendations
POST   /api/research/recommendations         # body: {positive: [...], negative: [...]}

GET    /api/research/basket
POST   /api/research/basket                  # body: {s2_paper_id, notes?}
DELETE /api/research/basket/:s2_paper_id
POST   /api/research/basket/import-to-library  # body: {ids: [...]}
GET    /api/research/basket/export             # 返回 markdown text/plain

GET    /api/library/papers/exists?dois=doi1,doi2,...   # 列表渲染时批量"已在库中"判定
```

所有路由通过现有 `AuthMiddleware` 保护（已经覆盖 `/api/*`），篮子是单用户共享的，无需额外 ACL。

## 3. 前端设计

### 3.1 页面骨架 `web/research.html`

复用 `library.html` 的页面模板（`<nav>` + `data-i18n` + 主题加载脚本）。导航激活态 `<a href="/research" class="active">`。

```
.research-page
├── .research-toolbar          顶部搜索框 + "从文献库选种子" + API key 状态指示
├── .research-main             flex 双栏
│   ├── .research-seed-pane    左 2/3：种子卡 + tabs + list + filter bar
│   └── .research-basket-pane  右 1/3：篮子 + 三个产出按钮
└── .research-empty            初始空态：检索引导
```

### 3.2 JS 模块 `web/static/js/research.js`

模仿 `library.js` 的组织方式（IIFE + 全局 `Research` 对象 + `init()` 入口）。

**核心状态**:

```js
const state = {
  seed: null,                // 当前种子论文 (S2 paper 对象)
  activeTab: "references",   // references | citations | recommendations | search | basketRec
  list: [],                  // 当前 tab 的列表
  page: 0,
  filters: { yearMin, yearMax, minCites, influentialOnly, sort },
  basket: [],                // 拉自 /api/research/basket
  history: [],               // 种子的导航栈，用于"返回"
};
```

**关键交互**:

- **搜索回车**: `/api/research/search` → 结果在左侧呈现一个伪 tab "搜索结果"
- **设为种子**: 把当前 seed 推入 `history` → 调 `paper/:id` → 默认进入 references tab
- **+ 篮**: `POST /api/research/basket`，本地 state.basket 立即更新（乐观更新，失败时回滚 + toast）
- **"已在库中"灰色标记**: 列表渲染前批量调 `/api/library/papers/exists`
- **从文献库选种子**: 复用 `library.js` 已有的 paper-picker 组件（如有）；否则新建一个简单列表 modal
- **基于篮子推荐更多**: `POST /api/research/recommendations` with positives = 篮子全部 id → 切到 `basketRec` 伪 tab
- **返回上一种子**: 从 `history` pop

**性能与配额防御**:
- 列表 lazy load + 翻页节流（避免在 1 req/s 档位连发请求）
- 任意 API 返回 429 时，UI 顶部显示一个细微的 rate-limit 警示条，并自动延迟下一次请求

### 3.3 i18n

按现有 `web/static/locales/{zh-CN,en}.json` 模式加 `research.*` 键：

```
research.title                  "调研" / "Research"
research.search.placeholder     "输入关键词 / DOI / arXiv ID / S2 paperId" / ...
research.tab.references / citations / recommendations / search / basketRec
research.action.addToBasket / setAsSeed / openInLibrary / removeFromBasket
research.basket.title / count / importAll / exportMarkdown / recommendMore / empty
research.filter.year / minCites / influentialOnly / sort
research.error.noApiKey / rateLimited / notFound / unknown
research.empty.hint
research.history.back
research.inLibrary
```

## 4. 配置 (settings)

`internal/service/settings.go` 新增字段：

- `s2_api_key`（可选）：通过 `LibraryRepository.GetAppSetting / UpsertAppSetting` 存到现有 `app_settings` 表，key = `"s2_api_key"`，明文存储，与现有 AI key 同等级
- 同时支持 env var `S2_API_KEY`：优先级 env > db setting，便于运维场景

`web/settings.html` 在"外部 API"分组里加一栏，文案 `settings.research.s2.*`，附文案"未填写时使用匿名速率（约 1 req/s）"，链接到 [api.semanticscholar.org](https://api.semanticscholar.org/api-docs)。配套 `GET/PUT /api/settings/research`。

## 5. 测试

### 单元测试

- `internal/service/research/s2_client_test.go`：`httptest.Server` 模拟 S2 各端点的成功 / 404 / 429 / 字段缺失。重点测 rate limiter、backoff、singleflight
- `internal/handler/research_test.go`：HTTP 层参数校验、错误转换、auth 隔离（用户 A 的篮子对 B 不可见）
- `internal/service/research/cache_test.go`：TTL 命中/过期/并发刷新单飞

### 集成测试

- `internal/service/research/integration_test.go`：起完整 server，灌 fake S2，跑「搜索 → 设种子 → 加篮 → 导入文献库」全流程。复用 `library_service_integration_test.go` 的脚手架

### 手动验收 checklist

> 状态备注（2026-05-01）: 这些是发布前 QA 项，不再表示功能未实现。

- [ ] 匿名（无 API key）能搜索、加种子、翻页，速率受限时 UI 正常提示
- [ ] 配置 API key 后，连续操作不再触发 429
- [ ] 篮子持久化跨浏览器会话
- [ ] "已在库中"标记对所有有 DOI 的论文准确
- [ ] 导出 markdown 包含标题、作者、年份、DOI、TLDR

## 6. 风险与已知边界

- **S2 匿名配额**: 1 req/s 极易触发 429。靠前端节流 + 后端 backoff + 缓存共同应对，但仍可能在密集探索时降级体验
- **arXiv-only 论文的"已在库中"判定**: 当前 exists 接口只比 DOI；下一阶段需要扩展 papers 表的 arXiv id 索引以及 exists 接口的多 ID 类型支持
- **中文期刊覆盖差**: S2 对中文期刊几乎不收录。文档/UI 提示要明确这是英文优先的工具
- **S2 PDF 链接的可达性**: `openAccessPdf` 不保证可下载。本 panel 只展示链接，实际抓取仍走现有 Unpaywall 流程

## 7. 实施分期建议

留给 writing-plans skill 决定具体步骤。但粗粒度建议：

1. 后端 S2 client + cache + 路由（无 UI，先用 curl 验证）
2. 前端 research 页基本壳：搜索 → 种子 → references 列表
3. 篮子持久化 + 导入文献库流程
4. 三 tab 完整 + 过滤排序 + 篮子推荐
5. 测试覆盖、i18n 完整化、settings 入口

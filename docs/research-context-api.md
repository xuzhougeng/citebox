# CiteBox 研究上下文集成 API（内置 MCP 服务）

本文档面向外部工具（如 Wisp）的开发者，说明如何通过 CiteBox 内置的 MCP 服务读取本地文献库的研究上下文。

实现代码：

- [internal/integration/](/home/xzg/project/paper_image_db/internal/integration/)：与传输层无关的只读门面、令牌、设置、信封与资产暂存区
- [internal/mcpserver/](/home/xzg/project/paper_image_db/internal/mcpserver/)：绑定回环地址的 HTTP MCP 适配层
- [internal/handler/integration.go](/home/xzg/project/paper_image_db/internal/handler/integration.go)：主服务上的设置与令牌管理接口

## 概述与定位

- CiteBox 是本地文献知识源：文献、图片、笔记、PDF 标注都保存在本机 SQLite 数据库中。
- Wisp 等外部工具是工作台：通过 MCP 协议按需读取上下文，不直接访问 CiteBox 的数据库文件。
- 当前版本（P0）全部接口只读：只暴露检索、上下文打包和资产下载，不提供任何写入能力。
- 双方不共享数据库：外部工具只能通过本文档描述的 HTTP 接口获取数据，数据格式以 `citebox.research-context/v1` 信封为准。

## 启用与连接

### 开启服务

内置 MCP 服务**默认关闭**。在 CiteBox 设置页的“研究上下文集成”卡片中开启：

1. 勾选“启用本地 MCP 服务”；
2. 确认端口（默认 `19831`，可改为其他未被占用的端口）；
3. 点击保存，服务立即按新设置启动或重新绑定。

服务只监听 `127.0.0.1`，MCP 端点地址为：

```
http://127.0.0.1:<port>/mcp
```

### 访问令牌（Token）

外部工具通过独立的集成令牌访问 MCP 服务，与 CiteBox Web 登录态无关：

- 在设置页点击生成/轮换令牌，明文只在轮换成功后的响应里显示**一次**，请立即复制保存；
- 服务端只持久化令牌的 SHA-256 哈希（`integration_tokens.token_hash`），无法找回明文；
- 令牌明文以 `cbx_` 开头，便于识别和泄露扫描；
- 轮换会吊销所有旧令牌并签发新令牌；吊销则让所有令牌立即失效；
- 令牌必须放在 `Authorization: Bearer` 请求头中，**绝不接受 query string 传令牌**。

令牌携带固定的只读权限范围（scopes）：

| Scope | 覆盖能力 |
| --- | --- |
| `library:read` | 文献/图片检索、文献上下文、PDF 全文检索、增量同步 |
| `notes:read` | 读取笔记实体（`citebox_get_entity` 访问 `citebox:note:...` 时校验） |
| `annotations:read` | 读取 PDF 标注实体（`citebox_get_entity` 访问 `citebox:annotation:...` 时校验） |
| `assets:read` | `citebox_export_asset` 与 `/assets/{id}` 资产下载 |

当前轮换出的令牌固定授予以上全部四个 scope。

### Wisp 连接配置示例

```json
{
  "url": "http://127.0.0.1:19831/mcp",
  "headers": {
    "Authorization": "Bearer cbx_..."
  }
}
```

### 设置管理接口

设置与令牌管理走主服务的同源 Cookie 会话接口（不是 MCP 端口），详见 [api.md](api.md) 的“外部集成（内置 MCP 服务）”一节：

- `GET / PUT /api/settings/integration`：读取/保存开关与端口；
- `POST /api/settings/integration/token/rotate`：轮换令牌，响应含一次性明文 `new_token`；
- `DELETE /api/settings/integration/token`：吊销全部令牌。

## 安全边界

- **默认关闭**：未在设置页显式启用前，MCP 服务不监听任何端口。
- **仅回环地址**：服务只绑定 `127.0.0.1`，不暴露到局域网；没有也不提供绑定 `0.0.0.0` 的选项。
- **独立令牌**：集成令牌与 Web 会话 Cookie 完全独立；MCP 服务不复用主服务的 Cookie 认证和 CORS 中间件。
- **令牌只走请求头**：`Authorization: Bearer cbx_...` 是唯一凭证传递方式，URL 参数中的令牌一律无效，避免令牌落入日志。
- **只存哈希**：数据库只保存 SHA-256 哈希，认证时先按哈希检索再做常量时间比较；明文仅在轮换时返回一次。
- **P0 全部只读**：所有工具只读，令牌 scope 也全部是 `*:read`。
- **Windows / WSL2 注意事项**：Windows 原生运行的 Wisp 访问 WSL2 内 CiteBox 的回环端口时，`localhost` 转发在 mirrored 与 NAT 两种网络模式下行为不同（mirrored 模式通常可直接互通，NAT 模式依赖 `localhostForwarding`，跨发行版/版本表现不一），部署时需按实际模式分别验证。**不要**为了图方便把服务绑到 `0.0.0.0`。
- **版权与隐私（local-first）**：PDF 全文、图片等资产始终保存在本机，仅通过回环地址在本机进程间传输；外部工具获取文献全文和图片后应遵守相应的版权与隐私约束，CiteBox 不会主动上传任何内容。

## MCP 协议

### 传输与端点

- 端点：`POST http://127.0.0.1:<port>/mcp`
- 协议：JSON-RPC 2.0，声明的 MCP `protocolVersion` 为 `2025-06-18`
- 模式：单请求-单响应；**不支持批量请求（batch），不支持 SSE 流式推送**
- 请求体上限 4 MB；`Content-Type` 统一为 `application/json`

支持的方法：

| 方法 | 说明 |
| --- | --- |
| `initialize` | 握手，返回 `protocolVersion`、`capabilities.tools` 与 `serverInfo`（`name: "citebox"`） |
| `ping` | 保活，返回空对象 |
| `tools/list` | 列出全部工具（name/description/inputSchema） |
| `tools/call` | 调用工具，参数为 `{"name": "...", "arguments": {...}}` |
| `notifications/*` | 任意通知直接受理，返回 `202 Accepted`，无响应体 |

`initialize` 响应示例：

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "protocolVersion": "2025-06-18",
    "capabilities": { "tools": {} },
    "serverInfo": { "name": "citebox", "version": "v0.16.0" }
  }
}
```

### 工具调用结果包装

`tools/call` 成功时，`result` 同时包含渲染好的文本和结构化数据：

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {
    "content": [{ "type": "text", "text": "{ ...格式化后的 JSON 文本... }" }],
    "structuredContent": { "...": "工具实际返回的结构化数据" }
  }
}
```

下文每个工具的“输出”均指 `structuredContent` 的内容。

领域错误（实体不存在、参数取值无效、前置条件不满足）不占用 JSON-RPC 错误通道，而是返回 `isError` 工具结果：

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "result": {
    "isError": true,
    "content": [{ "type": "text", "text": "figure not found" }]
  }
}
```

### 错误码约定

JSON-RPC 层错误（`error.code`）：

| 代码 | 含义 |
| --- | --- |
| `-32700` | Parse error：请求体不是合法 JSON 或读取失败 |
| `-32600` | Invalid Request：`jsonrpc` 字段不是 `"2.0"`，或令牌缺少目标工具所需 scope（message 形如 `insufficient scope: assets:read required`） |
| `-32601` | Method not found：未知方法或未知工具名（`Unknown tool: ...`） |
| `-32602` | Invalid params：工具参数 JSON 解码失败或必填校验失败 |
| `-32603` | Internal error：服务内部错误 |

HTTP 层错误：

- 缺少、无效或已吊销的令牌（含 scope 不满足的资产下载）：`401 {"error": "unauthorized"}`；
- `GET /mcp`、`POST /assets/...` 等方法不匹配：`405 Method not allowed`；
- 资产不存在或已过期：`404 {"error": "not found"}`。

## 工具一览

| 工具 | 所需 scope | 说明 |
| --- | --- | --- |
| `citebox_get_capabilities` | 任意有效令牌 | 返回集成能力描述 |
| `citebox_search_library` | `library:read` | 跨实体检索文献/图片/笔记/标注 |
| `citebox_get_paper_context` | `library:read` | 打包单篇文献的完整研究上下文 |
| `citebox_search_paper_text` | `library:read` | 在指定文献的 PDF 全文中检索子串 |
| `citebox_get_entity` | 按实体类型决定 | 按 `source_id` 获取单个实体信封 |
| `citebox_export_asset` | `assets:read` | 导出图片二进制，返回短时下载 URL |
| `citebox_list_changes` | `library:read` | 按水位线增量同步变更 |

`citebox_get_entity` 的 scope 按 `source_id` 指向的实体类型决定：`note` → `notes:read`，`annotation` → `annotations:read`，其余 → `library:read`。

### `citebox_get_capabilities`

描述当前集成：版本、schema、实体类型、scope、分页上限和可用工具列表。无参数。

输出示例：

```json
{
  "citebox_version": "v0.16.0",
  "research_context_schema": "citebox.research-context/v1",
  "transfer_package_schema": "figure-transfer-package.v1",
  "entity_types": ["paper", "figure", "note", "annotation"],
  "scopes": ["library:read", "notes:read", "annotations:read", "assets:read"],
  "max_page_size": 100,
  "max_changes_limit": 500,
  "tools": [
    "citebox_get_capabilities",
    "citebox_search_library",
    "citebox_get_paper_context",
    "citebox_search_paper_text",
    "citebox_get_entity",
    "citebox_export_asset",
    "citebox_list_changes"
  ]
}
```

### `citebox_search_library`

跨实体检索图书馆。结果按 paper → figure → note → annotation 的固定顺序合并（note 展开为“文献笔记”和“图片笔记”两个来源），用不透明游标分页。

输入：

```json
{
  "query": "transformer",
  "entity_types": ["paper", "figure"],
  "group_id": 3,
  "tags": [7],
  "updated_after": "2026-07-01T00:00:00Z",
  "cursor": "",
  "limit": 20
}
```

- `entity_types`：可选，为空表示全部四类；取值 `paper` / `figure` / `note` / `annotation`。
- `group_id`：可选，仅作用于 paper/figure。
- `tags`：可选，标签 ID 数组；**当前只应用第一个标签**。
- `updated_after`：可选，RFC3339；按批次后置过滤（见“已知限制”）。
- `cursor`：可选，上一页响应里的 `next_cursor`。
- `limit`：可选，默认 20，最大 100。

输出：

```json
{
  "items": [
    {
      "source_id": "citebox:paper:42",
      "entity_type": "paper",
      "revision": "2026-07-20T08:30:00Z",
      "title": "Attention Is All You Need",
      "snippet": "The dominant sequence transduction models...",
      "data": {
        "id": 42,
        "title": "Attention Is All You Need",
        "authors_text": "Vaswani, Shazeer, Parmar, ...",
        "journal": "NeurIPS",
        "published_at": "2017",
        "doi": "10.48550/arXiv.1706.03762",
        "figure_count": 3
      }
    }
  ],
  "next_cursor": "eyJvZmZzZXRzIjp7..."
}
```

- `revision` 为实体 `updated_at` 的 UTC RFC3339 表示；`snippet` 为按 200 rune 截断的摘要/图注/笔记/引用文本。
- 还有后续数据时才返回 `next_cursor`；游标是 base64url 编码的不透明字符串，不要解析。
- 各实体类型的 `data` 轻量载荷：
  - `paper`：`{id, title, authors_text, journal, published_at, doi, figure_count}`
  - `figure`：`{id, paper_id, paper_title, caption, page_number, display_label}`
  - `note`（文献笔记）：`{paper_id, text}`；`note`（图片笔记）：`{figure_id, paper_id, text}`
  - `annotation`：`{id, paper_id, paper_title, type, quote_text, note_text, page_start, page_end, color}`

### `citebox_get_paper_context`

打包单篇文献的研究上下文信封：元数据、摘要、笔记、图片、标注、标签、分组，可用 `include` 裁剪。

输入：

```json
{
  "paper_id": 42,
  "include": ["metadata", "abstract", "figures"],
  "figure_limit": 20,
  "annotation_limit": 50
}
```

- `paper_id`：必填。
- `include`：可选；为空表示包含全部小节。取值：`metadata`、`abstract`、`paper_notes`、`figure_notes`、`annotations`、`figures`、`tags`、`group`。
- `figure_limit`：可选，默认 20；只截断顶层图片（子图嵌套在顶层图片内）。
- `annotation_limit`：可选，默认 50。

输出（信封，`data` 按 `include` 出现的小节为准）：

```json
{
  "schema_version": "citebox.research-context/v1",
  "source_id": "citebox:paper:42",
  "entity_type": "paper",
  "revision": "2026-07-20T08:30:00Z",
  "data": {
    "metadata": {
      "id": 42,
      "title": "Attention Is All You Need",
      "authors_text": "Vaswani, Shazeer, Parmar, ...",
      "journal": "NeurIPS",
      "published_at": "2017",
      "doi": "10.48550/arXiv.1706.03762",
      "original_filename": "1706.03762.pdf",
      "extraction_status": "completed",
      "figure_count": 3,
      "created_at": "2026-06-01T02:00:00Z",
      "updated_at": "2026-07-20T08:30:00Z"
    },
    "abstract": "The dominant sequence transduction models...",
    "figures": [
      {
        "figure_id": 123,
        "source_id": "citebox:figure:123",
        "caption": "The Transformer - model architecture.",
        "display_label": "Fig 1",
        "page_number": 2,
        "subfigures": [
          {
            "figure_id": 124,
            "source_id": "citebox:figure:124",
            "caption": "Encoder stack",
            "display_label": "Fig 1a",
            "subfigure_label": "a"
          }
        ]
      }
    ]
  },
  "relations": [
    { "type": "figure", "source_id": "citebox:figure:123" }
  ],
  "assets": [
    { "kind": "figure_image", "figure_id": 123, "source_id": "citebox:figure:123" }
  ],
  "provenance": { "citebox_version": "v0.16.0" },
  "permissions": ["read"],
  "deep_link": "citebox://paper/42"
}
```

各小节说明：

- `metadata`：文献元数据，`created_at` / `updated_at` 为 UTC RFC3339。
- `abstract` / `paper_notes`：纯文本字符串。
- `figure_notes`：`[{figure_id, source_id, display_label, notes_text}]`；不受 `figure_limit` 限制，覆盖所有含笔记的图片（含子图）。
- `figures`：顶层图片数组，子图嵌套在 `subfigures` 字段。
- `annotations`：`PDFAnnotation` 模型 JSON 数组（`id, paper_id, type, page_start, page_end, quote_text, color, fragments, note_text, created_at, updated_at`）。
- `tags`：`Tag` 模型 JSON 数组；`group`：对象，`{}` 或 `{group_id, group_name}`。
- `relations`：关联实体指针，figure/annotation 用 `source_id`，tag/group 用 `{type, id, name}`。
- `assets`：资产描述符，**不直接含下载地址**；外部工具拿 `figure_id` 调 `citebox_export_asset` 按需换取。

### `citebox_search_paper_text`

在指定文献的 PDF 全文中做**大小写不敏感的子串检索**，返回 rune 安全的上下文窗口。

输入：

```json
{
  "paper_ids": [42, 43],
  "query": "multi-head attention",
  "limit": 12,
  "context_chars": 1200
}
```

- `paper_ids`、`query`：必填。
- `limit`：可选，默认 12，为跨文献的总命中数上限。
- `context_chars`：可选，默认 1200，为以命中位置为中心的上下文窗口大小（rune 数）。

输出：

```json
{
  "matches": [
    {
      "paper_id": 42,
      "page": 3,
      "snippet": "...we propose the Transformer, which relies entirely on multi-head attention...",
      "source_id": "citebox:paper:42",
      "revision": "2026-07-20T08:30:00Z"
    }
  ]
}
```

- `page` 为 1 起始页码；当文献没有逐页文本（`papers.pdf_page_texts` 为空）时退化为整篇 `pdf_text` 检索，`page` 为 `null`。
- 逐页文本由浏览器端 pdf.js 提取流程经 `POST /api/papers/{id}/pdf-text` 写入，见 [api.md](api.md)。

### `citebox_get_entity`

按 `source_id` 获取单个实体的信封。

输入：

```json
{ "source_id": "citebox:paper:42" }
```

输出为信封。`data` 内容按实体类型：

- `citebox:paper:42`：完整 `Paper` 模型 JSON；
- `citebox:figure:123`：完整 `Figure` 模型 JSON；
- `citebox:annotation:91`：完整 `PDFAnnotation` 模型 JSON；
- `citebox:note:paper:42:main` / `citebox:note:figure:123:main`：`{"text": "..."}`（笔记纯文本）。

```json
{
  "schema_version": "citebox.research-context/v1",
  "source_id": "citebox:note:paper:42:main",
  "entity_type": "note",
  "revision": "2026-07-20T08:30:00Z",
  "data": { "text": "核心贡献：完全用注意力机制替代循环与卷积……" },
  "provenance": { "citebox_version": "v0.16.0" },
  "permissions": ["read"],
  "deep_link": "citebox://paper/42"
}
```

限制：笔记名当前仅支持 `main`；其他名字返回参数无效错误。`source_id` 格式非法返回 `isError` 工具结果。

### `citebox_export_asset`

把图片二进制资产放入内存暂存区，返回带过期时间的下载描述。资产下载走 `GET /assets/{id}`，见“资产导出”一节。

输入：

```json
{ "kind": "figure_image", "id": 123 }
```

- `kind`：`figure_image`（单图原图）或 `figure_transfer_package`（图片迁移打包，ZIP）。
- `id`：图片（figure）ID。

输出：

```json
{
  "url": "http://127.0.0.1:19831/assets/9f2c1e4a7b3d40f8a1c5e6d2b8a09f31",
  "byte_size": 248113,
  "media_type": "image/png",
  "sha256": "b5d4045c3f466fa91fe2cc6abe79232a1a57cdf104f7a26e716e0a1e2789df78",
  "expires_at": "2026-08-03T12:41:17Z"
}
```

- `figure_transfer_package` 的 `media_type` 固定为 `application/zip`，包结构 schema 为 `figure-transfer-package.v1`。
- `expires_at` 为 UTC RFC3339，固定为导出后 10 分钟。

### `citebox_list_changes`

按每类实体的 `(updated_at, id)` 水位线增量同步变更，固定顺序 paper → figure → annotation。

输入：

```json
{
  "cursor": "eyJwYXBlciI6ey...",
  "entity_types": ["paper", "figure", "annotation"],
  "limit": 100
}
```

- `cursor`：可选；空串表示从头全量扫描。传入上一次响应的 `next_cursor` 即可续拉。
- `entity_types`：可选，为空等价于 `["paper", "figure", "annotation"]`；接受 `note` 但不产生效果（笔记变更依附在父 paper/figure 行上）。
- `limit`：可选，默认 100，最大 500，为本次返回的总变更数上限。

输出：

```json
{
  "changes": [
    {
      "operation": "updated",
      "source_id": "citebox:paper:42",
      "revision": "2026-07-20T08:30:00Z"
    },
    {
      "operation": "updated",
      "source_id": "citebox:figure:123",
      "revision": "2026-07-21T01:02:03Z"
    }
  ],
  "next_cursor": "eyJwYXBlciI6eyJ0Ijoi..."
}
```

详见“增量同步语义”。

## 信封与 source_id 约定

所有单实体响应统一使用 `citebox.research-context/v1` 信封：

| 字段 | 说明 |
| --- | --- |
| `schema_version` | 固定 `citebox.research-context/v1` |
| `source_id` | 实体的稳定标识，见下表 |
| `entity_type` | `paper` / `figure` / `note` / `annotation` |
| `revision` | 实体的 `updated_at`，UTC RFC3339；客户端可据此判断本地副本是否过期 |
| `data` | 实体数据载荷（因工具与实体类型而异） |
| `relations` | 可选，关联实体指针 |
| `assets` | 可选，资产描述符（不含下载地址） |
| `provenance` | 来源信息，当前含 `citebox_version` |
| `permissions` | 恒为 `["read"]` |
| `deep_link` | CiteBox 内深链，形如 `citebox://paper/42` |

`source_id` 格式：

| 实体 | 格式 | 示例 |
| --- | --- | --- |
| 文献 | `citebox:paper:<id>` | `citebox:paper:42` |
| 图片 | `citebox:figure:<id>` | `citebox:figure:123` |
| PDF 标注 | `citebox:annotation:<id>` | `citebox:annotation:91` |
| 文献笔记 | `citebox:note:paper:<paper_id>:main` | `citebox:note:paper:42:main` |
| 图片笔记 | `citebox:note:figure:<figure_id>:main` | `citebox:note:figure:123:main` |

- `deep_link` 形如 `citebox://paper/42`、`citebox://figure/123`、`citebox://annotation/91`；笔记实体的深链指向其父实体。
- `source_id` 大小写与段数敏感；解析失败返回参数无效错误。各表的 ID 空间相互独立，不要跨类型复用数字 ID。

## 增量同步语义

- **按类水位线**：游标内部是 paper/figure/annotation 三类各自的 `(updated_at, id)` 水位线（keyset 分页），相同 `updated_at` 的记录靠 `id` 续页。
- **`next_cursor` 总是返回**：即使当前没有任何变更，响应也带 `next_cursor`。客户端必须持久化最近一次游标用于后续轮询，否则下次会从头全量扫描。
- **只上报 upsert**：`operation` 目前恒为 `"updated"`；**删除不被追踪**，客户端如需发现删除，应定期全量比对。
- **笔记依附父实体**：文献/图片笔记的修改体现为父 paper/figure 行的变更，不产生独立的 note 变更记录；`entity_types` 里传 `note` 是合法但无效果的。
- **顺序固定**：每批变更按 paper → figure → annotation 顺序拼接，批内按 `(updated_at, id)` 升序。
- 变更只给指针（`source_id` + `revision`），客户端应再用 `citebox_get_entity` / `citebox_get_paper_context` 拉取最新内容，并以 `revision` 对比跳过未变化副本。

## 资产导出

- `citebox_export_asset` 把资产写入 MCP 服务进程的内存暂存区，返回短时 URL；暂存区不落盘、不跨重启保留。
- URL 形如 `http://127.0.0.1:<port>/assets/<随机 id>`，**有效期 10 分钟**（`expires_at`），过期后惰性清除，再访问返回 `404 {"error": "not found"}`。
- 下载**仍需** `Authorization: Bearer` 头，且令牌须含 `assets:read` scope——短时 URL 不是免密链接。
- 响应带原始 `Content-Type` 和 `Content-Disposition: attachment; filename="..."`。
- 完整性校验：下载完成后对字节流计算 SHA-256（小写十六进制），与 `citebox_export_asset` 返回的 `sha256` 比对，不一致即丢弃重取。

校验示例（bash）：

```bash
curl -s -H "Authorization: Bearer cbx_..." \
  -o figure.png "http://127.0.0.1:19831/assets/<id>"
sha256sum figure.png   # 与响应里的 sha256 对比
```

## 已知限制

- `citebox_list_changes` 的 `next_cursor` 总是返回（包括无变更时），客户端必须持久化；只上报 upsert，删除不追踪。
- `entity_types` 中的 `note` 在 `citebox_list_changes` 里是 no-op：笔记变更通过父 paper/figure 行体现。
- `citebox_search_library` 的 `tags` 过滤只应用 `tags[0]`，多标签交集暂不支持。
- `citebox_search_library` 的 `updated_after` 是按批次的后置过滤：可能返回不足 `limit` 的“短页”，但游标保持一致，继续翻页即可。
- `citebox_search_library` 的 `group_id` / `tags` 过滤不作用于 annotation 来源；标注检索只响应 `query`。
- `citebox_search_library` 结果按实体类型固定顺序合并，不做跨类型相关性排序。
- `citebox_search_paper_text` 在无逐页文本时返回 `page: null`（退化整篇检索）。
- `citebox_get_paper_context` 的 `figure_limit` 只截断顶层图片；`figure_notes` 不受其限制。
- 资产 URL 有效期 10 分钟且仍在内存中，服务重启后全部失效，需要重新导出。
- 传输为单请求模式：无 batch、无 SSE；并发能力取决于客户端自行并行发起多个 HTTP 请求。

# CiteBox 前后端 API 说明

本文档面向前端页面开发，说明当前 Web 前端如何向后端发请求、有哪些接口、常用参数是什么，以及不同类型接口的返回方式。

文档基于当前实现整理，主要参考：

- [web/static/js/api.js](/home/xzg/project/paper_image_db/web/static/js/api.js)
- [internal/app/server.go](/home/xzg/project/paper_image_db/internal/app/server.go)
- `internal/handler/*.go`

## 总览

- API 前缀：`/api`
- 鉴权方式：同源 Cookie 会话
- 前端默认请求方式：
  - JSON 接口：`fetch(..., { credentials: 'same-origin' })`
  - 下载接口：`Blob` 或直接 `<a href=...>`
  - 流式 AI 接口：`application/x-ndjson`
- 主要资源：
  - 文献：`/api/papers`
  - 图片：`/api/figures`
  - 分组：`/api/groups`
  - 标签：`/api/tags`
  - AI：`/api/ai/...`
    - 包括 `/api/ai/prompt-presets`
    - 包括 `/api/ai/translate`
    - 包括 `/api/ai/detect-figure-regions`
  - 版本检查：`/api/settings/version`
  - 提取器设置：`/api/settings/extractor`
  - AI 外部搜索源设置：`/api/settings/ai-external-search`
  - 桌面端关闭行为设置：`/api/settings/desktop-close`
  - 微信桥接设置：`/api/settings/weixin-bridge`
  - 今日推荐测试发图：`/api/settings/weixin-bridge/daily-recommendation/test`
  - Wolai 设置：`/api/settings/wolai`
  - Notion MCP 设置与 OAuth：`/api/settings/mcp/...`
  - Notion API 个人令牌设置：`/api/settings/notion-api/...`
  - Notion 图片笔记导出：`/api/notion/...`
  - Wolai 笔记导出：`/api/wolai/...`
  - 数据库备份导入导出：`/api/database/...`
  - 鉴权：`/api/auth/...`

## 前端请求约定

### 1. JSON 请求

前端统一通过 `requestJSON()` 发起：

- 自动带上 `credentials: 'same-origin'`
- 默认解析 JSON
- 若状态码不是 `2xx`，抛出 `Error`
- `401` 时自动清理旧登录态并跳转到 `/login`

失败时前端可读到：

- `error.message`
- `error.code`
- `error.status`
- `error.payload`

### 2. 下载请求

前端通过 `requestBlob()` 发起下载类 POST 接口，例如：

- `/api/ai/read/export`

特点：

- 自动处理 `Content-Disposition`
- 自动解析文件名
- 返回 `{ blob, filename }`

另外也有直接通过链接下载的接口：

- `/api/database/export`

### 3. 流式请求

AI 流式阅读通过：

- `POST /api/ai/read/stream`

划词翻译通过：

- `POST /api/ai/translate`

返回格式是按行分隔的 JSON，也就是 `ndjson`。前端逐行读取后触发 `onEvent(JSON.parse(line))`。

### 4. 错误响应格式

后端统一错误格式：

```json
{
  "success": false,
  "code": "invalid_argument",
  "error": "请求体格式错误"
}
```

特殊情况：

- 上传重复 PDF 时，错误响应里还可能带 `paper`，便于前端跳到已存在的文献。

## 接口分组

### 文献 Papers

#### `GET /api/papers`

用途：

- 文献库列表
- AI 助手左侧文献列表
- 文献笔记列表

常用查询参数：

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| `keyword` | string | 标题、摘要、全文、DOI、笔记、标签、分组等搜索 |
| `author` | string | 按作者字符串过滤，支持部分匹配，不区分大小写 |
| `keyword_scope` | string | 可选：`title_abstract` 搜索标题、摘要和 DOI，`full_text` 搜索标题、摘要、正文和 DOI；默认保留兼容模式 |
| `group_id` | int | 按分组过滤 |
| `tag_id` | int | 按文献标签过滤 |
| `status` | string | 按解析状态过滤 |
| `has_paper_notes` | bool | 仅返回带文献笔记的文献 |
| `sort_by` | string | 可选：`created_at` 按文献创建时间倒序；`updated_at` 按文献更新时间倒序 |
| `page` | int | 页码，从 1 开始 |
| `page_size` | int | 每页数量 |

返回：

- `papers`
- `total`
- `page`
- `page_size`
- `total_pages`

#### `GET /api/papers/{id}`

用途：

- 文献详情弹窗
- AI 助手选中文献详情
- 文献笔记编辑面板

返回单篇文献详情，包含：

- 基本信息
- `doi`
- `authors_text`
- `journal`
- `published_at`
- `abstract_text`
- `pdf_url`
- `figures`
- 文献标签
- `notes_text`（管理笔记）
- `paper_notes_text`（文献笔记）

#### `POST /api/papers`

用途：

- 上传 PDF

请求类型：

- `multipart/form-data`

表单字段：

| 字段 | 说明 |
| --- | --- |
| `pdf` | PDF 文件 |
| `title` | 可选标题 |
| `group_id` | 可选分组 |
| `tags` | 逗号分隔标签 |
| `extraction_mode` | 可选，`auto` 或 `manual`；`manual` 表示跳过自动解析，但文献仍会直接入库 |

说明：

- Web 上传页在 `manual` 模式下会默认调用浏览器 `pdf.js` 提取全文，并通过 `POST /api/papers/{id}/pdf-text` 保存
- 即便当前没有配置自动解析模型，只要能上传到手工流程，仍会走这条浏览器端全文提取链路
- 当全局 `extractor_profile` 设为 `manual` 时，前端上传和微信上传都会默认落到这条“只提全文、不自动提图”的流程

#### `POST /api/papers/import-by-doi`

用途：

- 输入 DOI 后，从 Open Access 来源查找并导入 PDF

请求类型：

- `application/json`

常用 JSON 字段：

```json
{
  "doi": "10.1038/nature12373",
  "title": "可选覆盖标题",
  "group_id": 1,
  "tags": ["review", "oa"],
  "extraction_mode": "manual"
}
```

说明：

- 当前后端会按顺序尝试 `Unpaywall`、`Europe PMC` 和 `PMC` 相关来源。
- `doi` 支持直接输入标准 DOI，也支持 `https://doi.org/...` 形式；后端会统一标准化后保存到 `papers.doi`。
- 导入成功后会尽量补全结构化文献信息，当前优先尝试 `Crossref` 和 `Europe PMC`，自动写入标题、摘要、作者、期刊/来源和发表时间等字段。
- 导入成功后会走和本地 PDF 上传相同的入库、去重、全文保存与自动解析链路。
- 若未找到合法可下载的 Open Access PDF，返回 `NOT_FOUND`。
- 若找到了 OA 记录但实际下载失败，返回 `UNAVAILABLE`。
- 为了启用更广覆盖的 `Unpaywall` 检索，建议配置环境变量 `OA_CONTACT_EMAIL`。

#### `POST /api/papers/{id}/refresh-doi-metadata`

用途：

- 对已有文献重新按 DOI 拉取结构化元数据
- 适合历史文献补录标题、摘要、作者、期刊和发表时间

请求类型：

- `application/json`

常用 JSON 字段：

```json
{
  "doi": "10.1038/nature12373"
}
```

说明：

- `doi` 可选；不传时默认使用当前文献已保存的 `papers.doi`。
- 成功后会更新 `title`、`abstract_text`、`authors_text`、`journal`、`published_at`，但不会覆盖标签、分组、管理笔记、文献笔记和 PDF 全文。
- 若没有可用 DOI，返回 `INVALID_ARGUMENT`。
- 若 DOI 查不到可用元数据，返回 `NOT_FOUND`。

#### `PUT /api/papers/{id}`

用途：

- 更新文献详情
- 保存管理笔记
- 保存文献笔记
- 保存或修正 PDF 全文

常用 JSON 字段：

```json
{
  "title": "Paper title",
  "doi": "10.1038/nature12373",
  "authors_text": "Ada Lovelace, Alan Turing",
  "journal": "Nature Communications",
  "published_at": "2023-01-18",
  "pdf_text": "完整 PDF 文本，可为空字符串",
  "abstract_text": "摘要",
  "notes_text": "管理笔记",
  "paper_notes_text": "文献笔记",
  "group_id": 1,
  "tags": ["tag-a", "tag-b"]
}
```

说明：

- `pdf_text` 为可选字段；传入时会覆盖当前全文内容。
- `pdf_text` 支持保存原始全文，也支持在前端按 Markdown 形式手动整理；后端会按原样存储。
- 若不传 `pdf_text`，后端会保留已有全文，不会因为只保存笔记或标签而清空正文。

#### `POST /api/papers/{id}/pdf-text`

用途：

- 单独保存或覆盖 PDF 全文
- 供人工框选页、浏览器端 pdf.js 提取等流程调用

请求体：

```json
{
  "pdf_text": "从 PDF 提取出的完整全文"
}
```

说明：

- 只更新 `pdf_text`，不会改动标题、标签、笔记或分组。
- `pdf_text` 不能为空字符串。

#### `GET /api/papers/{id}/pdf-annotations`

用途：

- PDF 阅读器加载某篇文献已有高亮

说明：

- 响应中的 `id` 和 `paper_id` 为字符串，避免浏览器端处理大整数 ID 时丢失精度

返回：

```json
{
  "success": true,
  "annotations": [
    {
      "id": "10",
      "paper_id": "42",
      "type": "highlight",
      "page_start": 3,
      "page_end": 3,
      "quote_text": "selected text",
      "color": "yellow",
      "fragments": [
        { "page": 3, "left": 0.12, "top": 0.34, "width": 0.28, "height": 0.018 }
      ],
      "note_text": "",
      "created_at": "2026-05-22T10:00:00Z",
      "updated_at": "2026-05-22T10:00:00Z"
    }
  ]
}
```

#### `POST /api/papers/{id}/pdf-annotations`

用途：

- PDF 阅读器保存一次划选高亮

请求体：

```json
{
  "type": "highlight",
  "quote_text": "selected text",
  "color": "yellow",
  "fragments": [
    { "page": 3, "left": 0.12, "top": 0.34, "width": 0.28, "height": 0.018 }
  ]
}
```

说明：

- `type` 可省略，默认 `highlight`；当前只允许 `highlight`
- `color` 可省略，默认 `yellow`；当前只允许 `yellow`
- `quote_text` 会 trim，不能为空，最多 10,000 字符
- `fragments` 必须包含 1 到 200 个归一化矩形；页码从 1 开始，坐标和宽高必须落在页面范围内
- `page_start` / `page_end` 由后端根据 `fragments` 计算，不信任客户端传值

返回：

```json
{
  "success": true,
  "annotation": {
    "id": "11",
    "paper_id": "42",
    "type": "highlight",
    "page_start": 3,
    "page_end": 3,
    "quote_text": "selected text",
    "color": "yellow",
    "fragments": [
      { "page": 3, "left": 0.12, "top": 0.34, "width": 0.28, "height": 0.018 }
    ],
    "note_text": "",
    "created_at": "2026-05-22T10:00:00Z",
    "updated_at": "2026-05-22T10:00:00Z"
  }
}
```

#### `GET /api/pdf-annotations`

用途：

- 全局高亮库按文献和高亮内容搜索所有 PDF 高亮
- 供 `/highlights` 页面展示、分页和跳转回 PDF 原文位置

查询参数：

- `query`：可选关键词，会匹配高亮文本、文献标题、原始文件名和 DOI
- `sort`：可选排序方式，支持 `updated_desc`、`updated_asc`、`created_desc`、`created_asc`，默认 `updated_desc`
- `page`：可选页码，从 1 开始，默认 1
- `page_size`：可选每页数量，默认 50，最大 100

说明：

- 当前只返回 PDF 高亮类型标注
- `id` 和 `paper_id` 为字符串，避免浏览器端处理大整数 ID 时丢失精度
- `paper_pdf_url` 存在时可用于打开 PDF；前端跳转时会附带 `paper_id`、`page` 和 `annotation_id`

返回：

```json
{
  "success": true,
  "annotations": [
    {
      "id": "11",
      "paper_id": "42",
      "paper_title": "Paper title",
      "paper_original_filename": "paper.pdf",
      "paper_pdf_url": "/files/papers/paper.pdf",
      "type": "highlight",
      "page_start": 3,
      "page_end": 3,
      "quote_text": "selected text",
      "color": "yellow",
      "fragments": [
        { "page": 3, "left": 0.12, "top": 0.34, "width": 0.28, "height": 0.018 }
      ],
      "note_text": "",
      "created_at": "2026-05-22T10:00:00Z",
      "updated_at": "2026-05-22T10:00:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "page_size": 50,
    "total": 1,
    "total_pages": 1
  }
}
```

#### `DELETE /api/papers/{id}/pdf-annotations/{annotation_id}`

用途：

- 删除当前文献下的一条 PDF 高亮

说明：

- 后端会校验 `annotation_id` 属于路径中的文献 ID；不属于时返回 `NOT_FOUND`

返回：

```json
{ "success": true }
```

#### `DELETE /api/papers/{id}`

用途：

- 删除整篇文献及其图片

#### `POST /api/papers/{id}/reextract`

用途：

- 重新触发解析

#### `GET /api/papers/{id}/manual-extraction`

用途：

- 获取人工框选工作区数据

#### `POST /api/papers/{id}/manual-extraction`

用途：

- 提交人工选框生成图片
- 人工流程中的全文提取不依赖此接口，前端会通过单独的 `POST /api/papers/{id}/pdf-text` 保存全文

请求体：

```json
{
  "regions": [
    {
      "page_number": 1,
      "x": 0.12,
      "y": 0.2,
      "width": 0.46,
      "height": 0.28,
      "source": "manual",
      "image_data": "data:image/png;base64,..."
    }
  ]
}
```

说明：

- `source` 可选，默认是 `manual`
- 内置 LLM 自动提图生成的图片会把 `source` 设成 `llm`

#### `GET /api/papers/{id}/manual-preview?page={n}`

用途：

- 获取人工处理页的 PDF 预览图

#### `POST /api/papers/purge`

用途：

- 清空整个文献库

### 图片 Figures

#### `GET /api/figures`

用途：

- 图片库
- 图片笔记页

常用查询参数：

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| `keyword` | string | 文献标题、caption、图片笔记、图片标签搜索 |
| `group_id` | int | 来源分组 |
| `tag_id` | int | 图片标签 |
| `has_notes` | bool | 仅显示带图片笔记的图片 |
| `sort_by` | string | 可选：`updated_at` 按图片更新时间倒序；`created_at` 按图片创建时间倒序；`paper_created_at_figure_index` 按文献创建时间倒序，文献内按 `Fig 1`、`Fig 2` 顺序 |
| `page` | int | 页码 |
| `page_size` | int | 每页数量 |

返回：

- `figures`
- `total`
- `page`
- `page_size`
- `total_pages`

补充字段：

- 图片返回里会带 `parent_figure_id` / `subfigure_label`，用于区分子图
- 如果图片已经绑定配色，还会带 `palette_count`、`palette_id`、`palette_name`、`palette_colors`
- 顶层图片列表默认只返回主图，不返回子图

#### `PUT /api/figures/{id}`

用途：

- 更新图片 caption
- 更新图片标签
- 更新图片笔记

#### `POST /api/figures/{id}/subfigures`

用途：

- 从当前一级大图里记录子图裁剪区域
- 子图只保存裁剪区域元数据，不再单独生成图片文件
- 可以手动指定子图字母后缀；如果不指定，后端会自动分配不重复的 `a` / `b` / `c`

请求体：

```json
{
  "regions": [
    {
      "x": 0.12,
      "y": 0.18,
      "width": 0.4,
      "height": 0.45,
      "label": "b",
      "caption": "Panel A"
    }
  ]
}
```

说明：

- 坐标使用相对当前图片的归一化比例，范围为 `0-1`
- `label` 可选，但只支持英文字母；如果传入大写，后端会自动转成小写
- 当前只支持从一级大图提取子图，不支持“子图再拆子图”
- 子图不会出现在图片库或图片笔记页；它主要作为主图下的局部区域，用于预览和提取配色

#### `GET /api/figures/{id}/image`

用途：

- 读取指定图片的展示图像
- 对主图直接返回原始图片文件
- 对子图按父图坐标动态裁剪并返回预览图

#### `GET /api/figures/{id}/transfer-package`

用途：

- 导出单张主图或子图的 Figure Transfer Package
- 返回 `application/zip` 附件，文件名为 `citebox-figure-{id}-transfer-package.zip`
- 供 ScientificFigureLibrary、个人 Gallery 或其他策展工具消费；CiteBox 不在此接口生成 R 代码或写入外部图库

ZIP 固定包含两个文件：

```text
citebox-figure-{id}-transfer-package.zip
├── manifest.json
└── figure.<ext>
```

当前 manifest 遵循 Figure Transfer Package v1：schema 为 `figure-transfer-package.v1`，version 为数值 `1`。示例：

```json
{
  "schema": "figure-transfer-package.v1",
  "version": 1,
  "producer": {
    "name": "CiteBox",
    "version": "v0.31.0"
  },
  "exportedAt": "2026-08-01T02:03:04Z",
  "source": {
    "sourceId": "citebox:figure:42",
    "figureId": 42,
    "parentFigureId": null,
    "figureLabel": "Fig 2",
    "subfigureLabels": ["a", "b"],
    "caption": "Figure caption",
    "page": 7,
    "paper": {
      "id": 9,
      "title": "Paper title",
      "authors": ["Ada Lovelace", "Alan Turing"],
      "year": 2024,
      "publishedAt": "2024-06-03",
      "journal": "Journal name",
      "doi": "10.1234/example",
      "url": "https://doi.org/10.1234/example"
    },
    "license": {
      "scope": "unknown",
      "text": null
    },
    "extractionMethod": "auto"
  },
  "figure": {
    "file": "figure.png",
    "mediaType": "image/png",
    "bytes": 12345,
    "sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    "kind": "figure",
    "number": 2
  }
}
```

字段与兼容规则：

- `source.sourceId` 是同一 CiteBox 文献库内稳定的 `citebox:figure:{id}` 标识；重复导出同一 Figure 时保持不变
- `figure.kind` 为 `figure` 或 `subfigure`；子图会把 `source.parentFigureId` 设为父图 ID，并把自身标签放入 `source.subfigureLabels`；主图的 `subfigureLabels` 列出其已有子图标签
- `paper.authors` 由 CiteBox 作者文本按逗号拆分；无法从 `publishedAt` 读取年份时 `year` 为 `null`；`journal`、`doi`、`url` 缺失时为 `null`
- DOI 存在且格式有效时，`paper.url` 使用 DOI 解析地址；否则为 `null`
- CiteBox 当前没有论文级授权字段，因此 `license.scope` 显式为 `unknown`、`license.text` 为 `null`，不会推断授权状态
- `figure.mediaType` 仅支持 `image/png`、`image/jpeg`、`image/webp`、`image/svg+xml`、`application/pdf`，其他媒体类型的图片无法导出
- 包内不包含原始文件名、机器绝对路径、数据库路径、本地 PDF 地址或 `/files/...` 私有位置

校验规则：

- ZIP 必须只包含 `manifest.json` 和 manifest 指定的 `figure.<ext>`，且不得包含目录、重复项或路径穿越项
- manifest 必须严格匹配 `figure-transfer-package.v1`（version `1`），主图与子图的父子字段必须自洽
- 消费方必须同时核对 `figure.bytes` 和 `figure.sha256`；不匹配时必须拒绝该包
- CiteBox 在响应前会执行同样的结构、字节数和 SHA-256 校验；无法生成有效包时返回错误，不发送损坏附件

#### `POST /api/figures/{id}/palette`

用途：

- 从某张子图里提取并保存一组绑定配色
- 当前只支持对子图调用，不支持直接对一级大图保存配色
- 同一张图片最多保留一组当前配色；再次调用会更新这组颜色

请求体：

```json
{
  "colors": ["#AABBCC", "#DDEEFF"]
}
```

可选字段：

- `name`：自定义配色名称；不传时后端会默认生成如 `Fig 1a 配色`

返回：

- `palette`
- `paper`

说明：

- `palette.figure_id` 表示这组颜色绑定到哪一张图片
- 前端通常会在子图卡片底部直接调用这个接口，而不是做独立的“无来源配色”保存

常用 JSON 字段：

```json
{
  "caption": "figure caption",
  "notes_text": "图片笔记",
  "tags": ["confocal", "sam"]
}
```

说明：

- `caption` 和 `notes_text` 都支持按需更新
- 成功后返回的是更新后的整篇 `paper`，便于前端同步当前文献状态

#### `DELETE /api/figures/{id}`

用途：

- 删除单张图片

### 配色 Palettes

#### `GET /api/palettes`

用途：

- 配色库列表页
- 按来源文献、分组和子图说明回看已保存配色

常用查询参数：

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| `keyword` | string | 配色名称、文献标题、子图 caption、子图后缀搜索 |
| `group_id` | int | 来源分组 |
| `sort_by` | string | 可选：`updated_at` 按配色更新时间倒序；`created_at` 按配色创建时间倒序；`paper_created_at_figure_index` 按文献创建时间倒序，文献内按 `Fig 1`、`Fig 2` 顺序 |
| `page` | int | 页码 |
| `page_size` | int | 每页数量 |

返回：

- `palettes`
- `total`
- `page`
- `page_size`
- `total_pages`

返回项里会包含：

- `figure_id`、`figure_display_label`、`parent_display_label`
- `image_url`，方便直接回看来源图片
- `colors`，即 `#RRGGBB` 数组

#### `DELETE /api/palettes/{id}`

用途：

- 删除一组已保存配色
- 只移除配色绑定，不删除原始图片或子图记录

### 分组 Groups

#### `GET /api/groups`

返回：

- `groups`

#### `POST /api/groups`

请求体：

```json
{
  "name": "Plant",
  "description": "植物相关文献"
}
```

#### `PUT /api/groups/{id}`

请求体同创建。

#### `DELETE /api/groups/{id}`

用途：

- 删除分组

### 标签 Tags

#### `GET /api/tags`

常用查询参数：

| 参数 | 说明 |
| --- | --- |
| `scope` | `paper` 或 `figure` |

返回：

- `tags`

#### `POST /api/tags`

请求体：

```json
{
  "scope": "paper",
  "name": "flowering",
  "color": "#A45C40"
}
```

#### `PUT /api/tags/{id}`

请求体：

```json
{
  "name": "updated-tag",
  "color": "#416788"
}
```

#### `DELETE /api/tags/{id}`

用途：

- 删除标签及关联

### AI 接口

#### `GET /api/ai/settings`

用途：

- 获取 AI 配置页当前设置

#### `GET /api/ai/settings/defaults`

用途：

- 获取后端推荐默认 AI 设置

#### `PUT /api/ai/settings`

用途：

- 兼容性接口：整块保存 AI 设置

主要字段：

- `provider`
- `api_key`
- `base_url`
- `model`
- `openai_legacy_mode`
- `models`
- `scene_models`
- `temperature`
- `max_output_tokens`
- `max_figures`
- `system_prompt`
- `qa_prompt`
- `figure_prompt`
- `tag_prompt`
- `group_prompt`
- `translate_prompt`
- `tts_prompt`
- `translation`
- `role_prompts`

说明：

- `scene_models` 中支持按场景绑定模型，包括 `assistant_master_model_id`、`assistant_subagent_model_id`、`translate_model_id`、`im_intent_model_id` 和 `tts_model_id`
- AI 助手对话最终回答使用 `assistant_master_model_id`；会话标题、历史摘要等轻量后台任务使用 `assistant_subagent_model_id`。当前全文检索工具本身不调用 LLM，后续查询改写/任务拆解会复用 Sub-Agent 绑定。
- `translation` 为翻译规则设置，例如：

```json
{
  "primary_language": "中文",
  "target_language": "英文"
}
```

- `role_prompts` 是用户自定义的角色 Prompt 列表；每个角色包含：
  - `name`
  - `prompt`

#### `PUT /api/ai/settings/models`

用途：

- 单独保存模型配置、场景绑定、温度、图片数量上限、上下文预算、单会话 pin 上限、翻译规则，以及 `@image-gen` 的图像生成配置

请求体：

```json
{
  "models": [
    {
      "id": "default-openai",
      "name": "OpenAI Default",
      "provider": "openai",
      "api_key": "...",
      "base_url": "https://api.openai.com",
      "model": "gpt-4.1-mini",
      "max_output_tokens": 1200,
      "openai_legacy_mode": false,
      "supports_images": true
    }
  ],
  "scene_models": {
    "default_model_id": "default-openai",
    "assistant_master_model_id": "default-openai",
    "assistant_subagent_model_id": "default-openai",
    "qa_model_id": "default-openai",
    "im_intent_model_id": "default-openai",
    "figure_model_id": "default-openai",
    "tag_model_id": "default-openai",
    "group_model_id": "default-openai",
    "translate_model_id": "default-openai",
    "tts_model_id": "default-openai"
  },
  "temperature": 0.2,
  "max_figures": 0,
  "pin_papers_limit": 5,
  "context_budget_tokens": 32000,
  "image_gen": {
    "enabled": true,
    "api_key": "sk-image",
    "base_url": "https://api.openai.com",
    "model": "gpt-image-2",
    "size": "1024x1024",
    "quality": "high"
  },
  "translation": {
    "primary_language": "中文",
    "target_language": "英文"
  }
}
```

说明：

- `image_gen` 为可选字段；如果本次请求未传，后端会保留当前已保存的图像生成配置。
- `image_gen.size` 目前支持：`1024x1024`、`1024x1536`、`1536x1024`。
- `image_gen.quality` 目前支持：`low`、`medium`、`high`。

#### `PUT /api/ai/settings/prompts`

用途：

- 单独保存 Prompt 配置

请求体：

```json
{
  "system_prompt": "你是一名帮助用户阅读科研论文的 AI 助手。",
  "qa_prompt": "请结合全文和图片回答用户问题。",
  "figure_prompt": "优先说明图像内容、结论和局限。",
  "tag_prompt": "优先复用已有图片标签。",
  "group_prompt": "优先复用已有分组。",
  "translate_prompt": "只返回译文正文。",
  "tts_prompt": "删除 Markdown 和图片引用，把文本整理成适合 TTS 直接朗读的版本。"
}
```

#### `GET /api/ai/role-prompts`

用途：

- 单独获取用户保存的角色 Prompt 列表

返回：

```json
{
  "role_prompts": [
    {
      "name": "证据审查",
      "prompt": "你是一名严格的论文审稿人，优先指出证据链、方法缺口和结论边界。"
    }
  ]
}
```

#### `PUT /api/ai/role-prompts`

用途：

- 保存、覆盖或清空用户角色 Prompt 列表

请求体：

```json
{
  "role_prompts": [
    {
      "name": "证据审查",
      "prompt": "你是一名严格的论文审稿人，优先指出证据链、方法缺口和结论边界。"
    }
  ]
}
```

说明：

- AI 助手聊天框支持通过 `@角色名` 直接调用已保存的角色 Prompt。
- 旧的 `/api/ai/prompt-presets` 仍作为兼容别名保留，但语义已与 `role-prompts` 一致。

#### AI 助手会话接口

会话接口统一位于 `/api/ai/conversations`，用于 AI 页面侧边栏、已钉文献、内部搜索、外部搜索和流式对话。

常用接口：

- `GET /api/ai/conversations?q=&limit=&offset=`：列出会话。
- `GET /api/ai/conversations/{id}`：读取会话详情、已钉文献和最近消息。
- `PATCH /api/ai/conversations/{id}`：更新标题或内部搜索开关。
- `POST /api/ai/conversations/{id}/papers`：给会话 pin 文献。
- `DELETE /api/ai/conversations/{id}/papers/{paper_id}`：移除 pin。
- `POST /api/ai/conversations/{id}/messages`：发送消息，返回 `application/x-ndjson` 流。
- `POST /api/ai/conversations/new/messages`：创建新会话并发送第一条消息。

更新会话请求体示例：

```json
{
  "title": "单细胞证据检索",
  "strict_evidence": true
}
```

发送消息请求体示例：

```json
{
  "content": "@PubMed 这些文献是否支持单细胞 RNA-seq 可以解析植物表皮发育轨迹？",
  "paper_id": 42,
  "intent_hint": "external_search",
  "search_goal_hint": "evidence",
  "sources": ["pubmed"],
  "context": {
    "source": "ai",
    "paper_id": 42
  }
}
```

字段说明：

- `content`：用户问题或主张。
- `paper_id`：可选，仅用于新会话或发送时自动 pin 当前文献。
- `intent_hint`：可选的一次性路由提示。支持 `library_search`（查全库）、`external_search`（查外部）、`paper_read`（读文献）、`figure_lookup`（看图/图文）、`remote_mcp`（调用已配置的 Notion MCP）。省略时由后端按内容和上下文自动判断。前端在用户输入 `@PubMed` / `@SemanticScholar` / `@Library` / `@Figure` / `@Notion` 等工具标签时会自动填充该字段。
- `search_goal_hint`：可选字符串。支持 `discovery` 和 `evidence`；用于显式指定外部搜索目标，优先级高于 planner 推断出的 `search_goal`。`discovery` 适合找方向、找综述、扩展候选，`evidence` 适合核查具体断言、找直接出处。前端快捷入口通常会和 `intent_hint == "external_search"` 一起发送；后端也会把合法的 `search_goal_hint` 视为显式工具请求信号，用它优先走 orchestrator 而不是旧外部证据注入路径。非法值会被安全忽略，仍按 planner / 默认回退逻辑执行。
- `sources`：可选字符串数组，仅在 `intent_hint == "external_search"` 时被读取。取值为外部源 ID 子集，例如 `["pubmed"]`、`["semantic_scholar"]` 或 `["pubmed","semantic_scholar"]`。当用户在输入框打了 `@PubMed`/`@SemanticScholar` 显式指定外部源时，前端会带上此字段；后端会与设置中"已启用源"取交集执行检索，被显式指定但未启用的源会以 `ErrSourceDisabled` 写入失败列表，并在 `Process.Note` 中提示"用户显式指定但未启用的源: …（请前往设置页启用）"。省略或为空数组时，等同当前默认行为（跑所有已启用源）。
- `context`：可选上下文对象，支持 `source`、`paper_id`、`paper_ids`、`figure_id`。用于指定当前文献、对比文献或图片上下文。当 `intent_hint == "library_search"` 且 `paper_ids` 非空时（典型场景：用户同时输入 `@Library @<paper>`），后端会把候选集裁剪到该 PaperIDs 集合内。
- `replace_last`：可选布尔值。为 `true` 时，服务端会先删除当前会话最后一轮用户消息及其后的回答、流程和结果卡片，再用本次 `content` 重新发送；仅适用于已有会话。
- `strict_evidence`：兼容字段；历史上对应“内部搜索”开关。当前主 UI 使用 `intent_hint` 和 `context` 调度工具。没有显式 `intent_hint`/`context` 时，旧内部搜索语义仍保留。
- `include_external_evidence`：兼容字段；历史上对应“外部搜索”开关。没有显式 `intent_hint`/`context` 时，仍使用旧外部 Semantic Scholar 证据检索语义。

消息流返回 `application/x-ndjson`。除既有 `meta`、`delta`、`final`、`error` 外，AI 助手工具调度还可能返回：

- `process`：紧凑流程摘要，用于展示扫描阶段、命中数和状态。
- `cards`：结构化结果卡片，例如 `paper_hit`、`external_paper`、`paper_read`、`paper_compare`、`figure_result`。
  - `paper_hit.payload.highlight_terms`：本地全文扫描实际使用的检索词数组，前端用于在证据片段中高亮命中词；旧消息可能没有该字段。
  - `external_paper.payload`：外部出处检索卡片。常见字段包括 `matched_query`、`reason`、`search_goal`、`tier`、`year_label`、`article_role`、`matched_constraints`、`matched_preferences`、`evidence_annotations`。
    - `search_goal`：`discovery` 或 `evidence`。`discovery` 表示“找方向 / 找候选 / 找综述”一类外部摸排；`evidence` 表示“核查具体断言 / 找直接出处”一类证据检索。
    - `tier`：候选分层结果。当前会返回 `strong_match`、`weak_match`、`needs_review`（内部还可能出现 `drop`，但不会作为结果卡片返回给前端）。
    - `year_label`：优先展示的年份标签，例如同时包含 online year / issue year 的字符串；旧消息或没有年份细分时可以只看 `year`。
    - `matched_constraints`：分类阶段确认已满足的硬约束（来自 planner 的 `must_match`）。
    - `matched_preferences`：分类阶段确认已命中的软偏好（来自 planner 的 `soft_preferences`）。
    - `article_role`：分类阶段归纳的文献角色，例如 `primary_study`、`review`、`meta_analysis`、`method` 等。
    - `evidence_annotations`：仅在需要说明证据对应关系时返回，标注用户原句片段、候选原文证据、支持状态和简短理由。
    - 语义约定：外部检索先做多查询召回，再由 Sub-Agent 做候选分层和证据判定。`discovery` 模式会保留 `weak_match` / `needs_review` 候选，便于继续筛选；`evidence` 模式只把 `strong_match` 作为正式证据卡片返回，弱相关或待核查候选只体现在流程摘要中，不进入最终证据卡片和 citations。
  - `figure_result.payload.evidence_text` / `evidence_location`：当图文检索由全文候选文献回退产生时，记录支持该候选图的本地全文证据片段和位置；旧消息或直接图注命中可能没有该字段。
- `citations`：证据引用数组，用于脚注和结果卡片引用。
- 当工具调度已经完成但最终模型回答失败或返回空文本时，服务端会优先返回已完成的工具证据，并在最终 assistant 消息中使用 `mode="tool_fallback"`；客户端应按普通 assistant 文本和已返回结果卡片展示。

搜索模式说明：

- 默认证据来源是本地文献库的标题、摘要、笔记和 `pdf_text`；已钉文献会优先参与检索，但未 pin 的入库文献也可被命中。
- 本地证据检索现在按 Master/Sub-Agent 流程执行：Master 模型先把用户请求改写成精确全文扫描词，Sub-Agent 模型再逐篇判定候选文献是否真正符合需求，最后由 Master 基于判定结果生成回答。
- 本地候选召回仍使用关键词扩展和字面全文扫描，例如 `ATAC 数据` 会扩展匹配 `ATAC-seq`、`chromatin accessibility`、`scATAC-seq` 等表述；泛词（如“数据”“文章”）不会单独作为召回词。
- 外部出处检索现在支持 Master 生成多个 Semantic Scholar 查询式并行召回；结果按外部 ID / DOI / 标题去重，再由 Sub-Agent 判断候选文本中哪句话能对应用户原句，最终只把通过判定或需要人工核查的候选交给 Master 汇总。
- 图文检索会先检索图片 caption、笔记、标签和来源文献标题；若没有命中且未限定单篇文献，会用同一组关键词做本地全文候选文献扫描，再返回候选文献下可供检查的图片，并在 `figure_result` 中附带全文证据。
- 不使用 embedding，不使用向量数据库。
- 外部搜索可以独立开启；在同时开启内部搜索时作为本地证据的补充。Semantic Scholar 限流或失败时，本地证据仍可继续用于回答。
- `/api/research/*` 调研接口在 Semantic Scholar 返回 `429 Too Many Requests` 时，会返回标准错误壳，并在可判定时额外带上 `used_api_key` 布尔字段，帮助前端区分“本次请求已携带 API Key”还是“本次请求未携带 API Key”。

示例：

```json
{
  "success": false,
  "code": "UNAVAILABLE",
  "error": "Semantic Scholar 限流，请稍后再试",
  "used_api_key": true
}
```

- `used_api_key=true` 表示运行中的调研客户端这次请求会发送 `x-api-key`
- `used_api_key=false` 表示这次请求走的是匿名额度，前端可提示用户检查 `/settings#settings-external-sources`
- 助手消息的 `citations_json` 会保存本地或外部搜索证据片段，前端用 `[n]` 脚注展示。

#### `GET /api/ai-generated-images/:id/file`

用途：

- 返回指定 AI 生成图片的原始 PNG 字节流。

说明：

- 需要登录鉴权（由全局 `authMiddleware` 覆盖）。
- 若 `ai_generated_images` 表中不存在对应 `id` 的行，返回 `404`。
- 路径参数 `id` 为 `ai_generated_images` 表的整数主键。
- 响应头：`Content-Type: image/png`，`Cache-Control: private, max-age=86400`。

#### 会话流式事件：图片生成

当用户发送的消息中包含 `@image-gen` 标签时，会话流会按顺序额外发出以下事件类型：

- `image_prompt_drafting` — 视觉理解阶段已启动。Payload：`{ "turn_run_id": <int> }`。
- `image_prompt_drafted` — 视觉理解阶段已生成图片 Prompt。Payload：`{ "turn_run_id": <int>, "prompt": "<text>" }`。
- `image_generating` — 即将调用图片生成 API。Payload：`{ "turn_run_id": <int>, "model": "<str>", "size": "<str>", "quality": "<str>", "cost_estimate_usd": <float> }`。
- `image_generated`（成功）— 最终卡片已落盘。Payload：`{ "turn_run_id": <int>, "card": { ... } }`。
- `image_failed`（任意失败）— Payload：`{ "turn_run_id": <int>, "stage": "vision|image_api|save", "reason": "<str>" }`。

成功落盘的结果卡片 `card_type` 为 `"generated_image"`，Payload 结构如下：

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

#### 已钉文献携带图片列表

`GET /api/ai-conversations/:id` 现在会在每条已钉文献条目上附带 `figures` 数组，供前端 `@figure` 提及调板使用。每个条目包含 `id`、`page_number`、`figure_index`、`caption`。子图不在其中。

#### 发送消息请求体扩展

`POST /api/ai-conversations/:id/messages` 现在支持 `context.figure_ids: [<int>]`，当消息文本中包含 `@figure-<id>` 提及时由前端填充。每个 id 必须指向已钉文献下的图片。

#### `POST /api/ai/settings/check-model`

用途：

- 校验某个模型配置是否可用

请求体通常是单个模型配置对象：

```json
{
  "id": "default-openai",
  "name": "OpenAI Default",
  "provider": "openai",
  "api_key": "...",
  "base_url": "https://api.openai.com",
  "model": "gpt-4.1-mini",
  "max_output_tokens": 1200,
  "openai_legacy_mode": false,
  "supports_images": true,
  "omit_temperature": false,
  "thinking_enabled": false,
  "reasoning_effort": ""
}
```

OpenAI 兼容模型配置说明：

- `openai_legacy_mode=true` 时使用 Chat Completions，适合 DeepSeek 等兼容网关。
- `supports_images=false` 时，该模型不会接收 PDF 图片输入；`paper_qa` 会自动降级为文本问答，图片解读等必须视觉输入的场景会提示更换模型或打开该能力。
- `omit_temperature=true` 会跳过 `temperature` 字段；`gpt-5*`、`o1*`、`o3*`、`o4*`、`o5*` 模型也会自动跳过，避免模型检查返回 `Unsupported parameter: 'temperature'`。
- `thinking_enabled=true` 在 Chat Completions 请求体加入 `{"thinking":{"type":"enabled"}}`；在 Responses 模式下会启用 `reasoning` 对象，未设置 `reasoning_effort` 时默认使用 `medium`。
- `reasoning_effort` 可填 `minimal`、`low`、`medium`、`high`、`xhigh`；Chat Completions 发送为 `reasoning_effort`，Responses 发送为 `reasoning.effort`。
- DeepSeek 根地址 `https://api.deepseek.com` 在 Chat Completions 模式下会调用 `/chat/completions`；其他 OpenAI 兼容网关仍按 `/v1/chat/completions` 拼接。

#### `POST /api/ai/read`

用途：

- 非流式 AI 阅读
- 返回完整 JSON 结果

请求体：

```json
{
  "paper_id": 1,
  "figure_id": 12,
  "action": "paper_qa",
  "question": "请总结这篇文章",
  "include_figures": false,
  "history": [
    {
      "question": "上一轮问题",
      "answer": "上一轮回答"
    }
  ]
}
```

`action` 当前支持：

- `paper_qa`
- `figure_interpretation`
- `tag_suggestion`
- `group_suggestion`

可选字段：

- `include_figures`: 仅 `paper_qa` 支持。设为 `false` 时，本次问答不附带论文图片，适合 PDF 划选解释或只支持文本的模型；省略时保持默认行为，按“图像数量上限”附带图片。

#### `POST /api/ai/translate`

用途：

- 桌面端划词翻译
- 不依赖 `paper_id`
- 根据 AI 配置中的翻译规则自动判断方向

请求体：

```json
{
  "text": "这是需要翻译的内容"
}
```

返回体示例：

```json
{
  "success": true,
  "provider": "openai",
  "model": "gpt-4.1-mini",
  "mode": "responses",
  "source_language": "中文",
  "target_language": "英文",
  "translation": "This is the translated text."
}
```

#### `POST /api/ai/detect-figure-regions`

用途：

- 内置 LLM 提图流程会调用这个接口
- 前端先用 `pdf.js` 渲染单页，再把页面图像发给后端
- 后端调用当前“图片场景”多模态模型，返回归一化后的主图坐标

请求体示例：

```json
{
  "paper_id": 123,
  "page_number": 4,
  "page_width": 1280,
  "page_height": 1810,
  "image_data": "data:image/jpeg;base64,..."
}
```

返回体示例：

```json
{
  "success": true,
  "provider": "openai",
  "model": "gpt-4.1-mini",
  "page_number": 4,
  "regions": [
    {
      "x": 0.11,
      "y": 0.17,
      "width": 0.62,
      "height": 0.51,
      "confidence": 0.93
    }
  ]
}
```

#### `POST /api/ai/read/stream`

用途：

- 流式 AI 阅读
- 主要用于自由提问和图片解读的流式输出

请求体和 `/api/ai/read` 相同。

当前支持的 `action`：

- `paper_qa`
- `figure_interpretation`

事件类型常见有：

- `meta`
- `delta`
- `final`
- `done`
- `error`

说明：

- `meta` 会先返回模型、模式、文献 ID、问题文本等元信息
- `delta` 是增量文本片段，前端可即时拼接渲染
- `final` 会返回标准化后的完整结果对象

#### `POST /api/ai/read/export`

用途：

- 导出单轮 AI 回答
- 导出整段 AI 对话

请求体：

```json
{
  "paper_id": 1,
  "answer": "单轮 Markdown",
  "content": "整段对话 Markdown",
  "scope": "turn",
  "turn_index": 0
}
```

说明：

- `scope = "turn"` 时导出单轮回答
- `scope = "conversation"` 时导出整段对话
- 返回 `application/zip`

### 提取器设置

#### `GET /api/settings/version`

用途：

- 获取当前运行版本，以及与 GitHub 最新正式 Release 的比较结果

查询参数：

| 参数 | 说明 |
| --- | --- |
| `refresh=1` | 强制刷新，不走服务端短时缓存 |

返回字段包括：

- `current_version`
- `build_time`
- `latest_version`
- `latest_release_url`
- `published_at`
- `checked_at`
- `status`
- `is_latest`
- `has_update`
- `message`

状态值说明：

- `latest`：当前就是最新正式版本
- `update_available`：GitHub Release 上有更高版本
- `ahead`：当前构建高于或晚于最新正式 Release，例如开发构建
- `unknown`：当前版本号不可比较，或暂时无法获取远端版本信息

#### `GET /api/settings/extractor`

用途：

- 获取当前提取服务配置

返回字段包括：

- `extractor_profile`
- `pdf_text_source`
- `extractor_url`
- `extractor_jobs_url`
- `extractor_token`
- `extractor_file_field`
- `timeout_seconds`
- `poll_interval_seconds`
- `effective_extractor_url`
- `effective_jobs_url`

#### `PUT /api/settings/extractor`

用途：

- 保存提取器配置

常用字段：

- `extractor_profile`：`manual`、`pdffigx_v1` 或 `open_source_vision`
- `pdf_text_source`：兼容旧字段保留，但当前由后端按 `extractor_profile` 自动归一化；`manual` / `open_source_vision` 固定为 `pdfjs`，`pdffigx_v1` 固定为 `extractor`
- 其余字段与提取接口地址、鉴权和超时设置相同

#### `GET /api/settings/research`

用途：

- 获取调研服务 API 凭据
- Semantic Scholar 配置用于 `/research` 调研页
- PubMed / NCBI 配置用于 AI 助手外部搜索中的 PubMed 来源

返回示例：

```json
{
  "s2_api_key": "",
  "pubmed_api_key": "",
  "pubmed_email": "",
  "pubmed_tool": "citebox"
}
```

#### `PUT /api/settings/research`

用途：

- 保存 Semantic Scholar 与 PubMed / NCBI 凭据
- 保存后热更新运行中的 Semantic Scholar 与 PubMed 客户端；若服务端环境变量显式配置了对应值，运行时客户端继续优先使用环境变量值

请求体示例：

```json
{
  "s2_api_key": "",
  "pubmed_api_key": "",
  "pubmed_email": "user@example.org",
  "pubmed_tool": "citebox"
}
```

说明：

- `pubmed_api_key`、`pubmed_email`、`pubmed_tool` 均可留空；留空时 PubMed 使用 NCBI 匿名访问
- `pubmed_tool` 省略或为空时，前端默认写入 `citebox`

#### `GET /api/settings/ai-external-search`

用途：

- 获取 AI 助手外部搜索源配置
- 默认 `sources` 为 `["pubmed"]`

返回示例：

```json
{
  "sources": ["pubmed", "semantic_scholar"]
}
```

字段说明：

- `sources`：AI 助手外部搜索启用源，可包含 `pubmed`、`semantic_scholar`
- PubMed / NCBI 凭据由 `/api/settings/research` 管理；为兼容旧客户端，此接口仍可能返回已保存的 PubMed 字段

#### `PUT /api/settings/ai-external-search`

用途：

- 保存 AI 助手外部搜索源配置
- 若请求体包含 PubMed 字段，也会兼容保存并热更新 PubMed 客户端；设置页默认通过 `/api/settings/research` 管理 PubMed 凭据

请求体示例：

```json
{
  "sources": ["pubmed"]
}
```

说明：

- `sources` 可包含 `pubmed`、`semantic_scholar`
- `sources` 也可以为空数组，表示不启用 AI 外部搜索源
- 未传 PubMed 字段时，已保存的 PubMed / NCBI 凭据会被保留，不会被清空
- Semantic Scholar API key 和 PubMed / NCBI 凭据均由研究数据库配置接口管理

#### `GET /api/settings/desktop-close`

用途：

- 获取桌面端关闭窗口行为设置

返回示例：

```json
{
  "action": "ask"
}
```

字段说明：

- `action` 可能是：
  - `ask`：每次关闭窗口都弹出确认
  - `minimize`：关闭窗口时直接最小化到托盘
  - `exit`：关闭窗口时直接退出桌面应用

#### `PUT /api/settings/desktop-close`

用途：

- 更新桌面端关闭窗口行为设置

请求体示例：

```json
{
  "action": "minimize"
}
```

返回示例：

```json
{
  "success": true,
  "settings": {
    "action": "minimize"
  }
}
```

#### `GET /api/settings/wolai`

用途：

- 获取当前 Wolai 配置

返回字段包括：

- `token`
- `parent_block_id`
- `base_url`

#### `PUT /api/settings/wolai`

用途：

- 保存当前 Wolai 配置

请求体示例：

```json
{
  "token": "wolai-token",
  "parent_block_id": "page-or-block-id",
  "base_url": "https://openapi.wolai.com"
}
```

#### `POST /api/settings/wolai/test`

用途：

- 测试当前表单里的 Wolai token 是否可用
- 同时验证 token 是否能访问指定的 `parent_block_id`

成功后返回：

```json
{
  "success": true,
  "message": "Wolai token 可用，已验证目标块访问权限"
}
```

#### `POST /api/settings/wolai/test-page`

用途：

- 创建一个 Wolai 测试页面并写入测试文本
- 当前不会执行真实图片上传，只会写入一条图片导出 TODO 说明

成功后返回：

```json
{
  "success": true,
  "message": "Wolai 测试页面已创建，并写入测试文本与图片导出 TODO",
  "target_block_id": "page-or-block-id",
  "target_block_url": "https://www.wolai.com/..."
}
```

### Notion MCP 与 OAuth

Notion MCP 使用 Streamable HTTP。连接元数据保存在应用设置中，OAuth access/refresh token 保存在 `STORAGE_DIR/mcp/oauth-credentials.json`（目录权限 `0700`、文件权限 `0600`），不会写入数据库或返回前端。

#### `GET /api/settings/mcp`

返回 Notion MCP 的名称、URL、启用状态、认证方式、授权状态与已发现工具名。默认 URL 为 `https://mcp.notion.com/mcp`。

#### `POST /api/settings/mcp/oauth/start`

仅在用户点击“测试”或“保存”时调用。后端依次完成 OAuth Protected Resource Metadata、Authorization Server Metadata 和动态客户端注册，然后返回浏览器授权地址。

```json
{
  "mode": "test",
  "name": "Notion MCP",
  "url": "https://mcp.notion.com/mcp",
  "auth_method": "oauth",
  "enabled": true
}
```

响应包含 `flow_id` 与 `authorization_url`。`mode=test` 只验证授权和 `tools/list`，不会保存配置或凭据；`mode=save` 在授权和 MCP 握手成功后保存连接。

#### `GET /api/settings/mcp/oauth/status?flow_id=...`

前端授权期间轮询。`status` 为 `pending`、`complete` 或 `error`；成功时同时返回 `tool_names`。

#### `GET /api/settings/mcp/oauth/callback`

OAuth 服务重定向到此公开回调。回调通过一次性 `state` 关联内存中的授权流程，并返回可安全关闭的 HTML 页面。该接口无需 CiteBox 会话 Cookie，但只有匹配、未过期的 `state` 才能完成授权。

#### `DELETE /api/settings/mcp`

删除保存的 OAuth 凭据并禁用连接。

### Notion API 与原图导出

Notion API 使用用户在 Notion Developer Portal 创建的个人访问令牌。令牌只保存在 `STORAGE_DIR/notion-api/personal-access-token.json`（目录权限 `0700`、文件权限 `0600`），不会写入数据库、返回前端或回填到令牌输入框。该令牌只用于“保存到 Notion”的原图与笔记导出；AI 助手中的 `@Notion` 仍使用上面的 MCP OAuth 连接。

#### `GET /api/settings/notion-api`

返回是否已配置，以及验证令牌时取得的 Notion 用户 ID/名称；响应不包含令牌。

#### `POST /api/settings/notion-api/test`

验证请求体中的 `token`。当 `token` 为空时测试本机已保存的令牌。测试不会写入凭据。

#### `PUT /api/settings/notion-api`

先通过 `GET /v1/users/me` 验证 `token`，成功后再保存到本机凭据文件。

```json
{
  "token": "ntn_..."
}
```

#### `DELETE /api/settings/notion-api`

删除本机保存的 Notion API 个人访问令牌。

#### `POST /api/notion/figures/{id}/notes`

把当前图片及图片笔记保存到 Notion。请求体：

```json
{
  "notes_text": "当前编辑器里的图片笔记草稿"
}
```

后端使用 Notion File Upload API 上传原始图片字节，并把文件作为原生 `image` block 写入页面。单张图片使用 `single_part` 直接上传，当前限制为 20 MB；不再压缩图片、不再生成 HTML attachment，也不需要公网图床。

后端会自动创建工作区级私有顶层页面 `CiteBox 图片笔记`，并为每篇 CiteBox 文献创建一个子页面。同一 `paper_id` 后续导出的原图和图片笔记会追加到同一个 Notion 文献页面。页面映射按个人令牌对应的 Notion 用户保存到应用设置 `notion_api_export_pages`；若目录或文献页被删除/归档，会自动重建映射。

成功响应：

```json
{
  "success": true,
  "message": "原始图片与笔记已保存到 Notion",
  "target_page_id": "notion-page-id",
  "target_page_url": "https://www.notion.so/...",
  "image_embedded": true
}
```

#### `GET /api/settings/weixin-bridge`

用途：

- 获取当前微信 IM 桥接开关与今日推荐配置

响应示例：

```json
{
  "enabled": true,
  "daily_recommendation": {
    "enabled": true,
    "send_time": "09:00"
  }
}
```

#### `PUT /api/settings/weixin-bridge`

用途：

- 保存当前微信 IM 桥接开关与今日推荐配置

请求体：

```json
{
  "enabled": true,
  "daily_recommendation": {
    "enabled": true,
    "send_time": "09:00"
  }
}
```

说明：

- `daily_recommendation.send_time` 使用 `HH:MM` 24 小时格式
- 留空时会自动回退到默认值 `09:00`
- 如果后台轮询检测到微信会话已过期，服务会自动把 `enabled` 关闭，避免继续重试轮询；重新绑定后可再手动开启

#### `POST /api/settings/weixin-bridge/daily-recommendation/test`

用途：

- 使用当前表单里的今日推荐配置立即向已绑定微信发送一张随机图片
- 该接口不会保存配置，也不会写入当天的定时发送状态

请求体：

```json
{
  "enabled": true,
  "send_time": "09:00"
}
```

响应示例：

```json
{
  "success": true,
  "message": "测试图片已发送到微信：Cell Atlas · Fig 1"
}
```

#### `GET /api/settings/tts`

用途：

- 获取当前独立的 TTS 配置
- `resource_id` 留空时会按默认值 `seed-tts-2.0` 返回

响应示例：

```json
{
  "app_id": "1234567890",
  "access_key": "doubao-access-key",
  "resource_id": "seed-tts-2.0",
  "speaker": "zh_female_shuangkuaisisi_moon_bigtts",
  "weixin_voice_output_enabled": true
}
```

#### `PUT /api/settings/tts`

用途：

- 保存独立的 TTS 配置
- 保存后，微信 `/ask`、`/qa` 会在成功回复后追加 TTS 音频
- 微信 `/testvoice` 也会直接调用当前已保存的 TTS 配置，合成一段 Hello World 测试音频
- `weixin_voice_output_enabled` 用于控制微信 `/ask`、`/qa`、`/testvoice` 是否真的发送语音附件；`/voiceoff` 与 `/voiceon` 也会更新这个值

请求体：

```json
{
  "app_id": "1234567890",
  "access_key": "doubao-access-key",
  "resource_id": "seed-tts-2.0",
  "speaker": "zh_female_shuangkuaisisi_moon_bigtts",
  "weixin_voice_output_enabled": true
}
```

#### `POST /api/settings/tts/test`

用途：

- 使用当前请求里的 TTS 表单配置直接合成一段测试音频
- 不会保存配置，也不依赖当前微信桥接开关
- 成功后返回原始音频文件流，供设置页内直接试听

请求体：

```json
{
  "app_id": "1234567890",
  "access_key": "doubao-access-key",
  "resource_id": "seed-tts-2.0",
  "speaker": "zh_female_shuangkuaisisi_moon_bigtts"
}
```

返回：

- 原始音频文件流，默认是 `audio/mpeg`
- `Content-Disposition` 会携带类似 `tts-test.mp3` 的文件名
- 当前测试文本固定为 `Hello World from CiteBox test voice`

### 数据库导入导出

#### `GET /api/database/export`

用途：

- 导出当前数据库备份

返回：

- 原始 `.db` 文件流
- `Content-Disposition` 文件名类似 `library_backup_YYYYMMDD_HHMMSS.db`

说明：

- 前端设置页目前通过 `<a href="/api/database/export">` 直接触发下载

#### `POST /api/database/import`

用途：

- 从备份文件恢复数据库

请求类型：

- `multipart/form-data`

表单字段：

| 字段 | 说明 |
| --- | --- |
| `database` | `.db` / `.sqlite` / `.sqlite3` 文件 |

### Wolai 笔记导出

#### `POST /api/wolai/papers/{id}/notes`

用途：

- 把当前文献笔记追加保存到 Wolai
- 使用配置页已保存的 `token` 和 `parent_block_id`

请求体示例：

```json
{
  "notes_text": "当前编辑器里的文献笔记草稿"
}
```

成功后返回：

```json
{
  "success": true,
  "message": "文献笔记已保存到 Wolai",
  "target_block_id": "page-or-block-id"
}
```

说明：

- 如果前端传了 `notes_text`，后端会优先导出当前草稿
- 后端会把标题、分组、标签、摘要等元信息整理成带标题层级的 Wolai blocks
- 笔记正文里的 Markdown 标题、列表、引用、代码块、分割线会转换成对应的 Wolai block，而不是原样写成纯文本
- 如果笔记里包含 Markdown 图片引用，当前不会上传图片到 Wolai，而是写入 TODO 占位，等待后续完成

#### `POST /api/wolai/figures/{id}/notes`

用途：

- 把当前图片笔记追加保存到 Wolai

请求体示例：

```json
{
  "notes_text": "当前编辑器里的图片笔记草稿"
}
```

成功后返回：

```json
{
  "success": true,
  "message": "图片笔记已保存到 Wolai",
  "target_block_id": "page-or-block-id"
}
```

说明：

- 后端会追加来源文献、页码 / 图号、caption、分组、标签等元信息，并整理成带标题层级的 Wolai blocks
- 笔记正文里的 Markdown 标题、列表、引用、代码块、分割线会转换成对应的 Wolai block，而不是原样写成纯文本
- 如果笔记里包含 Markdown 图片引用，当前不会上传图片到 Wolai，而是写入 TODO 占位，等待后续完成
- 这些接口不会替代原有本地保存接口，只负责额外导出到 Wolai

### 鉴权 Auth

#### `POST /api/auth/login`

请求体：

```json
{
  "username": "citebox",
  "password": "******",
  "remember_login": true
}
```

成功后：

- 后端写入会话 Cookie
- 当 `remember_login=true` 时，还会额外写入长期有效的“记住登录状态” Cookie
- 返回 `{ "success": true, "message": "登录成功" }`

#### `GET /api/auth/settings`

用途：

- 获取当前认证设置摘要
- 返回管理员用户名、密码是否已落库、微信桥接开关，以及微信绑定摘要

响应示例：

```json
{
  "username": "citebox",
  "password_from_db": false,
  "remember_login_enabled": true,
  "weixin_bridge": {
    "enabled": true,
    "daily_recommendation": {
      "enabled": true,
      "send_time": "09:00"
    }
  },
  "weixin_binding": {
    "bound": true,
    "account_id": "xxx@im.bot",
    "user_id": "xxx@im.wechat",
    "base_url": "https://ilinkai.weixin.qq.com",
    "bound_at": "2026-03-22T07:12:34Z"
  }
}
```

#### `POST /api/auth/remember-login`

用途：

- 开启或关闭当前浏览器的“记住登录状态”
- 开启后，即使普通会话 Cookie 因浏览器关闭而消失，后续请求也会自动恢复登录

请求体：

```json
{
  "enabled": true
}
```

成功后返回：

- `success`
- `remember_login_enabled`
- `message`

#### `POST /api/auth/weixin/bind`

用途：

- 发起一次新的微信绑定二维码流程

成功后返回：

```json
{
  "qrcode": "session-id",
  "qrcode_content": "https://...",
  "qrcode_data_url": "data:image/png;base64,...",
  "status": "wait",
  "message": "请使用微信扫码完成绑定"
}
```

#### `GET /api/auth/weixin/bind/status?qrcode=<session-id>`

用途：

- 轮询当前二维码绑定状态
- 在状态变为 `confirmed` 时保存绑定凭证到 `app_settings.weixin_binding`

状态值：

- `wait`
- `scaned`
- `confirmed`
- `expired`

#### `POST /api/auth/change-password`

请求体：

```json
{
  "current_password": "old-password",
  "new_password": "new-password"
}
```

成功后：

- 清空所有会话
- 清空所有“记住登录状态”令牌
- 当前用户需要重新登录

#### `POST /api/auth/logout`

用途：

- 登出并清理会话 Cookie
- 同时清理当前浏览器上的“记住登录状态”

## 与 API 配套的文件访问 URL

这部分不是 `/api`，但前端会直接使用：

### `GET /files/papers/{stored_pdf_name}`

用途：

- 打开 PDF

### 总览 Overview

#### `GET /api/overview/summary`

用途：填充 `/overview` 页面的 Research Dashboard 面板。

返回：

```json
{
  "recent_papers": [
    {"id": 42, "title": "Diffusion Models for ...", "created_at": "2026-05-02T10:11:12Z"}
  ]
}
```

#### `GET /api/overview/daily-figure`

用途：填充 `/overview` 页面的「今日推荐图片」。

返回：日期决定的同一张图（同一日多次请求结果相同）。

```json
{
  "id": 199,
  "caption": "Figure 3: ...",
  "label": "图 3",
  "paper": "Diffusion Models for ...",
  "page": 5
}
```

库为空时返回 `412 Precondition Failed`。

#### `GET /api/overview/status`

用途：填充 `/overview` 页面的 Status 面板。

返回：

```json
{
  "server_time": "2026-05-02T10:11:12Z",
  "weixin_bridge": {
    "enabled": true,
    "daily_recommendation_on": true,
    "daily_recommendation_at": "08:30"
  }
}
```

`weixin_bridge` 仅在桥接已启用时存在 `daily_recommendation_*` 字段。

### TTS 合成

#### `POST /api/ai/tts`

用途：把任意文本合成为音频。供 AI 助手页的 🔊 按钮调用。

请求：

```json
{ "text": "要朗读的文本" }
```

返回：`audio/mpeg`（或对应类型）二进制流。Content-Disposition 为 `inline`。

需要在 设置 → TTS 配置中先填好 AppID / AccessKey / Speaker；否则返回 `412`。

### `GET /files/figures/{filename}`

用途：

- 渲染图片缩略图
- 大图预览
- AI Markdown 图像展示

通常这些 URL 不是前端手工拼出来的，而是后端在返回的 `paper.pdf_url`、`figure.image_url` 中提供。

## 当前前端 API 封装入口

统一入口在：

- [api.js](/home/xzg/project/paper_image_db/web/static/js/api.js)

核心封装有 3 个：

- `requestJSON(path, options)`
- `requestBlob(path, options)`
- `readPaperWithAIStream(data, options)`

如果后续新增接口，建议：

1. 先在 `internal/app/server.go` 注册路由
2. 在对应 `handler` 中定义请求体和响应
3. 在 `web/static/js/api.js` 增加前端封装
4. 同步更新本文档

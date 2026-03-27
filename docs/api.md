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
  - 桌面端关闭行为设置：`/api/settings/desktop-close`
  - 微信桥接设置：`/api/settings/weixin-bridge`
  - 今日推荐测试发图：`/api/settings/weixin-bridge/daily-recommendation/test`
  - Wolai 设置：`/api/settings/wolai`
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
- AI 伴读左侧文献列表
- 文献笔记列表

常用查询参数：

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| `keyword` | string | 标题、摘要、全文、DOI、笔记、标签、分组等搜索 |
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
- AI 伴读选中文献详情
- 文献笔记编辑面板

返回单篇文献详情，包含：

- 基本信息
- `doi`
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
- 导入成功后会走和本地 PDF 上传相同的入库、去重、全文保存与自动解析链路。
- 若未找到合法可下载的 Open Access PDF，返回 `NOT_FOUND`。
- 若找到了 OA 记录但实际下载失败，返回 `UNAVAILABLE`。
- 为了启用更广覆盖的 `Unpaywall` 检索，建议配置环境变量 `OA_CONTACT_EMAIL`。

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

- `scene_models` 中支持按场景绑定模型，包括 `translate_model_id`、`im_intent_model_id` 和 `tts_model_id`
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

- 单独保存模型配置、场景绑定、温度、图片数量上限和翻译规则

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
      "openai_legacy_mode": false
    }
  ],
  "scene_models": {
    "default_model_id": "default-openai",
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
  "translation": {
    "primary_language": "中文",
    "target_language": "英文"
  }
}
```

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
      "name": "严格证据模式",
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
      "name": "严格证据模式",
      "prompt": "你是一名严格的论文审稿人，优先指出证据链、方法缺口和结论边界。"
    }
  ]
}
```

说明：

- AI 伴读聊天框支持通过 `@角色名` 直接调用已保存的角色 Prompt。
- 旧的 `/api/ai/prompt-presets` 仍作为兼容别名保留，但语义已与 `role-prompts` 一致。

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
  "openai_legacy_mode": false
}
```

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
  "password": "******"
}
```

成功后：

- 后端写入会话 Cookie
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
- 当前用户需要重新登录

#### `POST /api/auth/logout`

用途：

- 登出并清理会话 Cookie

## 与 API 配套的文件访问 URL

这部分不是 `/api`，但前端会直接使用：

### `GET /files/papers/{stored_pdf_name}`

用途：

- 打开 PDF

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

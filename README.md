# CiteBox

> Collect. Cite. Create.

一个基于 Go + SQLite + 原生前端的文献数据库。核心目标只有一条主线:

1. 上传一篇 PDF。
2. 服务端先创建文献条目，再在后台调用外部 PDF 解析后端。
3. 前端轮询文献状态，等待 PDF 原文、框选结果和提取图片回填完成。
4. 在前端以“文献”为单位浏览、分组和打标签。

## 当前能力

- PDF 文献上传与入库
- 对接后端解析服务，后台异步保存提取图片和框选结果
- 文献列表、详情查看、删除
- 图片预览页，集中浏览所有提取图片
- 文献分组管理
- 文献标签管理
- 按关键词、分组、标签、解析状态筛选

## 技术栈

- 后端: Go 1.20+
- 数据库: SQLite
- 前端: HTML + CSS + JavaScript

## 目录结构

```text
citebox/
├── cmd/desktop/main.go
├── cmd/server/main.go
├── internal/
│   ├── config/
│   ├── handler/
│   ├── middleware/
│   ├── model/
│   ├── repository/
│   └── service/
├── web/
│   ├── index.html
│   ├── figures.html
│   ├── groups.html
│   ├── tags.html
│   ├── upload.html
│   └── static/
├── data/
│   ├── library.db
│   └── library/
│       ├── papers/
│       └── figures/
└── README.md
```

## 运行

```bash
go run cmd/server/main.go
```

如果你是直接从源码目录开发，推荐用：

```bash
make dev
```

它会先检查并准备 `pdf.js` 运行时资源，再启动服务，避免人工处理页缺少前端依赖。

如果你想直接以桌面客户端打开，而不是手动起服务再开浏览器：

```bash
go run ./cmd/desktop
```

默认地址:

- Web: `http://localhost:8080`
- 账号: `citebox`
- 密码: `citebox123`

桌面模式说明:

- 会在本机随机端口启动内置服务，并嵌进原生窗口，不再额外打开浏览器
- 默认数据目录会切到用户配置目录，例如 Linux 下是 `~/.config/CiteBox/`
- 如果仍想自定义路径，继续使用 `STORAGE_DIR`、`DATABASE_PATH`、`UPLOAD_DIR` 即可

## 关键环境变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `SERVER_PORT` | `8080` | 服务端口 |
| `STORAGE_DIR` | `./data/library` | 文献 PDF 与提取图片存储根目录 |
| `DATABASE_PATH` | `./data/library.db` | SQLite 数据库路径 |
| `MAX_UPLOAD_SIZE` | `262144000` | 单个 PDF 最大体积，默认 250MB |
| `PDF_EXTRACTOR_PROFILE` | `pdffigx_v1` | PDF 提取方案默认类型；可选 `pdffigx_v1` 或 `open_source_vision`（内置 LLM 坐标提取） |
| `PDF_EXTRACTOR_PDF_TEXT_SOURCE` | `extractor` | PDF 全文默认来源；`pdffigx_v1` 可选 `extractor` 或 `pdfjs`，`open_source_vision` 会固定使用 `pdfjs` |
| `PDF_EXTRACTOR_URL` | 空 | PDF 解析后端 base URL 或完整提取接口地址 |
| `PDF_EXTRACTOR_JOBS_URL` | 空 | 可选；仅在异步任务地址和 base URL 不一致时才需要单独覆盖 |
| `PDF_EXTRACTOR_TOKEN` | 空 | 解析后端 Bearer Token |
| `PDF_EXTRACTOR_FILE_FIELD` | `file` | 上传到解析后端时的文件字段名 |
| `PDF_EXTRACTOR_TIMEOUT_SECONDS` | `300` | 调用解析后端超时秒数 |
| `PDF_EXTRACTOR_POLL_INTERVAL_SECONDS` | `2` | 轮询异步任务状态的间隔秒数 |
| `ADMIN_USERNAME` | `citebox` | 登录用户名 |
| `ADMIN_PASSWORD` | `citebox123` | 初始登录密码 |
| `WEIXIN_BRIDGE_ENABLED` | `false` | 微信 IM 桥接的默认开关；若尚未在设置页保存过桥接配置，则使用该默认值 |

## 解析后端约定

推荐直接对接 `pdffigx` 的异步接口：

- 提交任务: `POST /api/v1/jobs`
- 轮询状态: `GET /api/v1/jobs/{job_id}`
- 获取结果: `GET /api/v1/jobs/{job_id}/result`

如果只配置了 `PDF_EXTRACTOR_URL`，应用会优先把它当成 `pdffigx` 的 base URL 处理：

- 同步提取自动指向 `/api/v1/extract`
- 异步任务自动指向 `/api/v1/jobs`

只有当你的部署把 jobs 接口放在别的地址时，才需要额外设置 `PDF_EXTRACTOR_JOBS_URL`。

### 配置示例

`pdffigx` 默认跑在 `http://127.0.0.1:8000` 时，推荐这样配：

```bash
export PDF_EXTRACTOR_URL=http://127.0.0.1:8000
```

这时 CiteBox 会自动解析为：

```bash
POST http://127.0.0.1:8000/api/v1/extract
POST http://127.0.0.1:8000/api/v1/jobs
```

如果你已经写了完整接口地址，也一样支持：

```bash
export PDF_EXTRACTOR_URL=http://127.0.0.1:8000/api/v1/extract
```

如果出现 `405 Method Not Allowed`，通常说明你当前运行的版本还在把请求发到错误路径，或者环境变量仍然保留着旧值。

无论同步还是异步，CiteBox 都会优先按 `pdffigx v1` 契约请求：

- `image_mode=base64`
- `include_boxes=true`
- `persist_artifacts=false`

`pdffigx_v1` 模式下，全文来源会按设置页中的“全文来源”切换：

- `extractor`：请求里会带 `include_pdf_text=true`
- `pdfjs`：请求里会带 `include_pdf_text=false`，全文改由浏览器端 `pdf.js` 提取并通过 `/api/papers/{id}/pdf-text` 保存

推荐用法：

- 标准 `pdffigx` 部署：保持“全文来源 = 解析服务返回”
- `open_source_vision`：浏览器用 `pdf.js` 渲染 PDF 页面，后端调用 CiteBox 已配置的多模态模型返回图片坐标，再通过手工提取接口自动入库
- 只走手工标注时：上传完成后也会默认使用浏览器 `pdf.js` 提取全文并保存，即便没有配置自动解析模型

当前后端期望解析服务返回 JSON，至少兼容下面这类结构:

```json
{
  "success": true,
  "message": "",
  "pdf_text": "全文文本",
  "boxes": [
    { "page": 1, "bbox": [10, 20, 100, 120] }
  ],
  "figures": [
    {
      "filename": "figure_1.png",
      "content_type": "image/png",
      "page_number": 1,
      "figure_index": 1,
      "caption": "Figure 1",
      "bbox": { "x1": 10, "y1": 20, "x2": 100, "y2": 120 },
      "data": "BASE64_IMAGE_DATA"
    }
  ]
}
```

也兼容以下别名字段:

- `text` / `full_text`
- `images` 替代 `figures`
- `page` 替代 `page_number`
- `index` 替代 `figure_index`
- `mime_type` 替代 `content_type`
- `box` 替代 `bbox`
- `base64` 替代 `data`

如果没有配置解析后端，上传页会自动切到“手工标注”；文献仍会入库，并默认尝试用浏览器 `pdf.js` 保存全文，但不会自动生成提取图片。

## 微信 IM

当前版本支持单用户微信 IM 桥接。接入方式：

1. 在设置页打开“微信 IM 桥接”。
2. 在设置页完成微信绑定。
3. 服务会在后台长轮询微信消息，并复用现有文献检索、文献问答、图片解读和笔记写入能力。

补充说明：

- `WEIXIN_BRIDGE_ENABLED` 现在只作为默认值；首次启动后如果你还没在设置页保存过桥接配置，会回退到这个环境变量
- 一旦在设置页保存过“微信 IM 桥接”开关，后续以数据库中的运行时配置为准，不必再依赖环境变量

已支持的交互：

- 微信 IM 优先响应 slash 命令；普通文字/语音会先通过 LLM 识别成最合适的 slash 操作，识别失败时才返回帮助
- `/search 自然语言`：自动理解是在找文献还是图片，拆成约 5 个关键词后搜索并汇总最可能的 1-3 条结果
- `/search-papers 自然语言`：强制只搜索文献
- `/search-figures 自然语言`：强制只搜索图片
- `/recent`：查看最近几篇文献
- `/paper 1`：选择检索结果中的文献；如果上一条刚返回候选，普通文字如“我想看看第三篇文献”也会自动路由到这里
- `/figures`：列出当前文献的图片
- `/figure 1`：选择图片检索结果或当前文献中的图片；普通文字如“我想看看第二张图”也会自动路由到这里，并在原图仍存在时回发图片预览
- 直接发送 PDF 文件：自动导入文献并切换到该文献
- `/ask 问题` 或 `/qa 问题`：对当前文献提问
- `/interpret 问题`：解读当前选中的图片
- `/note 内容`：追加文献或图片笔记
- `/status`：查看当前上下文
- `/reset`：清空当前 IM 上下文
- `/help`：查看帮助

说明：

- 微信文件消息当前只自动接收 PDF；其他文件类型会提示不支持
- 入库后会复用现有去重逻辑；重复 PDF 会直接切换到已有文献
- 图片预览回发依赖本地提取图文件仍在 `figures/` 目录中；如果文件已缺失，仍会保留图片选择状态，但不会发送预览

## 子图说明

- 子图只保存为挂在主图上的裁剪区域元数据，不会再额外生成一份独立图片文件
- 图片库、图片笔记页等顶层图片列表默认只展示主图；子图只在主图详情的子图面板里查看
- 子图预览、配色提取等能力会基于主图按需裁剪，不依赖单独落盘的子图文件
- 子图后缀只支持英文字母；手动输入大写时会自动转成小写，留空则按 `a/b/c/...` 自动补位

## API 概览

### Papers

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/papers` | 获取文献列表 |
| `POST` | `/api/papers` | 上传 PDF，立即返回已入库文献并在后台触发解析 |
| `GET` | `/api/papers/:id` | 获取文献详情 |
| `POST` | `/api/papers/:id/reextract` | 对失败文献重新提交后台解析 |
| `PUT` | `/api/papers/:id` | 更新标题、分组、标签 |
| `DELETE` | `/api/papers/:id` | 删除文献及其资源文件 |

支持查询参数:

- `keyword`
- `group_id`
- `tag_id`
- `status`
- `page`
- `page_size`

### Figures

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/figures` | 获取提取图片列表 |

支持查询参数:

- `keyword`
- `group_id`
- `tag_id`
- `page`
- `page_size`

### Groups

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/groups` | 获取分组列表 |
| `POST` | `/api/groups` | 创建分组 |
| `PUT` | `/api/groups/:id` | 更新分组 |
| `DELETE` | `/api/groups/:id` | 删除分组 |

### Tags

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/tags` | 获取标签列表 |
| `POST` | `/api/tags` | 创建标签 |
| `PUT` | `/api/tags/:id` | 更新标签 |
| `DELETE` | `/api/tags/:id` | 删除标签 |

## 数据模型

SQLite 中现在主要使用 5 张表:

- `papers`: 文献主表，保存标题、PDF 文件名、PDF 原文、框选结果、解析状态、解析任务 ID、所属分组
- `paper_figures`: 每篇文献提取出的图片
- `groups`: 文献分组
- `tags`: 标签定义
- `paper_tags`: 文献和标签的关联表

## 关于命名

**CiteBox** = **cite** (引用/文献) + **box** (盒子)

寓意：一个专门存放、整理学术引用和文献的容器。简单、可靠，像 Dropbox 一样把重要的文献安全收纳。

## 验证

当前代码已通过:

```bash
gofmt -w cmd/server/main.go internal/config/config.go internal/model/*.go internal/repository/*.go internal/service/*.go internal/handler/*.go
go test ./...
```

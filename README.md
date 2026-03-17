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
- 账号: `wanglab`
- 密码: `wanglab789`

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
| `PDF_EXTRACTOR_URL` | 空 | PDF 解析后端 base URL 或完整提取接口地址 |
| `PDF_EXTRACTOR_JOBS_URL` | 空 | 可选；仅在异步任务地址和 base URL 不一致时才需要单独覆盖 |
| `PDF_EXTRACTOR_TOKEN` | 空 | 解析后端 Bearer Token |
| `PDF_EXTRACTOR_FILE_FIELD` | `file` | 上传到解析后端时的文件字段名 |
| `PDF_EXTRACTOR_TIMEOUT_SECONDS` | `300` | 调用解析后端超时秒数 |
| `PDF_EXTRACTOR_POLL_INTERVAL_SECONDS` | `2` | 轮询异步任务状态的间隔秒数 |
| `ADMIN_USERNAME` | `wanglab` | 登录用户名 |
| `ADMIN_PASSWORD` | `wanglab789` | 初始登录密码 |

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
- `include_pdf_text=true`
- `include_boxes=true`
- `persist_artifacts=false`

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

如果没有配置解析后端，文献仍会入库，但状态会快速转为 `failed`，不会有提取图片。

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

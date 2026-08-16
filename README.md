[简体中文](./README.md) | [English](./README.en.md)

# CiteBox

> Collect. Cite. Create.

CiteBox 是一个面向论文整理与图片阅读的本地优先工作台，基于 Go、SQLite 和原生 HTML/CSS/JavaScript 构建，同时提供 Web 服务端和桌面客户端两种运行方式。

它的核心目标不是“做一个通用文献管理器”，而是围绕 PDF、正文、图片、笔记和 AI 阅读建立一条紧凑工作流：

1. 导入论文 PDF，或通过 DOI 导入可获取的 Open Access PDF。
2. 自动或手工提取正文与图片。
3. 围绕文献、图片、标签、分组和笔记持续整理。
4. 在 AI 助手页继续问答、翻译、总结和沉淀结果。

## 当前能力

- 文献导入：支持本地 PDF 上传、基于 DOI 导入 Open Access PDF，以及从本机 Zotero Local API 按 collection 导入。
- 多种提取模式：支持外部自动提取、内置多模态坐标识别和纯手工标注三种流程。
- 文献工作台：文献库、图片库、分组、标签、笔记、配色和手工补图页面均已内置。
- AI 阅读：支持围绕正文和图片做问答、图片解读、Tag 建议、流式输出和导出。
- 集成能力：支持 Zotero Local API 连接器、Notion MCP、Notion 原图笔记导出、Wolai 笔记写回、微信 IM 桥接、TTS 配置和版本检查。
- 双端运行：既可作为本地 Web 服务运行，也可作为嵌入式桌面应用运行。

### 调研 / Research panel

`/research` 提供基于 Semantic Scholar Graph API 的单跳文献拓展能力：搜索关键词、查看 references / citations / recommendations、把感兴趣的论文加入"调研篮"，再一键导入文献库。

Semantic Scholar API key 可在 设置 → 外部 API 里填写，或通过 `S2_API_KEY` 环境变量提供。未填写时使用匿名速率（约 1 req/s）。

## 技术栈

- Go 1.21+
- SQLite
- 原生 HTML / CSS / JavaScript
- `webview_go` 桌面壳

## 快速开始

### Web 模式

```bash
make run
```

默认地址：

- `http://localhost:8080`
- 默认账号：`citebox`
- 默认密码：`citebox123`

如果只想在本地快速调试并关闭登录鉴权：

```bash
make dev
```

### 桌面模式

```bash
make run-desktop
```

桌面模式会在本机随机端口启动内置服务，并通过原生窗口加载界面；默认数据目录会切到系统用户配置目录，例如：

- Linux：`~/.config/CiteBox/`
- macOS：`~/Library/Application Support/CiteBox/`
- Windows：`%AppData%/CiteBox/`

### 常用命令

```bash
make build
make build-desktop
make test
make prepare-web-assets
```

## 配置说明

大部分功能都可以在应用内的“设置”页完成配置；如果需要通过环境变量启动，最常用的是这些：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `SERVER_PORT` | `8080` | Web 服务端口 |
| `DATABASE_PATH` | `./data/library.db` | SQLite 数据库路径 |
| `STORAGE_DIR` | `./data/library` | PDF 与提取图片的存储目录 |
| `ADMIN_USERNAME` | `citebox` | 登录用户名 |
| `ADMIN_PASSWORD` | `citebox123` | 初始登录密码 |
| `DISABLE_AUTH` | `false` | 设为真后关闭登录鉴权，适合本地开发 |
| `OA_CONTACT_EMAIL` | 空 | DOI 导入时用于增强 Open Access 检索覆盖 |
| `PDF_EXTRACTOR_PROFILE` | 自动提取 | 提取模式，可在设置页切换自动提取、内置多模态识别或手工模式 |
| `PDF_EXTRACTOR_URL` | 空 | 外部提取服务地址 |
| `PDF_EXTRACTOR_TOKEN` | 空 | 外部提取服务的 Bearer Token |
| `CODEX_BIN` | 自动发现 | Codex CLI 可执行文件路径；桌面端无法从常见安装目录发现 Codex 时使用 |
| `WEIXIN_BRIDGE_ENABLED` | `false` | 微信 IM 桥接默认开关，首次保存设置后以数据库配置为准 |

### 提取模式

- 外部自动提取：对接外部提取服务，适合标准自动提图流程。
- 内置多模态识别：使用已配置的多模态模型识别图片坐标，正文优先由 `pdf.js` 提取。
- 手工模式：不自动提图，但仍支持保存 PDF 全文，之后可进入手工标注页补录图片。

### Notion 集成

CiteBox 的 Notion 配置包含两个彼此独立的入口：

- **Notion MCP**：通过 OAuth 连接 `https://mcp.notion.com/mcp`，供 AI 助手中的 `@Notion` 调用 Notion 工具。
- **Notion API**：使用个人访问令牌，通过 Notion File Upload API 把图片笔记中的原始图片、说明和笔记写入 Notion。同一篇文献的图片笔记会归档到同一个页面。

个人访问令牌的简化配置步骤：

1. 打开 [Notion Developer Portal 的“个人访问令牌”](https://app.notion.com/developers/tokens)。不要进入“连接 → 新连接”；本地 CiteBox 使用的是你本人的 Notion 权限，不需要创建独立机器人连接。
2. 点击“新建令牌 / New token”，名称可填写 `CiteBox-Notion-File-Upload`，选择需要使用的工作区，勾选 **Notion API**；不需要 Workers。有效期可选 7、30、90、180 天或 1 年，未选择时默认为 1 年。
3. 点击“创建令牌 / Create token”后立即复制 `ntn_...` 令牌。完整值只显示一次；请勿提交到 Git 或发送给他人。然后在 CiteBox 的“设置 → Notion → Notion API”中测试并保存。

MCP OAuth 凭据和 API 令牌均只保存在本机；设置接口不会回显已保存的 API 令牌。

## 文档

- [前后端 API 说明](./docs/api.md)
- [数据库说明](./docs/database.md)
- [macOS 开发说明](./docs/macos-development.md)

README 只保留高层概览。接口、表结构和迁移细节请以 `docs/` 下文档以及当前代码实现为准。

## 项目结构

```text
.
├── cmd/
│   ├── server/
│   └── desktop/
├── internal/
│   ├── app/
│   ├── config/
│   ├── handler/
│   ├── repository/
│   ├── service/
│   └── ...
├── web/
│   ├── *.html
│   └── static/
├── docs/
├── scripts/
└── Makefile
```

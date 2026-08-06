# CiteBox Connector 浏览器扩展

Zotero Connector 式的一键入库插件：在论文页面（出版商站点、arXiv、doi.org 直链、PDF 页面等）点击扩展图标，即可将文献保存到本地运行的 CiteBox 文库。

## 功能简介

- 自动从页面提取 DOI、标题、PDF 直链（`citation_doi` / `citation_title` / `citation_pdf_url` 等标准元数据，含 arXiv 页面识别与正文 DOI 兜底扫描）
- 一键入库：优先按 DOI 让服务器解析下载（走开放获取渠道），失败时自动降级为浏览器直接抓取页面 PDF 并上传（携带浏览器 Cookie，可获取机构订阅的付费墙 PDF）
- 重复检测：库中已存在时提示并给出查看链接
- 中英双语界面（跟随浏览器语言）

## 安装方法

扩展已随 CiteBox 发布包附带，无需单独下载：

- **Windows / Linux 包**：扩展位于解压后目录的 `extension/` 文件夹
- **macOS DMG**：扩展位于 DMG 根目录的 `extension/` 文件夹
- **源码仓库**：扩展就在仓库根目录的 `extension/` 目录

安装步骤：

1. 打开 Chrome / Edge，访问 `chrome://extensions`
2. 打开右上角「开发者模式」
3. 点击「加载已解压的扩展程序」，选择上述 `extension/` 目录

## 配置方法

1. 在 CiteBox Web 界面的设置页生成「集成令牌」（形如 `cbx_...`）
2. 右键扩展图标 →「选项」（或点击弹窗中的「打开设置」）
3. 填入服务器地址（默认 `http://localhost:8080`）与集成令牌，点击「保存」
4. 点击「测试连接」确认配置可用：显示「连接成功」即配置正确；401 表示令牌无效；无法连接时请确认 CiteBox 正在运行

## 使用说明

1. 打开任意论文页面（如出版商文章页、arXiv 摘要页）
2. 点击工具栏中的 CiteBox Connector 图标
3. 弹窗会显示检测到的标题与 DOI，点击「保存到 CiteBox」
4. 入库成功后点击「在 CiteBox 中打开」即可跳转到 Web 端查看

## 入库策略

扩展按以下顺序尝试入库：

1. **按 DOI 入库**：检测到 DOI 时，调用 `POST /api/papers/import-by-doi`，由 CiteBox 服务器解析元数据并下载开放获取（OA）PDF。这是首选路径，元数据最完整。
2. **PDF 抓取上传（自动降级）**：未检测到 DOI，或策略 1 因「找不到 OA PDF」等业务原因失败（401/409 除外）时，由扩展的 background 直接 `fetch` 页面的 `citation_pdf_url`（或本身就是 PDF 的当前页面 URL），校验响应为 PDF 后以 multipart 形式上传到 `POST /api/papers`。由于请求携带浏览器 Cookie，机构订阅的付费墙 PDF 也能获取。

所有发往 CiteBox 服务器的请求都携带 `Authorization: Bearer <令牌>` 头，且统一由 background service worker 代发，不依赖页面 CORS。

## 常见问题

**提示「未认证（HTTP 401）」**
令牌缺失或已失效。请在 CiteBox 设置页重新生成集成令牌，并到扩展选项页更新后重试。

**提示「未找到可直接下载的 PDF」**
页面既没有可用 DOI，也没有可直接下载的 PDF（例如纯 HTML 摘要页且无 `citation_pdf_url` 元数据）。可以尝试打开出版商的 PDF 页面后再点击保存。

**提示「无法读取此页面」**
当前是 `chrome://`、浏览器商店等受限页面，或页面在扩展安装/更新前就已打开（刷新页面即可）。扩展无法在这类页面注入脚本。

**入库结果显示「库中已存在」**
该 DOI 或相同内容的 PDF 已在库中，点击「在 CiteBox 中打开」可直接查看已有条目。

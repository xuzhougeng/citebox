# 桌面端上传提示「PDF 内容为空」或「上传请求未收到文件内容」

适用：v0.38.0 起的桌面端（Wails v3 外壳），Windows 上最明显（issue #73）。

## 现象

在桌面端「上传 PDF」、设置页「导入数据库」或 Zotero 导入向导里补 PDF 时，任何文件都报「PDF 内容为空」。用浏览器打开同一实例的本地地址上传同一个文件则正常。

## 原因

桌面端窗口不走 `127.0.0.1` 监听端口，请求由 webview 直接交给进程内的 Go 处理器（Windows 上是 WebView2 的 `WebResourceRequested`，macOS / Linux 是自定义 URL scheme）。Windows 上这条路径转交请求体时会丢掉 `FormData` 里的文件内容：multipart 信封完整，但文件部件到达服务端时是 0 字节。普通 JSON / 文本请求体不受影响。macOS 的自定义 scheme 也有同类报告（wailsapp/wails#6068），因此修复对所有桌面平台生效。

Wails v3.0.0 在 Windows 上还会把没有 `Content-Length` 的请求体长度记为 0（wailsapp/wails#6316）。

## 修复

- 桌面注入脚本（`internal/desktopbridge/upload_body.js`）包装了 `window.fetch`：请求体是含 `File` / `Blob` 的 `FormData` 时，先在页面内读出文件内容，编码成一段内存中的 multipart 字节再发送。所有界面内上传都走 `fetch`，因此一并覆盖。
- 桌面外壳对没有声明长度的请求体把 `ContentLength` 设为 `-1`（未知），与 `net/http` 服务端一致。
- 服务端收到 0 字节的文件部件时返回「上传请求未收到文件内容（「文件名」收到 0 字节）」，不再误报为 PDF 本身的问题。

## 临时规避（旧版本）

升级到包含上述修复的版本即可。暂时无法升级时，可以用浏览器访问桌面端的本地监听端口上传，绕过 webview：

1. 桌面端每次启动都会在 `127.0.0.1` 上随机选一个端口，界面里不显示。Windows 上可在 PowerShell 中查询：

   ```powershell
   Get-NetTCPConnection -State Listen -OwningProcess (Get-Process citebox-desktop).Id | Where-Object LocalAddress -eq '127.0.0.1'
   ```

   结果中可能还有 MCP 集成端口（设置页「集成」里显示的那个），排除它即可。
2. 用浏览器打开 `http://127.0.0.1:<port>`，用账号 `citebox`（`ADMIN_USERNAME`）和当前密码登录（未在设置页改过时为 `ADMIN_PASSWORD`，默认 `citebox123`）。
3. 在浏览器里上传；文献进入同一个库，桌面端刷新后即可看到。

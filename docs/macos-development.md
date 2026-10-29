# macOS 开发注意事项

## 背景

CiteBox 桌面版基于 Wails v3：应用 handler 在进程内通过 Wails 的自定义协议交给原生 WebView（macOS 上是 WKWebView）。这套模式在 Windows 上通常比较接近浏览器心智，但在 macOS 上会出现更多平台差异：链接跳转、`window.open`、资源查看、下载、编辑菜单、右键菜单、快捷键和窗口焦点都可能和预期不同。

这份文档用于沉淀已经踩过的坑，避免后续功能继续重复回归。

## 已确认的差异与推荐做法

### 1. 站内跳转不能照搬 Windows 的 `target="_blank"` / `window.open`

现象：

- 站内 PDF、图片、viewer 页面在 macOS 桌面版里不能简单依赖 `target="_blank"`。
- 同源页面如果直接 `window.open()`，行为和 Windows 不一致，容易跳出应用内导航预期。

当前做法：

- [internal/desktopbridge](../internal/desktopbridge/bridge.go) 的桥接脚本由 `desktopbridge.InjectScript` 注入到每个 HTML 页面的 `<head>` 开头。
- 同源站内路由统一改为应用内原地跳转。
- 外部链接统一走 `citeboxDesktopOpenExternal`，由 Wails 的 `Browser.OpenURL` 打开系统浏览器。

开发约束：

- 不要给站内 PDF、图片、`/viewer` 之类的链接再加 `target="_blank"`。
- 不要直接用 `window.open()` 打开站内资源；优先走 `Utils.openResourceViewer()` 或普通站内 href。
- 如果确实需要打开系统外链，必须明确区分“站内”和“站外”。

### 2. 图片/PDF 查看器必须显式维护返回路径和弹层恢复状态

现象：

- 从文献弹层、图片弹层、笔记弹层进入资源查看器后，macOS 下直接依赖浏览器历史经常回不到正确位置。
- 打开原图或 PDF 后，如果不显式保存返回上下文，很容易丢失当前弹层状态。

当前做法：

- 统一通过 `Utils.resourceViewerURL()` / `Utils.openResourceViewer()` 构造查看器入口。
- 通过 `Utils.buildResourceViewerBackURL()`、`captureModalRestoreState()`、`restoreModalState()` 保存并恢复弹层上下文。
- `viewer.js` 会根据 `back` 参数和 `document.referrer` 决定是 `history.back()` 还是显式跳回。

开发约束：

- 从弹层进入 viewer 时，不要手写 `/viewer?...` 字符串，直接复用 `Utils` 的 helper。
- 新增“打开原图 / 打开 PDF / 查看资源”入口时，必须验证返回后是否能恢复到原来的文献、图片或笔记弹层。
- 图片查看问题通常不是单点 bug，而是“打开方式 + 返回状态 + 焦点恢复”三件事一起决定的。

### 3. 桌面端下载不能只依赖 `<a download>`

现象：

- WebView 环境下，浏览器式下载行为并不稳定。
- 在 macOS 桌面版里，直接点击 `<a download>` 不一定能得到符合预期的保存体验。

当前做法：

- 前端统一走 `Utils.saveBlobDownload()`。
- 如果桌面端注入了 `citeboxDesktopSaveFile`，则转成 base64 后走 Wails 的原生保存对话框。
- 浏览器环境才退回到 `a[download]`。

开发约束：

- 新增导出、下载图片、下载 Markdown、导出数据库等功能时，不要直接手写 `a[download]`。
- 优先产出 `Blob`，然后走 `Utils.saveBlobDownload()`。

### 4. 文本编辑、右键菜单和快捷键要优先保护原生行为

现象：

- 自定义右键菜单很容易把 `textarea`、`input`、`contenteditable` 的原生菜单覆盖掉。
- macOS 下如果快捷键没有被正确路由，`Cmd/Ctrl + A/C/X/V`、方向键等操作会触发系统提示音。
- 资源查看器或图片弹层的左右方向键如果没有 `preventDefault()`，在边界状态或焦点落在按钮上时也会出现提示音。

当前做法：

- 划词翻译菜单默认避开可编辑元素和 `[data-native-context-menu="true"]`。
- `cmd/desktop/menu.go` 自建应用菜单，其中 `编辑` 菜单的撤销/重做/剪切/拷贝/粘贴/全选使用 Wails 的角色项（macOS 上对应 `undo:`/`copy:` 等原生 selector），文本框里的系统快捷键因此可用。
- 当前桌面桥接中已经增加文本控件的剪贴板读写兜底，用来处理 `Cmd/Ctrl + A/C/X/V`。
- 图片弹层和图片笔记弹层里的左右切换已显式 `preventDefault()` / `stopPropagation()`。

开发约束：

- 任何全局 `keydown` 逻辑都必须先判断是否在可编辑元素上。
- 自定义右键菜单要给编辑区域留逃生口，必要时使用 `[data-native-context-menu="true"]`。
- 用方向键控制图片、文档或列表导航时，要明确处理默认行为，避免把按键继续传给 macOS。

### 5. macOS 桌面壳需要额外的原生集成

现象：

- WebView 本身不负责应用菜单、窗口焦点、系统打开外链、保存对话框、应用图标等完整桌面能力。
- Wails 的自定义协议在 macOS 上有两处和浏览器不同：WKWebView 不跟随 handler 返回的 3xx 重定向（会直接显示响应体），也不保存响应里的 `Set-Cookie`。

当前做法：

- `cmd/desktop` 用 Wails 的托盘、窗口、对话框、剪贴板和单实例 API 提供这些能力；`DesktopService` 的方法通过 `window.citeboxDesktop*` 暴露给页面。
- `desktopbridge.InjectScript` 把页面导航的 3xx 改写成客户端跳转。
- `desktopbridge.CookieStore` 在 Go 侧代管 cookie；带过期时间的 cookie 写入数据库目录下的 `desktop-cookies.json`（权限 0600）。桌面 WebView 本身不再依赖登录 cookie，见下文“桌面端免登录”。
- `desktopicon` 在 Windows 上显式设置窗口图标。

开发约束：

- 只要是“浏览器原生能做，但桌面 WebView 不稳定”的能力，优先考虑补一个原生 bridge，而不是继续堆前端绕路逻辑。
- 新增桌面能力时，优先在 `cmd/desktop` 的 `DesktopService` 里用 Wails API 实现，再在 shim 里映射成 `window.citeboxDesktop*`，不要把平台分支散落到页面脚本里。
- 前端不要依赖 `document.cookie`；桌面端的 cookie 只存在于 Go 侧。

### 6. “最小化到托盘”不是普通最小化，根因在 macOS 窗口状态机

> 以下记录的是旧 cgo/webview_go 实现踩过的坑。现在托盘、隐藏、Dock reopen 都由 Wails 处理：隐藏时显示托盘图标，恢复时隐藏它（`SystemTray.Show/Hide`）。迁移时已用 `scripts/macos-desktop-ui-smoke.zsh smoke`、连续多次“隐藏到托盘 → 点托盘恢复”（隐藏前文本框保持焦点）以及托盘菜单退出验证过不再崩溃；改动这条链路时仍需重跑这些检查。

现象：

- 用户点击关闭按钮，选择“最小化到托盘”后，窗口会消失，但 Dock 图标点不开，或者恢复/退出时直接崩溃。
- 这个问题表面看像 web 页面没有恢复成功，实际根因不在页面层，而在 macOS 原生窗口和应用状态机。

错误原因：

- `orderOut:` 只是把窗口隐藏，不会自动把 Dock 左键连接到“恢复主窗口”这条路径。如果只代理 `NSWindowDelegate`，没有补 `NSApplicationDelegate applicationShouldHandleReopen:hasVisibleWindows:`，Dock 点击不会回到已有的 `openWindow` / 激活窗口逻辑。
- “托盘隐藏”和 `miniaturize:` 不是一回事。`miniaturize:` 的语义是最小化到 Dock，不等于隐藏到托盘；如果为了让 Dock 能点开而改成 `miniaturize:`，只是把语义改错了。
- 隐藏前如果不结束编辑态、不清理 first responder，WebKit/WebView 仍可能保留输入法和焦点链路，窗口被 `orderOut:` 后容易在后续焦点切换中崩溃。
- 恢复窗口或退出应用时，手动移除 `NSStatusItem` 是高风险操作。我们这次实测里，恢复路径和退出路径里的崩溃点都落在状态栏图标销毁附近；更稳的做法是恢复时只负责把窗口拉回前台，退出时直接交给 `NSApplication terminate:`，让系统在进程结束时清理状态栏图标。
- 之前为了找“可聚焦控件”，手动递归视图树并强行设置 first responder，也会放大 WebKit 焦点状态不一致的问题。恢复窗口时优先使用标准的 `activateIgnoringOtherApps:` + `makeKeyAndOrderFront:`。

当前做法：

- 托盘隐藏继续使用 `orderOut:`，保留“隐藏窗口但进程仍存活”的语义。
- 在 app delegate 上补 `applicationShouldHandleReopen:hasVisibleWindows:`，让 Dock 左键也走主窗口恢复逻辑。
- 在隐藏前调用 `endEditingFor:` 和 `makeFirstResponder:nil`，先收尾原生编辑态。
- 恢复窗口时只做窗口激活与前置，不在这条路径里手动移除状态栏图标。
- 退出应用时直接调用 `terminate:`，不额外手动拆状态栏项。

开发约束：

- 不要把“点 Dock 能恢复”误解成“应该改成 `miniaturize:`”。托盘隐藏优先保持 `orderOut:` 语义，再单独补 Dock reopen。
- 不要在 `windowShouldClose:` 里混用 `orderOut:`, `close`, `performClose`, `Destroy`, `terminate`。这几条路径分别对应隐藏、关闭窗口、销毁 WebView、结束进程，混在一起很容易造成双重关闭或悬空句柄。
- 如果需要接管 `window.delegate` 或 `app.delegate`，必须保留原 delegate 的 selector 转发，避免把 webview 原生生命周期处理吃掉。
- 如果恢复窗口后还想清掉状态栏图标，必须先确认不会触发原生崩溃；在没有充分验证前，宁可保留状态栏图标，也不要在恢复链路上主动销毁它。

### 7. 桌面端免登录：只信任进程内 handler

现象：

- 桌面版曾要求对自己进程内的服务输入用户名密码，登录页还显示默认账号密码。

当前做法：

- `app.Server.DesktopHandler()` 用 `middleware.TrustDesktopRequests` 包一层，给请求的 context 打上“来自桌面 WebView”的标记，`AuthMiddleware` 见到标记即视为已登录。`cmd/desktop` 把它交给 Wails 的 asset handler（自定义 URL scheme，进程内调用，不经过 socket）。
- context 标记无法由客户端伪造（不是 header、cookie 或参数），只有直接调用这个 handler 的代码能拿到。
- 同时开着的 loopback 监听（`127.0.0.1:随机端口`，供 MCP / 集成 / 浏览器插件使用）仍用 `Server.Serve` 里的普通 handler，照旧要求会话 cookie 或 Bearer 集成令牌。
- `cmd/desktop/server_tag_guard.go` 让带 Wails `server` 或 `mcp` build tag 的构建直接编译失败：前者会把 asset handler 变成网络 HTTP 服务，后者会开一个能在窗口里执行脚本的网络端点。
- 桥接脚本在桌面端隐藏设置页里“记住登录状态”和“登出”两项（WebView 始终视为已登录，它们没有意义）；“修改密码”保留，因为它仍保护 loopback 端口和 Web 版。

开发约束：

- 不要把 `DesktopHandler()` 挂到任何 listener 上，也不要把“来自 127.0.0.1”当成免登录依据——本机任何进程都能连 loopback。
- 新增桌面专用接口时同样走 `DesktopService`，不要为桌面端单独放开鉴权路径。

### 8. 原生菜单、快捷键与标题栏

菜单（`cmd/desktop/menu.go`，标签随界面语言在中文 / 英文之间切换）：

| 菜单 | 项目 | 快捷键 |
| --- | --- | --- |
| CiteBox | 关于 CiteBox / 设置…（`/settings`）/ 服务 / 隐藏 / 隐藏其他 / 全部显示 / 退出 | 设置 `Cmd+,`，隐藏 `Cmd+H`，退出 `Cmd+Q` |
| 文件 | 上传 PDF…（`/upload`）/ 关闭窗口 | `Cmd+O` / `Cmd+W` |
| 编辑 | 撤销 / 重做 / 剪切 / 拷贝 / 粘贴 / 粘贴并匹配样式 / 删除 / 全选 | 系统默认（`Cmd+Z`、`Shift+Cmd+Z`、`Cmd+X/C/V`、`Cmd+A`） |
| 显示 | 后退 / 前进 / 重新载入 / 实际大小 / 放大 / 缩小 / 切换全屏幕 /（开发构建）开发者工具 | `Cmd+[` / `Cmd+]` / `Cmd+R` / `Cmd+0` / `Cmd++` / `Cmd+-` / `Ctrl+Cmd+F` / `Option+Cmd+I` |
| 前往 | 总览 `/`、文献库 `/library`、图片库 `/figures`、笔记 `/notes`、AI 助手 `/ai`、文献搜索 `/research` | `Cmd+1` … `Cmd+6` |
| 窗口 | 最小化 / 缩放 / 前置全部窗口（Window 角色，系统会列出窗口） | `Cmd+M` |
| 帮助 | 使用手册（`/manual`）/ 功能说明（`/guide`）/ 反馈问题（系统浏览器打开 GitHub Issues） | |

- 菜单跳转通过 `window.ExecJS("window.location.assign(...)")` / `history.back()` 完成，执行前会把窗口从托盘恢复。
- “关闭窗口”用 `CloseWindow` 角色（`performClose:`），和红色关闭按钮一样走关闭确认弹窗；“退出”是普通菜单项，直接调用 `shell.quit()`，和托盘菜单的退出一致。
- “开发者工具”只在开发构建（`buildinfo.Version` 为 `dev`，如 `make run-desktop`）或设置了环境变量 `CITEBOX_DESKTOP_DEVTOOLS=1` 时出现。
- 菜单语言：启动时取服务端外观设置里的 `language`，没有则用系统首选语言；之后桥接脚本监听 `<html lang>` 的变化（i18n.js 每次加载页面都会设置），通过 `DesktopService.SyncAppearance` 通知 Go 侧，语言变化时重建应用菜单和托盘菜单。

标题栏：

- 主窗口使用 `MacTitleBarHiddenInset`：没有单独的标题栏条，网页顶栏直接位于红绿灯下方。
- shim 在 macOS 上设置 `window.__CITEBOX_DESKTOP_INSET_TITLEBAR__`，桥接脚本据此给 `<html>` 加 `desktop-mac` class 并注入样式：`.navbar`、`.viewer-toolbar`、`.overview-topbar` 左侧留出红绿灯空间，并设置 `--wails-draggable: drag` 作为拖动区域（双击可缩放窗口）；其中的链接、按钮、输入框等交互元素设为 `--wails-draggable: no-drag`，保证点击正常。页面代码不需要知道这些。
- 新增顶栏样式的页面时，如果顶栏不是这三个 class 之一，需要把它加到 `internal/desktopbridge/bridge.go` 的 `headers` 列表，否则内容会被红绿灯遮住且窗口无法拖动。
- 窗口外观跟随应用主题：暗黑主题用 `NSAppearanceNameDarkAqua`，暖色 / 明亮主题用 `NSAppearanceNameAqua`；窗口背景色与各主题的 `--bg` 一致，避免加载时闪白。启动时按服务端保存的主题设置，之后由桥接脚本监听 `<html data-theme>` 变化实时同步（Wails 只在创建窗口时应用 `Mac.Appearance`，运行时通过 `cmd/desktop/native_darwin.go` 调用 `NSWindow setAppearance:`）。

## 开发时的建议检查清单

- 在 macOS 桌面版里手动点一次站内 PDF、原图、viewer 跳转，确认没有跳出应用或丢失返回状态。
- 从文献弹层、图片弹层、笔记弹层进入资源查看器，再返回，确认状态能恢复。
- 测试导出、下载图片、下载 Markdown、数据库导出，确认走的是原生保存面板。
- 在 `textarea`、`input`、PDF 文本编辑器里测试右键、复制、粘贴、全选、方向键，确认没有系统提示音。
- 如果新增了全局键盘事件，分别验证浏览器版和 macOS 桌面版。
- 如果新增了“打开外链”能力，确认站内仍在应用内打开，站外走系统浏览器。
- 如果修改了关闭行为、托盘隐藏、Dock 恢复或状态栏菜单，至少验证一次“关闭到托盘 -> Dock 恢复 -> 再次隐藏/恢复 -> 直接退出”。
- 如果修改了菜单或顶栏：确认顶栏链接可点、空白处可拖动窗口、`Cmd+1…6` / `Cmd+,` / `Cmd+[` 能跳转，切换界面语言和暗黑主题后菜单语言和窗口外观随之变化。

## 相关历史提交

- `d268240` `Add macOS desktop runtime hooks`
- `8153606` `Fix desktop download flows in web UI`
- `aa3e4e9` `Keep resource viewer navigation in-app`
- `fc9bedd` `Fix resource viewer restore navigation`
- `880f9d2` `Add desktop text selection translation`
- `ec6c220` `Fix desktop PDF text context menu handling`
- `8e5a3a3` `Add image viewer zoom, tag presets, and markdown full-text preview`
- `9d22087` `Add desktop icons and hide Windows console`

后续如果再遇到新的 macOS 桌面差异，优先把“现象、原因、解决方式、约束”补到这份文档，而不是只留在提交信息里。

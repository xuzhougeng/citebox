# macOS 开发注意事项

## 背景

CiteBox 桌面版的运行方式是“本地 HTTP 服务 + 原生 WebView”。这套模式在 Windows 上通常比较接近浏览器心智，但在 macOS 上会出现更多平台差异：链接跳转、`window.open`、资源查看、下载、编辑菜单、右键菜单、快捷键和窗口焦点都可能和预期不同。

这份文档用于沉淀已经踩过的坑，避免后续功能继续重复回归。

## 已确认的差异与推荐做法

### 1. 站内跳转不能照搬 Windows 的 `target="_blank"` / `window.open`

现象：

- 站内 PDF、图片、viewer 页面在 macOS 桌面版里不能简单依赖 `target="_blank"`。
- 同源页面如果直接 `window.open()`，行为和 Windows 不一致，容易跳出应用内导航预期。

当前做法：

- 在 [internal/desktopruntime/configure_darwin.go](../internal/desktopruntime/configure_darwin.go) 注入 `desktopBridgeScript`。
- 同源站内路由统一改为应用内原地跳转。
- 外部链接统一走 `citeboxDesktopOpenExternal`，由 `NSWorkspace` 打开系统浏览器。

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
- 如果桌面端注入了 `citeboxDesktopSaveFile`，则转成 base64 后走原生保存面板 `NSSavePanel`。
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
- macOS runtime 会安装原生 `Edit` 菜单，并在启动时处理窗口焦点。
- 当前桌面桥接中已经增加文本控件的剪贴板读写兜底，用来处理 `Cmd/Ctrl + A/C/X/V`。
- 图片弹层和图片笔记弹层里的左右切换已显式 `preventDefault()` / `stopPropagation()`。

开发约束：

- 任何全局 `keydown` 逻辑都必须先判断是否在可编辑元素上。
- 自定义右键菜单要给编辑区域留逃生口，必要时使用 `[data-native-context-menu="true"]`。
- 用方向键控制图片、文档或列表导航时，要明确处理默认行为，避免把按键继续传给 macOS。

### 5. macOS 桌面壳需要额外的原生集成

现象：

- WebView 本身不负责应用菜单、窗口焦点、系统打开外链、保存对话框、应用图标等完整桌面能力。

当前做法：

- `desktopruntime.Configure()` 负责安装 App 菜单、Edit 菜单、窗口激活、外链打开、文件保存等桥接。
- `desktopicon` 负责在 macOS 上设置应用图标。

开发约束：

- 只要是“浏览器原生能做，但桌面 WebView 不稳定”的能力，优先考虑补一个原生 bridge，而不是继续堆前端绕路逻辑。
- 新增桌面能力时，优先放在 `internal/desktopruntime` 或 `internal/desktopicon` 这类平台桥接层，不要把平台分支散落到页面脚本里。

### 6. “最小化到托盘”不是普通最小化，根因在 macOS 窗口状态机

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

## 开发时的建议检查清单

- 在 macOS 桌面版里手动点一次站内 PDF、原图、viewer 跳转，确认没有跳出应用或丢失返回状态。
- 从文献弹层、图片弹层、笔记弹层进入资源查看器，再返回，确认状态能恢复。
- 测试导出、下载图片、下载 Markdown、数据库导出，确认走的是原生保存面板。
- 在 `textarea`、`input`、PDF 文本编辑器里测试右键、复制、粘贴、全选、方向键，确认没有系统提示音。
- 如果新增了全局键盘事件，分别验证浏览器版和 macOS 桌面版。
- 如果新增了“打开外链”能力，确认站内仍在应用内打开，站外走系统浏览器。
- 如果修改了关闭行为、托盘隐藏、Dock 恢复或状态栏菜单，至少验证一次“关闭到托盘 -> Dock 恢复 -> 再次隐藏/恢复 -> 直接退出”。

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

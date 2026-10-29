package main

import (
	"encoding/json"
	"os"
	"runtime"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/xuzhougeng/citebox/internal/buildinfo"
)

const (
	menuLangZh = "zh-CN"
	menuLangEn = "en"

	projectIssuesURL = "https://github.com/" + buildinfo.ReleaseRepository + "/issues"
)

// menuStrings holds the native menu and tray labels. The web UI's locale
// files only reach the webview, so the shell keeps its own small table for
// the two supported UI languages.
type menuStrings struct {
	About, Settings, Services, Hide, HideOthers, ShowAll, Quit string

	File, UploadPDF, CloseWindow string

	Edit, Undo, Redo, Cut, Copy, Paste, PasteAndMatchStyle, Delete, SelectAll string

	View, Back, Forward, Reload, ActualSize, ZoomIn, ZoomOut, FullScreen, DevTools string

	Go, Overview, Library, Figures, Notes, AI, Research string

	Window, Minimize, Zoom, BringAllToFront string

	Help, Manual, Guide, ReportIssue string

	TrayOpen, TrayQuit string
}

var menuStringTable = map[string]menuStrings{
	menuLangEn: {
		About:      "About " + desktopAppName,
		Settings:   "Settings…",
		Services:   "Services",
		Hide:       "Hide " + desktopAppName,
		HideOthers: "Hide Others",
		ShowAll:    "Show All",
		Quit:       "Quit " + desktopAppName,

		File:        "File",
		UploadPDF:   "Upload PDF…",
		CloseWindow: "Close Window",

		Edit:               "Edit",
		Undo:               "Undo",
		Redo:               "Redo",
		Cut:                "Cut",
		Copy:               "Copy",
		Paste:              "Paste",
		PasteAndMatchStyle: "Paste and Match Style",
		Delete:             "Delete",
		SelectAll:          "Select All",

		View:       "View",
		Back:       "Back",
		Forward:    "Forward",
		Reload:     "Reload",
		ActualSize: "Actual Size",
		ZoomIn:     "Zoom In",
		ZoomOut:    "Zoom Out",
		FullScreen: "Toggle Full Screen",
		DevTools:   "Developer Tools",

		Go:       "Go",
		Overview: "Overview",
		Library:  "Library",
		Figures:  "Figures",
		Notes:    "Notes",
		AI:       "AI Assistant",
		Research: "Literature Search",

		Window:          "Window",
		Minimize:        "Minimize",
		Zoom:            "Zoom",
		BringAllToFront: "Bring All to Front",

		Help:        "Help",
		Manual:      desktopAppName + " Manual",
		Guide:       "Feature Guide",
		ReportIssue: "Report an Issue…",

		TrayOpen: "Open " + desktopAppName,
		TrayQuit: "Quit " + desktopAppName,
	},
	menuLangZh: {
		About:      "关于 " + desktopAppName,
		Settings:   "设置…",
		Services:   "服务",
		Hide:       "隐藏 " + desktopAppName,
		HideOthers: "隐藏其他",
		ShowAll:    "全部显示",
		Quit:       "退出 " + desktopAppName,

		File:        "文件",
		UploadPDF:   "上传 PDF…",
		CloseWindow: "关闭窗口",

		Edit:               "编辑",
		Undo:               "撤销",
		Redo:               "重做",
		Cut:                "剪切",
		Copy:               "拷贝",
		Paste:              "粘贴",
		PasteAndMatchStyle: "粘贴并匹配样式",
		Delete:             "删除",
		SelectAll:          "全选",

		View:       "显示",
		Back:       "后退",
		Forward:    "前进",
		Reload:     "重新载入",
		ActualSize: "实际大小",
		ZoomIn:     "放大",
		ZoomOut:    "缩小",
		FullScreen: "切换全屏幕",
		DevTools:   "开发者工具",

		Go:       "前往",
		Overview: "总览",
		Library:  "文献库",
		Figures:  "图片库",
		Notes:    "笔记",
		AI:       "AI 助手",
		Research: "文献搜索",

		Window:          "窗口",
		Minimize:        "最小化",
		Zoom:            "缩放",
		BringAllToFront: "前置全部窗口",

		Help:        "帮助",
		Manual:      desktopAppName + " 使用手册",
		Guide:       "功能说明",
		ReportIssue: "反馈问题…",

		TrayOpen: "打开 " + desktopAppName,
		TrayQuit: "退出 " + desktopAppName,
	},
}

// normalizeMenuLang maps a UI language or locale ("zh-CN", "zh_Hans_CN",
// "en-US", ...) to one of the menu languages.
func normalizeMenuLang(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if lang == "" {
		return ""
	}
	if strings.HasPrefix(lang, "zh") {
		return menuLangZh
	}
	return menuLangEn
}

func stringsForLang(lang string) menuStrings {
	if s, ok := menuStringTable[normalizeMenuLang(lang)]; ok {
		return s
	}
	return menuStringTable[menuLangEn]
}

// devToolsEnabled reports whether the View menu offers the web inspector:
// only in dev builds (no version stamped by the Makefile) or when
// CITEBOX_DESKTOP_DEVTOOLS is set.
func devToolsEnabled() bool {
	if buildinfo.CurrentVersion() == "dev" {
		return true
	}
	value := strings.ToLower(strings.TrimSpace(os.Getenv("CITEBOX_DESKTOP_DEVTOOLS")))
	return value != "" && value != "0" && value != "false"
}

// goRoute is one entry of the Go menu.
type goRoute struct {
	label func(menuStrings) string
	path  string
	key   string
}

var goRoutes = []goRoute{
	{func(s menuStrings) string { return s.Overview }, "/", "1"},
	{func(s menuStrings) string { return s.Library }, "/library", "2"},
	{func(s menuStrings) string { return s.Figures }, "/figures", "3"},
	{func(s menuStrings) string { return s.Notes }, "/notes", "4"},
	{func(s menuStrings) string { return s.AI }, "/ai", "5"},
	{func(s menuStrings) string { return s.Research }, "/research", "6"},
}

// buildApplicationMenu builds the native menu bar in the given UI language.
func buildApplicationMenu(shell *desktopShell, lang string) *application.Menu {
	s := stringsForLang(lang)
	bar := application.NewMenu()

	if runtime.GOOS == "darwin" {
		appMenu := application.NewMenu()
		addItems(appMenu,
			application.NewRole(application.About).SetLabel(s.About),
			application.NewMenuItemSeparator(),
			application.NewMenuItem(s.Settings).SetAccelerator("CmdOrCtrl+,").OnClick(shell.navigateHandler("/settings")),
			application.NewMenuItemSeparator(),
			application.NewServicesMenu().SetLabel(s.Services),
			application.NewMenuItemSeparator(),
			application.NewRole(application.Hide).SetLabel(s.Hide),
			application.NewRole(application.HideOthers).SetLabel(s.HideOthers),
			application.NewRole(application.UnHide).SetLabel(s.ShowAll),
			application.NewMenuItemSeparator(),
			quitItem(shell, s),
		)
		addItems(bar, application.NewSubmenu(desktopAppName, appMenu))
	}

	fileMenu := application.NewMenu()
	addItems(fileMenu,
		application.NewMenuItem(s.UploadPDF).SetAccelerator("CmdOrCtrl+o").OnClick(shell.navigateHandler("/upload")),
		application.NewMenuItemSeparator(),
		// The CloseWindow role sends performClose: on macOS, so it goes through
		// the same close prompt as the red window button.
		application.NewRole(application.CloseWindow).SetLabel(s.CloseWindow),
	)
	if runtime.GOOS != "darwin" {
		addItems(fileMenu,
			application.NewMenuItem(s.Settings).SetAccelerator("CmdOrCtrl+,").OnClick(shell.navigateHandler("/settings")),
			application.NewMenuItemSeparator(),
			quitItem(shell, s),
		)
	}
	addItems(bar, application.NewSubmenu(s.File, fileMenu))

	editMenu := application.NewMenu()
	addItems(editMenu,
		application.NewRole(application.Undo).SetLabel(s.Undo),
		application.NewRole(application.Redo).SetLabel(s.Redo),
		application.NewMenuItemSeparator(),
		application.NewRole(application.Cut).SetLabel(s.Cut),
		application.NewRole(application.Copy).SetLabel(s.Copy),
		application.NewRole(application.Paste).SetLabel(s.Paste),
	)
	if runtime.GOOS == "darwin" {
		addItems(editMenu, application.NewRole(application.PasteAndMatchStyle).SetLabel(s.PasteAndMatchStyle))
	}
	// Wails gives Delete a bare Backspace accelerator; drop it so the key
	// always reaches the page.
	deleteItem := application.NewRole(application.Delete).SetLabel(s.Delete)
	deleteItem.RemoveAccelerator()
	addItems(editMenu,
		deleteItem,
		application.NewRole(application.SelectAll).SetLabel(s.SelectAll),
	)
	addItems(bar, application.NewSubmenu(s.Edit, editMenu))

	viewMenu := application.NewMenu()
	addItems(viewMenu,
		application.NewMenuItem(s.Back).SetAccelerator("CmdOrCtrl+[").OnClick(shell.execJSHandler("history.back()")),
		application.NewMenuItem(s.Forward).SetAccelerator("CmdOrCtrl+]").OnClick(shell.execJSHandler("history.forward()")),
		application.NewMenuItemSeparator(),
		application.NewRole(application.Reload).SetLabel(s.Reload),
		application.NewMenuItemSeparator(),
		application.NewRole(application.ResetZoom).SetLabel(s.ActualSize),
		application.NewRole(application.ZoomIn).SetLabel(s.ZoomIn),
		application.NewRole(application.ZoomOut).SetLabel(s.ZoomOut),
		application.NewMenuItemSeparator(),
		application.NewRole(application.ToggleFullscreen).SetLabel(s.FullScreen),
	)
	if devToolsEnabled() {
		// NewRole returns nil for OpenDevTools in Wails production builds.
		if item := application.NewRole(application.OpenDevTools); item != nil {
			addItems(viewMenu, application.NewMenuItemSeparator(), item.SetLabel(s.DevTools))
		}
	}
	addItems(bar, application.NewSubmenu(s.View, viewMenu))

	goMenu := application.NewMenu()
	for _, route := range goRoutes {
		addItems(goMenu, application.NewMenuItem(route.label(s)).
			SetAccelerator("CmdOrCtrl+"+route.key).
			OnClick(shell.navigateHandler(route.path)))
	}
	addItems(bar, application.NewSubmenu(s.Go, goMenu))

	windowMenu := application.NewMenu()
	addItems(windowMenu,
		application.NewRole(application.Minimise).SetLabel(s.Minimize),
		application.NewRole(application.Zoom).SetLabel(s.Zoom),
	)
	if runtime.GOOS == "darwin" {
		addItems(windowMenu,
			application.NewMenuItemSeparator(),
			application.NewRole(application.BringAllToFront).SetLabel(s.BringAllToFront),
		)
	}
	// The WindowMenu role makes macOS list the open windows in this menu.
	addItems(bar, application.NewSubmenu(s.Window, windowMenu).SetRole(application.WindowMenu))

	helpMenu := application.NewMenu()
	addItems(helpMenu,
		application.NewMenuItem(s.Manual).OnClick(shell.navigateHandler("/manual")),
		application.NewMenuItem(s.Guide).OnClick(shell.navigateHandler("/guide")),
		application.NewMenuItemSeparator(),
		application.NewMenuItem(s.ReportIssue).OnClick(func(*application.Context) {
			_ = shell.app.Browser.OpenURL(projectIssuesURL)
		}),
	)
	addItems(bar, application.NewSubmenu(s.Help, helpMenu))

	return bar
}

func quitItem(shell *desktopShell, s menuStrings) *application.MenuItem {
	// A plain item rather than the Quit role (terminate:), so quitting from the
	// menu takes the same path as the tray: no close prompt.
	return application.NewMenuItem(s.Quit).
		SetAccelerator("CmdOrCtrl+q").
		OnClick(func(*application.Context) { shell.quit() })
}

func addItems(menu *application.Menu, items ...*application.MenuItem) {
	for _, item := range items {
		if item != nil {
			menu.Append(application.NewMenuFromItems(item))
		}
	}
}

func buildTrayMenu(shell *desktopShell, lang string) *application.Menu {
	s := stringsForLang(lang)
	menu := application.NewMenu()
	menu.Add(s.TrayOpen).OnClick(func(*application.Context) { shell.restore() })
	menu.AddSeparator()
	menu.Add(s.TrayQuit).OnClick(func(*application.Context) { shell.quit() })
	return menu
}

// navigateHandler returns a menu callback that shows the window and opens
// path in it.
func (s *desktopShell) navigateHandler(path string) func(*application.Context) {
	quoted, _ := json.Marshal(path)
	return s.execJSHandler("window.location.assign(" + string(quoted) + ")")
}

func (s *desktopShell) execJSHandler(js string) func(*application.Context) {
	return func(*application.Context) {
		if s.window == nil {
			return
		}
		s.restore()
		s.window.ExecJS(js)
	}
}

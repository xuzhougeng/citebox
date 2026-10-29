// Command desktop-wails is a prototype of the CiteBox desktop client built on
// Wails v3. It serves the existing app handler in-process and replaces the
// hand-written cgo tray, close-prompt and Dock-reopen code in
// internal/desktopruntime with Wails APIs.
package main

import (
	"bytes"
	"errors"
	"image/png"
	"log/slog"
	"net"
	"os"
	"sync"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/xuzhougeng/citebox/internal/app"
	"github.com/xuzhougeng/citebox/internal/appicon"
	"github.com/xuzhougeng/citebox/internal/config"
	"github.com/xuzhougeng/citebox/internal/desktopapp"
	"github.com/xuzhougeng/citebox/internal/desktopbridge"
	"github.com/xuzhougeng/citebox/internal/logging"
)

const (
	desktopAppName = "CiteBox"
	desktopAppID   = "com.xuzhougeng.citebox"
	windowWidth    = 1440
	windowHeight   = 960
	trayIconSize   = 64
)

func main() {
	logger := logging.New()
	slog.SetDefault(logger)

	cfg := config.Load()
	if err := cfg.ApplyDesktopDefaults(desktopAppName); err != nil {
		logger.Error("failed to resolve desktop data directory", "error", err)
		os.Exit(1)
	}

	webRoot, err := desktopapp.ResolveWebRoot()
	if err != nil {
		logger.Error("failed to resolve desktop web assets", "error", err)
		os.Exit(1)
	}

	server, err := app.NewServer(app.Options{
		Config:      cfg,
		Logger:      logger,
		WebRoot:     webRoot,
		DesktopMode: true,
	})
	if err != nil {
		logger.Error("failed to initialize desktop server", "error", err)
		os.Exit(1)
	}
	var closeOnce sync.Once
	closeServer := func() {
		closeOnce.Do(func() {
			if err := server.Close(); err != nil {
				logger.Warn("failed to close desktop server", "error", err)
				return
			}
			logger.Info("desktop server closed")
		})
	}
	defer closeServer()

	// The window is served in-process, but the loopback listener stays so
	// external tools (MCP, integrations) can still reach the local server.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		logger.Error("failed to open local listener", "error", err)
		os.Exit(1)
	}
	go func() {
		if err := server.Serve(listener); err != nil {
			logger.Error("desktop loopback server stopped", "error", err)
		}
	}()

	bridgeScript, err := desktopbridge.Script()
	if err != nil {
		logger.Error("failed to build desktop bridge script", "error", err)
		os.Exit(1)
	}

	iconPNG, err := renderIconPNG(appicon.DefaultSize)
	if err != nil {
		logger.Warn("failed to render app icon", "error", err)
	}
	trayPNG, err := renderIconPNG(trayIconSize)
	if err != nil {
		logger.Warn("failed to render tray icon", "error", err)
	}

	shell := &desktopShell{server: server}
	wailsApp := application.New(application.Options{
		Name:       desktopAppName,
		Icon:       iconPNG,
		OnShutdown: closeServer,
		Services:   []application.Service{application.NewService(&DesktopService{shell: shell})},
		Assets: application.AssetOptions{
			Handler:        injectBridge(withCookieJar(server.Handler()), shimScript+bridgeScript),
			DisableLogging: true,
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: desktopAppID,
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				shell.restore()
			},
		},
	})
	shell.app = wailsApp

	shell.window = wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:   "main",
		Title:  desktopAppName,
		Width:  windowWidth,
		Height: windowHeight,
		URL:    "/",
	})
	shell.window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		if shell.quitting.Load() {
			return
		}
		event.Cancel()
		shell.window.ExecJS(`window.__citeboxDesktopOpenClosePrompt && window.__citeboxDesktopOpenClosePrompt();`)
	})

	shell.tray = wailsApp.SystemTray.New()
	if trayPNG != nil {
		shell.tray.SetIcon(trayPNG)
	}
	shell.tray.SetTooltip(desktopAppName)
	trayMenu := wailsApp.NewMenu()
	trayMenu.Add("Open " + desktopAppName).OnClick(func(*application.Context) { shell.restore() })
	trayMenu.AddSeparator()
	trayMenu.Add("Quit " + desktopAppName).OnClick(func(*application.Context) { shell.quit() })
	shell.tray.SetMenu(trayMenu)
	shell.tray.OnClick(shell.restore)
	shell.tray.OnRightClick(shell.tray.OpenMenu)

	// Like the cgo shell, the tray icon only exists while the window is hidden.
	wailsApp.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		shell.tray.Hide()
	})
	wailsApp.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(*application.ApplicationEvent) {
		shell.restore()
	})

	if err := wailsApp.Run(); err != nil {
		logger.Error("desktop app stopped with error", "error", err)
	}
}

type desktopShell struct {
	app      *application.App
	window   *application.WebviewWindow
	tray     *application.SystemTray
	server   *app.Server
	quitting atomic.Bool
}

func (s *desktopShell) minimizeToTray() {
	s.tray.Show()
	s.window.Hide()
}

func (s *desktopShell) restore() {
	s.window.Show()
	s.window.Focus()
	s.tray.Hide()
}

func (s *desktopShell) quit() {
	s.quitting.Store(true)
	s.app.Quit()
}

func renderIconPNG(size int) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, appicon.Render(size)); err != nil {
		return nil, err
	}
	if buf.Len() == 0 {
		return nil, errors.New("empty icon")
	}
	return buf.Bytes(), nil
}

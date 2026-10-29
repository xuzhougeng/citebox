// Command desktop is the CiteBox desktop client. It runs the app server
// in-process and shows it in a Wails v3 window, with a tray icon, a close
// prompt and Dock reopen handled by Wails.
package main

import (
	"bytes"
	"errors"
	"image/png"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/xuzhougeng/citebox/internal/app"
	"github.com/xuzhougeng/citebox/internal/appicon"
	"github.com/xuzhougeng/citebox/internal/config"
	"github.com/xuzhougeng/citebox/internal/desktopapp"
	"github.com/xuzhougeng/citebox/internal/desktopbridge"
	"github.com/xuzhougeng/citebox/internal/desktopicon"
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

	cookies := desktopbridge.NewCookieStore(filepath.Join(filepath.Dir(cfg.DatabasePath), "desktop-cookies.json"), logger)

	// Wails exits a second instance inside application.New, so build the app
	// before the server: a second launch must not open the database. The
	// handler is filled in once the server exists.
	appHandler := &lazyHandler{}
	var closeServer func()

	shell := &desktopShell{}
	wailsApp := application.New(application.Options{
		Name: desktopAppName,
		Icon: iconPNG,
		OnShutdown: func() {
			if closeServer != nil {
				closeServer()
			}
		},
		Services: []application.Service{application.NewService(&DesktopService{shell: shell})},
		Assets: application.AssetOptions{
			Handler:        desktopbridge.InjectScript(desktopbridge.WithCookieStore(appHandler, cookies), shimScript+bridgeScript),
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
	closeServer = func() {
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

	shell.server = server
	appHandler.set(server.Handler())

	shell.window = wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:   "main",
		Title:  desktopAppName,
		Width:  windowWidth,
		Height: windowHeight,
		URL:    "/",
		Linux: application.LinuxWindow{
			Icon: iconPNG,
		},
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
		if runtime.GOOS == "windows" {
			applyWindowsWindowIcon(shell.window, logger)
		}
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

// applyWindowsWindowIcon sets the title-bar and taskbar icon explicitly, since
// Wails only reads it from the executable's resources.
func applyWindowsWindowIcon(window *application.WebviewWindow, logger *slog.Logger) {
	assets, err := desktopicon.EnsureAssets(desktopAppName)
	if err != nil {
		logger.Warn("failed to prepare desktop icon assets", "error", err)
		return
	}
	application.InvokeSync(func() {
		if err := desktopicon.ApplyWindowIcon(window.NativeWindow(), assets); err != nil {
			logger.Warn("failed to apply desktop icon", "error", err)
		}
	})
}

// lazyHandler serves 503 until the app server is ready.
type lazyHandler struct {
	handler atomic.Pointer[http.Handler]
}

func (l *lazyHandler) set(h http.Handler) {
	l.handler.Store(&h)
}

func (l *lazyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := l.handler.Load()
	if h == nil {
		http.Error(w, "CiteBox is starting", http.StatusServiceUnavailable)
		return
	}
	(*h).ServeHTTP(w, r)
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

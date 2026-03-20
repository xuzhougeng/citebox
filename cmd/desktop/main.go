package main

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	webview "github.com/webview/webview_go"

	"github.com/xuzhougeng/citebox/internal/app"
	"github.com/xuzhougeng/citebox/internal/config"
	"github.com/xuzhougeng/citebox/internal/desktopapp"
	"github.com/xuzhougeng/citebox/internal/desktopicon"
	"github.com/xuzhougeng/citebox/internal/desktopruntime"
	"github.com/xuzhougeng/citebox/internal/logging"
)

const (
	desktopAppName = "CiteBox"
	windowWidth    = 1440
	windowHeight   = 960
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
		Config:  cfg,
		Logger:  logger,
		WebRoot: webRoot,
	})
	if err != nil {
		logger.Error("failed to initialize desktop server", "error", err)
		os.Exit(1)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		logger.Error("failed to open local listener", "error", err)
		_ = server.Close()
		os.Exit(1)
	}

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- server.Serve(listener)
	}()

	baseURL := "http://" + listener.Addr().String()
	if err := waitForServerReady(baseURL+"/login", 5*time.Second); err != nil {
		logger.Error("desktop server did not become ready", "error", err)
		_ = server.Close()
		<-serverErrCh
		os.Exit(1)
	}

	w := webview.New(false)
	defer w.Destroy()

	iconAssets, err := desktopicon.EnsureAssets(desktopAppName)
	if err != nil {
		logger.Warn("failed to prepare desktop icon assets", "error", err)
	} else if err := desktopicon.ApplyWindowIcon(w.Window(), iconAssets); err != nil {
		logger.Warn("failed to apply desktop icon", "error", err)
	}

	go func() {
		if err := <-serverErrCh; err != nil {
			logger.Error("desktop server stopped unexpectedly", "error", err)
			w.Terminate()
		}
	}()

	w.SetTitle(desktopAppName)
	w.SetSize(windowWidth, windowHeight, webview.HintNone)
	if err := desktopruntime.Configure(w, desktopAppName); err != nil {
		logger.Warn("failed to configure desktop runtime integrations", "error", err)
	}
	w.Navigate(baseURL)
	w.Run()

	if err := server.Close(); err != nil {
		logger.Warn("failed to close desktop server", "error", err)
	}
}

func waitForServerReady(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{
		Timeout: 500 * time.Millisecond,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	for time.Now().Before(deadline) {
		response, err := client.Get(url)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode >= 200 && response.StatusCode < 500 {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	return errors.New("timeout waiting for local server")
}

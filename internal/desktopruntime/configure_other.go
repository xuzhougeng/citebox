//go:build !darwin

package desktopruntime

import (
	"fmt"
	"os/exec"
	"runtime"

	webview "github.com/webview/webview_go"
)

func Configure(w webview.WebView, _ string) error {
	if err := w.Bind("citeboxDesktopOpenExternal", func(url string) error {
		return openExternal(url)
	}); err != nil {
		return fmt.Errorf("bind external opener: %w", err)
	}

	return initDesktopBridge(w)
}

func openExternal(url string) error {
	command, args, err := externalOpenCommand(url)
	if err != nil {
		return err
	}

	if err := exec.Command(command, args...).Start(); err != nil {
		return fmt.Errorf("open external url: %w", err)
	}
	return nil
}

func externalOpenCommand(url string) (string, []string, error) {
	switch runtime.GOOS {
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}, nil
	case "linux":
		return "xdg-open", []string{url}, nil
	default:
		return "", nil, fmt.Errorf("open external url: unsupported platform %s", runtime.GOOS)
	}
}

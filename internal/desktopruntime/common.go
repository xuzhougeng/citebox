package desktopruntime

import (
	"fmt"
	"sync"

	webview "github.com/webview/webview_go"
)

var desktopClosePromptWindows sync.Map

func bindExternalOpener(w webview.WebView) error {
	if err := w.Bind("citeboxDesktopOpenExternal", func(url string) error {
		return openExternal(url)
	}); err != nil {
		return fmt.Errorf("bind external opener: %w", err)
	}
	return nil
}

func bindClosePromptActions(w webview.WebView, minimize func() error, exit func() error) error {
	desktopClosePromptWindows.Store(uintptr(w.Window()), w)

	if err := w.Bind("citeboxDesktopMinimizeToTray", func() error {
		return minimize()
	}); err != nil {
		return fmt.Errorf("bind desktop tray minimizer: %w", err)
	}

	if err := w.Bind("citeboxDesktopExitApp", func() error {
		return exit()
	}); err != nil {
		return fmt.Errorf("bind desktop app exit: %w", err)
	}

	return nil
}

func dispatchClosePrompt(windowToken uintptr) bool {
	value, ok := desktopClosePromptWindows.Load(windowToken)
	if !ok {
		return false
	}

	w, ok := value.(webview.WebView)
	if !ok {
		return false
	}

	w.Dispatch(func() {
		w.Eval(`window.__citeboxDesktopOpenClosePrompt && window.__citeboxDesktopOpenClosePrompt();`)
	})
	return true
}

package desktopruntime

import (
	"fmt"

	webview "github.com/webview/webview_go"
)

func bindExternalOpener(w webview.WebView) error {
	if err := w.Bind("citeboxDesktopOpenExternal", func(url string) error {
		return openExternal(url)
	}); err != nil {
		return fmt.Errorf("bind external opener: %w", err)
	}
	return nil
}

//go:build !darwin

package desktopruntime

import webview "github.com/webview/webview_go"

func Configure(w webview.WebView, _ string) error {
	w.Init(`window.__CITEBOX_DESKTOP__ = true;`)
	return nil
}

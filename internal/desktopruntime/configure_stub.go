//go:build !darwin

package desktopruntime

import webview "github.com/webview/webview_go"

func Configure(_ webview.WebView, _ string) error {
	return nil
}

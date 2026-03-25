//go:build !darwin && !linux && !windows

package desktopruntime

import (
	webview "github.com/webview/webview_go"
	"github.com/xuzhougeng/citebox/internal/desktopicon"
)

func Configure(w webview.WebView, _ string, _ desktopicon.Assets) error {
	if err := bindExternalOpener(w); err != nil {
		return err
	}
	return initDesktopBridge(w)
}

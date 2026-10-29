package desktopruntime

import (
	webview "github.com/webview/webview_go"

	"github.com/xuzhougeng/citebox/internal/desktopbridge"
)

func initDesktopBridge(w webview.WebView) error {
	script, err := desktopbridge.Script()
	if err != nil {
		return err
	}
	w.Init(script)
	return nil
}

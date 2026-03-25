//go:build !darwin && !linux && !windows

package desktopruntime

import (
	"unsafe"

	webview "github.com/webview/webview_go"
	"github.com/xuzhougeng/citebox/internal/desktopicon"
)

func Configure(w webview.WebView, _ string, _ desktopicon.Assets, _ ClosePreferenceStore) error {
	if err := bindExternalOpener(w); err != nil {
		return err
	}
	return initDesktopBridge(w)
}

func ActivateWindow(_ unsafe.Pointer) error {
	return nil
}

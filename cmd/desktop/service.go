package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/xuzhougeng/citebox/internal/model"
)

// shimScript maps the window.citeboxDesktop* functions the bridge script
// expects onto DesktopService methods, so the shared bridge and the pages
// stay unchanged. Wails's runtime.js is an ES module, so it is loaded with a
// dynamic import from this classic script.
const shimScript = `(function() {
    const runtime = import('/wails/runtime.js');
    const call = (method) => (...args) => runtime.then((wails) => wails.Call.ByName('main.DesktopService.' + method, ...args));
    window.citeboxDesktopOpenExternal = call('OpenExternal');
    window.citeboxDesktopMinimizeToTray = call('MinimizeToTray');
    window.citeboxDesktopExitApp = call('ExitApp');
    window.citeboxDesktopGetClosePreference = call('GetClosePreference');
    window.citeboxDesktopSetClosePreference = call('SetClosePreference');
    window.citeboxDesktopReadClipboardText = call('ReadClipboardText');
    window.citeboxDesktopWriteClipboardText = call('WriteClipboardText');
    window.citeboxDesktopSaveFile = call('SaveFile');

    // Page errors are invisible without a web inspector, so send them to the
    // desktop log.
    const logError = call('LogFrontendError');
    window.addEventListener('error', (event) => {
        void logError(location.pathname, String(event.message || event.error || 'error')).catch(() => {});
    });
    window.addEventListener('unhandledrejection', (event) => {
        const reason = event.reason;
        void logError(location.pathname, String((reason && (reason.stack || reason.message)) || reason)).catch(() => {});
    });
})();
`

// DesktopService backs the window.citeboxDesktop* functions the bridge script
// expects. The JS shim calls each method by name through the Wails runtime.
type DesktopService struct {
	shell *desktopShell
}

func (d *DesktopService) OpenExternal(url string) error {
	return d.shell.app.Browser.OpenURL(url)
}

func (d *DesktopService) MinimizeToTray() {
	d.shell.minimizeToTray()
}

func (d *DesktopService) ExitApp() {
	d.shell.quit()
}

func (d *DesktopService) GetClosePreference() (string, error) {
	settings, err := d.shell.server.GetDesktopCloseSettings()
	if err != nil {
		return model.DesktopCloseActionAsk, err
	}
	return settings.Action, nil
}

func (d *DesktopService) SetClosePreference(action string) (string, error) {
	settings, err := d.shell.server.UpdateDesktopCloseSettings(model.DesktopCloseSettings{Action: action})
	if err != nil {
		return model.DesktopCloseActionAsk, err
	}
	return settings.Action, nil
}

func (d *DesktopService) LogFrontendError(page string, message string) {
	slog.Warn("frontend error", "page", page, "message", message)
}

func (d *DesktopService) ReadClipboardText() (string, error) {
	text, ok := d.shell.app.Clipboard.Text()
	if !ok {
		return "", errors.New("clipboard text unavailable")
	}
	return text, nil
}

func (d *DesktopService) WriteClipboardText(text string) error {
	if !d.shell.app.Clipboard.SetText(text) {
		return errors.New("failed to write clipboard text")
	}
	return nil
}

func (d *DesktopService) SaveFile(filename string, dataBase64 string) (map[string]bool, error) {
	data, err := base64.StdEncoding.DecodeString(dataBase64)
	if err != nil {
		return nil, fmt.Errorf("decode file data: %w", err)
	}

	path, err := d.shell.app.Dialog.SaveFile().
		SetFilename(filename).
		CanCreateDirectories(true).
		AttachToWindow(d.shell.window).
		PromptForSingleSelection()
	if err != nil {
		return nil, err
	}
	if path == "" {
		return map[string]bool{"saved": false}, nil
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}
	return map[string]bool{"saved": true}, nil
}

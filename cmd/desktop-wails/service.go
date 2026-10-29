package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"

	"github.com/xuzhougeng/citebox/internal/model"
)

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

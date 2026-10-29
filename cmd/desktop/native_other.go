//go:build !darwin

package main

import (
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// systemLanguage returns the locale from the usual environment variables.
func systemLanguage() string {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
}

// setWindowDarkAppearance is a no-op outside macOS.
func setWindowDarkAppearance(*application.WebviewWindow, bool) {}

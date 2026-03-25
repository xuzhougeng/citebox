//go:build !darwin

package desktopruntime

import (
	"fmt"
	"os/exec"
	"runtime"
)

func openExternal(url string) error {
	command, args, err := externalOpenCommand(url)
	if err != nil {
		return err
	}

	if err := exec.Command(command, args...).Start(); err != nil {
		return fmt.Errorf("open external url: %w", err)
	}
	return nil
}

func externalOpenCommand(url string) (string, []string, error) {
	switch runtime.GOOS {
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}, nil
	case "linux":
		return "xdg-open", []string{url}, nil
	default:
		return "", nil, fmt.Errorf("open external url: unsupported platform %s", runtime.GOOS)
	}
}

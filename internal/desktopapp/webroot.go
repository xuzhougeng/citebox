package desktopapp

import (
	"fmt"
	"os"
	"path/filepath"
)

func ResolveWebRoot() (string, error) {
	executablePath, err := os.Executable()
	if err != nil {
		return "", err
	}

	return ResolveWebRootFromExecutable(executablePath)
}

func ResolveWebRootFromExecutable(executablePath string) (string, error) {
	if executablePath == "" {
		return "", fmt.Errorf("missing executable path")
	}

	executableDir := filepath.Dir(executablePath)
	candidates := []string{
		filepath.Join(executableDir, "web"),
		filepath.Join(executableDir, "..", "Resources", "web"),
		"web",
	}

	for _, candidate := range candidates {
		webRoot := filepath.Clean(candidate)
		indexPath := filepath.Join(webRoot, "index.html")
		if _, err := os.Stat(indexPath); err == nil {
			return webRoot, nil
		}
	}

	return "", fmt.Errorf("cannot find web assets next to the executable, inside an app bundle, or current working directory")
}

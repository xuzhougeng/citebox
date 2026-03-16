package desktopicon

import (
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	"github.com/xuzhougeng/citebox/internal/appicon"
)

type Assets struct {
	PNGPath string
	ICOPath string
}

func EnsureAssets(appName string) (Assets, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil || cacheDir == "" {
		cacheDir = os.TempDir()
	}

	iconDir := filepath.Join(cacheDir, appName, "icons")
	if err := os.MkdirAll(iconDir, 0o755); err != nil {
		return Assets{}, err
	}

	icon := appicon.Render(appicon.DefaultSize)
	pngPath := filepath.Join(iconDir, "citebox-icon.png")
	icoPath := filepath.Join(iconDir, "citebox-icon.ico")

	if err := appicon.WritePNG(pngPath, icon); err != nil {
		return Assets{}, err
	}
	if err := appicon.WriteICO(icoPath, icon); err != nil {
		return Assets{}, err
	}

	return Assets{
		PNGPath: pngPath,
		ICOPath: icoPath,
	}, nil
}

func ApplyWindowIcon(window unsafe.Pointer, assets Assets) error {
	iconPath := assets.PNGPath
	if runtime.GOOS == "windows" {
		iconPath = assets.ICOPath
	}
	return applyNativeWindowIcon(window, iconPath)
}

package desktopicon

import (
	"encoding/binary"
	"os"
	"testing"
)

func TestEnsureAssetsWritesPNGAndICO(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	assets, err := EnsureAssets("CiteBoxTest")
	if err != nil {
		t.Fatalf("EnsureAssets() error = %v", err)
	}

	for _, path := range []string{assets.PNGPath, assets.ICOPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat(%q) error = %v", path, err)
		}
		if info.Size() == 0 {
			t.Fatalf("icon file %q is empty", path)
		}
	}

	icoBytes, err := os.ReadFile(assets.ICOPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", assets.ICOPath, err)
	}

	if got := binary.LittleEndian.Uint16(icoBytes[2:4]); got != 1 {
		t.Fatalf("unexpected ico type: %d", got)
	}
	if got := binary.LittleEndian.Uint16(icoBytes[4:6]); got != 1 {
		t.Fatalf("unexpected ico image count: %d", got)
	}
}

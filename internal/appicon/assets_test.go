package appicon

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestWritePNGAndICO(t *testing.T) {
	root := t.TempDir()
	img := Render(DefaultSize)

	pngPath := filepath.Join(root, "icon.png")
	icoPath := filepath.Join(root, "icon.ico")

	if err := WritePNG(pngPath, img); err != nil {
		t.Fatalf("WritePNG() error = %v", err)
	}
	if err := WriteICO(icoPath, img); err != nil {
		t.Fatalf("WriteICO() error = %v", err)
	}

	icoBytes, err := os.ReadFile(icoPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if got := binary.LittleEndian.Uint16(icoBytes[2:4]); got != 1 {
		t.Fatalf("unexpected ico type: %d", got)
	}
	if got := binary.LittleEndian.Uint16(icoBytes[4:6]); got != 1 {
		t.Fatalf("unexpected ico count: %d", got)
	}
}

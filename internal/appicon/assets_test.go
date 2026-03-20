package appicon

import (
	"encoding/binary"
	"image"
	"image/color"
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

func TestRenderIsCenteredAtLargeSizes(t *testing.T) {
	img, ok := Render(1024).(*image.NRGBA)
	if !ok {
		t.Fatalf("Render() did not return *image.NRGBA")
	}

	if alpha := img.NRGBAAt(48, 48).A; alpha != 0 {
		t.Fatalf("expected transparent padding in top-left, got alpha %d", alpha)
	}

	center := img.NRGBAAt(512, 512)
	if center.A == 0 {
		t.Fatalf("expected rendered icon content near the center")
	}
}

func TestRenderUsesRoundedTileBackground(t *testing.T) {
	img, ok := Render(DefaultSize).(*image.NRGBA)
	if !ok {
		t.Fatalf("Render() did not return *image.NRGBA")
	}

	if alpha := img.NRGBAAt(DefaultSize/2, 12).A; alpha != 0 {
		t.Fatalf("expected transparent area above the rounded tile, got alpha %d", alpha)
	}

	sample := img.NRGBAAt(DefaultSize/2, DefaultSize/2)
	if sample == (color.NRGBA{}) {
		t.Fatalf("expected opaque icon color at the center")
	}
}

package appicon

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
)

const DefaultSize = 256

func Render(size int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	accent := color.RGBA{R: 0xC1, G: 0x7F, B: 0x59, A: 0xFF}
	accentDark := color.RGBA{R: 0x7A, G: 0x4D, B: 0x38, A: 0xFF}
	page := color.RGBA{R: 0xF5, G: 0xF0, B: 0xE8, A: 0xFF}

	fillRect(img, 0, 0, size, size, accent)
	fillRect(img, 42, 36, 72, 220, accentDark)
	fillRect(img, 68, 36, 214, 220, page)

	strokeRect(img, 68, 36, 214, 220, accentDark, 8)
	drawLine(img, 42, 202, 61, 183, accentDark, 10)
	fillRect(img, 61, 183, 214, 193, accentDark)
	fillRect(img, 94, 86, 182, 98, accent)
	fillRect(img, 94, 124, 164, 136, accent)

	return img
}

func WritePNG(path string, img image.Image) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return png.Encode(file, img)
}

func WriteICO(path string, img image.Image) error {
	var pngBuffer bytes.Buffer
	if err := png.Encode(&pngBuffer, img); err != nil {
		return err
	}

	pngData := pngBuffer.Bytes()
	header := make([]byte, 22)
	binary.LittleEndian.PutUint16(header[0:], 0)
	binary.LittleEndian.PutUint16(header[2:], 1)
	binary.LittleEndian.PutUint16(header[4:], 1)
	header[6] = 0
	header[7] = 0
	header[8] = 0
	header[9] = 0
	binary.LittleEndian.PutUint16(header[10:], 1)
	binary.LittleEndian.PutUint16(header[12:], 32)
	binary.LittleEndian.PutUint32(header[14:], uint32(len(pngData)))
	binary.LittleEndian.PutUint32(header[18:], uint32(len(header)))

	data := append(header, pngData...)
	return os.WriteFile(path, data, 0o644)
}

func fillRect(img *image.RGBA, x0, y0, x1, y1 int, c color.Color) {
	draw.Draw(img, image.Rect(x0, y0, x1, y1), image.NewUniform(c), image.Point{}, draw.Src)
}

func strokeRect(img *image.RGBA, x0, y0, x1, y1 int, c color.Color, thickness int) {
	fillRect(img, x0, y0, x1, y0+thickness, c)
	fillRect(img, x0, y1-thickness, x1, y1, c)
	fillRect(img, x0, y0, x0+thickness, y1, c)
	fillRect(img, x1-thickness, y0, x1, y1, c)
}

func drawLine(img *image.RGBA, x0, y0, x1, y1 int, c color.Color, thickness int) {
	dx := x1 - x0
	dy := y1 - y0
	steps := max(abs(dx), abs(dy))
	if steps == 0 {
		fillRect(img, x0, y0, x0+thickness, y0+thickness, c)
		return
	}

	for i := 0; i <= steps; i++ {
		x := x0 + dx*i/steps
		y := y0 + dy*i/steps
		fillRect(img, x, y, x+thickness, y+thickness, c)
	}
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

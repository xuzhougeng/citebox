package appicon

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

const DefaultSize = 256

func Render(size int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))

	tileTop := color.RGBA{R: 0xD8, G: 0x94, B: 0x6C, A: 0xFF}
	tileBottom := color.RGBA{R: 0xB8, G: 0x73, B: 0x52, A: 0xFF}
	tileHighlightTop := color.RGBA{R: 0xFF, G: 0xEA, B: 0xD8, A: 0x52}
	tileHighlightBottom := color.RGBA{R: 0xFF, G: 0xEA, B: 0xD8, A: 0x00}
	shadow := color.RGBA{R: 0x52, G: 0x2E, B: 0x21, A: 0x34}
	pageShadow := color.RGBA{R: 0x52, G: 0x2E, B: 0x21, A: 0x26}
	pageFill := color.RGBA{R: 0xFB, G: 0xF6, B: 0xEF, A: 0xFF}
	pageFold := color.RGBA{R: 0xE7, G: 0xD5, B: 0xC6, A: 0xFF}
	band := color.RGBA{R: 0x73, G: 0x45, B: 0x33, A: 0xFF}
	lineStrong := color.RGBA{R: 0xB9, G: 0x72, B: 0x52, A: 0xFF}
	lineSoft := color.RGBA{R: 0xC9, G: 0x8A, B: 0x68, A: 0xFF}

	tileRect := scaledRect(size, 0.11, 0.10, 0.89, 0.88)
	tileRadius := float64(size) * 0.17

	fillRoundedRect(img, tileRect.offset(0, float64(size)*0.035), tileRadius, shadow)
	fillRoundedRectGradient(img, tileRect, tileRadius, tileTop, tileBottom)
	fillRoundedRectGradient(img, tileRect.inset(float64(size)*0.015, float64(size)*0.015), tileRadius-float64(size)*0.015, tileHighlightTop, tileHighlightBottom)

	pageRect := scaledRect(size, 0.28, 0.19, 0.72, 0.77)
	pageRadius := float64(size) * 0.06
	fillRoundedRect(img, pageRect.offset(float64(size)*0.015, float64(size)*0.02), pageRadius, pageShadow)
	fillRoundedRect(img, pageRect, pageRadius, pageFill)

	bandRect := scaledRect(size, 0.32, 0.23, 0.39, 0.73)
	fillRoundedRect(img, bandRect, float64(size)*0.018, band)

	foldSize := float64(size) * 0.11
	fold := [3]point{
		{x: pageRect.maxX - foldSize, y: pageRect.minY},
		{x: pageRect.maxX, y: pageRect.minY},
		{x: pageRect.maxX, y: pageRect.minY + foldSize},
	}
	fillTriangle(img, fold[0], fold[1], fold[2], pageFold)
	drawLine(img, fold[0], fold[2], band, float64(size)*0.012)

	fillRoundedRect(img, scaledRect(size, 0.44, 0.31, 0.64, 0.355), float64(size)*0.016, lineStrong)
	fillRoundedRect(img, scaledRect(size, 0.44, 0.42, 0.64, 0.465), float64(size)*0.016, lineStrong)
	fillRoundedRect(img, scaledRect(size, 0.44, 0.53, 0.59, 0.57), float64(size)*0.014, lineSoft)
	fillRoundedRect(img, scaledRect(size, 0.44, 0.61, 0.56, 0.648), float64(size)*0.014, lineSoft)

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

type point struct {
	x float64
	y float64
}

type rect struct {
	minX float64
	minY float64
	maxX float64
	maxY float64
}

func (r rect) inset(dx, dy float64) rect {
	return rect{
		minX: r.minX + dx,
		minY: r.minY + dy,
		maxX: r.maxX - dx,
		maxY: r.maxY - dy,
	}
}

func (r rect) offset(dx, dy float64) rect {
	return rect{
		minX: r.minX + dx,
		minY: r.minY + dy,
		maxX: r.maxX + dx,
		maxY: r.maxY + dy,
	}
}

func scaledRect(size int, minX, minY, maxX, maxY float64) rect {
	scale := float64(size)
	return rect{
		minX: minX * scale,
		minY: minY * scale,
		maxX: maxX * scale,
		maxY: maxY * scale,
	}
}

func fillRoundedRect(img *image.NRGBA, r rect, radius float64, c color.RGBA) {
	fillRoundedRectGradient(img, r, radius, c, c)
}

func fillRoundedRectGradient(img *image.NRGBA, r rect, radius float64, top, bottom color.RGBA) {
	if radius < 0 {
		radius = 0
	}

	minX := clampInt(int(math.Floor(r.minX-1)), 0, img.Bounds().Dx())
	minY := clampInt(int(math.Floor(r.minY-1)), 0, img.Bounds().Dy())
	maxX := clampInt(int(math.Ceil(r.maxX+1)), 0, img.Bounds().Dx())
	maxY := clampInt(int(math.Ceil(r.maxY+1)), 0, img.Bounds().Dy())

	for y := minY; y < maxY; y++ {
		py := float64(y) + 0.5
		t := 0.0
		if height := r.maxY - r.minY; height > 0 {
			t = clamp((py-r.minY)/height, 0, 1)
		}
		rowColor := lerpColor(top, bottom, t)

		for x := minX; x < maxX; x++ {
			px := float64(x) + 0.5
			coverage := roundedRectCoverage(px, py, r, radius)
			if coverage <= 0 {
				continue
			}

			src := rowColor
			src.A = uint8(math.Round(float64(src.A) * coverage))
			blendPixel(img, x, y, src)
		}
	}
}

func fillTriangle(img *image.NRGBA, a, b, c point, fill color.RGBA) {
	minX := clampInt(int(math.Floor(min3(a.x, b.x, c.x)-1)), 0, img.Bounds().Dx())
	minY := clampInt(int(math.Floor(min3(a.y, b.y, c.y)-1)), 0, img.Bounds().Dy())
	maxX := clampInt(int(math.Ceil(max3(a.x, b.x, c.x)+1)), 0, img.Bounds().Dx())
	maxY := clampInt(int(math.Ceil(max3(a.y, b.y, c.y)+1)), 0, img.Bounds().Dy())

	denominator := ((b.y - c.y) * (a.x - c.x)) + ((c.x - b.x) * (a.y - c.y))
	if denominator == 0 {
		return
	}

	for y := minY; y < maxY; y++ {
		py := float64(y) + 0.5
		for x := minX; x < maxX; x++ {
			px := float64(x) + 0.5
			w1 := (((b.y - c.y) * (px - c.x)) + ((c.x - b.x) * (py - c.y))) / denominator
			w2 := (((c.y - a.y) * (px - c.x)) + ((a.x - c.x) * (py - c.y))) / denominator
			w3 := 1 - w1 - w2
			if w1 < 0 || w2 < 0 || w3 < 0 {
				continue
			}

			blendPixel(img, x, y, fill)
		}
	}
}

func drawLine(img *image.NRGBA, start, end point, stroke color.RGBA, thickness float64) {
	minX := clampInt(int(math.Floor(min(start.x, end.x)-thickness)), 0, img.Bounds().Dx())
	minY := clampInt(int(math.Floor(min(start.y, end.y)-thickness)), 0, img.Bounds().Dy())
	maxX := clampInt(int(math.Ceil(max(start.x, end.x)+thickness)), 0, img.Bounds().Dx())
	maxY := clampInt(int(math.Ceil(max(start.y, end.y)+thickness)), 0, img.Bounds().Dy())
	radius := thickness / 2
	if radius <= 0 {
		radius = 0.5
	}

	for y := minY; y < maxY; y++ {
		py := float64(y) + 0.5
		for x := minX; x < maxX; x++ {
			px := float64(x) + 0.5
			distance := distanceToSegment(px, py, start, end)
			coverage := clamp(0.5+radius-distance, 0, 1)
			if coverage <= 0 {
				continue
			}

			src := stroke
			src.A = uint8(math.Round(float64(src.A) * coverage))
			blendPixel(img, x, y, src)
		}
	}
}

func roundedRectCoverage(px, py float64, r rect, radius float64) float64 {
	cx := (r.minX + r.maxX) / 2
	cy := (r.minY + r.maxY) / 2
	halfW := (r.maxX - r.minX) / 2
	halfH := (r.maxY - r.minY) / 2
	radius = min(radius, min(halfW, halfH))

	qx := math.Abs(px-cx) - (halfW - radius)
	qy := math.Abs(py-cy) - (halfH - radius)
	outside := math.Hypot(max(qx, 0), max(qy, 0)) + min(max(qx, qy), 0) - radius
	return clamp(0.5-outside, 0, 1)
}

func distanceToSegment(px, py float64, start, end point) float64 {
	dx := end.x - start.x
	dy := end.y - start.y
	if dx == 0 && dy == 0 {
		return math.Hypot(px-start.x, py-start.y)
	}

	t := (((px - start.x) * dx) + ((py - start.y) * dy)) / ((dx * dx) + (dy * dy))
	t = clamp(t, 0, 1)
	closestX := start.x + (dx * t)
	closestY := start.y + (dy * t)
	return math.Hypot(px-closestX, py-closestY)
}

func blendPixel(img *image.NRGBA, x, y int, src color.RGBA) {
	if !image.Pt(x, y).In(img.Bounds()) || src.A == 0 {
		return
	}

	dst := img.NRGBAAt(x, y)
	srcA := float64(src.A) / 255
	dstA := float64(dst.A) / 255
	outA := srcA + (dstA * (1 - srcA))
	if outA <= 0 {
		img.SetNRGBA(x, y, color.NRGBA{})
		return
	}

	srcR := float64(src.R) / 255
	srcG := float64(src.G) / 255
	srcB := float64(src.B) / 255
	dstR := float64(dst.R) / 255
	dstG := float64(dst.G) / 255
	dstB := float64(dst.B) / 255

	outR := ((srcR * srcA) + (dstR * dstA * (1 - srcA))) / outA
	outG := ((srcG * srcA) + (dstG * dstA * (1 - srcA))) / outA
	outB := ((srcB * srcA) + (dstB * dstA * (1 - srcA))) / outA

	img.SetNRGBA(x, y, color.NRGBA{
		R: uint8(math.Round(outR * 255)),
		G: uint8(math.Round(outG * 255)),
		B: uint8(math.Round(outB * 255)),
		A: uint8(math.Round(outA * 255)),
	})
}

func lerpColor(a, b color.RGBA, t float64) color.RGBA {
	return color.RGBA{
		R: uint8(math.Round(lerp(float64(a.R), float64(b.R), t))),
		G: uint8(math.Round(lerp(float64(a.G), float64(b.G), t))),
		B: uint8(math.Round(lerp(float64(a.B), float64(b.B), t))),
		A: uint8(math.Round(lerp(float64(a.A), float64(b.A), t))),
	}
}

func lerp(a, b, t float64) float64 {
	return a + ((b - a) * t)
}

func clamp(value, lower, upper float64) float64 {
	if value < lower {
		return lower
	}
	if value > upper {
		return upper
	}
	return value
}

func clampInt(value, lower, upper int) int {
	if value < lower {
		return lower
	}
	if value > upper {
		return upper
	}
	return value
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func min3(a, b, c float64) float64 {
	return min(a, min(b, c))
}

func max3(a, b, c float64) float64 {
	return max(a, max(b, c))
}

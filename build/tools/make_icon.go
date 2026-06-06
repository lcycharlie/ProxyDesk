//go:build ignore

package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

var (
	bgTop    = color.RGBA{18, 184, 166, 255}
	bgBottom = color.RGBA{7, 89, 133, 255}
	ink      = color.RGBA{255, 255, 255, 255}
	inkSoft  = color.RGBA{209, 250, 244, 255}
	shadow   = color.RGBA{15, 23, 42, 70}
)

func main() {
	outDir := filepath.Join("build", "windows")
	must(os.MkdirAll(outDir, 0755))

	sizes := []int{16, 24, 32, 48, 64, 128, 256}
	images := make([][]byte, 0, len(sizes))
	for _, size := range sizes {
		img := renderIcon(size)
		images = append(images, encodeDIB(img))
		if size == 256 {
			var buf bytes.Buffer
			must(png.Encode(&buf, img))
			must(os.WriteFile(filepath.Join(outDir, "ProxyDesk.png"), buf.Bytes(), 0644))
		}
	}
	must(writeICO(filepath.Join(outDir, "ProxyDesk.ico"), sizes, images))
}

func renderIcon(size int) *image.RGBA {
	scale := 4
	large := image.NewRGBA(image.Rect(0, 0, size*scale, size*scale))
	drawIcon(large, float64(scale))
	return downsample(large, size, scale)
}

func drawIcon(img *image.RGBA, scale float64) {
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	s := float64(w)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx := float64(x) + 0.5
			fy := float64(y) + 0.5
			alpha := roundedRectAlpha(fx, fy, s*0.04, s*0.04, s*0.92, s*0.92, s*0.21)
			if alpha <= 0 {
				continue
			}
			t := fy / s
			base := mix(bgTop, bgBottom, t)
			img.SetRGBA(x, y, withAlpha(base, alpha))
		}
	}

	fillRound(img, s*0.18, s*0.2, s*0.64, s*0.64, s*0.18, color.RGBA{255, 255, 255, 28})
	fillRound(img, s*0.22, s*0.25, s*0.56, s*0.56, s*0.14, color.RGBA{4, 47, 74, 60})

	fillRound(img, s*0.28, s*0.26, s*0.13, s*0.49, s*0.055, ink)
	fillRound(img, s*0.28, s*0.26, s*0.39, s*0.13, s*0.065, ink)
	fillRound(img, s*0.55, s*0.26, s*0.13, s*0.28, s*0.065, ink)
	fillRound(img, s*0.28, s*0.48, s*0.38, s*0.12, s*0.06, ink)

	fillCircle(img, s*0.68, s*0.69, s*0.055, shadow)
	fillCircle(img, s*0.49, s*0.72, s*0.045, shadow)
	fillCircle(img, s*0.70, s*0.47, s*0.045, shadow)
	strokeLine(img, s*0.52, s*0.70, s*0.66, s*0.69, s*0.025, inkSoft)
	strokeLine(img, s*0.67, s*0.65, s*0.70, s*0.51, s*0.025, inkSoft)
	fillCircle(img, s*0.68, s*0.69, s*0.044, ink)
	fillCircle(img, s*0.49, s*0.72, s*0.034, inkSoft)
	fillCircle(img, s*0.70, s*0.47, s*0.034, inkSoft)
}

func fillRound(img *image.RGBA, x, y, w, h, r float64, c color.RGBA) {
	for py := int(math.Floor(y)); py < int(math.Ceil(y+h)); py++ {
		for px := int(math.Floor(x)); px < int(math.Ceil(x+w)); px++ {
			a := roundedRectAlpha(float64(px)+0.5, float64(py)+0.5, x, y, w, h, r)
			if a > 0 {
				blend(img, px, py, withAlpha(c, a))
			}
		}
	}
}

func fillCircle(img *image.RGBA, cx, cy, r float64, c color.RGBA) {
	minX, maxX := int(cx-r-1), int(cx+r+1)
	minY, maxY := int(cy-r-1), int(cy+r+1)
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
			a := clamp(r+0.5-d, 0, 1)
			if a > 0 {
				blend(img, x, y, withAlpha(c, a))
			}
		}
	}
}

func strokeLine(img *image.RGBA, x1, y1, x2, y2, width float64, c color.RGBA) {
	minX := int(math.Min(x1, x2) - width - 1)
	maxX := int(math.Max(x1, x2) + width + 1)
	minY := int(math.Min(y1, y2) - width - 1)
	maxY := int(math.Max(y1, y2) + width + 1)
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			d := pointLineDistance(float64(x)+0.5, float64(y)+0.5, x1, y1, x2, y2)
			a := clamp(width/2+0.5-d, 0, 1)
			if a > 0 {
				blend(img, x, y, withAlpha(c, a))
			}
		}
	}
}

func roundedRectAlpha(px, py, x, y, w, h, r float64) float64 {
	qx := math.Abs(px-(x+w/2)) - w/2 + r
	qy := math.Abs(py-(y+h/2)) - h/2 + r
	d := math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - r
	return clamp(0.5-d, 0, 1)
}

func pointLineDistance(px, py, x1, y1, x2, y2 float64) float64 {
	dx, dy := x2-x1, y2-y1
	if dx == 0 && dy == 0 {
		return math.Hypot(px-x1, py-y1)
	}
	t := clamp(((px-x1)*dx+(py-y1)*dy)/(dx*dx+dy*dy), 0, 1)
	return math.Hypot(px-(x1+t*dx), py-(y1+t*dy))
}

func downsample(src *image.RGBA, size, scale int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a uint32
			for sy := 0; sy < scale; sy++ {
				for sx := 0; sx < scale; sx++ {
					c := src.RGBAAt(x*scale+sx, y*scale+sy)
					r += uint32(c.R)
					g += uint32(c.G)
					b += uint32(c.B)
					a += uint32(c.A)
				}
			}
			div := uint32(scale * scale)
			dst.SetRGBA(x, y, color.RGBA{uint8(r / div), uint8(g / div), uint8(b / div), uint8(a / div)})
		}
	}
	return dst
}

func writeICO(path string, sizes []int, images [][]byte) error {
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, uint16(0))
	binary.Write(&buf, binary.LittleEndian, uint16(1))
	binary.Write(&buf, binary.LittleEndian, uint16(len(images)))
	offset := 6 + len(images)*16
	for i, img := range images {
		sizeByte := byte(sizes[i])
		if sizes[i] == 256 {
			sizeByte = 0
		}
		buf.WriteByte(sizeByte)
		buf.WriteByte(sizeByte)
		buf.WriteByte(0)
		buf.WriteByte(0)
		binary.Write(&buf, binary.LittleEndian, uint16(1))
		binary.Write(&buf, binary.LittleEndian, uint16(32))
		binary.Write(&buf, binary.LittleEndian, uint32(len(img)))
		binary.Write(&buf, binary.LittleEndian, uint32(offset))
		offset += len(img)
	}
	for _, img := range images {
		buf.Write(img)
	}
	return os.WriteFile(path, buf.Bytes(), 0644)
}

func encodeDIB(img *image.RGBA) []byte {
	width := img.Bounds().Dx()
	height := img.Bounds().Dy()
	pixelBytes := width * height * 4
	maskStride := ((width + 31) / 32) * 4
	maskBytes := maskStride * height

	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, uint32(40))
	binary.Write(&buf, binary.LittleEndian, int32(width))
	binary.Write(&buf, binary.LittleEndian, int32(height*2))
	binary.Write(&buf, binary.LittleEndian, uint16(1))
	binary.Write(&buf, binary.LittleEndian, uint16(32))
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	binary.Write(&buf, binary.LittleEndian, uint32(pixelBytes))
	binary.Write(&buf, binary.LittleEndian, int32(0))
	binary.Write(&buf, binary.LittleEndian, int32(0))
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	binary.Write(&buf, binary.LittleEndian, uint32(0))

	for y := height - 1; y >= 0; y-- {
		for x := 0; x < width; x++ {
			c := img.RGBAAt(x, y)
			buf.WriteByte(c.B)
			buf.WriteByte(c.G)
			buf.WriteByte(c.R)
			buf.WriteByte(c.A)
		}
	}
	buf.Write(make([]byte, maskBytes))
	return buf.Bytes()
}

func mix(a, b color.RGBA, t float64) color.RGBA {
	return color.RGBA{
		R: uint8(float64(a.R)*(1-t) + float64(b.R)*t),
		G: uint8(float64(a.G)*(1-t) + float64(b.G)*t),
		B: uint8(float64(a.B)*(1-t) + float64(b.B)*t),
		A: 255,
	}
}

func withAlpha(c color.RGBA, a float64) color.RGBA {
	c.A = uint8(float64(c.A) * clamp(a, 0, 1))
	return c
}

func blend(img *image.RGBA, x, y int, src color.RGBA) {
	if !(image.Point{x, y}.In(img.Bounds())) {
		return
	}
	dst := img.RGBAAt(x, y)
	sa := float64(src.A) / 255
	da := float64(dst.A) / 255
	outA := sa + da*(1-sa)
	if outA == 0 {
		img.SetRGBA(x, y, color.RGBA{})
		return
	}
	r := (float64(src.R)*sa + float64(dst.R)*da*(1-sa)) / outA
	g := (float64(src.G)*sa + float64(dst.G)*da*(1-sa)) / outA
	b := (float64(src.B)*sa + float64(dst.B)*da*(1-sa)) / outA
	img.SetRGBA(x, y, color.RGBA{uint8(r), uint8(g), uint8(b), uint8(outA * 255)})
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

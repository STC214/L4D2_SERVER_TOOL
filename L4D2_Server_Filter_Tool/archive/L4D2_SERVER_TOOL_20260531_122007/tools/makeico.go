package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
)

func main() {
	in, err := os.Open("001.jpg")
	must(err)
	defer in.Close()
	img, err := jpeg.Decode(in)
	must(err)

	sizes := []int{256, 128, 64, 48, 32, 16}
	pngs := make([][]byte, 0, len(sizes))
	for _, size := range sizes {
		dst := resizeCover(img, size)
		var b bytes.Buffer
		must(png.Encode(&b, dst))
		pngs = append(pngs, b.Bytes())
	}

	out, err := os.Create("app.ico")
	must(err)
	defer out.Close()

	must(binary.Write(out, binary.LittleEndian, uint16(0)))
	must(binary.Write(out, binary.LittleEndian, uint16(1)))
	must(binary.Write(out, binary.LittleEndian, uint16(len(sizes))))

	offset := uint32(6 + 16*len(sizes))
	for i, size := range sizes {
		w := byte(size)
		h := byte(size)
		if size == 256 {
			w, h = 0, 0
		}
		out.Write([]byte{w, h, 0, 0})
		must(binary.Write(out, binary.LittleEndian, uint16(1)))
		must(binary.Write(out, binary.LittleEndian, uint16(32)))
		must(binary.Write(out, binary.LittleEndian, uint32(len(pngs[i]))))
		must(binary.Write(out, binary.LittleEndian, offset))
		offset += uint32(len(pngs[i]))
	}
	for _, p := range pngs {
		_, err := out.Write(p)
		must(err)
	}
}

func resizeCover(src image.Image, size int) *image.RGBA {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	side := sw
	if sh < side {
		side = sh
	}
	cropX := b.Min.X + (sw-side)/2
	cropY := b.Min.Y + (sh-side)/2
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx := float64(cropX) + (float64(x)+0.5)*float64(side)/float64(size) - 0.5
			fy := float64(cropY) + (float64(y)+0.5)*float64(side)/float64(size) - 0.5
			dst.Set(x, y, bilinear(src, fx, fy))
		}
	}
	return dst
}

func bilinear(img image.Image, x, y float64) color.RGBA {
	b := img.Bounds()
	x0 := clampInt(int(math.Floor(x)), b.Min.X, b.Max.X-1)
	y0 := clampInt(int(math.Floor(y)), b.Min.Y, b.Max.Y-1)
	x1 := clampInt(x0+1, b.Min.X, b.Max.X-1)
	y1 := clampInt(y0+1, b.Min.Y, b.Max.Y-1)
	tx := x - math.Floor(x)
	ty := y - math.Floor(y)
	c00 := rgba(img.At(x0, y0))
	c10 := rgba(img.At(x1, y0))
	c01 := rgba(img.At(x0, y1))
	c11 := rgba(img.At(x1, y1))
	return color.RGBA{
		R: byteClamp(lerp(lerp(float64(c00.R), float64(c10.R), tx), lerp(float64(c01.R), float64(c11.R), tx), ty)),
		G: byteClamp(lerp(lerp(float64(c00.G), float64(c10.G), tx), lerp(float64(c01.G), float64(c11.G), tx), ty)),
		B: byteClamp(lerp(lerp(float64(c00.B), float64(c10.B), tx), lerp(float64(c01.B), float64(c11.B), tx), ty)),
		A: 255,
	}
}

func rgba(c color.Color) color.RGBA {
	r, g, b, a := c.RGBA()
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
}

func lerp(a, b, t float64) float64 {
	return a + (b-a)*t
}

func byteClamp(v float64) uint8 {
	if v < 0 {
		v = 0
	}
	if v > 255 {
		v = 255
	}
	return uint8(v + 0.5)
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

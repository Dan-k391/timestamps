// Command genicon renders the Timestamps logo into the app .ico and the
// browser extension PNGs.
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
	"strconv"
)

type rgba struct{ r, g, b, a float64 }

func capsule(px, py, x0, x1, y, r float64) float64 {
	// distance to segment (x0,y)-(x1,y) minus radius
	cx := math.Max(x0, math.Min(x1, px))
	return math.Hypot(px-cx, py-y) - r
}

func roundRect(px, py, r float64) float64 {
	qx := math.Abs(px-0.5) - (0.5 - r)
	qy := math.Abs(py-0.5) - (0.5 - r)
	return math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - r
}

func sample(x, y float64) rgba {
	if roundRect(x, y, 0.23) > 0 {
		return rgba{}
	}
	t := (x*0.55 + y*0.45)
	c1 := rgba{0x63, 0x66, 0xF1, 1}
	c2 := rgba{0x0E, 0xA5, 0xA0, 1}
	bg := rgba{c1.r + (c2.r-c1.r)*t, c1.g + (c2.g-c1.g)*t, c1.b + (c2.b-c1.b)*t, 1}
	bars := []struct{ x0, x1, y, a float64 }{
		{0.24, 0.60, 0.315, 1},
		{0.40, 0.76, 0.50, 0.92},
		{0.24, 0.48, 0.685, 0.80},
	}
	for _, b := range bars {
		if capsule(x, y, b.x0, b.x1, b.y, 0.068) <= 0 {
			return rgba{255*b.a + bg.r*(1-b.a), 255*b.a + bg.g*(1-b.a), 255*b.a + bg.b*(1-b.a), 1}
		}
	}
	return bg
}

func render(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	const ss = 6
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			var r, g, b, a float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					c := sample((float64(px)+(float64(sx)+0.5)/ss)/float64(size), (float64(py)+(float64(sy)+0.5)/ss)/float64(size))
					r += c.r * c.a
					g += c.g * c.a
					b += c.b * c.a
					a += c.a
				}
			}
			n := float64(ss * ss)
			if a > 0 {
				img.SetNRGBA(px, py, color.NRGBA{uint8(r / a), uint8(g / a), uint8(b / a), uint8(math.Round(a / n * 255))})
			}
		}
	}
	return img
}

func pngBytes(img image.Image) []byte {
	var b bytes.Buffer
	png.Encode(&b, img)
	return b.Bytes()
}

func main() {
	root := os.Args[1]
	sizes := []int{16, 20, 24, 32, 40, 48, 64, 128, 256}
	var entries [][]byte
	for _, s := range sizes {
		entries = append(entries, pngBytes(render(s)))
	}
	var ico bytes.Buffer
	binary.Write(&ico, binary.LittleEndian, []uint16{0, 1, uint16(len(sizes))})
	off := 6 + 16*len(sizes)
	for i, s := range sizes {
		w := uint8(s)
		if s >= 256 {
			w = 0
		}
		ico.Write([]byte{w, w, 0, 0})
		binary.Write(&ico, binary.LittleEndian, []uint16{1, 32})
		binary.Write(&ico, binary.LittleEndian, []uint32{uint32(len(entries[i])), uint32(off)})
		off += len(entries[i])
	}
	for _, e := range entries {
		ico.Write(e)
	}
	os.MkdirAll(filepath.Join(root, "assets"), 0o755)
	os.WriteFile(filepath.Join(root, "assets", "app.ico"), ico.Bytes(), 0o644)
	for _, dir := range []string{"extension", filepath.Join("extension", "firefox")} {
		for _, s := range []int{16, 48, 128} {
			os.WriteFile(filepath.Join(root, dir, "icon"+strconv.Itoa(s)+".png"), pngBytes(render(s)), 0o644)
		}
	}
	os.WriteFile(filepath.Join(root, "web", "icon.png"), pngBytes(render(64)), 0o644)
}

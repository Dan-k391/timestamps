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

// segDist is the distance from (px,py) to the segment (ax,ay)-(bx,by).
func segDist(px, py, ax, ay, bx, by float64) float64 {
	vx, vy := bx-ax, by-ay
	t := math.Max(0, math.Min(1, ((px-ax)*vx+(py-ay)*vy)/(vx*vx+vy*vy)))
	return math.Hypot(px-(ax+t*vx), py-(ay+t*vy))
}

var (
	rim   = rgba{0x1E, 0x29, 0x3B, 1} // dial rim and hands
	dial  = rgba{0xFF, 0xFF, 0xFF, 1}
	green = rgba{0x22, 0xC5, 0x5E, 1} // active time
	red   = rgba{0xEF, 0x44, 0x44, 1} // away time
)

// sample draws a clock dial whose outer ring shows a day of activity:
// a long green "active" arc and a shorter red "away" arc.
func sample(x, y float64) rgba {
	dx, dy := x-0.5, y-0.5
	r := math.Hypot(dx, dy)
	ang := math.Mod(math.Atan2(dx, -dy)*180/math.Pi+360, 360) // clockwise from 12 o'clock
	switch {
	case r > 0.49:
		return rgba{}
	case r > 0.445:
		return rim
	}
	if r < 0.058 || segDist(x, y, 0.5, 0.5, 0.5, 0.23) < 0.034 {
		return rim
	}
	h := 128 * math.Pi / 180
	if segDist(x, y, 0.5, 0.5, 0.5+0.2*math.Sin(h), 0.5-0.2*math.Cos(h)) < 0.04 {
		return rim
	}
	if r > 0.305 {
		if ang > 4 && ang < 160 {
			return green
		}
		if ang > 205 && ang < 282 {
			return red
		}
	}
	return dial
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

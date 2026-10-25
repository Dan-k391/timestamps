// Command genicon renders the Timestamps logo into the app .ico, the macOS
// .icns and the browser extension PNGs.
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

// renderMac draws the logo the macOS way: the dial at the size of the
// system's round icons (about 80% of the canvas) on a soft shadow.
func renderMac(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	const scale = 0.82
	ss := 6
	if size >= 256 {
		ss = 3
	}
	radius := 0.49 * scale
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			var r, g, b, a float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					u := (float64(px) + (float64(sx)+0.5)/float64(ss)) / float64(size)
					v := (float64(py) + (float64(sy)+0.5)/float64(ss)) / float64(size)
					c := sample((u-0.5)/scale+0.5, (v-0.5)/scale+0.5)
					r += c.r * c.a
					g += c.g * c.a
					b += c.b * c.a
					a += c.a
				}
			}
			n := float64(ss * ss)
			da := a / n
			// The shadow falls a little below the dial and fades out.
			u, v := (float64(px)+0.5)/float64(size), (float64(py)+0.5)/float64(size)
			d := math.Hypot(u-0.5, v-0.5-0.012) - radius
			sa := 0.0
			if d < 0.03 {
				sa = 0.3 * math.Pow(math.Min(1, 1-d/0.03), 2)
			}
			oa := da + (1-da)*sa
			if oa <= 0 {
				continue
			}
			k := 0.0
			if a > 0 {
				k = da / oa / a // dial colour weight per accumulated sample
			}
			img.SetNRGBA(px, py, color.NRGBA{uint8(math.Round(r * k)), uint8(math.Round(g * k)), uint8(math.Round(b * k)), uint8(math.Round(oa * 255))})
		}
	}
	return img
}

// icns packs PNG renderings into an Apple icon file.
func icns() []byte {
	pngs := map[int][]byte{}
	for _, s := range []int{16, 32, 64, 128, 256, 512, 1024} {
		pngs[s] = pngBytes(renderMac(s))
	}
	var body bytes.Buffer
	for _, e := range []struct {
		typ  string
		size int
	}{{"icp4", 16}, {"icp5", 32}, {"icp6", 64}, {"ic07", 128}, {"ic08", 256}, {"ic09", 512}, {"ic10", 1024}, {"ic11", 32}, {"ic12", 64}, {"ic13", 256}, {"ic14", 512}} {
		body.WriteString(e.typ)
		binary.Write(&body, binary.BigEndian, uint32(8+len(pngs[e.size])))
		body.Write(pngs[e.size])
	}
	var out bytes.Buffer
	out.WriteString("icns")
	binary.Write(&out, binary.BigEndian, uint32(8+body.Len()))
	out.Write(body.Bytes())
	return out.Bytes()
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
	os.WriteFile(filepath.Join(root, "assets", "Timestamps.icns"), icns(), 0o644)
	for _, dir := range []string{"extension", filepath.Join("extension", "firefox")} {
		for _, s := range []int{16, 48, 128} {
			os.WriteFile(filepath.Join(root, dir, "icon"+strconv.Itoa(s)+".png"), pngBytes(render(s)), 0o644)
		}
	}
	os.WriteFile(filepath.Join(root, "web", "icon.png"), pngBytes(render(64)), 0o644)
}

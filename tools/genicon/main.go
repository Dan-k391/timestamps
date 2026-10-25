// Command genicon renders the Timestamps logo into the app .ico, the web
// icon and the browser extension PNGs: the day ring the tray shows, drawn
// as it looks at 18:00 (three quarters full, orange). The running app draws
// the live ring itself.
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strconv"

	"timestamps/internal/ring"
)

const frac = 0.75

func render(size int) *image.NRGBA { return ring.Draw(size, frac, ring.Color(frac)) }

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

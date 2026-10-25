package iconcolor

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// icon draws a w×w image: background bg, and fg over the given fraction of
// its rows.
func icon(bg, fg color.NRGBA, frac float64) *image.NRGBA {
	const w = 32
	img := image.NewNRGBA(image.Rect(0, 0, w, w))
	for y := 0; y < w; y++ {
		for x := 0; x < w; x++ {
			c := bg
			if float64(y) < frac*w {
				c = fg
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func hue(t *testing.T, hexc string) float64 {
	t.Helper()
	var r, g, b int
	if _, err := fmtSscan(hexc, &r, &g, &b); err != nil {
		t.Fatal(err)
	}
	h, _, _ := hsl(float64(r)/255, float64(g)/255, float64(b)/255)
	return h
}

func TestOf(t *testing.T) {
	white := color.NRGBA{255, 255, 255, 255}
	clear := color.NRGBA{}
	for _, c := range []struct {
		name   string
		img    image.Image
		hue    float64 // wanted hue (±20°), or -1 for Neutral
		minLum float64
	}{
		{"orange on white (Claude)", icon(white, color.NRGBA{217, 119, 87, 255}, 0.6), 15, 0},
		{"yellow folder with a blue strip (Explorer)", icon(color.NRGBA{40, 110, 210, 255}, color.NRGBA{250, 200, 60, 255}, 0.7), 44, 0},
		{"blue whale on white (DeepSeek)", icon(white, color.NRGBA{77, 107, 254, 255}, 0.3), 230, 0},
		{"black and white logo (ChatGPT)", icon(white, color.NRGBA{20, 20, 20, 255}, 0.5), -1, 0},
		{"transparent", icon(clear, clear, 0), -1, 0},
		{"very dark navy is lifted", icon(clear, color.NRGBA{0, 30, 70, 255}, 1), 214, 0.4},
	} {
		got := Of(c.img)
		if c.hue < 0 {
			if got != Neutral {
				t.Errorf("%s: %s, want neutral", c.name, got)
			}
			continue
		}
		if d := hue(t, got) - c.hue; d > 20 || d < -20 {
			t.Errorf("%s: %s has hue %.0f, want about %.0f", c.name, got, hue(t, got), c.hue)
		}
	}
}

func TestICO(t *testing.T) {
	src := icon(color.NRGBA{}, color.NRGBA{30, 160, 90, 255}, 1) // green
	// An .ico with a 32-bit bitmap.
	var bmp bytes.Buffer
	w := src.Bounds().Dx()
	hdr := make([]byte, 40)
	binary.LittleEndian.PutUint32(hdr, 40)
	binary.LittleEndian.PutUint32(hdr[4:], uint32(w))
	binary.LittleEndian.PutUint32(hdr[8:], uint32(2*w))
	binary.LittleEndian.PutUint16(hdr[12:], 1)
	binary.LittleEndian.PutUint16(hdr[14:], 32)
	bmp.Write(hdr)
	for y := w - 1; y >= 0; y-- {
		for x := 0; x < w; x++ {
			c := src.NRGBAAt(x, y)
			bmp.Write([]byte{c.B, c.G, c.R, c.A})
		}
	}
	bmp.Write(make([]byte, w*w/8)) // AND mask
	for _, data := range [][]byte{bmp.Bytes(), pngBytes(t, src)} {
		ico := make([]byte, 22)
		binary.LittleEndian.PutUint16(ico[2:], 1)
		binary.LittleEndian.PutUint16(ico[4:], 1)
		ico[6], ico[7] = byte(w), byte(w)
		binary.LittleEndian.PutUint32(ico[14:], uint32(len(data)))
		binary.LittleEndian.PutUint32(ico[18:], 22)
		got, ok := FromBytes(append(ico, data...))
		if !ok || hue(t, got) < 130 || hue(t, got) > 160 {
			t.Errorf("ico: %s %v, want green", got, ok)
		}
	}
	if _, ok := FromBytes([]byte("<svg/>")); ok {
		t.Error("svg should not be read")
	}
}

func fmtSscan(s string, r, g, b *int) (int, error) { return fmt.Sscanf(s, "%02x%02x%02x", r, g, b) }

func pngBytes(t *testing.T, img image.Image) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

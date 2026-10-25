// Package iconcolor picks a color that stands for an icon: the main hue of
// its colored pixels (white, black and gray parts are ignored), toned so it
// reads well as a bar or timeline block on light and dark backgrounds.
// Icons that are mostly black, white or gray get a neutral gray.
package iconcolor

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif" // decoders for site icons
	_ "image/jpeg"
	"image/png"
	"math"
)

// Neutral is the color of icons without a clear hue.
const Neutral = "7C8594"

// FromBytes returns the color (RRGGBB, no "#") of a PNG, ICO, JPEG or GIF
// icon; ok is false when the image can't be read.
func FromBytes(b []byte) (hex string, ok bool) {
	img, err := decode(b)
	if err != nil {
		return "", false
	}
	return Of(img), true
}

// Of returns the color (RRGGBB) standing for img.
func Of(img image.Image) string {
	b := img.Bounds()
	step := 1
	if n := b.Dx(); n > 64 || b.Dy() > 64 {
		if b.Dy() > n {
			n = b.Dy()
		}
		step = n / 64
	}
	type bin struct{ w, r, g, b float64 }
	var bins [36]bin
	var opaque, colored float64
	for y := b.Min.Y; y < b.Max.Y; y += step {
		for x := b.Min.X; x < b.Max.X; x += step {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if c.A < 128 {
				continue
			}
			opaque++
			r, g, bl := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
			h, _, l := hsl(r, g, bl)
			chroma := math.Max(r, math.Max(g, bl)) - math.Min(r, math.Min(g, bl))
			if chroma < 0.16 || l < 0.08 || l > 0.95 {
				continue
			}
			colored++
			i := int(h/10) % 36
			bins[i].w += chroma
			bins[i].r += r * chroma
			bins[i].g += g * chroma
			bins[i].b += bl * chroma
		}
	}
	if opaque == 0 || colored/opaque < 0.1 {
		return Neutral
	}
	// The strongest run of three neighbouring hue bins.
	best, bestW := 0, -1.0
	for i := range bins {
		w := bins[(i+35)%36].w + bins[i].w + bins[(i+1)%36].w
		if w > bestW {
			best, bestW = i, w
		}
	}
	var r, g, bl float64
	for _, j := range []int{(best + 35) % 36, best, (best + 1) % 36} {
		r, g, bl = r+bins[j].r, g+bins[j].g, bl+bins[j].b
	}
	h, s, l := hsl(r/bestW, g/bestW, bl/bestW)
	s = math.Min(math.Max(s, 0.45), 0.85)
	l = math.Min(math.Max(l, 0.42), 0.6)
	return hex(hslToRGB(h, s, l))
}

func hsl(r, g, b float64) (h, s, l float64) {
	mx, mn := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	l = (mx + mn) / 2
	d := mx - mn
	if d == 0 {
		return 0, 0, l
	}
	s = d / (1 - math.Abs(2*l-1))
	switch mx {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h, s, l
}

func hslToRGB(h, s, l float64) (r, g, b float64) {
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return r + m, g + m, b + m
}

func hex(r, g, b float64) string {
	to := func(v float64) int { return int(math.Round(math.Min(math.Max(v, 0), 1) * 255)) }
	return fmt.Sprintf("%02X%02X%02X", to(r), to(g), to(b))
}

var errFormat = errors.New("unsupported icon")

func decode(b []byte) (image.Image, error) {
	if len(b) >= 4 && b[0] == 0 && b[1] == 0 && b[2] == 1 && b[3] == 0 {
		return decodeICO(b)
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	return img, err
}

// decodeICO reads the largest image of a .ico file: PNG, or a 32- or 24-bit
// bitmap.
func decodeICO(b []byte) (image.Image, error) {
	if len(b) < 6 {
		return nil, errFormat
	}
	n := int(binary.LittleEndian.Uint16(b[4:]))
	var data []byte
	area := -1
	for i := 0; i < n && 6+16*(i+1) <= len(b); i++ {
		e := b[6+16*i:]
		w, h := int(e[0]), int(e[1])
		if w == 0 {
			w = 256
		}
		if h == 0 {
			h = 256
		}
		size, off := int(binary.LittleEndian.Uint32(e[8:])), int(binary.LittleEndian.Uint32(e[12:]))
		if off < 0 || size <= 0 || off+size > len(b) || w*h <= area {
			continue
		}
		area, data = w*h, b[off:off+size]
	}
	if data == nil {
		return nil, errFormat
	}
	if bytes.HasPrefix(data, []byte("\x89PNG")) {
		return png.Decode(bytes.NewReader(data))
	}
	if len(data) < 40 {
		return nil, errFormat
	}
	hdr := int(binary.LittleEndian.Uint32(data))
	w := int(int32(binary.LittleEndian.Uint32(data[4:])))
	h := int(int32(binary.LittleEndian.Uint32(data[8:]))) / 2 // the height includes the mask
	bpp := int(binary.LittleEndian.Uint16(data[14:]))
	if comp := binary.LittleEndian.Uint32(data[16:]); comp != 0 || w <= 0 || h <= 0 || w > 512 || h > 512 || (bpp != 32 && bpp != 24) {
		return nil, errFormat
	}
	stride := (w*bpp/8 + 3) &^ 3
	px := data[hdr:]
	if len(px) < stride*h {
		return nil, errFormat
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		row := px[(h-1-y)*stride:] // bottom-up
		for x := 0; x < w; x++ {
			p := row[x*bpp/8:]
			a := uint8(255)
			if bpp == 32 {
				a = p[3]
			}
			img.SetNRGBA(x, y, color.NRGBA{R: p[2], G: p[1], B: p[0], A: a})
		}
	}
	return img, nil
}

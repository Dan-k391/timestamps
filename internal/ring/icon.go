package ring

import (
	"image"
	"image/color"
	"math"
)

// DrawIcon draws the ring the way macOS lays out app icons: at about 80%
// of the canvas, centered, over a soft shadow (the Dock and the .icns).
func DrawIcon(size int, frac float64, c color.NRGBA) *image.NRGBA {
	inner := int(math.Round(float64(size) * 0.8))
	off := (size - inner) / 2
	r := Draw(inner, frac, c)

	// The shadow: the ring's coverage, blurred and moved down a little.
	alpha := make([]float64, size*size)
	dy := int(math.Round(float64(size) * 0.012))
	for y := 0; y < inner; y++ {
		for x := 0; x < inner; x++ {
			if ty := y + off + dy; ty < size {
				alpha[ty*size+x+off] = float64(r.Pix[y*r.Stride+x*4+3]) / 255
			}
		}
	}
	blur(alpha, size, int(math.Max(1, math.Round(float64(size)*0.015))))

	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			sa := alpha[y*size+x] * 0.28
			var fc color.NRGBA
			if x >= off && x < off+inner && y >= off && y < off+inner {
				fc = r.NRGBAAt(x-off, y-off)
			}
			fa := float64(fc.A) / 255
			oa := fa + sa*(1-fa) // the ring over a black shadow
			if oa <= 0 {
				continue
			}
			ch := func(v uint8) uint8 { return uint8(math.Round(float64(v) * fa / oa)) }
			img.SetNRGBA(x, y, color.NRGBA{ch(fc.R), ch(fc.G), ch(fc.B), uint8(math.Round(oa * 255))})
		}
	}
	return img
}

// blur box-blurs a size×size plane twice in each direction (close to a
// Gaussian), in place.
func blur(p []float64, size, radius int) {
	tmp := make([]float64, len(p))
	pass := func(src, dst []float64, stride, step int) {
		for line := 0; line < size; line++ {
			at := func(i int) float64 { return src[line*stride+i*step] }
			var sum float64
			for i := 0; i <= radius && i < size; i++ {
				sum += at(i)
			}
			for i := 0; i < size; i++ {
				dst[line*stride+i*step] = sum / float64(2*radius+1)
				if j := i - radius; j >= 0 {
					sum -= at(j)
				}
				if j := i + radius + 1; j < size {
					sum += at(j)
				}
			}
		}
	}
	for k := 0; k < 2; k++ {
		pass(p, tmp, size, 1) // rows
		pass(tmp, p, 1, size) // columns
	}
}

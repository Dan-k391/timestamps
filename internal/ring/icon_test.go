package ring

import (
	"image/color"
	"testing"
)

func TestDrawIcon(t *testing.T) {
	red := color.NRGBA{255, 0, 0, 255}
	const s = 128
	img := DrawIcon(s, 0.5, red)
	// The ring is inset (macOS icon margins): the edges stay clear apart
	// from the faint shadow, the arc is on the right, the middle is empty.
	if c := img.NRGBAAt(s-2, s/2); c.A > 40 {
		t.Errorf("right edge should be margin: %v", c)
	}
	if c := img.NRGBAAt(s-s/10-6, s/2); c.R < 200 || c.A < 200 {
		t.Errorf("3 o'clock should be the arc: %v", c)
	}
	if c := img.NRGBAAt(s/2, s/2); c.A > 20 {
		t.Errorf("center should be empty: %v", c)
	}
}

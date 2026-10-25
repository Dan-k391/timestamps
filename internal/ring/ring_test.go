package ring

import (
	"image/color"
	"testing"
)

func TestColor(t *testing.T) {
	if c := Color(0); c != stops[0].c {
		t.Errorf("start %v", c)
	}
	if c := Color(1); c != stops[len(stops)-1].c {
		t.Errorf("end %v", c)
	}
	if c := Color(0.45); c != stops[1].c {
		t.Errorf("middle %v", c)
	}
	if c := Color(0.6); c.R < 242 || c.G > 197 || c.G < 146 {
		t.Errorf("between yellow and orange: %v", c)
	}
}

func TestDraw(t *testing.T) {
	red := color.NRGBA{255, 0, 0, 255}
	const s = 32
	img := Draw(s, 0.5, red)
	// Right half (3 o'clock) is on the arc, left half (9 o'clock) only track.
	right, left := img.NRGBAAt(s-4, s/2), img.NRGBAAt(3, s/2)
	if right.R < 200 || right.A < 200 {
		t.Errorf("3 o'clock should be the arc: %v", right)
	}
	if left.R > 200 || left.A == 0 || left.A > 160 {
		t.Errorf("9 o'clock should be faint track: %v", left)
	}
	if c := img.NRGBAAt(s/2, s/2); c.A != 0 {
		t.Errorf("center should be empty: %v", c)
	}
	// Nothing filled at the very start of the day.
	empty := Draw(s, 0, red)
	if c := empty.NRGBAAt(s-4, s/2); c.R > 200 {
		t.Errorf("empty ring has arc: %v", c)
	}
}

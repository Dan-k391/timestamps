//go:build windows

package win

import (
	"testing"
	"unsafe"
)

func TestRawInputLayout(t *testing.T) {
	var r rawInput
	if o := unsafe.Offsetof(r.Data); o != 24 {
		t.Errorf("the header is %d bytes, want 24", o)
	}
	if s := unsafe.Sizeof(rawMouse{}); s != 24 {
		t.Errorf("RAWMOUSE is %d bytes, want 24", s)
	}
	if s := unsafe.Sizeof(rawKeyboard{}); s != 16 {
		t.Errorf("RAWKEYBOARD is %d bytes, want 16", s)
	}
	if o := unsafe.Offsetof(rawMouse{}.ButtonFlags); o != 4 {
		t.Errorf("usButtonFlags at %d, want 4", o)
	}
}

func TestCaptureInput(t *testing.T) {
	CaptureInput(true)
	if inputThread == 0 {
		t.Skip("no raw input here")
	}
	CaptureInput(false)
	if inputThread != 0 {
		t.Error("switching off should forget the thread")
	}
}

func TestTakeInputOwnsItsMap(t *testing.T) {
	_, taken := TakeInput()
	var s inputState
	s.key(&inputTotal, inputKeys, &rawKeyboard{VKey: 0x41, MakeCode: 0x1E}, 1000)
	if len(taken) != 0 {
		t.Errorf("a key landed in the map already taken: %v", taken)
	}
	if n, keys := TakeInput(); n.Keys != 1 || keys[0x1E] != 1 {
		t.Errorf("next take: %+v, %v", n, keys)
	}
}

func TestInputCounts(t *testing.T) {
	var s inputState
	var n Input
	keys := map[uint16]int64{}
	at := uint32(1000)
	press := func(k rawKeyboard, ms uint32) { at += ms; s.key(&n, keys, &k, at) }
	a := rawKeyboard{VKey: 0x41, MakeCode: 0x1E}
	up := func(k rawKeyboard) rawKeyboard { k.Flags |= riKeyBreak; return k }
	press(a, 10)
	press(a, 500) // held: repeats
	press(a, 30)
	press(up(a), 30)
	press(a, 100)
	press(up(a), 100)
	press(rawKeyboard{VKey: 0xFF, MakeCode: 0x2A, Flags: riKeyE0}, 10)
	if n.Keys != 2 || keys[0x1E] != 2 || len(keys) != 1 {
		t.Errorf("keys = %d, %v", n.Keys, keys)
	}
	ctrl := rawKeyboard{VKey: 0x11, MakeCode: 0x1D}
	press(ctrl, 10)                                                    // left Ctrl
	press(rawKeyboard{VKey: 0x11, MakeCode: 0x1D, Flags: riKeyE0}, 10) // right Ctrl, the left one still held
	press(rawKeyboard{VKey: 0x13, MakeCode: 0x1D, Flags: riKeyE1}, 10) // Pause
	if keys[0x1D] != 1 || keys[0x11D] != 1 || keys[0x145] != 1 {
		t.Errorf("keys: %v", keys)
	}

	// Alt+Tab: Windows keeps the down of a hotkey's key from Raw Input, so
	// only the ups show, one per press of Tab.
	alt, tab := rawKeyboard{VKey: 0x12, MakeCode: 0x38}, rawKeyboard{VKey: 0x09, MakeCode: 0x0F}
	press(alt, 10)
	press(up(tab), 80)
	press(up(tab), 200)
	press(up(alt), 90)
	if keys[0x0F] != 2 || keys[0x38] != 1 {
		t.Errorf("Alt+Tab+Tab: %v", keys)
	}

	// The up was lost (Win+L): the next press counts, a long hold doesn't.
	win := rawKeyboard{VKey: 0x5B, MakeCode: 0x5B, Flags: riKeyE0}
	press(win, 10)
	press(win, 60000)
	press(up(win), 50)
	if keys[0x15B] != 2 {
		t.Errorf("a lost up: %v", keys)
	}
	press(a, 10)
	press(a, 1000) // the longest delay before a key repeats
	for i := 0; i < 50; i++ {
		press(a, 33)
	}
	press(up(a), 33)
	if keys[0x1E] != 3 {
		t.Errorf("a long hold: %v", keys)
	}
	at &^= 1 // an even tick: held is then one ahead of it
	press(a, 10)
	press(a, 500)
	press(a, 0) // two repeats on one tick
	press(up(a), 40)
	if keys[0x1E] != 4 {
		t.Errorf("repeats on one tick: %v", keys)
	}
	n = Input{}

	neg := int16(-240)
	s.mouse(&n, &rawMouse{ButtonFlags: 0x0001})                          // left down
	s.mouse(&n, &rawMouse{ButtonFlags: 0x0002})                          // left up
	s.mouse(&n, &rawMouse{ButtonFlags: 0x0004 | 0x0010})                 // right and middle down
	s.mouse(&n, &rawMouse{ButtonFlags: 0x0040})                          // button 4 down
	s.mouse(&n, &rawMouse{ButtonFlags: 0x0400, ButtonData: 120})         // wheel up
	s.mouse(&n, &rawMouse{ButtonFlags: 0x0400, ButtonData: uint16(neg)}) // wheel down, two notches
	s.mouse(&n, &rawMouse{ButtonFlags: 0x0800, ButtonData: 60})          // sideways
	s.mouse(&n, &rawMouse{ButtonData: 999})                              // no wheel flag
	if n.Left != 1 || n.Right != 1 || n.Middle != 1 || n.Other != 1 || n.Scroll != 420 {
		t.Errorf("mouse: %+v", n)
	}

	s.cursor(&n, 30, 40)
	s.cursor(&n, 30, 44)
	s.cursor(&n, 2000, 44) // placed, not moved
	s.cursor(&n, 2003, 48)
	if n.Move != 50+4+5 {
		t.Errorf("move = %d, want 59", n.Move)
	}

	// A game pins the cursor: only raw motion shows.
	n = Input{}
	s.cursor(&n, 500, 500)
	n = Input{}
	s.mouse(&n, &rawMouse{LastX: 300, LastY: 400})
	s.mouse(&n, &rawMouse{LastX: -300, LastY: -400})
	s.cursor(&n, 500, 500)
	if n.Move != 1000 {
		t.Errorf("pinned cursor: move = %d, want 1000", n.Move)
	}
	// Remote control: an absolute event says where, the cursor's step how far.
	n = Input{}
	s.mouse(&n, &rawMouse{Flags: 1, LastX: 65535, LastY: 65535})
	s.cursor(&n, 510, 500)
	if n.Move != 10 {
		t.Errorf("absolute event: move = %d, want 10", n.Move)
	}
	// On the desktop both show the same motion: it counts once.
	n = Input{}
	s.mouse(&n, &rawMouse{LastX: 6, LastY: 8})
	s.cursor(&n, 516, 508)
	if n.Move != 10 {
		t.Errorf("desktop move: %d, want 10", n.Move)
	}
	// Placed far away with no motion: nothing.
	n = Input{}
	s.cursor(&n, 2500, 500)
	if n.Move != 0 {
		t.Errorf("placed: move = %d", n.Move)
	}
}

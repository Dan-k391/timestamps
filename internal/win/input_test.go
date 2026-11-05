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
	s.key(&inputTotal, inputKeys, &rawKeyboard{VKey: 0x41, MakeCode: 0x1E})
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
	down := &rawKeyboard{VKey: 0x41, MakeCode: 0x1E}
	s.key(&n, keys, down)
	s.key(&n, keys, down) // held: repeats
	s.key(&n, keys, down)
	s.key(&n, keys, &rawKeyboard{VKey: 0x41, MakeCode: 0x1E, Flags: riKeyBreak})
	s.key(&n, keys, down)
	s.key(&n, keys, &rawKeyboard{VKey: 0xFF, MakeCode: 0x2A, Flags: riKeyE0})
	if n.Keys != 2 || keys[0x1E] != 2 || len(keys) != 1 {
		t.Errorf("keys = %d, %v", n.Keys, keys)
	}
	s.key(&n, keys, &rawKeyboard{VKey: 0x11, MakeCode: 0x1D})                 // left Ctrl
	s.key(&n, keys, &rawKeyboard{VKey: 0x11, MakeCode: 0x1D, Flags: riKeyE0}) // right Ctrl, the left one still held
	s.key(&n, keys, &rawKeyboard{VKey: 0x13, MakeCode: 0x1D, Flags: riKeyE1}) // Pause
	if keys[0x1D] != 1 || keys[0x11D] != 1 || keys[0x145] != 1 {
		t.Errorf("keys: %v", keys)
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
}

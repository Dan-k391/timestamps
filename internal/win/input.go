//go:build windows

package win

import (
	"log"
	"math"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Keyboard and mouse use comes from Raw Input: unlike a low-level hook it
// sits outside the input path, so it can't slow the mouse down and Windows
// doesn't quietly remove it.

// Input holds keyboard and mouse totals.
type Input struct {
	Keys, Left, Right, Middle, Other int64
	Move                             int64 // pixels
	Scroll                           int64 // wheel delta, 120 a notch
}

const (
	wmInput        = 0x00FF
	ridInput       = 0x10000003
	ridevRemove    = 0x00000001
	ridevInputSink = 0x00000100
	rimTypeMouse   = 0
	rimKeyboard    = 1
	riKeyBreak     = 1
	riKeyE0        = 2
	riKeyE1        = 4

	cursorSample  = 33   // ms between cursor samples: 30 a second, as a 1000 Hz mouse sends far more
	cursorMaxStep = 1000 // pixels: a longer step is the cursor being placed, not moved
)

type rawInputDevice struct {
	UsagePage, Usage uint16
	Flags            uint32
	Target           uintptr
}

type rawInput struct {
	Type, Size     uint32
	Device, WParam uintptr
	Data           [24]byte // a rawMouse or a rawKeyboard
}

type rawMouse struct {
	Flags, _                uint16
	ButtonFlags, ButtonData uint16
	RawButtons              uint32
	LastX, LastY            int32
	Extra                   uint32
}

type rawKeyboard struct {
	MakeCode, Flags, Reserved, VKey uint16
	Message, Extra                  uint32
}

var (
	inputMu     sync.Mutex
	inputTotal  Input
	inputKeys   = map[uint16]int64{} // by scan code, 0x100 marking the E0 ones
	inputThread uint32               // the running loop's thread, for CaptureInput alone
)

// TakeInput returns the totals, and the presses per key, since it last did.
func TakeInput() (Input, map[uint16]int64) {
	inputMu.Lock()
	defer inputMu.Unlock()
	n, keys := inputTotal, inputKeys
	inputTotal, inputKeys = Input{}, map[uint16]int64{}
	return n, keys
}

// CaptureInput starts or stops counting.
func CaptureInput(on bool) {
	if inputThread != 0 {
		pPostThreadMessageW.Call(uintptr(inputThread), 0x0012, 0, 0) // WM_QUIT
		inputThread = 0
	}
	if on {
		ready := make(chan uint32)
		go inputLoop(ready)
		inputThread = <-ready // 0 if it failed
	}
}

func inputLoop(ready chan<- uint32) {
	runtime.LockOSThread() // never unlocked: the thread and its window end with this function
	cls := utf16Ptr("Static")
	hwnd, _, _ := pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(cls)), 0, 0, 0, 0, 0, 0, ^uintptr(2), 0, 0, 0) // HWND_MESSAGE
	devs := [2]rawInputDevice{{1, 6, ridevInputSink, hwnd}, {1, 2, ridevInputSink, hwnd}}                        // keyboard, mouse
	if r, _, err := pRegisterRawInputDevices.Call(uintptr(unsafe.Pointer(&devs)), 2, unsafe.Sizeof(devs[0])); r == 0 {
		log.Print("counting keyboard and mouse use: ", err)
		ready <- 0
		return
	}
	ready <- windows.GetCurrentThreadId() // its queue exists now: WM_QUIT can't be lost
	var m MSG
	var s inputState
	for GetMessage(&m) {
		if m.Message == wmInput {
			s.read(m.LParam, m.Time)
		}
		TranslateDispatch(&m)
	}
	devs = [2]rawInputDevice{{1, 6, ridevRemove, 0}, {1, 2, ridevRemove, 0}}
	pRegisterRawInputDevices.Call(uintptr(unsafe.Pointer(&devs)), 2, unsafe.Sizeof(devs[0]))
}

type inputState struct {
	held    [512]bool // a held key repeats; it counts once
	sampled uint32    // when the cursor was last read
	x, y    int32
}

func (s *inputState) read(h uintptr, at uint32) {
	var raw rawInput
	size := uint32(unsafe.Sizeof(raw))
	if r, _, _ := pGetRawInputData.Call(h, ridInput, uintptr(unsafe.Pointer(&raw)), uintptr(unsafe.Pointer(&size)), unsafe.Offsetof(raw.Data)); int32(r) <= 0 {
		return
	}
	inputMu.Lock()
	defer inputMu.Unlock()
	switch raw.Type {
	case rimKeyboard:
		s.key(&inputTotal, inputKeys, (*rawKeyboard)(unsafe.Pointer(&raw.Data)))
	case rimTypeMouse:
		s.mouse(&inputTotal, (*rawMouse)(unsafe.Pointer(&raw.Data)))
		if at-s.sampled >= cursorSample {
			s.sampled = at
			var p POINT
			pGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
			s.cursor(&inputTotal, p.X, p.Y)
		}
	}
}

func (s *inputState) key(n *Input, keys map[uint16]int64, k *rawKeyboard) {
	if k.VKey == 0 || k.VKey >= 255 { // 255: a fake key, part of another key's scan codes
		return
	}
	id := k.MakeCode & 0xFF
	if k.Flags&riKeyE0 != 0 {
		id |= 0x100
	}
	if k.Flags&riKeyE1 != 0 {
		id = 0x145 // Pause
	}
	down := k.Flags&riKeyBreak == 0
	if down && !s.held[id] {
		n.Keys++
		keys[id]++
	}
	s.held[id] = down
}

func (s *inputState) mouse(n *Input, m *rawMouse) {
	f := m.ButtonFlags
	if f&0x0001 != 0 { // RI_MOUSE_LEFT_BUTTON_DOWN
		n.Left++
	}
	if f&0x0004 != 0 { // RI_MOUSE_RIGHT_BUTTON_DOWN
		n.Right++
	}
	if f&0x0010 != 0 { // RI_MOUSE_MIDDLE_BUTTON_DOWN
		n.Middle++
	}
	for _, b := range []uint16{0x0040, 0x0100} { // buttons 4 and 5
		if f&b != 0 {
			n.Other++
		}
	}
	if f&(0x0400|0x0800) != 0 { // RI_MOUSE_WHEEL, RI_MOUSE_HWHEEL
		d := int64(int16(m.ButtonData))
		if d < 0 {
			d = -d
		}
		n.Scroll += d
	}
}

func (s *inputState) cursor(n *Input, x, y int32) {
	d := math.Hypot(float64(x-s.x), float64(y-s.y))
	s.x, s.y = x, y
	if d <= cursorMaxStep {
		n.Move += int64(math.Round(d))
	}
}

package tracker

import (
	"image"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"timestamps/internal/mac"
	"timestamps/internal/store"
)

// The macOS side of the tracker. There are no window handles: the
// "window" passed around is the identity of the frontmost app's focused
// window, and what was seen of it is kept here.

var (
	frontMu sync.Mutex
	fronts  = map[uintptr]mac.Front{}
)

func foregroundWindow() uintptr {
	f, ok := mac.FrontApp()
	if !ok || f.PID <= 0 {
		return 0
	}
	h := uintptr(f.Window)
	frontMu.Lock()
	if len(fronts) > 16 {
		fronts = map[uintptr]mac.Front{}
	}
	fronts[h] = f
	frontMu.Unlock()
	return h
}

func front(h uintptr) (mac.Front, bool) {
	frontMu.Lock()
	defer frontMu.Unlock()
	f, ok := fronts[h]
	return f, ok
}

// The frontmost app is always on screen.
func windowVisible(h uintptr) bool { return true }

func windowTitle(h uintptr) string {
	f, _ := front(h)
	return f.Title
}

func idleMillis() uint32 {
	s := mac.IdleSeconds()
	if s < 0 {
		return 0
	}
	if s > 4e6 {
		s = 4e6
	}
	return uint32(s * 1000)
}

func screenLocked() bool { return mac.ScreenLocked() }

func captureInput(on bool) { mac.CaptureInput(on) }

// macScan gives the PC scan code (see web/js/keyboard.js) of a Mac key code.
var macScan = [128]uint16{
	0x00: 0x1E, 0x01: 0x1F, 0x02: 0x20, 0x03: 0x21, 0x04: 0x23, 0x05: 0x22, 0x06: 0x2C, 0x07: 0x2D, 0x08: 0x2E, 0x09: 0x2F,
	0x0B: 0x30, 0x0C: 0x10, 0x0D: 0x11, 0x0E: 0x12, 0x0F: 0x13, 0x10: 0x15, 0x11: 0x14, 0x12: 0x02, 0x13: 0x03, 0x14: 0x04,
	0x15: 0x05, 0x16: 0x07, 0x17: 0x06, 0x18: 0x0D, 0x19: 0x0A, 0x1A: 0x08, 0x1B: 0x0C, 0x1C: 0x09, 0x1D: 0x0B, 0x1E: 0x1B,
	0x1F: 0x18, 0x20: 0x16, 0x21: 0x1A, 0x22: 0x17, 0x23: 0x19, 0x24: 0x1C, 0x25: 0x26, 0x26: 0x24, 0x27: 0x28, 0x28: 0x25,
	0x29: 0x27, 0x2A: 0x2B, 0x2B: 0x33, 0x2C: 0x35, 0x2D: 0x31, 0x2E: 0x32, 0x2F: 0x34, 0x30: 0x0F, 0x31: 0x39, 0x32: 0x29,
	0x33: 0x0E, 0x35: 0x01, 0x36: 0x15C, 0x37: 0x15B, 0x38: 0x2A, 0x39: 0x3A, 0x3A: 0x38, 0x3B: 0x1D, 0x3C: 0x36, 0x3D: 0x138, 0x3E: 0x11D, 0x41: 0x53, 0x43: 0x37, 0x45: 0x4E, 0x47: 0x45, 0x4B: 0x135, 0x4C: 0x11C, 0x4E: 0x4A, 0x52: 0x52,
	0x53: 0x4F, 0x54: 0x50, 0x55: 0x51, 0x56: 0x4B, 0x57: 0x4C, 0x58: 0x4D, 0x59: 0x47, 0x5B: 0x48, 0x5C: 0x49, 0x60: 0x3F,
	0x61: 0x40, 0x62: 0x41, 0x63: 0x3D, 0x64: 0x42, 0x65: 0x43, 0x67: 0x57, 0x6D: 0x44, 0x6F: 0x58, 0x73: 0x147, 0x74: 0x149,
	0x75: 0x153, 0x76: 0x3E, 0x77: 0x14F, 0x78: 0x3C, 0x79: 0x151, 0x7A: 0x3B, 0x7B: 0x14B, 0x7C: 0x14D, 0x7D: 0x150, 0x7E: 0x148,
}

func takeInput() (store.Input, map[uint16]int64) {
	in, codes := mac.TakeInput()
	keys := map[uint16]int64{}
	for c, n := range codes {
		if n > 0 && macScan[c] != 0 {
			keys[macScan[c]] += int64(n)
		}
	}
	return store.Input(in), keys
}

var genericNames = map[string]bool{"electron": true, "java": true, "python": true, "python3": true, "node": true, "wine": true, "wine64-preloader": true}

func isGenericAppName(name string) bool {
	return genericNames[strings.ToLower(strings.TrimSpace(name))]
}

// appKey names an app the way exclusions and the browser list do: the
// bundle's file name in lower case ("google chrome"), as executable names
// can be shared (Visual Studio Code's is "Electron").
func appKey(f mac.Front) string {
	base := filepath.Base(f.Bundle)
	if strings.HasSuffix(strings.ToLower(base), ".app") {
		base = base[:len(base)-4]
	}
	if base == "." || base == "/" {
		return ""
	}
	return strings.ToLower(base)
}

func (t *Tracker) procInfo(h uintptr) (uint32, proc) {
	f, _ := front(h)
	pid := uint32(f.PID)
	if p, ok := t.procs[pid]; ok && time.Since(p.at) < 30*time.Second {
		return pid, p
	}
	p := proc{path: f.Bundle, exe: appKey(f), name: f.Name, at: time.Now()}
	if p.name == "" || isGenericAppName(p.name) {
		p.name = strings.TrimSuffix(filepath.Base(f.Bundle), ".app")
	}
	if len(t.procs) > 512 {
		t.procs = map[uint32]proc{}
	}
	t.procs[pid] = p
	return pid, p
}

// appIcon renders the app's icon as the Finder shows it.
func appIcon(path string, framework bool, h uintptr) []byte {
	return mac.AppIconPNG(path, 64)
}

func (t *Tracker) audioLoop() {
	tk := time.NewTicker(time.Second)
	defer tk.Stop()
	for {
		select {
		case <-t.stop:
			return
		case <-tk.C:
		}
		if !t.settings.Load().TreatSoundAsActive || idleMillis() < 15000 {
			continue
		}
		if mac.SoundPlaying() {
			t.lastSound.Store(time.Now().UnixMilli())
		}
	}
}

// grabScreen captures the display showing the window fg, or all displays.
func grabScreen(fg uintptr, all bool) (*image.RGBA, error) {
	f, ok := front(fg)
	ok = ok && f.W > 0
	return mac.Capture(f.X+f.W/2, f.Y+f.H/2, ok, all)
}

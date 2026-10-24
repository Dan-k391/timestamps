//go:build windows

package win

import (
	"encoding/binary"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ForegroundWindow returns the window the user is currently working in.
func ForegroundWindow() uintptr {
	h, _, _ := pGetForegroundWindow.Call()
	return h
}

// WindowText returns a window's title.
func WindowText(hwnd uintptr) string {
	n, _, _ := pGetWindowTextLengthW.Call(hwnd)
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	r, _, _ := pGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), n+1)
	return windows.UTF16ToString(buf[:r])
}

// ClassName returns a window's class name.
func ClassName(hwnd uintptr) string {
	var buf [256]uint16
	r, _, _ := pGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), 256)
	return windows.UTF16ToString(buf[:r])
}

// WindowPID returns the id of the process owning a window.
func WindowPID(hwnd uintptr) uint32 {
	var pid uint32
	pGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return pid
}

// IsVisible reports whether a window is shown.
func IsVisible(hwnd uintptr) bool {
	r, _, _ := pIsWindowVisible.Call(hwnd)
	return r != 0
}

// IsWindow reports whether hwnd still identifies a window.
func IsWindow(hwnd uintptr) bool {
	r, _, _ := pIsWindow.Call(hwnd)
	return r != 0
}

const processQueryLimited = 0x1000

// ProcessPath returns the full image path of a process.
func ProcessPath(pid uint32) string {
	h, err := windows.OpenProcess(processQueryLimited, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, 1024)
	n := uint32(len(buf))
	r, _, _ := pQueryFullProcessImageNameW.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)))
	if r == 0 {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

// ProcessCommandLine reads a process's command line (Windows 8.1+).
func ProcessCommandLine(pid uint32) string {
	h, err := windows.OpenProcess(processQueryLimited, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]byte, 8192)
	var ret uint32
	for i := 0; i < 2; i++ {
		st, _, _ := pNtQueryInformationProcess.Call(uintptr(h), 60, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&ret)))
		if st == 0 {
			break
		}
		if ret > uint32(len(buf)) && i == 0 {
			buf = make([]byte, ret)
			continue
		}
		return ""
	}
	// UNICODE_STRING { USHORT Length; USHORT MaximumLength; PWSTR Buffer } followed by the text.
	length := int(binary.LittleEndian.Uint16(buf[0:2]))
	ptr := uintptr(binary.LittleEndian.Uint64(buf[8:16]))
	base := uintptr(unsafe.Pointer(&buf[0]))
	off := int(ptr - base)
	if off < 16 || off+length > len(buf) {
		return ""
	}
	u := make([]uint16, length/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(buf[off+i*2:])
	}
	return windows.UTF16ToString(u)
}

// SplitCommandLine splits a command line into arguments like the shell does.
func SplitCommandLine(cmd string) []string {
	if cmd == "" {
		return nil
	}
	p := utf16Ptr(cmd)
	var argc int32
	r, _, _ := pCommandLineToArgvW.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&argc)))
	if r == 0 {
		return nil
	}
	defer windows.LocalFree(windows.Handle(r))
	out := make([]string, 0, argc)
	argv := unsafe.Slice((**uint16)(unsafe.Pointer(r)), argc)
	for _, a := range argv {
		out = append(out, windows.UTF16PtrToString(a))
	}
	return out
}

var (
	enumMu     sync.Mutex
	enumHost   uint32
	enumFound  uintptr
	enumFoundP uint32
	enumCB     = windows.NewCallback(func(hwnd, _ uintptr) uintptr {
		pid := WindowPID(hwnd)
		if pid != 0 && pid != enumHost {
			enumFound, enumFoundP = hwnd, pid
			return 0
		}
		return 1
	})
)

// ResolveUWP finds the real app behind an ApplicationFrameHost window.
func ResolveUWP(hwnd uintptr, hostPID uint32) (uintptr, uint32) {
	enumMu.Lock()
	defer enumMu.Unlock()
	enumHost, enumFound, enumFoundP = hostPID, 0, 0
	pEnumChildWindows.Call(hwnd, enumCB, 0)
	return enumFound, enumFoundP
}

// IdleMillis returns how long ago the last keyboard or mouse input happened.
func IdleMillis() uint32 {
	var lii struct{ cbSize, dwTime uint32 }
	lii.cbSize = 8
	if r, _, _ := pGetLastInputInfo.Call(uintptr(unsafe.Pointer(&lii))); r == 0 {
		return 0
	}
	now, _, _ := pGetTickCount.Call()
	return uint32(now) - lii.dwTime
}

// IsLocked reports whether the interactive desktop is not the user's
// default desktop (lock screen, UAC prompt, sign-in screen).
func IsLocked() bool {
	h, _, _ := pOpenInputDesktop.Call(0, 0, 0x0001)
	if h == 0 {
		return true
	}
	defer pCloseDesktop.Call(h)
	var name [128]uint16
	var need uint32
	r, _, _ := pGetUserObjectInformationW.Call(h, 2, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)*2), uintptr(unsafe.Pointer(&need)))
	if r == 0 {
		return false
	}
	return !strings.EqualFold(windows.UTF16ToString(name[:]), "Default")
}

// FileDescription reads the FileDescription (falling back to ProductName)
// from an executable's version resource.
func FileDescription(path string) string {
	p := utf16Ptr(path)
	size, _, _ := pGetFileVersionInfoSizeW.Call(uintptr(unsafe.Pointer(p)), 0)
	if size == 0 {
		return ""
	}
	buf := make([]byte, size)
	if r, _, _ := pGetFileVersionInfoW.Call(uintptr(unsafe.Pointer(p)), 0, size, uintptr(unsafe.Pointer(&buf[0]))); r == 0 {
		return ""
	}
	query := func(sub string) ([]byte, bool) {
		q := utf16Ptr(sub)
		var ptr uintptr
		var n uint32
		r, _, _ := pVerQueryValueW.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(q)), uintptr(unsafe.Pointer(&ptr)), uintptr(unsafe.Pointer(&n)))
		if r == 0 || ptr == 0 {
			return nil, false
		}
		off := int(ptr - uintptr(unsafe.Pointer(&buf[0])))
		if off < 0 || off >= len(buf) {
			return nil, false
		}
		return buf[off:], true
	}
	var trans []string
	if t, ok := query(`\VarFileInfo\Translation`); ok && len(t) >= 4 {
		trans = append(trans, fmt.Sprintf("%04x%04x", binary.LittleEndian.Uint16(t[0:]), binary.LittleEndian.Uint16(t[2:])))
	}
	trans = append(trans, "040904b0", "040904e4", "080404b0", "000004b0")
	for _, field := range []string{"FileDescription", "ProductName"} {
		for _, tr := range trans {
			b, ok := query(`\StringFileInfo\` + tr + `\` + field)
			if !ok {
				continue
			}
			var u []uint16
			for i := 0; i+1 < len(b); i += 2 {
				c := binary.LittleEndian.Uint16(b[i:])
				if c == 0 {
					break
				}
				u = append(u, c)
			}
			if s := strings.TrimSpace(windows.UTF16ToString(u)); s != "" {
				return s
			}
		}
	}
	return ""
}

// AppDisplayName picks a friendly name for an executable.
func AppDisplayName(path string) string {
	if d := FileDescription(path); d != "" && !strings.EqualFold(d, "Application Frame Host") {
		return d
	}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if base == "" {
		return "Unknown"
	}
	return strings.ToUpper(base[:1]) + base[1:]
}

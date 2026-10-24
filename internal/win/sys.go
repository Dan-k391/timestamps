//go:build windows

package win

import (
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

// SetAutostart adds or removes the app from the per-user Run key.
func SetAutostart(name, command string, on bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if on {
		return k.SetStringValue(name, command)
	}
	if err := k.DeleteValue(name); err != nil && err != registry.ErrNotExist {
		return err
	}
	return nil
}

// Autostart reports whether the Run key entry exists.
func Autostart(name string) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(name)
	return err == nil
}

// SingleInstance acquires a named mutex; false means another copy runs.
func SingleInstance(name string) bool {
	p := utf16Ptr(name)
	_, err := windows.CreateMutex(nil, false, p)
	return err != windows.ERROR_ALREADY_EXISTS
}

type openFileName struct {
	StructSize    uint32
	Owner         uintptr
	Instance      uintptr
	Filter        *uint16
	CustomFilter  *uint16
	MaxCustFilter uint32
	FilterIndex   uint32
	File          *uint16
	MaxFile       uint32
	FileTitle     *uint16
	MaxFileTitle  uint32
	InitialDir    *uint16
	Title         *uint16
	Flags         uint32
	FileOffset    uint16
	FileExtension uint16
	DefExt        *uint16
	CustData      uintptr
	FnHook        uintptr
	TemplateName  *uint16
	PvReserved    uintptr
	DwReserved    uint32
	FlagsEx       uint32
}

// OpenFileDialog shows the standard open dialog. filter uses the Win32
// "Name\x00*.ext\x00" format.
func OpenFileDialog(owner uintptr, title, filter, initialDir string) string {
	buf := make([]uint16, 1024)
	f, _ := windows.UTF16FromString(filter)
	f = append(f, 0)
	ofn := openFileName{
		Owner:   owner,
		Filter:  &f[0],
		File:    &buf[0],
		MaxFile: uint32(len(buf)),
		Title:   utf16Ptr(title),
		Flags:   0x00001000 | 0x00000800 | 0x00080000, // FILEMUSTEXIST | PATHMUSTEXIST | EXPLORER
	}
	if initialDir != "" {
		ofn.InitialDir = utf16Ptr(initialDir)
	}
	ofn.StructSize = uint32(unsafe.Sizeof(ofn))
	if r, _, _ := pGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn))); r == 0 {
		return ""
	}
	return windows.UTF16ToString(buf)
}

// SystemDarkMode reports whether Windows apps use the dark theme.
func SystemDarkMode() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("AppsUseLightTheme")
	return err == nil && v == 0
}

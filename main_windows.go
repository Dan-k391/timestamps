package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/windows"

	"timestamps/internal/win"
)

const dataDirHelp = `%LOCALAPPDATA%\Timestamps`

func platformInit() { win.SetDPIAware() }

func defaultDataDir() string { return filepath.Join(os.Getenv("LOCALAPPDATA"), "Timestamps") }

// singleInstance takes a mutex named after the data folder; false means
// another copy already uses that folder.
func singleInstance(dataDir string) bool {
	return win.SingleInstance(`Local\Timestamps-` + strconv.FormatUint(uint64(hashString(strings.ToLower(dataDir))), 16))
}

func alert(title, text string) { win.MessageBox(title, text) }

// redirectedDataDir returns where files written to dir really go when this
// process runs inside another app's MSIX container (Windows then redirects
// AppData writes into that app's package folder, and ends the process with
// that app), or "" when they stay in dir. A new file's final path shows it;
// folders still look normal.
func redirectedDataDir(dir string) string {
	p := filepath.Join(dir, ".where-"+strconv.Itoa(os.Getpid()))
	f, err := os.Create(p)
	if err != nil {
		return ""
	}
	defer os.Remove(p)
	defer f.Close()
	buf := make([]uint16, 1024)
	n, err := windows.GetFinalPathNameByHandle(windows.Handle(f.Fd()), &buf[0], uint32(len(buf)), 0)
	if err != nil || n == 0 || int(n) >= len(buf) {
		return ""
	}
	real := filepath.Dir(strings.TrimPrefix(windows.UTF16ToString(buf[:n]), `\\?\`))
	l := strings.ToLower(real)
	if !strings.Contains(l, `\packages\`) || !strings.Contains(l, `\localcache\`) || strings.Contains(strings.ToLower(dir), `\localcache\`) {
		return ""
	}
	return real
}

// redirectStderr makes f the process's standard error, where the Go runtime
// writes crash reports (it looks the handle up each time).
func redirectStderr(f *os.File) {
	windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(f.Fd()))
	os.Stderr = f
}

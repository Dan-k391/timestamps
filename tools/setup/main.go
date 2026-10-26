//go:build windows && setup

// Command setup installs Timestamps for the current user, without admin
// rights: the program in %LOCALAPPDATA%\Programs\Timestamps, a Start menu
// shortcut (and one on the desktop if wanted), and an entry in Apps &
// features whose uninstaller keeps the tracked data.
//
//	setup.exe            asks, installs and starts Timestamps
//	setup.exe /S         installs without questions
//	setup.exe /update    the same, run by the app to update itself
//	uninstall.exe /uninstall
//
// Build it with build-setup.ps1, which puts the app in payload/.
package main

import (
	_ "embed"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

//go:embed payload/timestamps.exe
var payload []byte

var version = "dev" // set with -ldflags "-X main.version=..."

const (
	appName      = "Timestamps"
	exeName      = "Timestamps.exe"
	uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\Timestamps`
	runKey       = `Software\Microsoft\Windows\CurrentVersion\Run`
)

var (
	user32         = windows.NewLazySystemDLL("user32.dll")
	pMessageBoxW   = user32.NewProc("MessageBoxW")
	pFindWindowW   = user32.NewProc("FindWindowW")
	pGetUILanguage = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage")
)

func main() {
	args := strings.Join(os.Args[1:], " ")
	switch {
	case strings.Contains(args, "/remove"): // the copy in %TEMP% that does the uninstalling
		remove(os.Args[len(os.Args)-1])
	case strings.Contains(args, "/uninstall"):
		uninstall()
	default:
		install(strings.Contains(args, "/S") || strings.Contains(args, "/update"))
	}
}

func installDir() string { return filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", appName) }

// ---- install ----------------------------------------------------------------

func install(quiet bool) {
	dir := installDir()
	if !quiet && !ask(tr("install", version, dir)) {
		return
	}
	desktop := !quiet && ask(tr("desktop"))
	stopRunning()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fail(err)
		return
	}
	exe := filepath.Join(dir, exeName)
	if err := writeFile(exe, payload); err != nil {
		fail(err)
		return
	}
	if self, err := os.Executable(); err == nil {
		if b, err := os.ReadFile(self); err == nil {
			writeFile(filepath.Join(dir, "uninstall.exe"), b)
		}
	}
	ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
	if p, err := windows.KnownFolderPath(windows.FOLDERID_Programs, 0); err == nil {
		shortcut(filepath.Join(p, appName+".lnk"), exe, dir)
	}
	if desktop {
		if p, err := windows.KnownFolderPath(windows.FOLDERID_Desktop, 0); err == nil {
			shortcut(filepath.Join(p, appName+".lnk"), exe, dir)
		}
	}
	register(dir, exe)
	start(exe)
	if !quiet {
		tell(tr("installed"))
	}
}

// start runs the installed app through Explorer, so it doesn't inherit the
// setup's job or container: the setup may itself have been started by an
// app whose children Windows confines (an MSIX app's container redirects
// their AppData and ends them with it).
func start(exe string) {
	if exec.Command("explorer.exe", exe).Start() != nil {
		exec.Command(exe).Start()
	}
}

// writeFile replaces a file, retrying while the old program is closing.
func writeFile(path string, b []byte) error {
	tmp := path + ".new"
	if err := os.WriteFile(tmp, b, 0o755); err != nil {
		return err
	}
	var err error
	for i := 0; i < 40; i++ {
		if err = os.Rename(tmp, path); err == nil {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	os.Remove(tmp)
	return err
}

// stopRunning asks a running Timestamps to exit and waits for it.
func stopRunning() {
	c := &http.Client{Timeout: 2 * time.Second}
	for _, port := range []int{47613, 47614, 47615, 47616} {
		req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/api/internal/quit", port), nil)
		req.Header.Set("X-Timestamps-Instance", "1")
		if resp, err := c.Do(req); err == nil {
			resp.Body.Close()
		}
	}
	cls, _ := windows.UTF16PtrFromString("TimestampsShell")
	for i := 0; i < 60; i++ {
		if h, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(cls)), 0); h == 0 {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)
}

// register adds the Apps & features entry.
func register(dir, exe string) {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, uninstallKey, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	k.SetStringValue("DisplayName", appName)
	k.SetStringValue("DisplayVersion", version)
	k.SetStringValue("Publisher", "ZJJ-2785")
	k.SetStringValue("DisplayIcon", exe)
	k.SetStringValue("InstallLocation", dir)
	k.SetStringValue("URLInfoAbout", "https://github.com/ZJJ-2785/timestamps")
	k.SetStringValue("UninstallString", `"`+filepath.Join(dir, "uninstall.exe")+`" /uninstall`)
	k.SetStringValue("QuietUninstallString", `"`+filepath.Join(dir, "uninstall.exe")+`" /uninstall /S`)
	k.SetDWordValue("NoModify", 1)
	k.SetDWordValue("NoRepair", 1)
	k.SetDWordValue("EstimatedSize", uint32(len(payload)/1024))
}

// ---- uninstall --------------------------------------------------------------

// uninstall runs from the install folder, so it hands the work to a copy of
// itself in %TEMP% that can delete the folder.
func uninstall() {
	quiet := strings.Contains(strings.Join(os.Args[1:], " "), "/S")
	if !quiet && !ask(tr("uninstall", filepath.Join(os.Getenv("LOCALAPPDATA"), appName))) {
		return
	}
	self, err := os.Executable()
	if err != nil {
		fail(err)
		return
	}
	b, err := os.ReadFile(self)
	if err != nil {
		fail(err)
		return
	}
	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("timestamps-uninstall-%d.exe", os.Getpid()))
	if err := os.WriteFile(tmp, b, 0o755); err != nil {
		fail(err)
		return
	}
	mode := "/remove"
	if quiet {
		mode = "/remove /S"
	}
	exec.Command(tmp, append(strings.Fields(mode), filepath.Dir(self))...).Start()
}

func remove(dir string) {
	time.Sleep(time.Second) // let uninstall.exe exit
	stopRunning()
	for _, f := range []*windows.KNOWNFOLDERID{windows.FOLDERID_Programs, windows.FOLDERID_Desktop} {
		if p, err := windows.KnownFolderPath(f, 0); err == nil {
			os.Remove(filepath.Join(p, appName+".lnk"))
		}
	}
	if k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE); err == nil {
		k.DeleteValue(appName)
		k.Close()
	}
	registry.DeleteKey(registry.CURRENT_USER, uninstallKey)
	if strings.EqualFold(filepath.Base(dir), appName) { // never delete anything else
		for i := 0; i < 20 && os.RemoveAll(dir) != nil; i++ {
			time.Sleep(250 * time.Millisecond)
		}
	}
	if !strings.Contains(strings.Join(os.Args[1:], " "), "/S") {
		tell(tr("uninstalled"))
	}
	// Delete this temporary copy once it has exited.
	self, _ := os.Executable()
	cmd := exec.Command("cmd")
	// The raw command line: cmd doesn't read Go's escaped quotes.
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000, // CREATE_NO_WINDOW
		CmdLine: `cmd /c ping -n 3 127.0.0.1 >nul & del "` + self + `"`}
	cmd.Start()
}

// ---- shortcuts ----------------------------------------------------------------

var (
	clsidShellLink  = ole.NewGUID("{00021401-0000-0000-C000-000000000046}")
	iidIShellLinkW  = ole.NewGUID("{000214F9-0000-0000-C000-000000000046}")
	iidIPersistFile = ole.NewGUID("{0000010B-0000-0000-C000-000000000046}")
)

func vcall(obj uintptr, idx int, args ...uintptr) uintptr {
	vtbl := *(*uintptr)(unsafe.Pointer(obj))
	fn := *(*uintptr)(unsafe.Pointer(vtbl + uintptr(idx)*unsafe.Sizeof(uintptr(0))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{obj}, args...)...)
	return r
}

// shortcut writes a .lnk to target.
func shortcut(lnk, target, workdir string) {
	unk, err := ole.CreateInstance(clsidShellLink, iidIShellLinkW)
	if err != nil {
		return
	}
	link := uintptr(unsafe.Pointer(unk))
	defer vcall(link, 2)
	t, _ := windows.UTF16PtrFromString(target)
	w, _ := windows.UTF16PtrFromString(workdir)
	d, _ := windows.UTF16PtrFromString(appName)
	vcall(link, 20, uintptr(unsafe.Pointer(t))) // SetPath
	vcall(link, 9, uintptr(unsafe.Pointer(w)))  // SetWorkingDirectory
	vcall(link, 7, uintptr(unsafe.Pointer(d)))  // SetDescription
	pf := new(uintptr)
	if vcall(link, 0, uintptr(unsafe.Pointer(iidIPersistFile)), uintptr(unsafe.Pointer(pf))) != 0 || *pf == 0 {
		return
	}
	defer vcall(*pf, 2)
	p, _ := windows.UTF16PtrFromString(lnk)
	os.MkdirAll(filepath.Dir(lnk), 0o755)
	vcall(*pf, 6, uintptr(unsafe.Pointer(p)), 1) // Save
}

// ---- messages -----------------------------------------------------------------

func chinese() bool {
	r, _, _ := pGetUILanguage.Call()
	return r&0x3ff == 0x04 // LANG_CHINESE
}

var texts = map[string][2]string{
	"install":     {"Install Timestamps %s?\n\nIt is installed for you only, in\n%s\n\nYour tracked data stays as it is.", "安装 Timestamps %s？\n\n只为当前用户安装，位置：\n%s\n\n已有的跟踪数据保持不变。"},
	"desktop":     {"Also put a shortcut on the desktop?", "同时在桌面上创建快捷方式？"},
	"installed":   {"Timestamps is installed and running.\n\nYou'll find it in the Start menu and in the notification area.", "Timestamps 已安装并正在运行。\n\n可以在开始菜单和通知区域中找到它。"},
	"uninstall":   {"Remove Timestamps?\n\nYour tracked data in %s is kept.", "卸载 Timestamps？\n\n%s 中的跟踪数据会保留。"},
	"uninstalled": {"Timestamps has been removed.", "Timestamps 已卸载。"},
	"failed":      {"Timestamps couldn't be installed:\n%v", "无法安装 Timestamps：\n%v"},
}

func tr(key string, args ...any) string {
	t := texts[key][0]
	if chinese() {
		t = texts[key][1]
	}
	if len(args) > 0 {
		return fmt.Sprintf(t, args...)
	}
	return t
}

func box(text string, flags uintptr) uintptr {
	t, _ := windows.UTF16PtrFromString(text)
	c, _ := windows.UTF16PtrFromString(appName + " " + version)
	r, _, _ := pMessageBoxW.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), flags)
	return r
}

func ask(text string) bool { return box(text, 0x04|0x20|0x10000) == 6 } // MB_YESNO | MB_ICONQUESTION | MB_SETFOREGROUND; IDYES
func tell(text string)     { box(text, 0x40|0x10000) }                  // MB_ICONINFORMATION
func fail(err error)       { box(tr("failed", err), 0x10|0x10000) }     // MB_ICONERROR

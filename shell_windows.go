//go:build windows

package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	ole "github.com/go-ole/go-ole"
	webview2 "github.com/jchv/go-webview2"

	"timestamps/internal/store"
	"timestamps/internal/win"
)

type menuItem = win.MenuItem

// native is the Windows shell: notification area icon and WebView2 window.
type native struct {
	shell   *win.Shell
	view    webview2.WebView
	hwnd    uintptr
	iconBig uintptr
	iconSm  uintptr
}

// platformText overrides shellText where Windows needs its own words.
var platformText = map[string]map[string]string{}

func systemChinese() bool { return win.UILanguage() == 0x04 }

func (a *App) notify(title, text string) { a.shell.Notify(title, text) }
func (a *App) setTip(tip string)         { a.shell.SetTip(tip) }

func (a *App) run(showWindow bool) {
	a.iconBig = win.LoadAppIcon(win.SystemMetric(11))
	a.iconSm = win.LoadAppIcon(win.SystemMetric(49))
	a.shell = win.NewShell("Timestamps", a.iconSm)
	a.shell.OnLeftClick = func() { a.openWindow("") }
	a.shell.OnBalloon = func() { a.openWindow("day?away=1") }
	a.shell.OnMenu = a.menu
	a.shell.OnCommand = a.command
	a.shell.OnSuspend = a.t.Suspend
	a.shell.OnEndSession = func() {
		a.t.Stop()
		a.db.Close()
	}
	go a.watch()
	go a.tipLoop()
	if showWindow {
		a.shell.Invoke(func() { a.openWindow("") })
	}
	var m win.MSG
	for win.GetMessage(&m) {
		win.TranslateDispatch(&m)
	}
	a.shell.Remove()
}

// Show opens (or focuses) the app window at a route.
func (a *App) Show(route string) {
	a.shell.Invoke(func() { a.openWindow(route) })
}

func (a *App) openWindow(route string) {
	if a.view != nil {
		if route != "" {
			b, _ := json.Marshal(route)
			a.view.Eval("window.__navigate && window.__navigate(" + string(b) + ")")
		}
		win.ShowWindowFront(a.hwnd)
		return
	}
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath:  filepath.Join(a.dataDir, "webview"),
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title: "Timestamps", Width: 1440, Height: 900, IconId: 1, Center: true,
		},
	})
	if w == nil {
		// No WebView2 runtime: fall back to the default browser.
		win.ShellOpen(a.srv.URL() + "#/" + route)
		return
	}
	a.view = w
	a.hwnd = uintptr(w.Window())
	dpi := win.WindowDPI(a.hwnd)
	win.PlaceWindow(a.hwnd, 1440*dpi/96, 900*dpi/96, false)
	w.SetSize(960*dpi/96, 620*dpi/96, webview2.HintMin)
	win.SetWindowIcon(a.hwnd, a.iconBig, a.iconSm)
	win.SetDarkTitleBar(a.hwnd, a.prefersDark())
	w.Bind("tsSetDark", func(dark bool) { win.SetDarkTitleBar(a.hwnd, dark) })
	w.Bind("tsQuit", func() { a.quit() })
	w.Init("window.__desktop = true;")
	w.Navigate(a.srv.URL() + "#/" + strings.TrimPrefix(route, "/"))
	w.Run() // nested message loop until the window is closed
	a.view, a.hwnd = nil, 0
	if a.exiting {
		win.PostQuit()
		return
	}
	if !a.t.Settings().CloseToTray {
		a.quit()
		return
	}
	if a.db.Meta("ui.trayHint") == "" {
		a.db.SetMeta("ui.trayHint", "1")
		a.shell.Notify("Timestamps", a.Text("stillRunning"))
	}
}

func (a *App) prefersDark() bool {
	switch a.t.Settings().Theme {
	case "dark":
		return true
	case "light":
		return false
	}
	return win.SystemDarkMode()
}

func (a *App) quit() {
	a.shell.Invoke(func() {
		a.exiting = true
		if a.hwnd != 0 {
			win.PostMessage(a.hwnd, win.WM_CLOSE, 0, 0)
			return
		}
		win.PostQuit()
	})
}

// Open opens a file, folder or URL with the default handler.
func (a *App) Open(target string) { win.ShellOpen(target) }

// Reveal shows a file in Explorer.
func (a *App) Reveal(path string) { win.RevealInExplorer(path) }

// PickFile shows a native open dialog.
func (a *App) PickFile(title, filter string) string {
	if filter == "" {
		filter = "ManicTime database (ManicTimeReports.db)\x00*.db\x00All files\x00*.*\x00"
	}
	res := make(chan string, 1)
	go func() {
		runtime.LockOSThread()
		ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
		defer ole.CoUninitialize()
		dir := filepath.Join(os.Getenv("LOCALAPPDATA"), `Finkit\ManicTime`)
		res <- win.OpenFileDialog(a.hwnd, title, filter, dir)
	}()
	return <-res
}

// PickFolder shows the native folder picker, starting in dir.
func (a *App) PickFolder(title, dir string) string {
	res := make(chan string, 1)
	go func() {
		runtime.LockOSThread()
		ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
		defer ole.CoUninitialize()
		res <- win.PickFolderDialog(a.hwnd, title, dir)
	}()
	return <-res
}

// ApplySettings reacts to settings changes that touch the OS.
func (a *App) ApplySettings(old, cur store.Settings) {
	if old.RunAtStartup != cur.RunAtStartup || (cur.RunAtStartup && !win.Autostart("Timestamps")) {
		exe, _ := os.Executable()
		if err := win.SetAutostart("Timestamps", `"`+exe+`" --minimized`, cur.RunAtStartup); err != nil {
			log.Println("autostart:", err)
		}
	}
	if old.Theme != cur.Theme && a.hwnd != 0 {
		dark := a.prefersDark()
		a.shell.Invoke(func() { win.SetDarkTitleBar(a.hwnd, dark) })
	}
}

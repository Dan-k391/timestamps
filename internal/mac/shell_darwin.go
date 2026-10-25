package mac

/*
#include "mac.h"
*/
import "C"

import (
	"encoding/json"
	"strings"
	"unsafe"
)

// MenuItem is an entry of the menu bar icon's menu.
type MenuItem struct {
	ID        int        `json:"id,omitempty"`
	Text      string     `json:"text,omitempty"`
	Separator bool       `json:"sep,omitempty"`
	Disabled  bool       `json:"disabled,omitempty"`
	Checked   bool       `json:"checked,omitempty"`
	Sub       []MenuItem `json:"sub,omitempty"`
}

// Shell is the application: menu bar icon, app window and application
// menu. The callbacks run on the main thread.
type Shell struct {
	OnMenu         func() []MenuItem
	OnCommand      func(id int)
	OnOpen         func(route string) // the Dock icon, Settings…, a notification
	OnLaunched     func()
	OnWindowClosed func()
	OnSuspend      func()
	OnTerminate    func() // the app quits right after it returns
}

var shell = &Shell{}

// Prepare connects to the window server early, so app details are current
// before the shell runs. Call it on the main thread.
func Prepare() { C.tsAppPrepare() }

// NewShell sets up the application; labels names the items of the
// application menu. Call it on the main thread.
func NewShell(labels map[string]string) *Shell {
	b, _ := json.Marshal(labels)
	cs := C.CString(string(b))
	defer C.free(unsafe.Pointer(cs))
	C.tsAppInit(cs)
	return shell
}

// Run runs the application on the main thread until it quits.
func (s *Shell) Run() { C.tsAppRun() }

func cstr(s string) (*C.char, func()) {
	cs := C.CString(s)
	return cs, func() { C.free(unsafe.Pointer(cs)) }
}

// SetTip sets the menu bar icon's tooltip.
func (s *Shell) SetTip(tip string) {
	cs, free := cstr(tip)
	defer free()
	C.tsSetTip(cs)
}

// Notify shows a notification; clicking it opens the window at route.
func (s *Shell) Notify(title, text, route string) {
	t, f1 := cstr(title)
	defer f1()
	x, f2 := cstr(text)
	defer f2()
	r, f3 := cstr(route)
	defer f3()
	C.tsNotify(t, x, r)
}

// ShowWindow opens the window at url, or brings it forward and runs script
// in it when it is already open.
func (s *Shell) ShowWindow(url, script string) {
	u, f1 := cstr(url)
	defer f1()
	j, f2 := cstr(script)
	defer f2()
	C.tsShowWindow(u, j)
}

// Appearances of the window.
const (
	AppearanceSystem = 0
	AppearanceLight  = 1
	AppearanceDark   = 2
)

// SetAppearance makes the window light, dark or follow the system.
func (s *Shell) SetAppearance(mode int) { C.tsSetAppearance(C.int(mode)) }

// Quit ends the app (OnTerminate runs first).
func (s *Shell) Quit() { C.tsQuit() }

// PickFile shows an open panel for files with these extensions (all files
// when there are none) and returns the chosen path, or "".
func PickFile(title string, exts []string, dir string) string {
	t, f1 := cstr(title)
	defer f1()
	e, f2 := cstr(strings.Join(exts, ","))
	defer f2()
	d, f3 := cstr(dir)
	defer f3()
	return goString(C.tsPickFile(t, e, d))
}

// PickFolder shows an open panel for a folder, starting in dir.
func PickFolder(title, dir string) string {
	t, f1 := cstr(title)
	defer f1()
	d, f2 := cstr(dir)
	defer f2()
	return goString(C.tsPickFolder(t, d))
}

// Open opens a file, folder or web address with its default app.
func Open(target string) {
	cs, free := cstr(target)
	defer free()
	C.tsOpen(cs)
}

// Reveal shows a file in the Finder.
func Reveal(path string) {
	cs, free := cstr(path)
	defer free()
	C.tsReveal(cs)
}

// Alert shows a message and waits for OK.
func Alert(title, text string) {
	t, f1 := cstr(title)
	defer f1()
	x, f2 := cstr(text)
	defer f2()
	C.tsAlert(t, x)
}

// Ask shows a question; true means the ok button.
func Ask(title, text, ok, cancel string) bool {
	t, f1 := cstr(title)
	defer f1()
	x, f2 := cstr(text)
	defer f2()
	o, f3 := cstr(ok)
	defer f3()
	c, f4 := cstr(cancel)
	defer f4()
	return bool(C.tsAsk(t, x, o, c))
}

// ---- called from Objective-C ----

//export tsGoMenu
func tsGoMenu() *C.char {
	items := []MenuItem{}
	if shell.OnMenu != nil {
		items = shell.OnMenu()
	}
	if items == nil {
		items = []MenuItem{}
	}
	b, _ := json.Marshal(items)
	return C.CString(string(b))
}

//export tsGoCommand
func tsGoCommand(id C.int) {
	if shell.OnCommand != nil {
		shell.OnCommand(int(id))
	}
}

//export tsGoOpen
func tsGoOpen(route *C.char) {
	if shell.OnOpen != nil {
		shell.OnOpen(C.GoString(route))
	}
}

//export tsGoLaunched
func tsGoLaunched() {
	if shell.OnLaunched != nil {
		shell.OnLaunched()
	}
}

//export tsGoWindowClosed
func tsGoWindowClosed() {
	if shell.OnWindowClosed != nil {
		shell.OnWindowClosed()
	}
}

//export tsGoSleep
func tsGoSleep() {
	if shell.OnSuspend != nil {
		shell.OnSuspend()
	}
}

//export tsGoTerminate
func tsGoTerminate() {
	if shell.OnTerminate != nil {
		shell.OnTerminate()
	}
}

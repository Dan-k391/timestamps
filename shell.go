//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	ole "github.com/go-ole/go-ole"
	webview2 "github.com/jchv/go-webview2"

	"timestamps/internal/ring"
	"timestamps/internal/server"
	"timestamps/internal/store"
	"timestamps/internal/tracker"
	"timestamps/internal/win"
)

// App is the desktop shell: tray icon, app window and notifications.
type App struct {
	db      *store.DB
	t       *tracker.Tracker
	srv     *server.Server
	dataDir string
	shell   *win.Shell
	view    webview2.WebView
	hwnd    uintptr
	exiting bool
	iconBig uintptr
	iconSm  uintptr
	ringKey string // what the tray ring shows now
}

const (
	cmdOpen = iota + 1
	cmdStopwatchStart
	cmdStopwatchStop
	cmdStopwatchDiscard
	cmdShot
	cmdPause15
	cmdPause60
	cmdPauseForever
	cmdResume
	cmdStats
	cmdExit
)

func newApp(db *store.DB, t *tracker.Tracker, srv *server.Server, dataDir string) *App {
	return &App{db: db, t: t, srv: srv, dataDir: dataDir}
}

func (a *App) lang() string {
	l := a.t.Settings().Language
	if l == "auto" || l == "" {
		if win.UILanguage() == 0x04 {
			return "zh"
		}
		return "en"
	}
	return l
}

// Text returns a localized shell string.
func (a *App) Text(key string) string {
	if s, ok := shellText[a.lang()][key]; ok {
		return s
	}
	return shellText["en"][key]
}

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

func fmtClock(secs int64) string {
	return fmt.Sprintf("%d:%02d:%02d", secs/3600, secs/60%60, secs%60)
}

func (a *App) menu() []win.MenuItem {
	items := []win.MenuItem{{ID: cmdOpen, Text: a.Text("open")}, {ID: cmdStats, Text: a.Text("statistics")}, {Separator: true}}
	st := a.srv.Timers().State()
	if sw, _ := st["stopwatch"].(*server.Stopwatch); sw != nil {
		label := a.Text("stopStopwatch") + "  " + fmtClock(time.Now().Unix()-sw.Start)
		if sw.Tags != "" {
			label += "  · " + sw.Tags
		}
		items = append(items, win.MenuItem{ID: cmdStopwatchStop, Text: label}, win.MenuItem{ID: cmdStopwatchDiscard, Text: a.Text("discardStopwatch")})
	} else {
		items = append(items, win.MenuItem{ID: cmdStopwatchStart, Text: a.Text("startStopwatch")})
	}
	items = append(items, win.MenuItem{ID: cmdShot, Text: a.Text("takeShot")})
	if l := a.t.Live(); l.State == "paused" {
		label := a.Text("resume")
		if l.PausedTill > 0 {
			label += "  (" + time.Unix(l.PausedTill, 0).Format("15:04") + ")"
		}
		items = append(items, win.MenuItem{ID: cmdResume, Text: label})
	} else {
		items = append(items, win.MenuItem{Text: a.Text("pause"), Sub: []win.MenuItem{
			{ID: cmdPause15, Text: a.Text("pause15")},
			{ID: cmdPause60, Text: a.Text("pause60")},
			{ID: cmdPauseForever, Text: a.Text("pauseForever")},
		}})
	}
	return append(items, win.MenuItem{Separator: true}, win.MenuItem{ID: cmdExit, Text: a.Text("exit")})
}

func (a *App) command(id int) {
	tm := a.srv.Timers()
	switch id {
	case cmdOpen:
		a.openWindow("")
	case cmdStats:
		a.openWindow("stats")
	case cmdStopwatchStart:
		tm.StartStopwatch("", "", 0)
		a.shell.Notify("Timestamps", a.Text("stopwatchStarted"))
	case cmdStopwatchStop:
		tm.StopStopwatch(true)
		a.t.Emit("tags")
	case cmdStopwatchDiscard:
		tm.StopStopwatch(false)
	case cmdShot:
		go a.t.Shots.Capture(a.t.Settings())
	case cmdPause15:
		a.t.Pause(15 * time.Minute)
	case cmdPause60:
		a.t.Pause(time.Hour)
	case cmdPauseForever:
		a.t.Pause(0)
	case cmdResume:
		a.t.Resume()
	case cmdExit:
		a.quit()
	}
}

// watch turns tracker events into notifications.
func (a *App) watch() {
	ch := a.t.Subscribe()
	for ev := range ch {
		if ev.Type != "away" {
			continue
		}
		aw := a.t.Aways()
		if len(aw) == 0 {
			continue
		}
		last := aw[len(aw)-1]
		mins := (last.E - last.S) / 60
		text := fmt.Sprintf(a.Text("awayText"), time.Unix(last.S, 0).Format("15:04"), time.Unix(last.E, 0).Format("15:04"), mins/60, mins%60)
		a.shell.Notify(a.Text("awayTitle"), text)
	}
}

func (a *App) tipLoop() {
	for {
		l := a.t.Live()
		day := time.Now().Format("2006-01-02")
		tip := "Timestamps"
		if s, e, err := store.DayRange(day); err == nil {
			if st, err := a.db.Stats(s, e, false); err == nil {
				tip += fmt.Sprintf(" · %s %dh %02dm", a.Text("activeToday"), st.Active/3600, st.Active/60%60)
			}
		}
		if l.State == "paused" {
			tip += " · " + a.Text("pausedTip")
		}
		a.shell.SetTip(tip)
		a.updateRing(l.State == "paused")
		time.Sleep(30 * time.Second)
	}
}

// updateRing redraws the tray icon, a ring that fills once a day, when it
// would look different: every 5 minutes, or when tracking pauses or resumes.
func (a *App) updateRing(paused bool) {
	now := time.Now()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	step := int(now.Sub(midnight) / (5 * time.Minute))
	key := fmt.Sprint(step, paused)
	if key == a.ringKey {
		return
	}
	a.ringKey = key
	frac := float64(step) * 5 * 60 / now.AddDate(0, 0, 1).Sub(midnight).Seconds()
	c := ring.Color(frac)
	if paused {
		c = ring.Gray
	}
	icon := win.IconFromImage(ring.Draw(win.SystemMetric(49), frac, c))
	if icon == 0 {
		return
	}
	a.shell.SetIcon(icon, func(old uintptr) {
		if old != a.iconSm {
			win.DestroyIcon(old)
		}
	})
}

// Notify shows a tray notification.
func (a *App) Notify(title, text string) { a.shell.Notify(title, text) }

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

var shellText = map[string]map[string]string{
	"en": {
		"open": "Open Timestamps", "statistics": "Statistics", "startStopwatch": "Start stopwatch",
		"stopStopwatch": "Stop stopwatch", "discardStopwatch": "Discard stopwatch", "takeShot": "Take screenshot now",
		"pause": "Pause tracking", "pause15": "For 15 minutes", "pause60": "For 1 hour", "pauseForever": "Until I resume",
		"resume": "Resume tracking", "exit": "Exit", "stillRunning": "Timestamps keeps tracking in the notification area. Right-click the icon to exit.",
		"awayTitle": "Welcome back", "awayText": "You were away %s–%s (%dh %02dm). Click to tag this time.",
		"countdownDone": "Countdown finished", "stopwatchRunning": "Stopwatch running", "stopwatchStarted": "Stopwatch started. Stop it from the tray icon or the app.",
		"activeToday": "active today", "pausedTip": "tracking paused",
	},
	"zh": {
		"open": "打开 Timestamps", "statistics": "统计", "startStopwatch": "开始秒表",
		"stopStopwatch": "停止秒表", "discardStopwatch": "放弃秒表", "takeShot": "立即截图",
		"pause": "暂停跟踪", "pause15": "15 分钟", "pause60": "1 小时", "pauseForever": "直到我恢复",
		"resume": "恢复跟踪", "exit": "退出", "stillRunning": "Timestamps 仍在通知区域中跟踪。右键单击图标可退出。",
		"awayTitle": "欢迎回来", "awayText": "您离开了 %s–%s（%d 小时 %02d 分）。点击为这段时间添加标签。",
		"countdownDone": "倒计时结束", "stopwatchRunning": "秒表运行中", "stopwatchStarted": "秒表已开始。可从托盘图标或应用中停止。",
		"activeToday": "今日活动", "pausedTip": "已暂停跟踪",
	},
}

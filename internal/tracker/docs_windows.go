//go:build windows

package tracker

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	ole "github.com/go-ole/go-ole"

	"timestamps/internal/store"
	"timestamps/internal/win"
)

var browserExes = map[string]bool{
	"chrome.exe": true, "msedge.exe": true, "brave.exe": true, "opera.exe": true, "vivaldi.exe": true,
	"chromium.exe": true, "browser.exe": true, "360se.exe": true, "360chrome.exe": true, "qqbrowser.exe": true,
	"sogouexplorer.exe": true, "2345explorer.exe": true, "liebao.exe": true, "centbrowser.exe": true,
	"arc.exe": true, "thorium.exe": true, "iexplore.exe": true,
	"firefox.exe": true, "waterfox.exe": true, "librewolf.exe": true, "floorp.exe": true, "zen.exe": true,
}

var firefoxExes = map[string]bool{"firefox.exe": true, "waterfox.exe": true, "librewolf.exe": true, "floorp.exe": true, "zen.exe": true}

// Office-style apps whose open document is available over COM.
var comDocs = map[string][][]string{
	"winword.exe":  {{"Word.Application", "ActiveDocument", "FullName"}},
	"excel.exe":    {{"Excel.Application", "ActiveWorkbook", "FullName"}},
	"powerpnt.exe": {{"PowerPoint.Application", "ActivePresentation", "FullName"}},
	"visio.exe":    {{"Visio.Application", "ActiveDocument", "FullName"}},
	"winproj.exe":  {{"MSProject.Application", "ActiveProject", "FullName"}},
	"wps.exe": {
		{"KWps.Application", "ActiveDocument", "FullName"},
		{"Ket.Application", "ActiveWorkbook", "FullName"},
		{"Kwpp.Application", "ActivePresentation", "FullName"},
	},
	"et.exe":  {{"Ket.Application", "ActiveWorkbook", "FullName"}},
	"wpp.exe": {{"Kwpp.Application", "ActivePresentation", "FullName"}},
}

// recentIndex maps file names to shortcuts in the user's Recent Items
// folder, which Windows updates whenever a file is opened via the shell or
// a common dialog.
type recentIndex struct {
	dir      string
	checked  time.Time
	modTime  time.Time
	byName   map[string]string
	resolved map[string]string
}

func newRecentIndex() *recentIndex {
	return &recentIndex{
		dir:      filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Recent`),
		byName:   map[string]string{},
		resolved: map[string]string{},
	}
}

func (r *recentIndex) refresh() {
	if time.Since(r.checked) < 3*time.Second {
		return
	}
	r.checked = time.Now()
	st, err := os.Stat(r.dir)
	if err != nil || st.ModTime().Equal(r.modTime) {
		return
	}
	r.modTime = st.ModTime()
	ents, err := os.ReadDir(r.dir)
	if err != nil {
		return
	}
	names := make(map[string]string, len(ents))
	for _, e := range ents {
		n := e.Name()
		if !strings.HasSuffix(strings.ToLower(n), ".lnk") {
			continue
		}
		names[strings.ToLower(strings.TrimSuffix(n, n[len(n)-4:]))] = filepath.Join(r.dir, n)
	}
	r.byName = names
	r.resolved = map[string]string{}
}

// lookup returns the full path of a recently opened file with this name.
func (r *recentIndex) lookup(name string) string {
	r.refresh()
	key := strings.ToLower(name)
	if p, ok := r.resolved[key]; ok {
		return p
	}
	lnk := r.byName[key]
	p := ""
	if lnk != "" {
		if t := resolveLink(lnk); t != "" && strings.EqualFold(filepath.Base(t), name) {
			p = t
		}
	}
	r.resolved[key] = p
	return p
}

// pathFromCommandLine matches a file name against a process's arguments.
func pathFromCommandLine(cmd, name string) string {
	for _, a := range win.SplitCommandLine(cmd) {
		a = strings.Trim(a, `"`)
		if filepath.IsAbs(a) && strings.EqualFold(filepath.Base(a), name) {
			return a
		}
	}
	return ""
}

// winDetector reads documents over COM (Office, Explorer), UI Automation
// (browser address bars), process command lines and Recent items.
type winDetector struct {
	u       *uia
	recent  *recentIndex
	shell   *ole.IDispatch
	cmdLine map[uint32]cmdEntry
}

type cmdEntry struct {
	cmd string
	at  time.Time
}

// newDetector joins a COM apartment on the calling (locked) thread.
func newDetector() detector {
	ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED)
	return &winDetector{u: newUIA(), recent: newRecentIndex(), cmdLine: map[uint32]cmdEntry{}}
}

func (d *winDetector) close() { ole.CoUninitialize() }

func (d *winDetector) commandLine(pid uint32) string {
	if e, ok := d.cmdLine[pid]; ok && time.Since(e.at) < time.Minute {
		return e.cmd
	}
	c := win.ProcessCommandLine(pid)
	if len(d.cmdLine) > 256 {
		d.cmdLine = map[uint32]cmdEntry{}
	}
	d.cmdLine[pid] = cmdEntry{c, time.Now()}
	return c
}

func (d *winDetector) detect(r docReq) Doc {
	if browserExes[r.exe] {
		return d.browserDoc(r)
	}
	if r.exe == "explorer.exe" {
		cls := win.ClassName(r.hwnd)
		if cls == "CabinetWClass" || cls == "ExploreWClass" {
			if p := explorerFolder(&d.shell, r.hwnd, r.title); p != "" {
				return fileDoc(p)
			}
		}
		return Doc{}
	}
	name := fileFromTitle(r.title)
	if chains, ok := comDocs[r.exe]; ok {
		for _, c := range chains {
			p := dispatchPath(c[0], c[1:]...)
			if p == "" {
				continue
			}
			if name == "" || strings.Contains(strings.ToLower(r.title), strings.ToLower(strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)))) {
				return fileDoc(p)
			}
		}
	}
	if name == "" {
		return Doc{}
	}
	if filepath.IsAbs(name) {
		return fileDoc(name)
	}
	if p := pathFromCommandLine(d.commandLine(r.pid), name); p != "" {
		return fileDoc(p)
	}
	if p := d.recent.lookup(name); p != "" {
		return fileDoc(p)
	}
	return Doc{Kind: store.KindFile, Key: name, Name: name, Grp: name}
}

func (d *winDetector) browserDoc(r docReq) Doc {
	page := PageTitle(r.title)
	if r.opts.extURL != nil {
		if raw, private, ok := r.opts.extURL(page); ok {
			return webDoc(raw, page, private)
		}
	}
	if !r.opts.addressBar {
		return Doc{}
	}
	raw := strings.TrimSpace(d.u.addressBar(r.hwnd, firefoxExes[r.exe]))
	return webDoc(raw, page, false)
}

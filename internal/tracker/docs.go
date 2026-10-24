//go:build windows

package tracker

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	ole "github.com/go-ole/go-ole"

	"timestamps/internal/store"
	"timestamps/internal/win"
)

// Doc is the document detected in the foreground window.
type Doc struct {
	Kind    int    // store.KindFile or store.KindWeb, 0 = none
	Key     string // full path or URL
	Name    string // file name or page title
	Grp     string // file name or domain
	Private bool
}

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

// IsBrowser reports whether an executable is a web browser.
func IsBrowser(exe string) bool { return browserExes[exe] }

var docExt = map[string]bool{}

func init() {
	for _, e := range strings.Fields(`doc docx docm dot dotx rtf odt wps wpt xls xlsx xlsm xlsb csv et ett ods ppt pptx pptm pps ppsx odp dps dpt
		pdf ofd txt md markdown log ini cfg conf json jsonc yaml yml toml xml html htm css scss less js mjs cjs ts tsx jsx vue svelte
		go py rb rs java kt kts c h cc cpp hpp cs fs vb php swift m mm sql sh bat cmd ps1 psm1 lua r dart scala clj ex exs erl hs
		ipynb tex bib epub mobi azw3 djvu chm png jpg jpeg gif bmp webp tif tiff svg ico heic raw psd psb ai eps indd xd fig sketch
		cdr afdesign afphoto mp3 wav flac m4a aac ogg mp4 mkv avi mov wmv flv webm m4v dwg dxf dwf rvt rfa ipt iam idw skp 3dm step stp
		igs iges stl obj fbx blend max ma mb c4d prt sldprt sldasm zip rar 7z tar gz bz2 xz iso vsd vsdx mpp one pub xps eml msg vcf
		unity uproject sln csproj vcxproj drawio xmind mm mmap kml gpx bak dat`) {
		docExt[e] = true
	}
}

var reFileToken = regexp.MustCompile(`([^\\/:*?"<>|\t]+?\.([A-Za-z0-9]{1,10}))(?:\s*[\[(].*?[\])])?\s*$`)

// fileFromTitle finds a file name (or full path) in a window title, e.g.
// "Report.docx - Word" or "● main.go - project - Visual Studio Code".
func fileFromTitle(title string) string {
	t := strings.TrimSpace(title)
	for _, p := range []string{"● ", "• ", "*", "○ "} {
		t = strings.TrimPrefix(t, p)
	}
	for _, sep := range []string{" - ", " — ", " – ", " | "} {
		t = strings.ReplaceAll(t, sep, "\x00")
	}
	for _, part := range strings.Split(t, "\x00") {
		part = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(part), "*"))
		part = strings.TrimSuffix(part, " [Read-Only]")
		part = strings.TrimSuffix(part, " [只读]")
		part = strings.TrimSuffix(part, " (Read-Only)")
		if len(part) > 2 && part[1] == ':' && (part[2] == '\\' || part[2] == '/') {
			if docExt[strings.TrimPrefix(strings.ToLower(filepath.Ext(part)), ".")] {
				return part
			}
		}
		if m := reFileToken.FindStringSubmatch(part); m != nil {
			if docExt[strings.ToLower(m[2])] && !strings.Contains(m[1], "://") {
				return strings.TrimSpace(m[1])
			}
		}
	}
	return ""
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

type docReq struct {
	hwnd  uintptr
	pid   uint32
	exe   string
	title string
	opts  docOpts
	resp  chan Doc
}

type docOpts struct {
	addressBar bool
	extURL     func(title string) (url string, private bool, ok bool)
}

// docWorker owns a COM apartment and answers document queries.
type docWorker struct {
	reqs    chan docReq
	mu      sync.Mutex
	busy    time.Time
	cmdMu   sync.Mutex
	cmdLine map[uint32]cmdEntry
}

type cmdEntry struct {
	cmd string
	at  time.Time
}

func newDocWorker() *docWorker {
	w := &docWorker{cmdLine: map[uint32]cmdEntry{}}
	w.start()
	return w
}

func (w *docWorker) start() {
	w.reqs = make(chan docReq, 1)
	reqs := w.reqs
	go func() {
		runtime.LockOSThread()
		ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED)
		defer ole.CoUninitialize()
		u := newUIA()
		recent := newRecentIndex()
		var shell *ole.IDispatch
		for r := range reqs {
			w.mu.Lock()
			w.busy = time.Now()
			w.mu.Unlock()
			d := w.detect(r, u, recent, &shell)
			w.mu.Lock()
			w.busy = time.Time{}
			w.mu.Unlock()
			r.resp <- d
		}
	}()
}

// query asks the worker for the document in a window, giving up after
// timeout. A worker stuck in a COM call for too long is replaced.
func (w *docWorker) query(r docReq, timeout time.Duration) (Doc, bool) {
	r.resp = make(chan Doc, 1)
	w.mu.Lock()
	busy := w.busy
	w.mu.Unlock()
	if !busy.IsZero() && time.Since(busy) > 20*time.Second {
		w.start() // abandon the hung apartment thread
	}
	select {
	case w.reqs <- r:
	default:
		return Doc{}, false
	}
	select {
	case d := <-r.resp:
		return d, true
	case <-time.After(timeout):
		return Doc{}, false
	}
}

func (w *docWorker) commandLine(pid uint32) string {
	w.cmdMu.Lock()
	defer w.cmdMu.Unlock()
	if e, ok := w.cmdLine[pid]; ok && time.Since(e.at) < time.Minute {
		return e.cmd
	}
	c := win.ProcessCommandLine(pid)
	if len(w.cmdLine) > 256 {
		w.cmdLine = map[uint32]cmdEntry{}
	}
	w.cmdLine[pid] = cmdEntry{c, time.Now()}
	return c
}

func (w *docWorker) detect(r docReq, u *uia, recent *recentIndex, shell **ole.IDispatch) Doc {
	if browserExes[r.exe] {
		return w.browserDoc(r, u)
	}
	if r.exe == "explorer.exe" {
		cls := win.ClassName(r.hwnd)
		if cls == "CabinetWClass" || cls == "ExploreWClass" {
			if p := explorerFolder(shell, r.hwnd, r.title); p != "" {
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
	if p := pathFromCommandLine(w.commandLine(r.pid), name); p != "" {
		return fileDoc(p)
	}
	if p := recent.lookup(name); p != "" {
		return fileDoc(p)
	}
	return Doc{Kind: store.KindFile, Key: name, Name: name, Grp: name}
}

func fileDoc(p string) Doc {
	base := filepath.Base(p)
	return Doc{Kind: store.KindFile, Key: p, Name: base, Grp: base}
}

var browserSuffix = regexp.MustCompile(`\s+[-—–]\s+(Google Chrome|Microsoft Edge|Brave|Opera|Vivaldi|Chromium|Mozilla Firefox|Firefox|Waterfox|LibreWolf|Floorp|Zen Browser|360安全浏览器|360极速浏览器|QQ浏览器|搜狗高速浏览器|Yandex|Arc|Thorium)(\s+(Beta|Dev|Canary|Nightly|Developer Edition))?$`)

var morePages = regexp.MustCompile(`\s+(and \d+ more pages?|和另外\s*\d+\s*个页面|和其他\s*\d+\s*个页面|及其他\s*\d+\s*个页面)(\s+[-—–]\s+.*)?$`)

// PageTitle strips the browser name, profile and "and N more pages" from a
// browser window title.
func PageTitle(title string) string {
	t := strings.ReplaceAll(title, "​", "") // Edge puts a zero-width space in "Microsoft Edge"
	t = browserSuffix.ReplaceAllString(t, "")
	t = morePages.ReplaceAllString(t, "")
	return strings.TrimSpace(t)
}

func (w *docWorker) browserDoc(r docReq, u *uia) Doc {
	page := PageTitle(r.title)
	if r.opts.extURL != nil {
		if raw, private, ok := r.opts.extURL(page); ok {
			return webDoc(raw, page, private)
		}
	}
	if !r.opts.addressBar {
		return Doc{}
	}
	raw := strings.TrimSpace(u.addressBar(r.hwnd, firefoxExes[r.exe]))
	return webDoc(raw, page, false)
}

var reHostLike = regexp.MustCompile(`^[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)+(:\d+)?(/|$)|^localhost(:\d+)?(/|$)|^\d+\.\d+\.\d+\.\d+(:\d+)?(/|$)`)

// webDoc normalises an address bar value into a website document.
func webDoc(raw, page string, private bool) Doc {
	if raw == "" || strings.ContainsAny(raw, " \t\n") {
		return Doc{}
	}
	low := strings.ToLower(raw)
	for _, p := range []string{"chrome://", "edge://", "about:", "brave://", "opera://", "vivaldi://", "chrome-extension://", "moz-extension://",
		"extension://", "ms-browser-extension://", "view-source:", "devtools://", "data:", "blob:", "javascript:", "chrome-search://", "edge-extension://"} {
		if strings.HasPrefix(low, p) {
			return Doc{}
		}
	}
	if !strings.Contains(low, "://") {
		if !reHostLike.MatchString(raw) {
			return Doc{}
		}
		raw = "https://" + raw
	} else if strings.HasPrefix(low, "file://") {
		if p := fileURLToPath(raw); p != "" {
			return fileDoc(p)
		}
		return Doc{}
	}
	if i := strings.IndexByte(raw, '#'); i > 0 {
		raw = raw[:i]
	}
	name := page
	if name == "" {
		name = raw
	}
	return Doc{Kind: store.KindWeb, Key: raw, Name: name, Grp: store.Domain(raw), Private: private}
}

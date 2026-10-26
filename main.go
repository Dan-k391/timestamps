// Timestamps is a fast, private time tracker for Windows and macOS: it
// records which applications, documents and websites you use, lets you tag
// time, and reports on it. All data stays in a local SQLite database.
package main

import (
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"timestamps/extension"
	"timestamps/internal/favicon"
	"timestamps/internal/server"
	"timestamps/internal/store"
	"timestamps/internal/tracker"
	"timestamps/web"
)

func init() { runtime.LockOSThread() }

func main() {
	minimized := flag.Bool("minimized", false, "start in the tray without opening the window")
	dataFlag := flag.String("data", "", "data directory (default "+dataDirHelp+")")
	flag.Parse()

	platformInit()
	dataDir := *dataFlag
	if dataDir == "" {
		dataDir = defaultDataDir()
	}
	dataDir, _ = filepath.Abs(dataDir)
	os.MkdirAll(dataDir, 0o755)

	if !singleInstance(dataDir) {
		signalRunning()
		return
	}
	// Started from inside another app's container (Claude's tools, for
	// one), Windows would keep the data in that app's own folder, out of
	// sight, and end Timestamps whenever that app restarts.
	if real := redirectedDataDir(dataDir); real != "" {
		alert("Timestamps", redirectedText(real))
		return
	}
	setupLog(dataDir)

	db, err := store.Open(filepath.Join(dataDir, "timestamps.db"))
	if err != nil {
		alert("Timestamps", "Could not open the database:\n"+err.Error())
		return
	}
	settings := db.Settings()
	go db.FillIconColors()

	extDir := filepath.Join(dataDir, "extension")
	unpack(extension.FS, extDir)

	shotsDir := filepath.Join(dataDir, "screenshots")
	if settings.ShotsDir != "" {
		shotsDir = settings.ShotsDir
	}
	t := tracker.New(db, shotsDir, filepath.Join(dataDir, "thumbs"))
	go t.Shots.MoveOldThumbs()
	var webFS fs.FS = web.FS
	if dir := os.Getenv("TIMESTAMPS_WEB_DIR"); dir != "" {
		webFS = os.DirFS(dir) // development: serve the UI from disk
	}
	// Copies of the browsers' icon caches, one folder per data folder.
	favTmp := filepath.Join(os.TempDir(), "timestamps-favicons-"+strconv.FormatUint(uint64(hashString(strings.ToLower(dataDir))), 16))
	fav := favicon.New(db, favTmp, func() bool { return t.Settings().SiteIcons })
	fav.OnFound = func() { t.Emit("icons") } // new website colors: views reload
	srv := &server.Server{DB: db, T: t, Fav: fav, Web: webFS, DataDir: dataDir, ExtDir: extDir}
	app := newApp(db, t, srv, dataDir)
	srv.Host = app
	if err := srv.Listen(); err != nil {
		alert("Timestamps", err.Error())
		return
	}
	go func() {
		if err := srv.Serve(); err != nil {
			log.Println("server:", err)
		}
	}()
	t.Start()
	log.Printf("Timestamps %s started, UI at %s", server.Version, srv.URL())

	app.run(!*minimized && !settings.StartMinimized)

	t.Stop()
	srv.Close()
	fav.Close()
	db.Close()
	log.Println("exited")
}

// redirectedText explains why Timestamps won't start with a redirected data
// folder. The app whose folder it is comes from the path:
// ...\Packages\<Name>_<publisher id>\LocalCache\...
func redirectedText(real string) string {
	owner := "another app"
	if i := strings.Index(strings.ToLower(real), `\packages\`); i >= 0 {
		owner = strings.SplitN(real[i+len(`\packages\`):], `\`, 2)[0]
		if j := strings.LastIndex(owner, "_"); j > 0 {
			owner = owner[:j]
		}
	}
	if systemChinese() {
		return "Timestamps 是从另一个应用（" + owner + "）内部启动的。Windows 会把数据存到该应用自己的文件夹里：\n" + real +
			"\n\n这样数据在别处看不到，而且该应用重启时 Timestamps 也会被关闭。\n\n请从开始菜单启动 Timestamps。"
	}
	return "Timestamps was started from inside another app (" + owner + "). Windows would keep its data in that app's own folder:\n" + real +
		"\n\nThere it is hidden from everything else, and Timestamps would close whenever that app restarts.\n\nStart Timestamps from the Start menu instead."
}

func hashString(s string) uint32 {
	h := uint32(2166136261)
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

func setupLog(dir string) {
	p := filepath.Join(dir, "timestamps.log")
	if st, err := os.Stat(p); err == nil && st.Size() > 2<<20 {
		os.Rename(p, p+".old")
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err == nil {
		log.SetOutput(f)
		// A crash (panic or fatal error) prints to standard error, which a
		// windowed app doesn't have: send it to the log too.
		redirectStderr(f)
	}
}

// signalRunning asks the already running instance to show its window.
func signalRunning() {
	c := &http.Client{Timeout: 2 * time.Second}
	for _, p := range server.Ports {
		req, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1:"+strconv.Itoa(p)+"/api/internal/show", nil)
		req.Header.Set("X-Timestamps-Instance", "1")
		if resp, err := c.Do(req); err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusNoContent {
				return
			}
		}
	}
}

// unpack writes an embedded directory tree to disk.
func unpack(src fs.FS, dst string) {
	fs.WalkDir(src, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == "." || strings.HasSuffix(p, ".go") {
			return nil
		}
		out := filepath.Join(dst, filepath.FromSlash(p))
		if d.IsDir() {
			os.MkdirAll(out, 0o755)
			return nil
		}
		b, err := fs.ReadFile(src, p)
		if err == nil {
			os.MkdirAll(filepath.Dir(out), 0o755)
			if old, err := os.ReadFile(out); err != nil || string(old) != string(b) {
				os.WriteFile(out, b, 0o644)
			}
		}
		return nil
	})
}

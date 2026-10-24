//go:build windows

// Timestamps is a fast, private time tracker for Windows: it records which
// applications, documents and websites you use, lets you tag time, and
// reports on it. All data stays in a local SQLite database.
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
	"timestamps/internal/win"
	"timestamps/web"
)

func init() { runtime.LockOSThread() }

func main() {
	minimized := flag.Bool("minimized", false, "start in the tray without opening the window")
	dataFlag := flag.String("data", "", "data directory (default %LOCALAPPDATA%\\Timestamps)")
	flag.Parse()

	win.SetDPIAware()
	dataDir := *dataFlag
	if dataDir == "" {
		dataDir = filepath.Join(os.Getenv("LOCALAPPDATA"), "Timestamps")
	}
	dataDir, _ = filepath.Abs(dataDir)
	os.MkdirAll(dataDir, 0o755)
	setupLog(dataDir)

	mutex := "Local\\Timestamps-" + strconv.FormatUint(uint64(hashString(strings.ToLower(dataDir))), 16)
	if !win.SingleInstance(mutex) {
		signalRunning()
		return
	}

	db, err := store.Open(filepath.Join(dataDir, "timestamps.db"))
	if err != nil {
		win.MessageBox("Timestamps", "Could not open the database:\n"+err.Error())
		return
	}
	settings := db.Settings()

	extDir := filepath.Join(dataDir, "extension")
	unpack(extension.FS, extDir)

	t := tracker.New(db, filepath.Join(dataDir, "screenshots"))
	var webFS fs.FS = web.FS
	if dir := os.Getenv("TIMESTAMPS_WEB_DIR"); dir != "" {
		webFS = os.DirFS(dir) // development: serve the UI from disk
	}
	// Copies of the browsers' icon caches, one folder per data folder.
	favTmp := filepath.Join(os.TempDir(), "timestamps-favicons-"+strconv.FormatUint(uint64(hashString(strings.ToLower(dataDir))), 16))
	fav := favicon.New(db, favTmp, func() bool { return t.Settings().SiteIcons })
	srv := &server.Server{DB: db, T: t, Fav: fav, Web: webFS, DataDir: dataDir, ExtDir: extDir}
	app := newApp(db, t, srv, dataDir)
	srv.Host = app
	if err := srv.Listen(); err != nil {
		win.MessageBox("Timestamps", err.Error())
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

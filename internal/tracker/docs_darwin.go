package tracker

import (
	"path/filepath"
	"strings"
	"time"

	"timestamps/internal/mac"
	"timestamps/internal/store"
)

// Browsers by app key (the lower-case bundle name).
var browserExes = map[string]bool{
	"safari": true, "safari technology preview": true, "google chrome": true, "google chrome beta": true, "google chrome dev": true,
	"google chrome canary": true, "microsoft edge": true, "microsoft edge beta": true, "microsoft edge dev": true, "microsoft edge canary": true,
	"brave browser": true, "brave browser beta": true, "brave browser nightly": true, "vivaldi": true, "chromium": true, "arc": true,
	"opera": true, "opera gx": true, "thorium": true, "orion": true, "firefox": true, "firefox developer edition": true,
	"firefox nightly": true, "waterfox": true, "librewolf": true, "floorp": true, "zen": true, "zen browser": true,
}

// browserDialect picks the scripting dictionary a browser speaks by its
// bundle identifier; 0 means it can't be asked (Firefox and its forks).
func browserDialect(bundleID string) int {
	id := strings.ToLower(bundleID)
	switch {
	case strings.HasPrefix(id, "com.apple.safari"), strings.HasPrefix(id, "com.kagi.kagimacos"):
		return mac.DialectSafari
	case strings.HasPrefix(id, "com.google.chrome"), strings.HasPrefix(id, "com.microsoft.edgemac"), strings.HasPrefix(id, "com.brave.browser"),
		strings.HasPrefix(id, "com.vivaldi.vivaldi"), strings.HasPrefix(id, "org.chromium.chromium"), strings.HasPrefix(id, "company.thebrowser."),
		strings.HasPrefix(id, "com.operasoftware.opera"), strings.HasPrefix(id, "org.chromium.thorium"):
		return mac.DialectChromium
	}
	return 0
}

// macDetector reads documents from the focused window (AXDocument), asks
// browsers for their tab, and looks up file names from window titles with
// Spotlight.
type macDetector struct {
	found map[string]spotHit
}

type spotHit struct {
	path string
	at   time.Time
}

func newDetector() detector { return &macDetector{found: map[string]spotHit{}} }

func (d *macDetector) close() {}

func (d *macDetector) detect(r docReq) Doc {
	if browserExes[r.exe] {
		return d.browserDoc(r)
	}
	if p := mac.FocusedDocument(int(r.pid)); p != "" {
		return fileDoc(p)
	}
	name := fileFromTitle(r.title)
	if name == "" {
		return Doc{}
	}
	if filepath.IsAbs(name) {
		return fileDoc(name)
	}
	if p := d.spotlight(name); p != "" {
		return fileDoc(p)
	}
	return Doc{Kind: store.KindFile, Key: name, Name: name, Grp: name}
}

// spotlight remembers lookups for a couple of minutes: titles repeat.
func (d *macDetector) spotlight(name string) string {
	if h, ok := d.found[name]; ok && time.Since(h.at) < 2*time.Minute {
		return h.path
	}
	p := mac.SpotlightPath(name)
	if len(d.found) > 256 {
		d.found = map[string]spotHit{}
	}
	d.found[name] = spotHit{p, time.Now()}
	return p
}

func (d *macDetector) browserDoc(r docReq) Doc {
	page := PageTitle(r.title)
	if r.opts.extURL != nil {
		if raw, private, ok := r.opts.extURL(page); ok {
			return webDoc(raw, page, private)
		}
	}
	if !r.opts.addressBar {
		return Doc{}
	}
	dialect := browserDialect(mac.BundleID(int(r.pid)))
	if dialect == 0 {
		return Doc{}
	}
	raw, incognito := mac.BrowserURL(int(r.pid), dialect)
	return webDoc(strings.TrimSpace(raw), page, incognito)
}

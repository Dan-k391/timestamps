// Package favicon finds website icons. It looks in the browsers' own icon
// caches first (the Chromium "Favicons" and Firefox "favicons.sqlite"
// databases, read from a private copy, so nothing leaves the computer), and
// then, when allowed, asks the site itself for /favicon.ico or the icon its
// home page links to. Found icons and misses are kept in the store.
package favicon

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Store keeps found icons (nil for none) with the time of the lookup.
type Store interface {
	SiteIcon(domain string) (icon []byte, checked int64, found bool)
	SetSiteIcon(domain string, icon []byte, checked int64)
}

const (
	retryMiss = 24 * time.Hour   // look again for sites that had no icon
	snapAge   = 20 * time.Minute // copy the browser caches again after this
	maxIcon   = 256 << 10
)

// Finder looks up website icons.
type Finder struct {
	store    Store
	tmp      string
	download func() bool // whether sites may be asked for their icon

	mu       sync.Mutex
	inflight map[string]chan struct{}

	snapMu sync.Mutex
	snapAt time.Time
	caches []cache

	sem    chan struct{} // limits concurrent downloads
	client *http.Client

	// OnFound is called after a website's icon was found and stored.
	OnFound func()
}

type cache struct {
	db      *sql.DB
	firefox bool
}

// New returns a Finder keeping copies of the browser caches in tmp.
func New(store Store, tmp string, download func() bool) *Finder {
	return &Finder{
		store: store, tmp: tmp, download: download,
		inflight: map[string]chan struct{}{},
		sem:      make(chan struct{}, 4),
		client: &http.Client{
			Timeout:   8 * time.Second,
			Transport: &http.Transport{Proxy: Proxy, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 6 * time.Second},
		},
	}
}

var hostRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

// Clean returns the lower-case host name of a site, or "" for things that
// can't have an icon (localhost, IP addresses, browser extension ids).
func Clean(domain string) string {
	d := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
	if h, _, err := net.SplitHostPort(d); err == nil {
		d = h
	}
	if len(d) > 253 || !hostRe.MatchString(d) || net.ParseIP(d) != nil {
		return ""
	}
	return d
}

// Icon returns the icon of a website (a host or registrable domain), or nil.
func (f *Finder) Icon(domain string) []byte {
	d := Clean(domain)
	if d == "" {
		return nil
	}
	if b, checked, ok := f.store.SiteIcon(d); ok && (b != nil || time.Since(time.Unix(checked, 0)) < retryMiss) {
		return b
	}
	// One lookup per site at a time; others wait for its result.
	f.mu.Lock()
	if ch, ok := f.inflight[d]; ok {
		f.mu.Unlock()
		<-ch
		b, _, _ := f.store.SiteIcon(d)
		return b
	}
	ch := make(chan struct{})
	f.inflight[d] = ch
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		delete(f.inflight, d)
		f.mu.Unlock()
		close(ch)
	}()
	b := f.fromBrowsers(d)
	if b == nil && f.download != nil && f.download() {
		b = f.fromSite(d)
	}
	f.store.SetSiteIcon(d, b, time.Now().Unix())
	if b != nil && f.OnFound != nil {
		f.OnFound()
	}
	return b
}

// Close closes and removes the copies of the browser caches.
func (f *Finder) Close() {
	f.snapMu.Lock()
	defer f.snapMu.Unlock()
	for _, c := range f.caches {
		c.db.Close()
	}
	f.caches, f.snapAt = nil, time.Time{}
	os.RemoveAll(f.tmp)
}

// ---- browser caches -------------------------------------------------------

// snapshot copies the browser caches (they are in use by the browsers).
func (f *Finder) snapshot() {
	for _, c := range f.caches {
		c.db.Close()
	}
	f.caches = nil
	os.RemoveAll(f.tmp)
	if err := os.MkdirAll(f.tmp, 0o755); err != nil {
		return
	}
	chromium, firefox := browserCaches()
	n := 0
	add := func(src string, ff bool) {
		n++
		dst := filepath.Join(f.tmp, fmt.Sprintf("%d.db", n))
		if copyFile(src, dst) != nil {
			return
		}
		copyFile(src+"-wal", dst+"-wal")
		db, err := sql.Open("sqlite3", "file:"+dst+"?_busy_timeout=2000")
		if err != nil {
			return
		}
		db.SetMaxOpenConns(2)
		f.caches = append(f.caches, cache{db: db, firefox: ff})
	}
	for _, p := range chromium {
		add(p, false)
	}
	for _, p := range firefox {
		add(p, true)
	}
	f.snapAt = time.Now()
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}

const chromiumQ = `SELECT m.page_url, b.image_data, b.width FROM icon_mapping m JOIN favicon_bitmaps b ON b.icon_id = m.icon_id
	WHERE (m.page_url LIKE ? OR m.page_url LIKE ? OR m.page_url LIKE ? OR m.page_url LIKE ?) AND length(b.image_data) > 0 LIMIT 500`

const firefoxQ = `SELECT p.page_url, i.data, i.width FROM moz_pages_w_icons p JOIN moz_icons_to_pages ip ON ip.page_id = p.id
	JOIN moz_icons i ON i.id = ip.icon_id
	WHERE (p.page_url LIKE ? OR p.page_url LIKE ? OR p.page_url LIKE ? OR p.page_url LIKE ?) AND length(i.data) > 0 LIMIT 500`

const firefoxRootQ = `SELECT icon_url, data, width FROM moz_icons
	WHERE root = 1 AND (icon_url LIKE ? OR icon_url LIKE ? OR icon_url LIKE ? OR icon_url LIKE ?) AND length(data) > 0 LIMIT 100`

// fromBrowsers returns the best icon the browsers have for pages of the site
// or its subdomains: the site's own (or www.) pages first, about 32 px.
func (f *Finder) fromBrowsers(d string) []byte {
	f.snapMu.Lock()
	defer f.snapMu.Unlock()
	if time.Since(f.snapAt) > snapAge {
		f.snapshot()
	}
	args := []any{"http://" + d + "/%", "https://" + d + "/%", "http://%." + d + "/%", "https://%." + d + "/%"}
	var best []byte
	bestScore := -1
	consider := func(db *sql.DB, q string) {
		rows, err := db.Query(q, args...)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var page string
			var data []byte
			var w int
			if rows.Scan(&page, &data, &w) != nil || !isImage(data) {
				continue
			}
			u, err := url.Parse(page)
			if err != nil {
				continue
			}
			host := strings.ToLower(u.Hostname())
			if host != d && !strings.HasSuffix(host, "."+d) {
				continue
			}
			score := w
			if w >= 32 {
				score = 200 - (w-32)/8 // 32 px, then the next larger
			}
			if host == d || host == "www."+d {
				score += 1000
			}
			if score > bestScore {
				best, bestScore = data, score
			}
		}
	}
	for _, c := range f.caches {
		if c.firefox {
			consider(c.db, firefoxQ)
			consider(c.db, firefoxRootQ)
		} else {
			consider(c.db, chromiumQ)
		}
	}
	return best
}

// ---- the site itself -------------------------------------------------------

var (
	linkRe = regexp.MustCompile(`(?is)<link\b[^>]*>`)
	relRe  = regexp.MustCompile(`(?is)\brel\s*=\s*["']?([^"'>]*)`)
	hrefRe = regexp.MustCompile(`(?is)\bhref\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
)

func (f *Finder) fromSite(d string) []byte {
	f.sem <- struct{}{}
	defer func() { <-f.sem }()
	for _, host := range []string{d, "www." + d} {
		if b, _ := f.get("https://"+host+"/favicon.ico", maxIcon); isImage(b) {
			return b
		}
	}
	for _, host := range []string{d, "www." + d} {
		page, final := f.get("https://"+host+"/", 512<<10)
		if page == nil {
			continue
		}
		for _, href := range iconLinks(page) {
			if strings.HasPrefix(href, "data:") {
				if b := dataURL(href); isImage(b) {
					return b
				}
				continue
			}
			u, err := final.Parse(href)
			if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
				continue
			}
			if b, _ := f.get(u.String(), maxIcon); isImage(b) {
				return b
			}
		}
		return nil
	}
	return nil
}

// get downloads up to max bytes; nil unless the answer is 200 and complete.
func (f *Finder) get(u string, max int64) ([]byte, *url.URL) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Timestamps")
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil || int64(len(b)) > max {
		return nil, nil
	}
	return b, resp.Request.URL
}

// iconLinks returns the hrefs of a page's <link rel="... icon ..."> tags.
func iconLinks(page []byte) []string {
	var out []string
	for _, tag := range linkRe.FindAll(page, 40) {
		rel := relRe.FindSubmatch(tag)
		if rel == nil || !bytes.Contains(bytes.ToLower(rel[1]), []byte("icon")) || bytes.Contains(bytes.ToLower(rel[1]), []byte("mask")) {
			continue
		}
		if m := hrefRe.FindSubmatch(tag); m != nil {
			href := strings.TrimSpace(string(bytes.Join(m[1:], nil)))
			if href != "" && !strings.HasSuffix(strings.ToLower(strings.SplitN(href, "?", 2)[0]), ".svg") {
				out = append(out, strings.ReplaceAll(href, "&amp;", "&"))
			}
		}
	}
	return out
}

func dataURL(s string) []byte {
	i := strings.Index(s, ";base64,")
	if i < 0 {
		return nil
	}
	b, _ := base64.StdEncoding.DecodeString(s[i+8:])
	return b
}

// isImage accepts the raster formats an <img> shows (no SVG: it can carry
// scripts).
func isImage(b []byte) bool {
	switch {
	case len(b) < 8:
		return false
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")), bytes.HasPrefix(b, []byte{0, 0, 1, 0}),
		bytes.HasPrefix(b, []byte("GIF8")), bytes.HasPrefix(b, []byte{0xFF, 0xD8, 0xFF}), bytes.HasPrefix(b, []byte("BM")):
		return true
	case len(b) >= 12 && bytes.HasPrefix(b, []byte("RIFF")) && string(b[8:12]) == "WEBP":
		return true
	}
	return false
}

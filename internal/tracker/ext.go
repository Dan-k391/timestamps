package tracker

import (
	"strings"
	"sync"
	"time"
)

// ExtReport is what the browser extension sends for the active tab.
type ExtReport struct {
	Browser   string `json:"browser"`
	Window    int    `json:"window"`
	URL       string `json:"url"`
	Title     string `json:"title"`
	Incognito bool   `json:"incognito"`
	at        time.Time
}

// ExtHub keeps the latest active tab per browser window, as reported by
// the optional browser extension. Reports are matched to the foreground
// window by page title, so any Chromium or Firefox build works.
type ExtHub struct {
	mu      sync.Mutex
	reports map[string]ExtReport
	seen    map[string]time.Time
}

// NewExtHub creates an empty hub.
func NewExtHub() *ExtHub {
	return &ExtHub{reports: map[string]ExtReport{}, seen: map[string]time.Time{}}
}

// Report stores an extension report.
func (h *ExtHub) Report(r ExtReport) {
	r.at = time.Now()
	h.mu.Lock()
	defer h.mu.Unlock()
	key := r.Browser + "/" + itoa(r.Window)
	h.reports[key] = r
	h.seen[r.Browser] = r.at
	for k, v := range h.reports {
		if time.Since(v.at) > 30*time.Minute {
			delete(h.reports, k)
		}
	}
}

// Connected lists browsers that reported in the last 10 minutes.
func (h *ExtHub) Connected() map[string]int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := map[string]int64{}
	for b, at := range h.seen {
		if time.Since(at) < 10*time.Minute {
			out[b] = at.Unix()
		}
	}
	return out
}

// Match finds the tab whose title matches the foreground page title.
func (h *ExtHub) Match(page string) (string, bool, bool) {
	if page == "" {
		return "", false, false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	var best ExtReport
	found := false
	for _, r := range h.reports {
		t := strings.TrimSpace(r.Title)
		if t == "" || r.URL == "" {
			continue
		}
		if t == page || strings.HasPrefix(page, t) || strings.HasPrefix(t, page) {
			if !found || r.at.After(best.at) {
				best, found = r, true
			}
		}
	}
	return best.URL, best.Incognito, found
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

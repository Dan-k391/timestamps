package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"timestamps/internal/favicon"
)

// Releases come from GitHub. On Windows the asset
// Timestamps-Setup-<version>.exe installs one (with /update it replaces the
// running app and restarts it); on macOS the app comes zipped as
// Timestamps-<version>-macos.zip (update_*.go).
const releasesAPI = "https://api.github.com/repos/ZJJ-2785/timestamps/releases/latest"

// UpdateInfo compares the newest release with this version.
type UpdateInfo struct {
	Current   string `json:"current"`
	Latest    string `json:"latest,omitempty"`
	Available bool   `json:"available"`
	Page      string `json:"page,omitempty"`  // release page
	Setup     string `json:"setup,omitempty"` // download for this platform
	Notes     string `json:"notes,omitempty"`
	Checked   int64  `json:"checked,omitempty"`
	Error     string `json:"error,omitempty"` // offline | notfound | bad
}

type updates struct {
	mu   sync.Mutex
	last UpdateInfo
}

var updateClient = &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{Proxy: favicon.Proxy}}

// CheckUpdate asks GitHub for the latest release, at most every 6 hours
// unless force.
func (s *Server) CheckUpdate(force bool) UpdateInfo {
	s.upd.mu.Lock()
	defer s.upd.mu.Unlock()
	if !force && s.upd.last.Checked > 0 && s.upd.last.Error == "" && time.Since(time.Unix(s.upd.last.Checked, 0)) < 6*time.Hour {
		return s.upd.last
	}
	info := UpdateInfo{Current: Version, Checked: time.Now().Unix()}
	req, _ := http.NewRequest(http.MethodGet, releasesAPI, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Timestamps/"+Version)
	resp, err := updateClient.Do(req)
	switch {
	case err != nil:
		info.Error = "offline"
	case resp.StatusCode == http.StatusNotFound:
		info.Error = "notfound" // no release, or the repository is private
	case resp.StatusCode != http.StatusOK:
		info.Error = "bad"
	default:
		var rel struct {
			Tag    string `json:"tag_name"`
			Page   string `json:"html_url"`
			Body   string `json:"body"`
			Assets []struct {
				Name string `json:"name"`
				URL  string `json:"browser_download_url"`
			} `json:"assets"`
		}
		if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rel) != nil {
			info.Error = "bad"
			break
		}
		info.Latest = strings.TrimPrefix(rel.Tag, "v")
		info.Page, info.Notes = rel.Page, rel.Body
		info.Available = newer(info.Latest, Version)
		for _, a := range rel.Assets {
			if updateAsset(a.Name) {
				info.Setup = a.URL
			}
		}
	}
	if resp != nil {
		resp.Body.Close()
	}
	s.upd.last = info
	return info
}

// newer reports whether version a (dotted numbers) is higher than b.
func newer(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			return x > y
		}
	}
	return false
}

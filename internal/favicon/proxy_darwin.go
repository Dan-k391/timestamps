package favicon

import (
	"net/http"
	"net/url"
	"os/exec"
	"sync"
	"time"
)

var (
	scutilMu       sync.Mutex
	scutilAt       time.Time
	scutilServer   string
	scutilOverride string
)

// Proxy uses the proxy from the environment, else the macOS network proxy
// setting (what tools such as ClashX set), so sites that need it can be
// reached. The setting is read again at most every 30 seconds.
func Proxy(req *http.Request) (*url.URL, error) {
	if u, err := http.ProxyFromEnvironment(req); u != nil || err != nil {
		return u, err
	}
	scutilMu.Lock()
	if time.Since(scutilAt) > 30*time.Second {
		scutilAt = time.Now()
		scutilServer, scutilOverride = "", ""
		if out, err := exec.Command("/usr/sbin/scutil", "--proxy").Output(); err == nil {
			scutilServer, scutilOverride = scutilProxy(string(out))
		}
	}
	server, override := scutilServer, scutilOverride
	scutilMu.Unlock()
	if server == "" {
		return nil, nil
	}
	return systemProxy(server, override, req.URL)
}

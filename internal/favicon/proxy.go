package favicon

import (
	"net/http"
	"net/url"
	"path"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// proxy uses the proxy from the environment, else the Windows proxy setting
// (what tools such as Clash set), so sites that need it can be reached.
func proxy(req *http.Request) (*url.URL, error) {
	if u, err := http.ProxyFromEnvironment(req); u != nil || err != nil {
		return u, err
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return nil, nil
	}
	defer k.Close()
	if on, _, err := k.GetIntegerValue("ProxyEnable"); err != nil || on == 0 {
		return nil, nil
	}
	server, _, _ := k.GetStringValue("ProxyServer")
	override, _, _ := k.GetStringValue("ProxyOverride")
	return systemProxy(server, override, req.URL)
}

// systemProxy picks the proxy for u from a Windows ProxyServer value
// ("host:port" or "http=host:port;https=host:port") and ProxyOverride list.
func systemProxy(server, override string, u *url.URL) (*url.URL, error) {
	host := strings.ToLower(u.Hostname())
	for _, p := range strings.Split(override, ";") {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "<local>" && !strings.Contains(host, ".") {
			return nil, nil
		}
		if ok, _ := path.Match(p, host); ok && p != "" {
			return nil, nil
		}
	}
	addr := ""
	for _, part := range strings.Split(server, ";") {
		part = strings.TrimSpace(part)
		if k, v, ok := strings.Cut(part, "="); ok {
			if strings.EqualFold(k, u.Scheme) || (addr == "" && strings.EqualFold(k, "http")) {
				addr = v
			}
		} else if part != "" && addr == "" {
			addr = part
		}
	}
	if addr == "" {
		return nil, nil
	}
	if !strings.Contains(addr, "://") {
		addr = "http://" + addr
	}
	return url.Parse(addr)
}

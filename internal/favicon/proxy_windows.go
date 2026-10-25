//go:build windows

package favicon

import (
	"net/http"
	"net/url"

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

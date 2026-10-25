package favicon

import (
	"net/url"
	"path"
	"strings"
)

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

// scutilProxy turns the output of macOS "scutil --proxy" into the Windows
// ProxyServer and ProxyOverride forms that systemProxy reads.
func scutilProxy(out string) (server, override string) {
	vals := map[string]string{}
	var except []string
	inList := false
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		k, v, ok := strings.Cut(line, " : ")
		switch {
		case strings.HasPrefix(line, "ExceptionsList"):
			inList = true
		case inList && line == "}":
			inList = false
		case inList && ok:
			except = append(except, strings.TrimSpace(v))
		case ok:
			vals[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	var parts []string
	for _, p := range []struct{ scheme, key string }{{"http", "HTTP"}, {"https", "HTTPS"}} {
		if vals[p.key+"Enable"] == "1" && vals[p.key+"Proxy"] != "" {
			addr := vals[p.key+"Proxy"]
			if port := vals[p.key+"Port"]; port != "" {
				addr += ":" + port
			}
			parts = append(parts, p.scheme+"="+addr)
		}
	}
	if vals["ExcludeSimpleHostnames"] == "1" {
		except = append(except, "<local>")
	}
	return strings.Join(parts, ";"), strings.Join(except, ";")
}

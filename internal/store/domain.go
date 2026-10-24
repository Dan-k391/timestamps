package store

import (
	"net"
	"strings"
)

// Two-label public suffixes (and shared hosting domains, where every
// subdomain belongs to someone else) that a registrable domain sits under.
// Kept in sync with BASE_SUFFIXES in web/js/fmt.js.
var multiSuffixes = map[string]bool{}

func init() {
	for _, s := range strings.Fields(`
		com.cn net.cn org.cn gov.cn edu.cn ac.cn mil.cn
		com.hk net.hk org.hk edu.hk gov.hk com.tw net.tw org.tw edu.tw gov.tw com.mo
		co.uk org.uk ac.uk gov.uk me.uk ltd.uk plc.uk net.uk sch.uk nhs.uk
		com.au net.au org.au edu.au gov.au co.nz net.nz org.nz ac.nz govt.nz
		co.jp ne.jp or.jp ac.jp go.jp co.kr ne.kr or.kr ac.kr go.kr
		com.sg edu.sg gov.sg com.my edu.my gov.my co.id ac.id go.id or.id co.th ac.th go.th in.th
		com.vn edu.vn gov.vn com.ph edu.ph gov.ph co.in net.in org.in ac.in gov.in
		com.br net.br org.br gov.br edu.br com.mx org.mx gob.mx edu.mx com.ar gob.ar com.co gov.co
		co.za org.za ac.za gov.za com.tr edu.tr gov.tr com.ua org.ua com.ru org.ru
		com.sa edu.sa gov.sa co.il ac.il gov.il com.eg edu.eg gov.eg com.pk edu.pk
		github.io gitlab.io gitee.io vercel.app netlify.app pages.dev workers.dev web.app firebaseapp.com
		herokuapp.com appspot.com blogspot.com azurewebsites.net cloudfront.net onrender.com fly.dev
		repl.co glitch.me surge.sh ngrok.io ngrok-free.app trycloudflare.com
	`) {
		multiSuffixes[s] = true
	}
}

// BaseDomain returns the registrable domain of a host, so subdomains of
// one site group together: "t.bilibili.com" -> "bilibili.com",
// "news.bbc.co.uk" -> "bbc.co.uk". IPs and single-label hosts are kept.
func BaseDomain(host string) string {
	h := strings.TrimSuffix(strings.ToLower(host), ".")
	if h == "" || !strings.Contains(h, ".") || net.ParseIP(h) != nil {
		return h
	}
	parts := strings.Split(h, ".")
	if len(parts) <= 2 {
		return h
	}
	if multiSuffixes[strings.Join(parts[len(parts)-2:], ".")] {
		return strings.Join(parts[len(parts)-3:], ".")
	}
	return strings.Join(parts[len(parts)-2:], ".")
}

package favicon

import (
	"bytes"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

type memStore map[string][]byte

func (m memStore) SiteIcon(d string) ([]byte, int64, bool) { b, ok := m[d]; return b, 0, ok }
func (m memStore) SetSiteIcon(d string, b []byte, _ int64) { m[d] = b }

func png(tag byte) []byte { return append([]byte("\x89PNG\r\n\x1a\n"), tag) }

func TestClean(t *testing.T) {
	for in, want := range map[string]string{
		"GitHub.com": "github.com", "a.b.example.com:8080": "a.b.example.com", "localhost": "", "127.0.0.1": "",
		"kloodnjmhgicecceindgbfpjencnhajh": "", "c": "", "bad_host.com": "", "-x.com": "", " bilibili.com. ": "bilibili.com",
	} {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIconLinks(t *testing.T) {
	page := []byte(`<head><link rel="stylesheet" href="a.css"><LINK REL="shortcut icon" HREF="/static/fav.ico?v=2&amp;x=1">
		<link rel=mask-icon href=/m.svg><link href='/i.svg' rel='icon'><link rel="apple-touch-icon" href=/touch.png></head>`)
	got := iconLinks(page)
	want := []string{"/static/fav.ico?v=2&x=1", "/touch.png"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("iconLinks = %q, want %q", got, want)
	}
}

func TestSystemProxy(t *testing.T) {
	u, _ := url.Parse("https://github.com/favicon.ico")
	for _, c := range []struct{ server, override, want string }{
		{"127.0.0.1:7890", "", "http://127.0.0.1:7890"},
		{"http=127.0.0.1:1;https=127.0.0.1:2", "", "http://127.0.0.1:2"},
		{"http=127.0.0.1:1", "", "http://127.0.0.1:1"},
		{"socks=127.0.0.1:1", "", ""},
		{"127.0.0.1:7890", "localhost;*.github.com;github.com", ""},
	} {
		p, _ := systemProxy(c.server, c.override, u)
		got := ""
		if p != nil {
			got = p.String()
		}
		if got != c.want {
			t.Errorf("systemProxy(%q, %q) = %q, want %q", c.server, c.override, got, c.want)
		}
	}
}

func TestScutilProxy(t *testing.T) {
	out := `<dictionary> {
  ExceptionsList : <array> {
    0 : *.local
    1 : 169.254/16
  }
  ExcludeSimpleHostnames : 1
  HTTPEnable : 1
  HTTPPort : 7890
  HTTPProxy : 127.0.0.1
  HTTPSEnable : 0
  HTTPSPort : 8080
  HTTPSProxy : proxy.example.com
  SOCKSEnable : 1
  SOCKSPort : 7891
  SOCKSProxy : 127.0.0.1
}`
	server, override := scutilProxy(out)
	if server != "http=127.0.0.1:7890" || override != "*.local;169.254/16;<local>" {
		t.Fatalf("scutilProxy = %q, %q", server, override)
	}
	if server, _ := scutilProxy("<dictionary> {\n  HTTPEnable : 0\n}"); server != "" {
		t.Fatalf("disabled proxy gave %q", server)
	}
}

// A fake Edge profile with icons for a site's pages and a subdomain.
func TestFromBrowsers(t *testing.T) {
	root := t.TempDir()
	dir := fakeProfile(t, root)
	os.MkdirAll(dir, 0o755)
	db, err := sql.Open("sqlite3", filepath.Join(dir, "Favicons"))
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE icon_mapping(id INTEGER PRIMARY KEY, page_url LONGVARCHAR NOT NULL, icon_id INTEGER, page_url_type INTEGER DEFAULT 0)`,
		`CREATE TABLE favicon_bitmaps(id INTEGER PRIMARY KEY, icon_id INTEGER NOT NULL, last_updated INTEGER DEFAULT 0, image_data BLOB, width INTEGER DEFAULT 0, height INTEGER DEFAULT 0, last_requested INTEGER DEFAULT 0)`,
		`INSERT INTO icon_mapping(page_url, icon_id) VALUES('https://gist.example.com/x', 1), ('https://www.example.com/a', 2), ('https://notexample.com/', 3), ('https://example.com/?u=.example.com/', 4)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	add := func(icon, w int, data []byte) {
		db.Exec(`INSERT INTO favicon_bitmaps(icon_id, image_data, width) VALUES(?, ?, ?)`, icon, data, w)
	}
	add(1, 32, png('g'))                // subdomain
	add(2, 16, png('s'))                // the site, small
	add(2, 32, png('m'))                // the site, 32 px: best
	add(2, 64, []byte("<svg></svg>xx")) // not raster
	add(3, 32, png('n'))                // another site
	db.Close()

	st := memStore{}
	f := New(st, filepath.Join(root, "tmp"), func() bool { return false })
	defer f.Close()
	if b := f.Icon("example.com"); !bytes.Equal(b, png('m')) {
		t.Fatalf("Icon(example.com) = %q", b)
	}
	if b := f.Icon("gist.example.com"); !bytes.Equal(b, png('g')) {
		t.Fatalf("Icon(gist.example.com) = %q", b)
	}
	if b := f.Icon("other.org"); b != nil {
		t.Fatalf("Icon(other.org) = %q", b)
	}
	if b, ok := st["other.org"]; !ok || b != nil {
		t.Fatal("a miss should be remembered")
	}
}

package store

import (
	"path/filepath"
	"testing"
	"time"
)

func openTemp(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestTagsReplaceOverlaps(t *testing.T) {
	db := openTemp(t)
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.Local).Unix()
	db.AddTag(base, base+3600, "A", "", false)
	// A tag in the middle splits the existing one.
	db.AddTag(base+600, base+1200, "B , x", "note", true)
	tags := db.TagsIn(base, base+3600)
	if len(tags) != 3 {
		t.Fatalf("want 3 segments, got %+v", tags)
	}
	if tags[0].Tags != "A" || tags[0].E != base+600 || tags[1].Tags != "B, x" || !tags[1].Billable || tags[2].S != base+1200 {
		t.Fatalf("unexpected split: %+v", tags)
	}
	db.ClearTags(base+300, base+1500)
	tags = db.TagsIn(base, base+3600)
	if len(tags) != 2 || tags[0].E != base+300 || tags[1].S != base+1500 {
		t.Fatalf("unexpected clear: %+v", tags)
	}
	db.RenameTag("A", "Alpha")
	if names := db.TagNames(); len(names) != 1 || names[0] != "Alpha" {
		t.Fatalf("rename: %v", names)
	}
}

func TestTruncateFromAndStats(t *testing.T) {
	db := openTemp(t)
	day := time.Date(2026, 10, 20, 0, 0, 0, 0, time.Local)
	s := day.Add(9 * time.Hour).Unix()
	app := db.AppID("code.exe", "VS Code", "", nil)
	title := db.TitleID("main.go - Visual Studio Code")
	db.BeginUsage(s, s+7200, Active)
	db.BeginApp(s, s+3600, app, title)
	db.BeginApp(s+3600, s+7200, app, db.TitleID("other"))
	// Idle detected: everything after s+1800 becomes away.
	db.TruncateFrom(s + 1800)
	db.BeginUsage(s+1800, s+7200, Away)
	st, err := db.Stats(day.Unix(), day.AddDate(0, 0, 1).Unix(), true)
	if err != nil {
		t.Fatal(err)
	}
	if st.Active != 1800 || st.Away != 5400 {
		t.Fatalf("active/away = %d/%d", st.Active, st.Away)
	}
	if len(st.Apps) != 1 || st.Apps[0].Secs != 1800 {
		t.Fatalf("apps: %+v", st.Apps)
	}
	if st.Days[0].First != s || st.Days[0].Last != s+1800 {
		t.Fatalf("attendance: %+v", st.Days[0])
	}
}

func TestAutoTagsAndTimesheet(t *testing.T) {
	db := openTemp(t)
	day := time.Date(2026, 10, 21, 0, 0, 0, 0, time.Local)
	s := day.Add(10 * time.Hour).Unix()
	code := db.AppID("code.exe", "Visual Studio Code", "", nil)
	chat := db.AppID("wechat.exe", "WeChat", "", nil)
	db.BeginApp(s, s+1200, code, db.TitleID("a.go"))
	db.BeginApp(s+1200, s+1260, chat, db.TitleID("chat"))
	db.BeginApp(s+1260, s+2400, code, db.TitleID("b.go"))
	rules := []Rule{{ID: "1", Tags: "Dev", Enabled: true, Match: "all", Conds: []Cond{{Field: "app", Op: "contains", Value: "visual studio"}}}}
	auto, err := db.AutoTags(day.Unix(), day.AddDate(0, 0, 1).Unix(), rules)
	if err != nil {
		t.Fatal(err)
	}
	// The one-minute chat gap is merged into a single Dev block.
	if len(auto) != 1 || auto[0].S != s || auto[0].E != s+2400 {
		t.Fatalf("autotags: %+v", auto)
	}
	db.SetRules(rules)
	if n, _ := db.ApplyAutoTags(day.Unix(), day.AddDate(0, 0, 1).Unix(), false); n != 1 {
		t.Fatalf("applied %d", n)
	}
	ts := db.Timesheet(day.Unix(), day.AddDate(0, 0, 1).Unix(), TimesheetOpts{Round: 900, RoundMode: "up"})
	if len(ts.Rows) != 1 || ts.Rows[0].Cells[0] != 2700 {
		t.Fatalf("timesheet: %+v", ts.Rows)
	}
}

func TestDeleteRangeKeepsOutside(t *testing.T) {
	db := openTemp(t)
	day := time.Date(2026, 10, 22, 0, 0, 0, 0, time.Local)
	s := day.Add(8 * time.Hour).Unix()
	db.BeginUsage(s, s+3*3600, Active)
	db.DeleteRange(s+3600, s+7200, []string{"usage"})
	d, _ := db.Day(day.Unix(), day.AddDate(0, 0, 1).Unix())
	if len(d.Usage) != 2 || d.Usage[0][1] != s+3600 || d.Usage[1][0] != s+7200 {
		t.Fatalf("usage after delete: %v", d.Usage)
	}
}

func TestUsageState(t *testing.T) {
	for key, want := range map[string][2]int{
		"ManicTime/Active":        {Active, 1},
		"ManicTime/Away":          {Away, 1},
		"ManicTime/SessionLocked": {Away, 1},
		"ManicTime/PowerOff":      {0, 0},
	} {
		st, keep := usageState(key)
		if keep != (want[1] == 1) || (keep && st != want[0]) {
			t.Errorf("usageState(%q) = %d, %v", key, st, keep)
		}
	}
}

func TestBaseDomain(t *testing.T) {
	for in, want := range map[string]string{
		"t.bilibili.com":     "bilibili.com",
		"www.bilibili.com":   "bilibili.com",
		"bilibili.com":       "bilibili.com",
		"news.bbc.co.uk":     "bbc.co.uk",
		"www.taobao.com.cn":  "taobao.com.cn",
		"dan-k391.github.io": "dan-k391.github.io",
		"localhost":          "localhost",
		"192.168.1.10":       "192.168.1.10",
		"a.b.c.example.org":  "example.org",
		"mail.google.com":    "google.com",
	} {
		if got := BaseDomain(in); got != want {
			t.Errorf("BaseDomain(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDomain(t *testing.T) {
	for in, want := range map[string]string{
		"https://www.bilibili.com/video/x": "bilibili.com",
		"http://localhost:8080/a":          "localhost",
		"https://user@sub.example.org:443": "sub.example.org",
	} {
		if got := Domain(in); got != want {
			t.Errorf("Domain(%q) = %q, want %q", in, got, want)
		}
	}
}

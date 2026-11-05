package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const keyStatsSample = "\xef\xbb\xbf" + `{
  "2026-10-01": {"date": "2026-10-01T00:00:00", "keyPresses": 10, "leftClicks": 3, "rightClicks": 2, "middleClicks": 1, "sideBackClicks": 1, "sideForwardClicks": 1,
    "mouseDistance": 1000.4, "scrollDistance": 2.5,
    "keyPressCounts": {"A": 4, "Shift+A": 1, "Ctrl+Shift+A": 1, "RCtrl": 2, "VolUp": 1, "Key231": 1},
    "appStats": {"msedge": {"AppName": "msedge", "DisplayName": "Microsoft Edge", "KeyPresses": 7, "LeftClicks": 3, "ScrollDistance": 2.5},
                 "Weixin": {"AppName": "Weixin", "DisplayName": "", "KeyPresses": 3, "RightClicks": 2}}},
  "2026-10-02": {"keyPresses": 5, "keyPressCounts": {"Space": 5}},
  "2026-10-05": {"keyPresses": 99, "keyPressCounts": {"Space": 99}, "appStats": {"msedge": {"KeyPresses": 99}}}
}`

func TestImportKeyStats(t *testing.T) {
	db := openTemp(t)
	at := func(d, h int) int64 { return time.Date(2026, 10, d, h, 0, 0, 0, time.Local).Unix() }
	path := filepath.Join(t.TempDir(), "history.json")
	os.WriteFile(path, []byte(keyStatsSample), 0o644)

	// Timestamps started counting on the 5th: that day stays its own.
	edge := db.AppID("msedge.exe", "Edge", "", nil)
	db.SaveInput(at(5, 15), DayStart(at(5, 15)), Input{Keys: 7}, map[int64]Input{edge: {Keys: 7}}, map[uint16]int64{0x39: 7})

	for run := 1; run <= 2; run++ { // a second import replaces the first
		res, err := db.ImportKeyStats(path)
		if err != nil {
			t.Fatal(err)
		}
		if res.Days != 2 || res.Keys != 15 {
			t.Errorf("run %d: %+v", run, res)
		}
		d, err := db.Day(at(1, 0), at(2, 0))
		if err != nil {
			t.Fatal(err)
		}
		if want := (Input{Keys: 10, Left: 3, Right: 2, Middle: 1, Other: 2, Move: 1000, Scroll: 300}); d.Input != want {
			t.Errorf("run %d: day %+v, want %+v", run, d.Input, want)
		}
		for h, in := range d.InputHours {
			if in != (Input{}) {
				t.Errorf("run %d: hour %d has %+v; KeyStats has no time of day", run, h, in)
			}
		}
		if a := d.InputApps[edge]; a.Keys != 7 || a.Left != 3 || a.Scroll != 300 {
			t.Errorf("run %d: edge %+v", run, a)
		}
		st, err := db.Stats(at(1, 0), at(6, 0), true)
		if err != nil {
			t.Fatal(err)
		}
		if st.Input.Keys != 22 || st.InputKeys[0x1E] != 6 || st.InputKeys[0x11D] != 2 || st.InputKeys[0] != 2 || st.InputKeys[0x39] != 12 {
			t.Errorf("run %d: stats %+v, keys %v", run, st.Input, st.InputKeys)
		}
		if d, _ := db.Day(at(5, 0), at(6, 0)); d.Input.Keys != 7 || d.InputApps[edge].Keys != 7 {
			t.Errorf("run %d: the own day changed: %+v", run, d.Input)
		}
	}
	var name string
	db.sql.QueryRow("SELECT name FROM app WHERE key = 'weixin.exe'").Scan(&name)
	if name != "Weixin" {
		t.Errorf("a new app: %q", name)
	}

	// The export has the same days under "history".
	export := filepath.Join(t.TempDir(), "export.json")
	os.WriteFile(export, []byte(`{"version": 1, "history": {"2026-10-03": {"keyPresses": 8, "keyPressCounts": {"E": 8}}}}`), 0o644)
	if res, err := db.ImportKeyStats(export); err != nil || res.Days != 1 || res.Keys != 8 {
		t.Errorf("export: %+v, %v", res, err)
	}

	if d, _ := db.Day(at(1, 0), at(2, 0)); d.Input.Keys != 10 {
		t.Errorf("days the export doesn't have: %+v", d.Input)
	}

	// A file with only days Timestamps counted itself changes nothing.
	late := filepath.Join(t.TempDir(), "late.json")
	os.WriteFile(late, []byte(`{"2026-10-05": {"keyPresses": 1}}`), 0o644)
	if res, err := db.ImportKeyStats(late); err != nil || res.Days != 0 {
		t.Errorf("late: %+v, %v", res, err)
	}
	if d, _ := db.Day(at(1, 0), at(2, 0)); d.Input.Keys != 10 {
		t.Errorf("after a file with nothing to import: %+v", d.Input)
	}

	// KeyStats for Mac is refused, not mapped wrongly.
	mac := filepath.Join(t.TempDir(), "mac.json")
	os.WriteFile(mac, []byte(`{"history": {"2026-10-03": {"keyPresses": 1, "appStats": {"com.google.Chrome": {"bundleId": "com.google.Chrome"}}}}}`), 0o644)
	if _, err := db.ImportKeyStats(mac); err == nil {
		t.Error("a Mac export: no error")
	}

	if got := db.Earliest(); got != at(1, 0) {
		t.Errorf("earliest: %s, want the first imported day", time.Unix(got, 0))
	}

	// Anything else leaves the data alone.
	bad := filepath.Join(t.TempDir(), "other.json")
	for _, s := range []string{"[1, 2]", `{"name": "something"}`, "not json"} {
		os.WriteFile(bad, []byte(s), 0o644)
		if _, err := db.ImportKeyStats(bad); err == nil {
			t.Errorf("%s: no error", s)
		}
	}
	if d, _ := db.Day(at(3, 0), at(4, 0)); d.Input.Keys != 8 {
		t.Errorf("after a bad file: %+v", d.Input)
	}
}

func TestKeyStatsCode(t *testing.T) {
	for name, want := range map[string]uint16{"A": 0x1E, "Ctrl+V": 0x2F, "Ctrl+Shift+Alt+Win+Z": 0x2C, "Alt+Tab": 0x0F, "F1": 0x3B, "F9": 0x43, "F10": 0x44, "F12": 0x58,
		"0": 0x0B, "1": 0x02, "Num+": 0x4E, "Ctrl+Num+": 0x4E, "NumEnter": 0x11C, "RWin": 0x15C, "VolUp": 0, "Key231": 0} {
		if got := keyStatsCode(name); got != want {
			t.Errorf("%s: %#x, want %#x", name, got, want)
		}
	}
}

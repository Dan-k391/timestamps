package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// What is kept per day stays on its date when the zone changes; so does a
// KeyStats import, which a second import still replaces.
func TestZoneKeepsDates(t *testing.T) {
	db := openTemp(t)
	t.Cleanup(func() { applyZone("") })
	set := func(zone string) {
		s := db.Settings()
		s.Timezone = zone
		if err := db.SaveSettings(s); err != nil {
			t.Fatal(err)
		}
	}
	day := func(date string) (Input, map[int64]Input, int64) {
		a, b, _ := DayRange(date)
		d, err := db.Day(a, b)
		if err != nil {
			t.Fatal(err)
		}
		st, err := db.Stats(a, b, true)
		if err != nil {
			t.Fatal(err)
		}
		return d.Input, d.InputApps, st.InputKeys[0x1E]
	}
	set("Asia/Shanghai")
	app := db.AppID("code.exe", "Code", "", nil)
	at := func(date string) int64 { a, _, _ := DayRange(date); return a }
	for date, n := range map[string]int64{"2026-11-03": 300, "2026-11-04": 400, "2026-11-05": 500} {
		s := at(date) + 12*3600
		db.SaveInput(s-s%InputSlot, at(date), Input{Keys: n}, map[int64]Input{app: {Keys: n}}, map[uint16]int64{0x1E: n})
	}
	path := filepath.Join(t.TempDir(), "history.json")
	os.WriteFile(path, []byte(`{"2026-10-01": {"keyPresses": 10, "keyPressCounts": {"A": 10}, "appStats": {"code": {"KeyPresses": 10}}}}`), 0o644)
	if _, err := db.ImportKeyStats(path); err != nil {
		t.Fatal(err)
	}
	for _, zone := range []string{"UTC", "America/New_York", "Pacific/Auckland", "Asia/Shanghai"} {
		set(zone)
		for date, n := range map[string]int64{"2026-11-03": 300, "2026-11-04": 400, "2026-11-05": 500} {
			if _, apps, keys := day(date); apps[app].Keys != n || keys != n {
				t.Errorf("%s, %s: app %d, key %d, want %d", zone, date, apps[app].Keys, keys, n)
			}
		}
		if in, apps, keys := day("2026-10-01"); in.Keys != 10 || apps[app].Keys != 10 || keys != 10 {
			t.Errorf("%s: import %+v, %d, %d", zone, in, apps[app].Keys, keys)
		}
		if _, err := db.ImportKeyStats(path); err != nil {
			t.Fatal(err)
		}
		if in, _, keys := day("2026-10-01"); in.Keys != 10 || keys != 10 {
			t.Errorf("%s: imported again: %+v, %d", zone, in, keys)
		}
	}
}

func TestZone(t *testing.T) {
	db := openTemp(t)
	t.Cleanup(func() { applyZone("") })
	s := db.Settings()
	s.Timezone = "Asia/Tokyo"
	v := db.Version()
	if err := db.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	if db.Version() == v {
		t.Error("a new timezone should change the version: days read differently")
	}
	a, b, err := DayRange("2026-11-05")
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 11, 4, 15, 0, 0, 0, time.UTC).Unix(); a != want || b != want+86400 {
		t.Errorf("Tokyo's Nov 5: %d–%d, want %d", a, b, want)
	}
	if got := DayStart(a + 3600); got != a {
		t.Errorf("day start: %d, want %d", got, a)
	}

	v = db.Version()
	db.SaveSettings(s)
	if db.Version() != v {
		t.Error("the same timezone should keep the version")
	}

	s.Timezone = "Nowhere/Land"
	db.SaveSettings(s)
	if time.Local != SystemZone || db.Settings().Timezone != "" {
		t.Errorf("an unknown timezone: %v, %q", time.Local, db.Settings().Timezone)
	}
	s.Timezone = "Asia/Tokyo"
	db.SaveSettings(s)
	s.Timezone = ""
	db.SaveSettings(s)
	if time.Local != SystemZone {
		t.Errorf("back to the system's: %v", time.Local)
	}
}

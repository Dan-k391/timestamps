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

// An import doesn't take a date Timestamps counted itself, even when a new
// zone has moved the first slot to the next date.
func TestImportKeepsOwnDate(t *testing.T) {
	db := openTemp(t)
	t.Cleanup(func() { applyZone("") })
	set := func(zone string) {
		s := db.Settings()
		s.Timezone = zone
		if err := db.SaveSettings(s); err != nil {
			t.Fatal(err)
		}
	}
	set("Asia/Shanghai")
	a, _, _ := DayRange("2026-10-10")
	s := a + 23*3600 + 1800
	db.SaveInput(s, a, Input{Keys: 3}, map[int64]Input{1: {Keys: 3}}, map[uint16]int64{0x39: 3})
	set("Asia/Tokyo") // 23:30 in Shanghai is 00:30 of the next date here
	path := filepath.Join(t.TempDir(), "history.json")
	os.WriteFile(path, []byte(`{"2026-10-10": {"keyPresses": 800, "keyPressCounts": {"A": 800}, "appStats": {"x": {"KeyPresses": 800}}}}`), 0o644)
	res, err := db.ImportKeyStats(path)
	if err != nil {
		t.Fatal(err)
	}
	a, b, _ := DayRange("2026-10-10")
	st, _ := db.Stats(a, b, true)
	if res.Days != 0 || st.InputKeys[0x39] != 3 {
		t.Errorf("import %+v, own key %d", res, st.InputKeys[0x39])
	}
}

// Zones a day apart: an imported day's new midnight can be an own slot. The
// change goes through, and an error would leave the old zone whole.
func TestZoneADayApart(t *testing.T) {
	db := openTemp(t)
	t.Cleanup(func() { applyZone("") })
	set := func(zone string) error {
		s := db.Settings()
		s.Timezone = zone
		return db.SaveSettings(s)
	}
	set("Pacific/Kiritimati")
	path := filepath.Join(t.TempDir(), "history.json")
	os.WriteFile(path, []byte(`{"2026-06-10": {"keyPresses": 10, "keyPressCounts": {"A": 10}}}`), 0o644)
	own, _, _ := DayRange("2026-06-11")
	db.SaveInput(own+3600, own, Input{Keys: 3}, nil, map[uint16]int64{0x39: 3})
	if _, err := db.ImportKeyStats(path); err != nil {
		t.Fatal(err)
	}
	// 25 hours west, the midnight of 06-10 is the instant of that own slot.
	if err := set("Pacific/Pago_Pago"); err != nil {
		t.Fatal(err)
	}
	a, b, _ := DayRange("2026-06-10")
	d, err := db.Day(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if d.Input.Keys < 10 {
		t.Errorf("imported day: %+v", d.Input)
	}
	if err := set("Pacific/Kiritimati"); err != nil {
		t.Fatal(err)
	}
	a, b, _ = DayRange("2026-06-10")
	if d, _ = db.Day(a, b); d.Input.Keys != 10 {
		t.Errorf("back: %+v", d.Input)
	}
}

// Days tile time in zones where clocks change at midnight, and a date a zone
// skipped ends the loop over days.
func TestDayBounds(t *testing.T) {
	t.Cleanup(func() { applyZone("") })
	for _, c := range []struct {
		zone, date string
		hours      int64
	}{
		{"America/Havana", "2024-03-10", 23},    // 00:00 skipped
		{"America/Havana", "2024-11-03", 25},    // 00:00 twice
		{"Asia/Beirut", "2024-03-31", 23},       // 00:00 skipped
		{"Africa/Cairo", "2005-04-29", 23},      // 00:00 skipped
		{"America/Sao_Paulo", "2018-02-18", 24}, // 00:00 goes back to 23:00 the day before
		{"America/New_York", "2026-03-08", 23},
		{"Pacific/Apia", "2011-12-30", 0}, // no such date
	} {
		applyZone(c.zone)
		a, b, err := DayRange(c.date)
		if err != nil {
			t.Fatal(err)
		}
		if b-a != c.hours*3600 {
			t.Errorf("%s %s: %d hours, want %d", c.zone, c.date, (b-a)/3600, c.hours)
		}
		if c.hours > 0 {
			if got := DayStart(b - 1); got != a {
				t.Errorf("%s %s: last second starts at %d, want %d", c.zone, c.date, got, a)
			}
			if got := time.Unix(a, 0).Format("2006-01-02"); got != c.date {
				t.Errorf("%s %s: starts on %s", c.zone, c.date, got)
			}
			if got := time.Unix(a-1, 0).Format("2006-01-02"); got >= c.date {
				t.Errorf("%s %s: began before its start, on %s", c.zone, c.date, got)
			}
		}
		from, _, _ := DayRange("2005-01-01")
		_, to, _ := DayRange("2026-12-31")
		bounds := dayStarts(from, to)
		for i := 1; i < len(bounds); i++ {
			if bounds[i] <= bounds[i-1] || DayStart(bounds[i]-1) != bounds[i-1] {
				t.Fatalf("%s: bounds %d, %d don't tile", c.zone, bounds[i-1], bounds[i])
			}
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

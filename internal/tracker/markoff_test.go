package tracker

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"timestamps/internal/store"
)

// Playing sound keeps you active, but doesn't bring you back once away.
func TestSoundKeepsActive(t *testing.T) {
	st := &store.Settings{TreatSoundAsActive: true}
	t0 := time.Now()
	tr := &Tracker{usageState: store.Active}
	tr.lastSound.Store(t0.UnixMilli())
	if last := tr.presence(t0, time.Hour, st); last.UnixMilli() != t0.UnixMilli() {
		t.Errorf("active: last = %v ago, want the sound's time", t0.Sub(last))
	}
	tr.usageState = store.Away
	if last := tr.presence(t0, time.Hour, st); !last.Equal(t0.Add(-time.Hour)) {
		t.Errorf("away: last = %v ago, want the last input's time", t0.Sub(last))
	}
}

// Deleting today clears the totals in memory too, and what isn't saved yet.
func TestDeleteRangeInput(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().Unix()
	day := store.DayStart(now)
	db.SaveInput(now-now%store.InputSlot, day, store.Input{Keys: 100}, nil, nil)
	tr := &Tracker{db: db, inputSlot: now - now%store.InputSlot, inputDay: day, inputToday: store.Input{Keys: 105}, input: store.Input{Keys: 5}}

	if err := tr.DeleteRange(day-2*86400, day-86400, []string{"usage"}); err != nil {
		t.Fatal(err)
	}
	if tr.inputToday.Keys != 105 || tr.inputNew {
		t.Errorf("another day deleted: today %+v, new %v", tr.inputToday, tr.inputNew)
	}
	tr.input = store.Input{Keys: 5}
	if err := tr.DeleteRange(day, day+86400, []string{"usage"}); err != nil {
		t.Fatal(err)
	}
	tr.saveInput()
	if stored, _ := db.InputOn(day); tr.inputToday.Keys != 0 || !tr.inputNew || stored.Keys != 0 {
		t.Errorf("today deleted: memory %+v, stored %+v, new %v", tr.inputToday, stored, tr.inputNew)
	}
}

// Input a save couldn't take stays in its slot until a save can, and settings
// and deletes wait for it.
func TestFailedSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	raw, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err := raw.Exec("CREATE TRIGGER nosave BEFORE INSERT ON input BEGIN SELECT RAISE(ABORT, 'no'); END"); err != nil {
		t.Fatal(err)
	}
	at := store.DayStart(time.Now().Unix()) - 3*86400 + 12*3600
	at -= at % store.InputSlot
	tr := &Tracker{db: db, inputSlot: at, inputDay: store.DayStart(at)}
	tr.countInput(at, store.Input{Keys: 5}, nil)
	tr.countInput(at+store.InputSlot, store.Input{Keys: 3}, nil)
	if err := tr.SaveSettings(db.Settings()); err == nil {
		t.Error("settings saved past a failed save")
	}
	if err := tr.DeleteRange(at, at+1, []string{"usage"}); err == nil {
		t.Error("deleted past a failed save")
	}
	if tr.inputToday.Keys != 8 {
		t.Errorf("today: %+v", tr.inputToday)
	}
	if _, err := raw.Exec("DROP TRIGGER nosave"); err != nil {
		t.Fatal(err)
	}
	if err := tr.saveInput(); err != nil {
		t.Fatal(err)
	}
	tr.saveInput()
	var first, second int64
	raw.QueryRow("SELECT keys FROM input WHERE t = ?", at).Scan(&first)
	raw.QueryRow("SELECT keys FROM input WHERE t = ?", at+store.InputSlot).Scan(&second)
	if first != 5 || second != 3 {
		t.Errorf("slots: %d, %d, want 5, 3", first, second)
	}
}

// A private app's input counts for the app, but not by key.
func TestAppInput(t *testing.T) {
	now := time.Now().Unix()
	tr := &Tracker{appRef: 7, inputSlot: now - now%store.InputSlot, inputDay: store.DayStart(now)}
	in := store.Input{Keys: 2, Left: 1}
	tr.countInput(now, in, map[uint16]int64{0x1E: 2})
	tr.countInput(now, in, map[uint16]int64{0x1E: 2})
	tr.appRef, tr.appPrivate = 8, true
	tr.countInput(now, in, map[uint16]int64{0x1E: 9})
	tr.countInput(now, store.Input{}, nil)
	if a := tr.inputApps[7]; a.Keys != 4 || a.Left != 2 || tr.inputApps[8].Keys != 2 || len(tr.inputApps) != 2 {
		t.Errorf("apps: %+v", tr.inputApps)
	}
	if tr.inputKeys[0x1E] != 4 {
		t.Errorf("keys: %v", tr.inputKeys)
	}
	if tr.input.Keys != 6 || tr.inputToday.Keys != 6 || tr.inputTodayApps[8].Keys != 2 {
		t.Errorf("totals: %+v, today %+v, %+v", tr.input, tr.inputToday, tr.inputTodayApps)
	}
	ev := tr.inputEvent()
	if ev == nil || ev.Input.Keys != 6 || ev.App != 8 || ev.AppInput.Keys != 2 || ev.Keys[0x1E] != 4 || tr.inputEvent() != nil {
		t.Errorf("event: %+v", ev)
	}
}

// After MarkOff the time is off until the next input, once the click's own
// input is past; locking the screen doesn't end it.
func TestMarkOffUntilInput(t *testing.T) {
	tr := &Tracker{usageState: store.Active}
	t0 := time.Now()
	at := func(d time.Duration) time.Time { return t0.Add(d) }

	tr.offAt = t0
	for _, c := range []struct {
		name      string
		now, idle time.Duration
		locked    bool
		off       bool
	}{
		{"the click", time.Second, time.Second, false, true},
		{"the hand leaving the mouse", 10 * time.Second, 8 * time.Second, false, true},
		{"Win+L pressed", 20 * time.Second, 0, false, true},
		{"the lock screen up", 21 * time.Second, time.Second, true, true},
		{"locked, an hour later", time.Hour, time.Hour, true, true},
		{"the PIN typed, unlocked", 2 * time.Hour, 0, false, true},
		{"input since unlocking", 2*time.Hour + offLockWait, 0, false, false},
	} {
		if tr.markedOff(at(c.now), c.idle, c.locked) != c.off {
			t.Errorf("%s: off = %v, want %v", c.name, !c.off, c.off)
		}
	}
	if !tr.offAt.IsZero() || !tr.offBack.IsZero() {
		t.Error("input should end it")
	}

	// A single bump of the mouse ends it too.
	tr.offAt = t0
	tr.markedOff(at(time.Hour), time.Minute, false)
	if tr.markedOff(at(time.Hour+offLockWait), time.Minute+offLockWait, false) {
		t.Error("a bump: still off")
	}

	tr.offAt = t0
	tr.Pause(0)
	if !tr.offAt.IsZero() {
		t.Error("pausing should end it")
	}
}

// Marking off ends the open usage row at once; nothing more is recorded.
func TestMarkOffCloses(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now()
	s := now.Add(-time.Hour).Unix()
	tr := &Tracker{db: db, usageState: store.Active, state: "active", lastTick: now.Add(-time.Second)}
	tr.usage = seg{id: db.BeginUsage(s, s, store.Active), s: s, e: s}
	tr.offAt = now // within offGrace of any input, so step needs no real idle time
	st := db.Settings()
	st.TrackInput = false
	if ev := tr.step(now, &st); ev != "state" || tr.state != "off" || tr.usage.id != 0 {
		t.Fatalf("step: event %q, state %q, usage row %d", ev, tr.state, tr.usage.id)
	}
	if l := tr.Live(); l.State != "off" || l.Since != now.Unix() {
		t.Errorf("live: %s since %d, want off since %d", l.State, l.Since, now.Unix())
	}
	if ev := tr.step(now.Add(time.Second), &st); ev != "" {
		t.Errorf("second step: event %q", ev)
	}
	d, err := db.Day(s-60, now.Unix()+60)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Usage) != 1 || d.Usage[0][0] != s || d.Usage[0][1] != now.Unix() || d.Usage[0][2] != store.Active {
		t.Errorf("usage rows: %v, want one active row ending at the click", d.Usage)
	}
}

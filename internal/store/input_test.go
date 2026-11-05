package store

import (
	"testing"
	"time"
)

func TestInput(t *testing.T) {
	db := openTemp(t)
	at := func(d, h, m int) int64 { return time.Date(2026, 11, d, h, m, 0, 0, time.Local).Unix() }
	slot := func(s int64) int64 { return s - s%InputSlot }
	add := func(s int64, in Input) { db.SaveInput(slot(s), DayStart(s), in, nil, nil) }
	add(at(4, 23, 50), Input{Keys: 1})
	add(at(5, 9, 0), Input{Keys: 10, Left: 2, Move: 100})
	add(at(5, 9, 0), Input{Keys: 5, Right: 1, Middle: 4, Other: 3, Scroll: 240})
	add(at(5, 23, 50), Input{Keys: 2})
	add(at(6, 0, 0), Input{Keys: 100})

	code, browser := db.AppID("code.exe", "Code", "", nil), db.AppID("edge.exe", "Edge", "", nil)
	five := DayStart(at(5, 9, 0))
	db.SaveInput(slot(at(5, 9, 0)), five, Input{}, map[int64]Input{code: {Keys: 7, Left: 1}, browser: {Left: 9, Middle: 2}}, map[uint16]int64{0x1E: 3, 0x11C: 1})
	db.SaveInput(slot(at(5, 10, 0)), five, Input{}, map[int64]Input{code: {Keys: 3}}, map[uint16]int64{0x1E: 2})
	db.SaveInput(slot(at(6, 9, 0)), DayStart(at(6, 9, 0)), Input{}, map[int64]Input{code: {Keys: 50}}, map[uint16]int64{0x1E: 50})

	d, err := db.Day(at(5, 0, 0), at(6, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if want := (Input{Keys: 17, Left: 2, Right: 1, Middle: 4, Other: 3, Move: 100, Scroll: 240}); d.Input != want {
		t.Errorf("day: %+v, want %+v", d.Input, want)
	}
	if len(d.InputHours) != 24 || d.InputHours[9].Keys != 15 || d.InputHours[23].Keys != 2 {
		t.Errorf("hours: %+v", d.InputHours)
	}
	one, err := db.Stats(at(5, 0, 0), at(6, 0, 0), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(one.InputApps) != 2 || one.InputApps[0].ID != code || one.InputApps[0].Keys != 10 || one.InputApps[0].Name != "Code" || one.InputApps[1].Left != 9 || one.InputApps[1].Middle != 2 {
		t.Errorf("apps: %+v", one.InputApps)
	}
	if one.InputKeys[0x1E] != 5 || one.InputKeys[0x11C] != 1 {
		t.Errorf("keys: %v", one.InputKeys)
	}
	if got, apps := db.InputOn(at(5, 0, 0)); got.Keys != 17 || apps[code].Keys != 10 || apps[browser].Left != 9 {
		t.Errorf("InputOn: %+v, %+v", got, apps)
	}
	if d.InputApps[code].Keys != 10 || d.InputApps[browser].Left != 9 {
		t.Errorf("day apps: %+v", d.InputApps)
	}

	st, err := db.Stats(at(4, 0, 0), at(7, 0, 0), true)
	if err != nil {
		t.Fatal(err)
	}
	if st.Input.Keys != 118 || st.Days[0].Input.Keys != 1 || st.Days[1].Input.Keys != 17 || st.Days[2].Input.Keys != 100 {
		t.Errorf("stats: %d in all, days %+v", st.Input.Keys, st.Days)
	}

	if err := db.DeleteRange(at(5, 0, 0), at(6, 0, 0), []string{"apps"}); err != nil {
		t.Fatal(err)
	}
	if st, _ := db.Stats(at(5, 0, 0), at(6, 0, 0), true); len(st.InputApps) != 0 || st.InputKeys[0x1E] != 5 {
		t.Errorf("after deleting the applications: %+v, %v", st.InputApps, st.InputKeys)
	}

	if err := db.DeleteRange(at(5, 0, 0), at(6, 0, 0), []string{"usage"}); err != nil {
		t.Fatal(err)
	}
	if d, _ := db.Day(at(5, 0, 0), at(6, 0, 0)); d.Input != (Input{}) {
		t.Errorf("after deleting the day: %+v", d.Input)
	}
	if d, _ := db.Day(at(6, 0, 0), at(7, 0, 0)); d.Input.Keys != 100 {
		t.Errorf("the next day: %+v", d.Input)
	}
	if st, _ := db.Stats(at(5, 0, 0), at(6, 0, 0), true); len(st.InputApps) != 0 || len(st.InputKeys) != 0 {
		t.Errorf("after deleting the day: %+v, %v", st.InputApps, st.InputKeys)
	}
	if st, _ := db.Stats(at(6, 0, 0), at(7, 0, 0), true); st.InputKeys[0x1E] != 50 {
		t.Errorf("the next day: %v", st.InputKeys)
	}
}

package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"os"
	"strings"
	"time"
)

// keyStatsDay is a day of KeyStats' history.json or of its export; the field
// names differ in case between the two, which encoding/json doesn't mind.
type keyStatsDay struct {
	keyStatsClicks
	KeyPresses     int64
	KeyPressCounts map[string]int64
	MouseDistance  float64 // pixels
	ScrollDistance float64 // wheel notches
	AppStats       map[string]keyStatsApp
}

type keyStatsApp struct {
	keyStatsClicks
	DisplayName    string
	BundleID       string // only KeyStats for Mac has it
	KeyPresses     int64
	ScrollDistance float64
}

type keyStatsClicks struct {
	LeftClicks, RightClicks, MiddleClicks, SideBackClicks, SideForwardClicks int64
}

// keyStatsScan gives the scan code (see web/js/keyboard.js) of a KeyStats key
// name. Other names (media keys, Key231) stay out of the keyboard, as 0.
var keyStatsScan = func() map[string]uint16 {
	m := map[string]uint16{
		"Esc": 0x01, "-": 0x0C, "=": 0x0D, "Backspace": 0x0E, "Tab": 0x0F, "[": 0x1A, "]": 0x1B, "Enter": 0x1C, ";": 0x27, "'": 0x28, "`": 0x29,
		"\\": 0x2B, ",": 0x33, ".": 0x34, "/": 0x35, "Space": 0x39, "CapsLock": 0x3A, "F11": 0x57, "F12": 0x58, "NumLock": 0x45, "ScrollLock": 0x46,
		"PrintScreen": 0x137, "Pause": 0x145, "Insert": 0x152, "Delete": 0x153, "Home": 0x147, "End": 0x14F, "PageUp": 0x149, "PageDown": 0x151,
		"Up": 0x148, "Down": 0x150, "Left": 0x14B, "Right": 0x14D,
		"LShift": 0x2A, "RShift": 0x36, "Shift": 0x2A, "LCtrl": 0x1D, "RCtrl": 0x11D, "Ctrl": 0x1D, "LAlt": 0x38, "RAlt": 0x138, "Alt": 0x38,
		"LWin": 0x15B, "RWin": 0x15C, "Apps": 0x15D,
		"Num0": 0x52, "Num1": 0x4F, "Num2": 0x50, "Num3": 0x51, "Num4": 0x4B, "Num5": 0x4C, "Num6": 0x4D, "Num7": 0x47, "Num8": 0x48, "Num9": 0x49,
		"Num.": 0x53, "Num/": 0x135, "Num*": 0x37, "Num-": 0x4A, "Num+": 0x4E, "NumEnter": 0x11C,
		// As Windows names them, which older KeyStats versions wrote.
		"Caps Lock": 0x3A, "Scroll Lock": 0x46, "Num Lock": 0x45, "Sys Req": 0x137, "Right Shift": 0x36, "Right Ctrl": 0x11D, "Right Alt": 0x138,
		"Num Del": 0x53, "Num /": 0x135, "Num *": 0x37, "Num -": 0x4A, "Num +": 0x4E, "Num Enter": 0x11C,
	}
	for c := '0'; c <= '9'; c++ {
		m["Num "+string(c)] = m["Num"+string(c)]
	}
	for _, row := range []struct {
		keys string
		code uint16
	}{{"1234567890", 0x02}, {"QWERTYUIOP", 0x10}, {"ASDFGHJKL", 0x1E}, {"ZXCVBNM", 0x2C}} {
		for i, c := range row.keys {
			m[string(c)] = row.code + uint16(i)
		}
	}
	for i := uint16(0); i < 10; i++ {
		m["F"+string(rune('1'+i))] = 0x3B + i
	}
	m["F10"] = 0x44
	return m
}()

// keyStatsCode is the scan code of a name in keyPressCounts. A shortcut
// (Ctrl+Shift+C) counts for its key: KeyStats doesn't count the modifiers of
// one, so the keys still add up to the day's total.
func keyStatsCode(name string) uint16 {
	for _, mod := range []string{"Ctrl+", "Shift+", "Alt+", "Win+"} {
		name = strings.TrimPrefix(name, mod)
	}
	return keyStatsScan[name]
}

func (c keyStatsClicks) input() Input {
	return Input{Left: c.LeftClicks, Right: c.RightClicks, Middle: c.MiddleClicks, Other: c.SideBackClicks + c.SideForwardClicks}
}

func keyStatsHistory(path string) (map[string]keyStatsDay, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	var export struct{ History map[string]keyStatsDay }
	if err := json.Unmarshal(b, &export); err == nil && export.History != nil {
		return export.History, nil
	}
	var history map[string]keyStatsDay
	if err := json.Unmarshal(b, &history); err != nil {
		return nil, errors.New("not a KeyStats history file")
	}
	return history, nil
}

// KeyStatsImport tells what ImportKeyStats brought in.
type KeyStatsImport struct {
	Days int   `json:"days"`
	Keys int64 `json:"keys"`
}

// ImportKeyStats reads KeyStats' daily history. Only the days before
// Timestamps started counting go in, so nothing is counted twice, replacing
// those days of an earlier import. KeyStats has no time of day: a day goes in
// as one row at its start, marked src 1, which the chart by hour leaves out.
func (db *DB) ImportKeyStats(path string) (KeyStatsImport, error) {
	var res KeyStatsImport
	history, err := keyStatsHistory(path)
	if err != nil {
		return res, err
	}
	settingsMu.RLock() // a zone change moving the days would miss the ones going in
	defer settingsMu.RUnlock()
	cutoff := DayStart(time.Now().Unix())
	var first *int64
	db.sql.QueryRow("SELECT MIN(t) FROM input WHERE src = 0").Scan(&first)
	if first != nil {
		cutoff = DayStart(*first)
	}
	// Days keep their date when the zone changes and slots don't, so the first own date can be before the first slot's.
	// shortcut: a date imported, then counted on after a move to a zone where it still runs, then imported again, loses those hours' keys and apps; to keep them, mark such a day own in SaveInput.
	var own *int64
	db.sql.QueryRow("SELECT MIN(day) FROM (SELECT day FROM input_app UNION SELECT day FROM input_key) WHERE day NOT IN (SELECT t FROM input WHERE src = 1)").Scan(&own)
	if own != nil && *own < cutoff {
		cutoff = *own
	}

	days := map[int64]keyStatsDay{}
	known := false
	for date, d := range history {
		start, _, err := DayRange(date)
		if err != nil {
			continue
		}
		known = true
		for _, a := range d.AppStats {
			if a.BundleID != "" {
				// shortcut: the Mac version names keys and apps differently and counts scrolling in tenths of a line; to import it, map those.
				return res, errors.New("the history of KeyStats for Mac can't be imported yet")
			}
		}
		if start < cutoff {
			days[start] = d
		}
	}
	if !known {
		return res, errors.New("not a KeyStats history file")
	}
	// An app's name differs between days (a blank, or a window title for python): take the latest, as AppID keeps the first it gets.
	appKey := func(name string) string { return strings.TrimSuffix(strings.ToLower(name), ".exe") + ".exe" }
	type named struct {
		day  int64
		name string
	}
	latest := map[string]named{}
	for day, d := range days {
		for name, a := range d.AppStats {
			n := latest[appKey(name)]
			if a.DisplayName != "" && (n.name == "" || day > n.day || day == n.day && a.DisplayName > n.name) {
				latest[appKey(name)] = named{day, a.DisplayName}
			}
		}
	}
	// Apps first: AppID writes outside the transaction below.
	apps := map[string]int64{}
	for _, d := range days {
		for name := range d.AppStats {
			if _, ok := apps[name]; !ok {
				display := latest[appKey(name)].name
				if display == "" {
					display = name
				}
				apps[name] = db.AppID(appKey(name), display, "", nil)
			}
		}
	}

	tx, err := db.sql.Begin()
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	for day, d := range days {
		tx.Exec("DELETE FROM input WHERE src = 1 AND t = ?", day)
		tx.Exec("DELETE FROM input_app WHERE day = ?", day)
		tx.Exec("DELETE FROM input_key WHERE day = ?", day)
		in := d.input()
		in.Keys, in.Move, in.Scroll = d.KeyPresses, int64(math.Round(d.MouseDistance)), int64(math.Round(d.ScrollDistance*120))
		if _, err := tx.Exec("INSERT INTO input(t, keys, lclick, rclick, mclick, oclick, move, scroll, src) VALUES(?, ?, ?, ?, ?, ?, ?, ?, 1)",
			day, in.Keys, in.Left, in.Right, in.Middle, in.Other, in.Move, in.Scroll); err != nil {
			return KeyStatsImport{}, err
		}
		byApp := map[int64]Input{}
		for name, a := range d.AppStats {
			n := a.input()
			n.Keys, n.Scroll = a.KeyPresses, int64(math.Round(a.ScrollDistance*120))
			sum := byApp[apps[name]]
			sum.Add(n)
			byApp[apps[name]] = sum
		}
		for id, a := range byApp {
			if _, err := tx.Exec("INSERT INTO input_app(day, app, keys, lclick, rclick, mclick, oclick, move, scroll) VALUES(?, ?, ?, ?, ?, ?, ?, 0, ?)",
				day, id, a.Keys, a.Left, a.Right, a.Middle, a.Other, a.Scroll); err != nil {
				return KeyStatsImport{}, err
			}
		}
		byKey := map[uint16]int64{}
		for name, n := range d.KeyPressCounts {
			byKey[keyStatsCode(name)] += n
		}
		for k, n := range byKey {
			if _, err := tx.Exec("INSERT INTO input_key(day, key, n) VALUES(?, ?, ?)", day, k, n); err != nil {
				return KeyStatsImport{}, err
			}
		}
		res.Days++
		res.Keys += in.Keys
	}
	if err := tx.Commit(); err != nil {
		return KeyStatsImport{}, err
	}
	db.touch()
	return res, nil
}

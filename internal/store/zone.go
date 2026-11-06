package store

import (
	"time"
	_ "time/tzdata" // Windows has no zone database for time.LoadLocation
)

// SystemZone is the computer's own timezone, whatever Settings.Timezone says.
var SystemZone = time.Local

// applyZone makes a timezone (an IANA name, "" for the system's) the local
// time of the whole program, so days, hours and the tray ring all follow it.
func applyZone(name string) {
	loc := SystemZone
	if name != "" {
		if l, err := time.LoadLocation(name); err == nil {
			loc = l
		}
	}
	time.Local = loc
}

// rekeyDays moves what is kept per day, under a midnight of the zone old, to
// the midnight of the same date now: it has no time of day to follow. It saves
// the settings naming the new zone with them, so the two can't part.
func (db *DB) rekeyDays(old *time.Location, settings string) error {
	rows, err := db.sql.Query("SELECT day FROM input_app UNION SELECT day FROM input_key UNION SELECT t FROM input WHERE src = 1")
	if err != nil {
		return err
	}
	var days []int64
	for rows.Next() {
		var d int64
		rows.Scan(&d)
		days = append(days, d)
	}
	rows.Close()
	tx, err := db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, day := range days {
		y, m, d := time.Unix(day, 0).In(old).Date()
		to := -dateStart(y, m, d) // negative for now: one date's new midnight can be another's old one
		for _, q := range []string{
			"INSERT INTO input_app(day, app, keys, lclick, rclick, mclick, oclick, move, scroll) SELECT ?, app, keys, lclick, rclick, mclick, oclick, move, scroll FROM input_app WHERE day = ? ON CONFLICT(day, app)" + inputSum,
			"INSERT INTO input_key(day, key, n) SELECT ?, key, n FROM input_key WHERE day = ? ON CONFLICT(day, key) DO UPDATE SET n = n + excluded.n",
			"UPDATE input SET t = ? WHERE src = 1 AND t = ?",
		} {
			if _, err := tx.Exec(q, to, day); err != nil {
				return err
			}
		}
	}
	for _, q := range []string{"DELETE FROM input_app WHERE day >= 0", "DELETE FROM input_key WHERE day >= 0",
		"UPDATE input_app SET day = -day", "UPDATE input_key SET day = -day", "UPDATE OR IGNORE input SET t = -t WHERE t < 0",
		"UPDATE input SET t = 1 - t WHERE t < 0"} { // zones a day apart: an own slot is on that midnight, and a second on is still that date
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	if _, err := tx.Exec("INSERT INTO meta(k, v) VALUES('settings', ?) ON CONFLICT(k) DO UPDATE SET v = excluded.v", settings); err != nil {
		return err
	}
	return tx.Commit()
}

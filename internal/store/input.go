package store

import "time"

// InputSlot is the length of a row of the input table in seconds: a quarter
// of an hour, as the days of every time zone start on one.
const InputSlot = 15 * 60

// Input is keyboard and mouse use: totals, never what was typed or where.
type Input struct {
	Keys   int64 `json:"keys"`
	Left   int64 `json:"left"`
	Right  int64 `json:"right"`
	Middle int64 `json:"middle"`
	Other  int64 `json:"other"`
	Move   int64 `json:"move"`   // pixels
	Scroll int64 `json:"scroll"` // wheel delta, 120 a notch
}

// Add adds o to in.
func (in *Input) Add(o Input) {
	in.Keys += o.Keys
	in.Left += o.Left
	in.Right += o.Right
	in.Middle += o.Middle
	in.Other += o.Other
	in.Move += o.Move
	in.Scroll += o.Scroll
}

// InputApp is an application's keyboard and mouse use.
type InputApp struct {
	Total
	Input
}

// DayStart returns when the local day of t began.
func DayStart(t int64) int64 {
	y, m, d := time.Unix(t, 0).Date()
	return dateStart(y, m, d)
}

const inputSum = ` DO UPDATE SET keys = keys + excluded.keys, lclick = lclick + excluded.lclick, rclick = rclick + excluded.rclick,
	mclick = mclick + excluded.mclick, oclick = oclick + excluded.oclick, move = move + excluded.move, scroll = scroll + excluded.scroll`

// SaveInput adds in to the slot starting at t, and apps (by app id) and keys
// (by scan code) to the day. Keys are kept per day, so that no order of
// typing can be read from them. On an error nothing is saved.
func (db *DB) SaveInput(t, day int64, in Input, apps map[int64]Input, keys map[uint16]int64) error {
	tx, err := db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("INSERT INTO input(t, keys, lclick, rclick, mclick, oclick, move, scroll) VALUES(?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(t)"+inputSum,
		t, in.Keys, in.Left, in.Right, in.Middle, in.Other, in.Move, in.Scroll); err != nil {
		return err
	}
	for id, a := range apps {
		if _, err := tx.Exec("INSERT INTO input_app(day, app, keys, lclick, rclick, mclick, oclick, move, scroll) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(day, app)"+inputSum,
			day, id, a.Keys, a.Left, a.Right, a.Middle, a.Other, a.Move, a.Scroll); err != nil {
			return err
		}
	}
	for k, n := range keys {
		if _, err := tx.Exec("INSERT INTO input_key(day, key, n) VALUES(?, ?, ?) ON CONFLICT(day, key) DO UPDATE SET n = n + excluded.n", day, k, n); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	db.touch()
	return nil
}

// InputOn returns the input of the local day starting at day: in all, and by
// app id.
func (db *DB) InputOn(day int64) (Input, map[int64]Input) {
	y, m, d := time.Unix(day, 0).Date()
	next := dateStart(y, m, d+1)
	apps, err := db.inputApps(day, next)
	if err != nil {
		apps = map[int64]Input{}
	}
	in, err := db.inputDays([]int64{day, next}, false)
	if err != nil {
		return Input{}, apps
	}
	return in[0], apps
}

// inputApps returns the input by app id of the days starting in [a, b).
func (db *DB) inputApps(a, b int64) (map[int64]Input, error) {
	rows, err := db.sql.Query("SELECT app, SUM(keys), SUM(lclick), SUM(rclick), SUM(mclick), SUM(oclick), SUM(move), SUM(scroll) FROM input_app WHERE day >= ? AND day < ? GROUP BY app", a, b)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]Input{}
	for rows.Next() {
		var id int64
		var in Input
		rows.Scan(&id, &in.Keys, &in.Left, &in.Right, &in.Middle, &in.Other, &in.Move, &in.Scroll)
		out[id] = in
	}
	return out, nil
}

// inputDetail fills in the input per application and per key of the days
// starting in [a, b).
func (db *DB) inputDetail(st *Stats, a, b int64) error {
	rows, err := db.sql.Query(`SELECT g.app, a.key, a.name, a.color, COALESCE(length(a.icon), 0), g.k, g.l, g.r, g.c, g.o, g.m, g.s FROM
		(SELECT app, SUM(keys) k, SUM(lclick) l, SUM(rclick) r, SUM(mclick) c, SUM(oclick) o, SUM(move) m, SUM(scroll) s FROM input_app WHERE day >= ? AND day < ? GROUP BY app) g
		JOIN app a ON a.id = g.app ORDER BY g.k + g.l + g.r + g.c + g.o DESC, g.m DESC`, a, b)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var x InputApp
		rows.Scan(&x.ID, &x.Key, &x.Name, &x.Color, &x.IconV, &x.Keys, &x.Left, &x.Right, &x.Middle, &x.Other, &x.Move, &x.Scroll)
		x.Icon = x.IconV > 0
		st.InputApps = append(st.InputApps, x)
	}
	keys, err := db.sql.Query("SELECT key, SUM(n) FROM input_key WHERE day >= ? AND day < ? GROUP BY key", a, b)
	if err != nil {
		return err
	}
	defer keys.Close()
	for keys.Next() {
		var k uint16
		var n int64
		keys.Scan(&k, &n)
		st.InputKeys[k] = n
	}
	return nil
}

// inputDays sums the input between each two of the ascending bounds; timed
// leaves out imported days, which have no time of day.
func (db *DB) inputDays(bounds []int64, timed bool) ([]Input, error) {
	out := make([]Input, len(bounds)-1)
	where := ""
	if timed {
		where = " AND src = 0"
	}
	rows, err := db.sql.Query("SELECT t, keys, lclick, rclick, mclick, oclick, move, scroll FROM input WHERE t >= ? AND t < ?"+where+" ORDER BY t", bounds[0], bounds[len(bounds)-1])
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	i := 0
	for rows.Next() {
		var t int64
		var in Input
		rows.Scan(&t, &in.Keys, &in.Left, &in.Right, &in.Middle, &in.Other, &in.Move, &in.Scroll)
		for t >= bounds[i+1] {
			i++
		}
		out[i].Add(in)
	}
	return out, nil
}

package store

import (
	"database/sql"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"time"
)

// Progress reports import progress.
type Progress func(stage string, done, total int64)

func mtTime(v any) int64 {
	switch t := v.(type) {
	case time.Time:
		return t.Unix()
	case string:
		if i := strings.IndexByte(t, '.'); i > 0 {
			t = t[:i]
		}
		for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", time.RFC3339} {
			if p, err := time.ParseInLocation(layout, t, time.UTC); err == nil {
				return p.Unix()
			}
		}
	case []byte:
		return mtTime(string(t))
	}
	return 0
}

// ImportManicTime copies the history from a ManicTime ManicTimeReports.db
// into this database. Previously imported data is replaced, and only time
// before this app started tracking is imported so nothing is counted twice.
func (db *DB) ImportManicTime(path string, progress Progress) (map[string]int64, error) {
	src, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(path)+"?mode=ro&_busy_timeout=15000")
	if err != nil {
		return nil, err
	}
	defer src.Close()
	if err := src.Ping(); err != nil {
		return nil, err
	}

	reports := map[string]int64{}
	rows, err := src.Query("SELECT ReportId, SchemaName FROM Ar_Timeline")
	if err != nil {
		return nil, fmt.Errorf("not a ManicTime reports database: %w", err)
	}
	for rows.Next() {
		var id int64
		var schema string
		rows.Scan(&id, &schema)
		reports[schema] = id
	}
	rows.Close()

	cutoff := int64(math.MaxInt64)
	var own sql.NullInt64
	db.sql.QueryRow("SELECT MIN(s) FROM usage_seg WHERE src = 0").Scan(&own)
	if own.Valid {
		cutoff = own.Int64
	}
	clipEnd := func(s, e int64) (int64, int64, bool) {
		if e > cutoff {
			e = cutoff
		}
		return s, e, e > s && s > 0
	}

	for _, t := range []string{TUsage, TApp, TDoc, TTag} {
		if _, err := db.sql.Exec("DELETE FROM " + t + " WHERE src = 1"); err != nil {
			return nil, err
		}
	}
	counts := map[string]int64{}
	count := func(rid int64) int64 {
		var n int64
		src.QueryRow("SELECT COUNT(*) FROM Ar_Activity WHERE ReportId = ?", rid).Scan(&n)
		return n
	}

	// batch runs fn inside transactions of up to 20k rows so the tracker can
	// keep writing between batches.
	type batcher struct {
		tx    *sql.Tx
		n     int
		stmts map[string]*sql.Stmt
	}
	b := &batcher{}
	begin := func() error {
		tx, err := db.sql.Begin()
		if err != nil {
			return err
		}
		b.tx, b.n, b.stmts = tx, 0, map[string]*sql.Stmt{}
		return nil
	}
	stmt := func(q string) *sql.Stmt {
		if s := b.stmts[q]; s != nil {
			return s
		}
		s, _ := b.tx.Prepare(q)
		b.stmts[q] = s
		return s
	}
	flush := func(force bool) error {
		b.n++
		if !force && b.n < 20000 {
			return nil
		}
		for _, s := range b.stmts {
			s.Close()
		}
		if err := b.tx.Commit(); err != nil {
			return err
		}
		return begin()
	}
	if err := begin(); err != nil {
		return nil, err
	}
	defer func() {
		if b.tx != nil {
			b.tx.Rollback()
		}
	}()

	// Computer usage.
	if rid, ok := reports["ManicTime/ComputerUsage"]; ok {
		total := count(rid)
		rows, err := src.Query(`SELECT a.StartUtcTime, a.EndUtcTime, COALESCE(g.Key, a.Name) FROM Ar_Activity a
			LEFT JOIN Ar_Group g ON g.ReportId = a.ReportId AND g.GroupId = a.GroupId
			WHERE a.ReportId = ? ORDER BY a.StartUtcTime`, rid)
		if err != nil {
			return nil, err
		}
		var done int64
		for rows.Next() {
			var sv, ev any
			var key string
			rows.Scan(&sv, &ev, &key)
			done++
			s, e, ok := clipEnd(mtTime(sv), mtTime(ev))
			if !ok {
				continue
			}
			state, keep := usageState(key)
			if !keep {
				continue
			}
			stmt("INSERT INTO usage_seg(s, e, state, src) VALUES(?, ?, ?, 1)").Exec(s, e, state)
			db.noteLen(TUsage, e-s)
			counts["usage"]++
			if done%5000 == 0 {
				progress("usage", done, total)
			}
			if err := flush(false); err != nil {
				rows.Close()
				return nil, err
			}
		}
		rows.Close()
	}

	// Applications: groups first, then activities.
	if rid, ok := reports["ManicTime/Applications"]; ok {
		groups := map[int64]int64{}
		rows, err := src.Query("SELECT GroupId, Key, Name, Color, COALESCE(Icon32, Icon16) FROM Ar_Group WHERE ReportId = ?", rid)
		if err != nil {
			return nil, err
		}
		type grp struct {
			gid              int64
			key, name, color string
			icon             []byte
		}
		var gs []grp
		for rows.Next() {
			var g grp
			var color sql.NullString
			rows.Scan(&g.gid, &g.key, &g.name, &color, &g.icon)
			g.color = color.String
			gs = append(gs, g)
		}
		rows.Close()
		for _, g := range gs {
			key := strings.ToLower(strings.SplitN(g.key, ";", 2)[0])
			if key == "" {
				key = strings.ToLower(g.name)
			}
			var id int64
			err := b.tx.QueryRow("SELECT id FROM app WHERE key = ?", key).Scan(&id)
			if err == sql.ErrNoRows {
				color := strings.TrimPrefix(g.color, "#")
				if len(color) != 6 {
					color = ColorFor(key)
				}
				var icon any
				if len(g.icon) > 8 && string(g.icon[1:4]) == "PNG" {
					icon = g.icon
				}
				res, err := b.tx.Exec("INSERT INTO app(key, name, path, color, icon) VALUES(?, ?, '', ?, ?)", key, g.name, color, icon)
				if err != nil {
					return nil, err
				}
				id, _ = res.LastInsertId()
			} else if err != nil {
				return nil, err
			}
			groups[g.gid] = id
		}
		db.mu.Lock()
		db.apps = map[string]int64{}
		db.mu.Unlock()

		total := count(rid)
		rows, err = src.Query("SELECT StartUtcTime, EndUtcTime, Name, GroupId FROM Ar_Activity WHERE ReportId = ? ORDER BY StartUtcTime", rid)
		if err != nil {
			return nil, err
		}
		titles := map[string]int64{}
		var done int64
		for rows.Next() {
			var sv, ev any
			var name string
			var gid sql.NullInt64
			rows.Scan(&sv, &ev, &name, &gid)
			done++
			s, e, ok := clipEnd(mtTime(sv), mtTime(ev))
			app := groups[gid.Int64]
			if !ok || app == 0 {
				continue
			}
			tid, ok := titles[name]
			if !ok {
				if err := stmt("SELECT id FROM title WHERE text = ?").QueryRow(name).Scan(&tid); err != nil {
					res, err := stmt("INSERT INTO title(text) VALUES(?)").Exec(name)
					if err != nil {
						continue
					}
					tid, _ = res.LastInsertId()
				}
				titles[name] = tid
			}
			stmt("INSERT INTO app_seg(s, e, app, title, src) VALUES(?, ?, ?, ?, 1)").Exec(s, e, app, tid)
			db.noteLen(TApp, e-s)
			counts["apps"]++
			if done%5000 == 0 {
				progress("apps", done, total)
			}
			if err := flush(false); err != nil {
				rows.Close()
				return nil, err
			}
		}
		rows.Close()
	}

	// Documents (files and websites).
	if rid, ok := reports["ManicTime/Documents"]; ok {
		total := count(rid)
		rows, err := src.Query(`SELECT a.StartUtcTime, a.EndUtcTime, a.Name, COALESCE(g.Name, ''), COALESCE(g.GroupType, ''), COALESCE(g.Key, '')
			FROM Ar_Activity a LEFT JOIN Ar_Group g ON g.ReportId = a.ReportId AND g.GroupId = a.GroupId
			WHERE a.ReportId = ? ORDER BY a.StartUtcTime`, rid)
		if err != nil {
			return nil, err
		}
		docs := map[string]int64{}
		var done int64
		for rows.Next() {
			var sv, ev any
			var name, gname, gtype, gkey string
			rows.Scan(&sv, &ev, &name, &gname, &gtype, &gkey)
			done++
			s, e, ok := clipEnd(mtTime(sv), mtTime(ev))
			if !ok || name == "" {
				continue
			}
			kind := KindFile
			lname := strings.ToLower(name)
			if strings.Contains(strings.ToLower(gtype), "web") || strings.HasPrefix(lname, "http://") || strings.HasPrefix(lname, "https://") {
				kind = KindWeb
			}
			grp, dname := gname, name
			if kind == KindFile {
				dname = filepath.Base(name)
				if grp == "" {
					grp = dname
				}
			} else if grp == "" {
				grp = Domain(name)
			}
			ck := fmt.Sprint(kind) + name
			did, ok := docs[ck]
			if !ok {
				if err := stmt("SELECT id FROM doc WHERE kind = ? AND key = ?").QueryRow(kind, name).Scan(&did); err != nil {
					res, err := stmt("INSERT INTO doc(kind, key, name, grp) VALUES(?, ?, ?, ?)").Exec(kind, name, dname, grp)
					if err != nil {
						continue
					}
					did, _ = res.LastInsertId()
				}
				docs[ck] = did
			}
			stmt("INSERT INTO doc_seg(s, e, doc, app, src) VALUES(?, ?, ?, 0, 1)").Exec(s, e, did)
			db.noteLen(TDoc, e-s)
			counts["docs"]++
			if done%5000 == 0 {
				progress("docs", done, total)
			}
			if err := flush(false); err != nil {
				rows.Close()
				return nil, err
			}
		}
		rows.Close()
		db.mu.Lock()
		db.docs = map[string]int64{}
		db.mu.Unlock()
	}

	// Tags.
	if rid, ok := reports["ManicTime/Tags"]; ok {
		rows, err := src.Query(`SELECT a.StartUtcTime, a.EndUtcTime, a.Name, COALESCE(l.Name, ''), COALESCE(a.Notes, ''), COALESCE(a.IsBillable, l.IsBillable, 0)
			FROM Ar_Activity a LEFT JOIN Ar_GroupList l ON l.ReportId = a.ReportId AND l.GroupListId = a.GroupListId
			WHERE a.ReportId = ? ORDER BY a.StartUtcTime`, rid)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var sv, ev any
			var name, list, notes string
			var bill int
			rows.Scan(&sv, &ev, &name, &list, &notes, &bill)
			s, e, ok := clipEnd(mtTime(sv), mtTime(ev))
			tags := NormalizeTags(list)
			if tags == "" {
				tags = NormalizeTags(name)
			}
			if !ok || tags == "" {
				continue
			}
			stmt("INSERT INTO tag_seg(s, e, tags, notes, billable, src) VALUES(?, ?, ?, ?, ?, 1)").Exec(s, e, tags, notes, bill != 0)
			db.noteLen(TTag, e-s)
			counts["tags"]++
			if err := flush(false); err != nil {
				rows.Close()
				return nil, err
			}
		}
		rows.Close()
	}
	for _, s := range b.stmts {
		s.Close()
	}
	if err := b.tx.Commit(); err != nil {
		return nil, err
	}
	b.tx = nil
	progress("finishing", 1, 1)
	db.sql.Exec("ANALYZE")
	db.SetMeta("import.manictime", time.Now().Format(time.RFC3339))
	db.touch()
	return counts, nil
}

// usageState maps a ManicTime computer-usage group (ManicTime/Active, Away,
// SessionLocked, PowerOff) to a usage state. Power off is not imported: time
// without records is how this app represents a switched-off computer.
func usageState(key string) (int, bool) {
	k := strings.ToLower(key)
	switch {
	case strings.Contains(k, "poweroff") || strings.Contains(k, "power off"):
		return 0, false
	case strings.Contains(k, "away") || strings.Contains(k, "lock"):
		return Away, true
	}
	return Active, true
}

// Domain extracts the host of a URL without a leading "www.".
func Domain(u string) string {
	s := u
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndexByte(s, '@'); i >= 0 {
		s = s[i+1:]
	}
	if h, _, ok := strings.Cut(s, ":"); ok && !strings.Contains(h, "[") {
		s = h
	}
	return strings.TrimPrefix(strings.ToLower(s), "www.")
}

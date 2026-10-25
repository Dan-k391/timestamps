package store

import (
	"database/sql"
	"strings"
)

// AppInfo describes a tracked application.
type AppInfo struct {
	ID    int64  `json:"id"`
	Key   string `json:"key"`
	Name  string `json:"name"`
	Path  string `json:"path"`
	Color string `json:"color"`
	Icon  bool   `json:"icon"`
	IconV int64  `json:"iconV,omitempty"` // changes when the icon does (cache key)
}

// DocInfo describes a tracked document (file, folder or web page).
type DocInfo struct {
	ID   int64  `json:"id"`
	Kind int    `json:"kind"`
	Key  string `json:"key"`
	Name string `json:"name"`
	Grp  string `json:"grp"`
}

// AppID returns the id for an application key, creating it on first use.
// icon is only called for new applications.
func (db *DB) AppID(key, name, path string, icon func() []byte) int64 {
	key = strings.ToLower(key)
	db.mu.Lock()
	id, ok := db.apps[key]
	db.mu.Unlock()
	if ok {
		return id
	}
	err := db.sql.QueryRow("SELECT id FROM app WHERE key = ?", key).Scan(&id)
	if err == sql.ErrNoRows {
		var ic []byte
		if icon != nil {
			ic = icon()
		}
		res, err := db.sql.Exec("INSERT INTO app(key, name, path, color, icon) VALUES(?, ?, ?, ?, ?)", key, name, path, ColorFor(key), ic)
		if err != nil {
			return 0
		}
		id, _ = res.LastInsertId()
	} else if err != nil {
		return 0
	} else if path != "" {
		// Fill in path/icon for apps first seen through an import.
		var hasIcon bool
		db.sql.QueryRow("SELECT icon IS NOT NULL FROM app WHERE id = ?", id).Scan(&hasIcon)
		if !hasIcon && icon != nil {
			if ic := icon(); ic != nil {
				db.sql.Exec("UPDATE app SET icon = ? WHERE id = ?", ic, id)
			}
		}
		db.sql.Exec("UPDATE app SET path = ? WHERE id = ? AND path = ''", path, id)
	}
	db.mu.Lock()
	db.apps[key] = id
	db.mu.Unlock()
	return id
}

// TitleID interns a window title.
func (db *DB) TitleID(text string) int64 {
	db.mu.Lock()
	id, ok := db.titles[text]
	db.mu.Unlock()
	if ok {
		return id
	}
	db.sql.QueryRow("INSERT INTO title(text) VALUES(?) ON CONFLICT(text) DO UPDATE SET text = text RETURNING id", text).Scan(&id)
	if id == 0 {
		db.sql.QueryRow("SELECT id FROM title WHERE text = ?", text).Scan(&id)
	}
	db.mu.Lock()
	if len(db.titles) > 20000 {
		db.titles = map[string]int64{}
	}
	db.titles[text] = id
	db.mu.Unlock()
	return id
}

// DocID interns a document.
func (db *DB) DocID(kind int, key, name, grp string) int64 {
	ck := string(rune('0'+kind)) + key
	db.mu.Lock()
	id, ok := db.docs[ck]
	db.mu.Unlock()
	if ok {
		return id
	}
	db.sql.QueryRow(`INSERT INTO doc(kind, key, name, grp) VALUES(?, ?, ?, ?)
		ON CONFLICT(kind, key) DO UPDATE SET name = excluded.name RETURNING id`, kind, key, name, grp).Scan(&id)
	db.mu.Lock()
	if len(db.docs) > 20000 {
		db.docs = map[string]int64{}
	}
	db.docs[ck] = id
	db.mu.Unlock()
	return id
}

// BeginUsage starts a computer-usage segment.
func (db *DB) BeginUsage(s, e int64, state int) int64 {
	return db.insert(TUsage, "INSERT INTO usage_seg(s, e, state) VALUES(?, ?, ?)", s, e, state)
}

// BeginApp starts an application segment.
func (db *DB) BeginApp(s, e, app, title int64) int64 {
	return db.insert(TApp, "INSERT INTO app_seg(s, e, app, title) VALUES(?, ?, ?, ?)", s, e, app, title)
}

// BeginDoc starts a document segment.
func (db *DB) BeginDoc(s, e, doc, app int64) int64 {
	return db.insert(TDoc, "INSERT INTO doc_seg(s, e, doc, app) VALUES(?, ?, ?, ?)", s, e, doc, app)
}

func (db *DB) insert(table, q string, args ...any) int64 {
	res, err := db.sql.Exec(q, args...)
	if err != nil {
		return 0
	}
	db.touch()
	db.noteLen(table, args[1].(int64)-args[0].(int64))
	id, _ := res.LastInsertId()
	return id
}

// Extend moves the end of a segment.
func (db *DB) Extend(table string, id, s, e int64) {
	if id == 0 {
		return
	}
	db.sql.Exec("UPDATE "+table+" SET e = ? WHERE id = ?", e, id)
	db.noteLen(table, e-s)
	db.touch()
}

// TruncateFrom cuts tracked (non-imported) usage, app and doc segments at
// cutoff: later segments are removed and spanning ones shortened. Used when
// idle time is retroactively reclassified as away.
func (db *DB) TruncateFrom(cutoff int64) {
	tx, err := db.sql.Begin()
	if err != nil {
		return
	}
	for _, t := range []string{TUsage, TApp, TDoc} {
		lo := db.lowS(t, cutoff)
		tx.Exec("DELETE FROM "+t+" WHERE s >= ? AND src = 0", cutoff)
		tx.Exec("UPDATE "+t+" SET e = ? WHERE s >= ? AND s < ? AND e > ? AND src = 0", cutoff, lo, cutoff, cutoff)
	}
	tx.Commit()
	db.touch()
}

// DeleteSeg removes one segment.
func (db *DB) DeleteSeg(table string, id int64) {
	db.sql.Exec("DELETE FROM "+table+" WHERE id = ?", id)
	db.touch()
}

// AddShot records a screenshot.
func (db *DB) AddShot(ts int64, w, h, bytes int) {
	db.sql.Exec("INSERT OR REPLACE INTO shot(ts, w, h, bytes) VALUES(?, ?, ?, ?)", ts, w, h, bytes)
	db.touch()
}

// ShotsBefore lists screenshot timestamps older than ts.
func (db *DB) ShotsBefore(ts int64) []int64 {
	rows, err := db.sql.Query("SELECT ts FROM shot WHERE ts < ? ORDER BY ts", ts)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var t int64
		rows.Scan(&t)
		out = append(out, t)
	}
	return out
}

// ShotsBetween lists screenshot timestamps in [a, b).
func (db *DB) ShotsBetween(a, b int64) []int64 {
	rows, err := db.sql.Query("SELECT ts FROM shot WHERE ts >= ? AND ts < ? ORDER BY ts", a, b)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var t int64
		rows.Scan(&t)
		out = append(out, t)
	}
	return out
}

// DeleteShots removes screenshot rows.
func (db *DB) DeleteShots(ts []int64) {
	tx, err := db.sql.Begin()
	if err != nil {
		return
	}
	for _, t := range ts {
		tx.Exec("DELETE FROM shot WHERE ts = ?", t)
	}
	tx.Commit()
	db.touch()
}

// SetAppIcon replaces an app's icon when it differs from the stored one.
func (db *DB) SetAppIcon(id int64, png []byte) {
	res, err := db.sql.Exec("UPDATE app SET icon = ? WHERE id = ? AND (icon IS NULL OR icon != ?)", png, id, png)
	if err != nil {
		return
	}
	if n, _ := res.RowsAffected(); n > 0 {
		db.touch()
	}
}

// AppIcon returns the PNG icon of an app.
func (db *DB) AppIcon(id int64) []byte {
	var b []byte
	db.sql.QueryRow("SELECT icon FROM app WHERE id = ?", id).Scan(&b)
	return b
}

// AppIconV returns the icon cache key of an app (0 when it has no icon).
func (db *DB) AppIconV(id int64) int64 {
	var v int64
	db.sql.QueryRow("SELECT COALESCE(length(icon), 0) FROM app WHERE id = ?", id).Scan(&v)
	return v
}

// Apps lists all applications.
func (db *DB) Apps() []AppInfo {
	rows, err := db.sql.Query("SELECT id, key, name, path, color, COALESCE(length(icon), 0) FROM app ORDER BY name")
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []AppInfo
	for rows.Next() {
		var a AppInfo
		rows.Scan(&a.ID, &a.Key, &a.Name, &a.Path, &a.Color, &a.IconV)
		a.Icon = a.IconV > 0
		out = append(out, a)
	}
	return out
}

// FixGenericNames renames apps stored under a framework name such as
// "Electron" to their program name ("dischord.exe" -> "Dischord").
func (db *DB) FixGenericNames(generic func(string) bool) {
	rows, err := db.sql.Query("SELECT id, key, name FROM app")
	if err != nil {
		return
	}
	type fix struct {
		id   int64
		name string
	}
	var fixes []fix
	for rows.Next() {
		var id int64
		var key, name string
		rows.Scan(&id, &key, &name)
		if !generic(name) {
			continue
		}
		base := strings.TrimSuffix(key, ".exe")
		if base == "" || generic(base) {
			continue
		}
		fixes = append(fixes, fix{id, strings.ToUpper(base[:1]) + base[1:]})
	}
	rows.Close()
	for _, f := range fixes {
		db.sql.Exec("UPDATE app SET name = ? WHERE id = ?", f.name, f.id)
	}
	if len(fixes) > 0 {
		db.touch()
	}
}

// SetAppColor changes an app colour.
func (db *DB) SetAppColor(id int64, color string) {
	db.sql.Exec("UPDATE app SET color = ? WHERE id = ?", strings.TrimPrefix(color, "#"), id)
	db.touch()
}

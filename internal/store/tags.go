package store

import (
	"database/sql"
	"sort"
	"strings"
)

// NormalizeTags turns "a,b ,, c" into "a, b, c".
func NormalizeTags(s string) string {
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.Join(strings.Fields(p), " "); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, ", ")
}

// clearRange trims, splits or removes tag segments overlapping [s, e),
// leaving the segment with id keep untouched.
func clearRange(tx *sql.Tx, lo, s, e, keep int64) error {
	rows, err := tx.Query("SELECT id, s, e, tags, notes, billable, src FROM tag_seg WHERE s >= ? AND s < ? AND e > ? AND id != ?", lo, e, s, keep)
	if err != nil {
		return err
	}
	type seg struct {
		id, s, e    int64
		tags, notes string
		bill        bool
		src         int
	}
	var segs []seg
	for rows.Next() {
		var g seg
		rows.Scan(&g.id, &g.s, &g.e, &g.tags, &g.notes, &g.bill, &g.src)
		segs = append(segs, g)
	}
	rows.Close()
	for _, g := range segs {
		switch {
		case g.s < s && g.e > e:
			tx.Exec("UPDATE tag_seg SET e = ? WHERE id = ?", s, g.id)
			tx.Exec("INSERT INTO tag_seg(s, e, tags, notes, billable, src) VALUES(?, ?, ?, ?, ?, ?)", e, g.e, g.tags, g.notes, g.bill, g.src)
		case g.s < s:
			tx.Exec("UPDATE tag_seg SET e = ? WHERE id = ?", s, g.id)
		case g.e > e:
			tx.Exec("UPDATE tag_seg SET s = ? WHERE id = ?", e, g.id)
		default:
			tx.Exec("DELETE FROM tag_seg WHERE id = ?", g.id)
		}
	}
	return nil
}

// AddTag puts a tag segment on [s, e), replacing whatever was tagged there.
func (db *DB) AddTag(s, e int64, tags, notes string, billable bool) (int64, error) {
	tags = NormalizeTags(tags)
	if e <= s || tags == "" {
		return 0, nil
	}
	tx, err := db.sql.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if err := clearRange(tx, db.lowS(TTag, s), s, e, 0); err != nil {
		return 0, err
	}
	res, err := tx.Exec("INSERT INTO tag_seg(s, e, tags, notes, billable) VALUES(?, ?, ?, ?, ?)", s, e, tags, notes, billable)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	db.noteLen(TTag, e-s)
	db.touch()
	return id, nil
}

// UpdateTag edits a tag segment (time range, tags, notes, billable).
func (db *DB) UpdateTag(id, s, e int64, tags, notes string, billable bool) error {
	tags = NormalizeTags(tags)
	if e <= s || tags == "" {
		return db.DeleteTags([]int64{id})
	}
	tx, err := db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := clearRange(tx, db.lowS(TTag, s), s, e, id); err != nil {
		return err
	}
	if _, err := tx.Exec("UPDATE tag_seg SET s = ?, e = ?, tags = ?, notes = ?, billable = ? WHERE id = ?", s, e, tags, notes, billable, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	db.noteLen(TTag, e-s)
	db.touch()
	return nil
}

// ClearTags removes tagging from [s, e).
func (db *DB) ClearTags(s, e int64) error {
	tx, err := db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := clearRange(tx, db.lowS(TTag, s), s, e, 0); err != nil {
		return err
	}
	db.touch()
	return tx.Commit()
}

// DeleteTags removes tag segments by id.
func (db *DB) DeleteTags(ids []int64) error {
	tx, err := db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range ids {
		tx.Exec("DELETE FROM tag_seg WHERE id = ?", id)
	}
	db.touch()
	return tx.Commit()
}

// SplitTag cuts a tag segment in two at time at.
func (db *DB) SplitTag(id, at int64) error {
	var t Tag
	err := db.sql.QueryRow("SELECT id, s, e, tags, notes, billable FROM tag_seg WHERE id = ?", id).Scan(&t.ID, &t.S, &t.E, &t.Tags, &t.Notes, &t.Billable)
	if err != nil || at <= t.S || at >= t.E {
		return err
	}
	tx, err := db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	tx.Exec("UPDATE tag_seg SET e = ? WHERE id = ?", at, id)
	tx.Exec("INSERT INTO tag_seg(s, e, tags, notes, billable) VALUES(?, ?, ?, ?, ?)", at, t.E, t.Tags, t.Notes, t.Billable)
	db.touch()
	return tx.Commit()
}

// TagSummary is one row of the tag manager.
type TagSummary struct {
	Tags     string `json:"tags"`
	Secs     int64  `json:"secs"`
	Count    int64  `json:"count"`
	Last     int64  `json:"last"`
	Color    string `json:"color"`
	Billable bool   `json:"billable"`
}

// AllTags lists every distinct tag combination with totals.
func (db *DB) AllTags() []TagSummary {
	metas := db.TagMetas()
	out := []TagSummary{}
	rows, err := db.sql.Query("SELECT tags, SUM(e - s), COUNT(*), MAX(e) FROM tag_seg GROUP BY tags ORDER BY MAX(e) DESC")
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var t TagSummary
		rows.Scan(&t.Tags, &t.Secs, &t.Count, &t.Last)
		m := metas[t.Tags]
		t.Color, t.Billable = m.Color, m.Billable
		if t.Color == "" {
			t.Color = ColorFor(t.Tags)
		}
		out = append(out, t)
	}
	return out
}

// RecentTags returns the most recently used tag combinations.
func (db *DB) RecentTags(n int) []string {
	out := []string{}
	rows, err := db.sql.Query("SELECT tags FROM tag_seg GROUP BY tags ORDER BY MAX(id) DESC LIMIT ?", n)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var t string
		rows.Scan(&t)
		out = append(out, t)
	}
	return out
}

// RenameTag renames a tag combination, or a single tag inside every
// combination when from contains no comma.
func (db *DB) RenameTag(from, to string) error {
	from, to = NormalizeTags(from), NormalizeTags(to)
	if from == "" || to == "" || from == to {
		return nil
	}
	combos := db.AllTags()
	tx, err := db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, c := range combos {
		var next string
		if c.Tags == from {
			next = to
		} else if !strings.Contains(from, ",") {
			parts := strings.Split(c.Tags, ", ")
			changed := false
			for i, p := range parts {
				if p == from {
					parts[i], changed = to, true
				}
			}
			if !changed {
				continue
			}
			next = NormalizeTags(strings.Join(parts, ","))
		} else {
			continue
		}
		tx.Exec("UPDATE tag_seg SET tags = ? WHERE tags = ?", next, c.Tags)
		tx.Exec("INSERT OR IGNORE INTO tag_meta(tag, color, billable) SELECT ?, color, billable FROM tag_meta WHERE tag = ?", next, c.Tags)
	}
	db.touch()
	return tx.Commit()
}

// DeleteTagEverywhere removes every segment with the given combination.
func (db *DB) DeleteTagEverywhere(tags string) error {
	_, err := db.sql.Exec("DELETE FROM tag_seg WHERE tags = ?", tags)
	db.touch()
	return err
}

// SetTagMeta stores colour / billable for a tag combination.
func (db *DB) SetTagMeta(tag, color string, billable bool) error {
	_, err := db.sql.Exec(`INSERT INTO tag_meta(tag, color, billable) VALUES(?, ?, ?)
		ON CONFLICT(tag) DO UPDATE SET color = excluded.color, billable = excluded.billable`, tag, strings.TrimPrefix(color, "#"), billable)
	db.touch()
	return err
}

// TagNames returns every individual tag (for autocompletion).
func (db *DB) TagNames() []string {
	set := map[string]bool{}
	for _, c := range db.AllTags() {
		for _, p := range strings.Split(c.Tags, ", ") {
			set[p] = true
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

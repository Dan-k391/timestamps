package store

import (
	"fmt"
	"os"
	"time"
)

func fmtDur(secs int64) string {
	return fmt.Sprintf("%d:%02d:%02d", secs/3600, secs/60%60, secs%60)
}

func fmtTime(t int64) string { return time.Unix(t, 0).In(time.Local).Format("2006-01-02 15:04:05") }

// ExportRows returns a timeline over [a, b) as a header row plus data rows.
func (db *DB) ExportRows(timeline string, a, b int64) ([][]string, error) {
	d, err := db.Day(a, b)
	if err != nil {
		return nil, err
	}
	var out [][]string
	switch timeline {
	case "usage":
		out = append(out, []string{"Start", "End", "Duration", "State"})
		for _, r := range d.Usage {
			state := "Active"
			if r[2] == Away {
				state = "Away"
			}
			out = append(out, []string{fmtTime(r[0]), fmtTime(r[1]), fmtDur(r[1] - r[0]), state})
		}
	case "apps":
		out = append(out, []string{"Start", "End", "Duration", "Application", "Process", "Window title"})
		for _, r := range d.Apps {
			app := d.AppMap[r[2]]
			out = append(out, []string{fmtTime(r[0]), fmtTime(r[1]), fmtDur(r[1] - r[0]), app.Name, app.Key, d.TitMap[r[3]]})
		}
	case "docs":
		out = append(out, []string{"Start", "End", "Duration", "Type", "Name", "Path / URL", "Group", "Application"})
		for _, r := range d.Docs {
			doc := d.DocMap[r[2]]
			kind := "File"
			if doc.Kind == KindWeb {
				kind = "Website"
			}
			out = append(out, []string{fmtTime(r[0]), fmtTime(r[1]), fmtDur(r[1] - r[0]), kind, doc.Name, doc.Key, doc.Grp, d.AppMap[r[3]].Name})
		}
	case "tags":
		out = append(out, []string{"Start", "End", "Duration", "Tags", "Notes", "Billable"})
		for _, t := range d.Tags {
			bill := "No"
			if t.Billable {
				bill = "Yes"
			}
			out = append(out, []string{fmtTime(t.S), fmtTime(t.E), fmtDur(t.E - t.S), t.Tags, t.Notes, bill})
		}
	default:
		return nil, fmt.Errorf("unknown timeline %q", timeline)
	}
	return out, nil
}

// DeleteRange removes data of the given timelines inside [a, b), trimming
// segments that cross the edges.
func (db *DB) DeleteRange(a, b int64, timelines []string) error {
	tx, err := db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	tables := map[string][2]string{
		"usage": {TUsage, "state, src"},
		"apps":  {TApp, "app, title, src"},
		"docs":  {TDoc, "doc, app, src"},
		"tags":  {TTag, "tags, notes, billable, src"},
	}
	for _, tl := range timelines {
		def, ok := tables[tl]
		if !ok {
			continue
		}
		t, cols := def[0], def[1]
		lo := db.lowS(t, a)
		// Segments covering the whole range keep their part after b.
		tx.Exec("INSERT INTO "+t+"(s, e, "+cols+") SELECT ?, e, "+cols+" FROM "+t+" WHERE s >= ? AND s < ? AND e > ?", b, lo, a, b)
		tx.Exec("DELETE FROM "+t+" WHERE s >= ? AND e <= ?", a, b)
		tx.Exec("UPDATE "+t+" SET e = ? WHERE s >= ? AND s < ? AND e > ?", a, lo, a, a)
		tx.Exec("UPDATE "+t+" SET s = ? WHERE s >= ? AND s < ? AND e > ?", b, a, b, b)
	}
	if contains(timelines, "shots") {
		tx.Exec("DELETE FROM shot WHERE ts >= ? AND ts < ?", a, b)
	}
	db.touch()
	return tx.Commit()
}

func contains(l []string, s string) bool {
	for _, v := range l {
		if v == s {
			return true
		}
	}
	return false
}

// Backup writes a consistent copy of the database to path.
func (db *DB) Backup(path string) error {
	os.Remove(path)
	_, err := db.sql.Exec("VACUUM INTO ?", path)
	return err
}

// Info summarises database contents.
type Info struct {
	Usage, Apps, Docs, Tags, Shots, Titles int64
	First                                  int64
	Imported                               string
}

// Info returns row counts for the settings page.
func (db *DB) Info() Info {
	var i Info
	db.sql.QueryRow("SELECT COUNT(*) FROM usage_seg").Scan(&i.Usage)
	db.sql.QueryRow("SELECT COUNT(*) FROM app_seg").Scan(&i.Apps)
	db.sql.QueryRow("SELECT COUNT(*) FROM doc_seg").Scan(&i.Docs)
	db.sql.QueryRow("SELECT COUNT(*) FROM tag_seg").Scan(&i.Tags)
	db.sql.QueryRow("SELECT COUNT(*) FROM shot").Scan(&i.Shots)
	db.sql.QueryRow("SELECT COUNT(*) FROM title").Scan(&i.Titles)
	db.sql.QueryRow("SELECT COALESCE(MIN(s), 0) FROM usage_seg").Scan(&i.First)
	i.Imported = db.Meta("import.manictime")
	return i
}

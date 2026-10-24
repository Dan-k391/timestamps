package store

import (
	"sort"
	"strings"
	"time"
)

// TimesheetOpts controls how tag time is grouped and rounded.
type TimesheetOpts struct {
	Level     int    // 0 = whole combination, n = first n tags
	Round     int64  // rounding step in seconds, 0 = none
	RoundMode string // nearest | up | down
	NeverZero bool   // round non-zero cells up to at least one step
	Billable  bool   // only billable time
}

// TimesheetRow is one tag row.
type TimesheetRow struct {
	Key   string   `json:"key"`
	Color string   `json:"color"`
	Cells []int64  `json:"cells"`
	Total int64    `json:"total"`
	Notes []string `json:"notes"`
}

// Timesheet is a tag × day grid.
type Timesheet struct {
	Days   []string       `json:"days"`
	Rows   []TimesheetRow `json:"rows"`
	Totals []int64        `json:"totals"`
	Total  int64          `json:"total"`
}

func roundSecs(v int64, o TimesheetOpts) int64 {
	if o.Round <= 0 || v == 0 {
		return v
	}
	var r int64
	switch o.RoundMode {
	case "up":
		r = (v + o.Round - 1) / o.Round * o.Round
	case "down":
		r = v / o.Round * o.Round
	default:
		r = (v + o.Round/2) / o.Round * o.Round
	}
	if r == 0 && o.NeverZero {
		r = o.Round
	}
	return r
}

// Timesheet builds the timesheet for [a, b).
func (db *DB) Timesheet(a, b int64, o TimesheetOpts) *Timesheet {
	bounds := dayStarts(a, b)
	n := len(bounds) - 1
	ts := &Timesheet{Days: make([]string, n), Rows: []TimesheetRow{}, Totals: make([]int64, n)}
	for i := 0; i < n; i++ {
		ts.Days[i] = time.Unix(bounds[i], 0).In(time.Local).Format("2006-01-02")
	}
	metas := db.TagMetas()
	rows := map[string]*TimesheetRow{}
	for _, t := range db.TagsIn(a, b) {
		if o.Billable && !t.Billable && !metas[t.Tags].Billable {
			continue
		}
		key := t.Tags
		if o.Level > 0 {
			parts := strings.Split(t.Tags, ", ")
			if len(parts) > o.Level {
				parts = parts[:o.Level]
			}
			key = strings.Join(parts, ", ")
		}
		r := rows[key]
		if r == nil {
			c := metas[key].Color
			if c == "" {
				c = ColorFor(key)
			}
			r = &TimesheetRow{Key: key, Color: c, Cells: make([]int64, n), Notes: []string{}}
			rows[key] = r
		}
		s, e := t.S, t.E
		if s < a {
			s = a
		}
		if e > b {
			e = b
		}
		splitDays(bounds, s, e, func(i int, ps, pe int64) { r.Cells[i] += pe - ps })
		if t.Notes != "" {
			r.Notes = append(r.Notes, t.Notes)
		}
	}
	for _, r := range rows {
		for i, v := range r.Cells {
			r.Cells[i] = roundSecs(v, o)
			r.Total += r.Cells[i]
			ts.Totals[i] += r.Cells[i]
		}
		ts.Total += r.Total
		ts.Rows = append(ts.Rows, *r)
	}
	sort.Slice(ts.Rows, func(i, j int) bool { return ts.Rows[i].Key < ts.Rows[j].Key })
	return ts
}

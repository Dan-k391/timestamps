package store

import (
	"sort"
	"strings"
	"time"
)

// SearchHit is one day with matching activity.
type SearchHit struct {
	Date  string  `json:"date"`
	Secs  int64   `json:"secs"`
	First int64   `json:"first"`
	Items []Total `json:"items"`
}

// SearchResult is the search view payload.
type SearchResult struct {
	Hits  []SearchHit `json:"hits"`
	Total int64       `json:"total"`
	Days  int         `json:"days"`
}

func likePattern(q string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(q) + "%"
}

type span struct {
	s, e int64
	name string
	kind int
	ref  int64
}

// Search finds every window title, app, document and tag matching q in
// [a, b), grouped by day (newest first).
func (db *DB) Search(q string, a, b int64) (*SearchResult, error) {
	res := &SearchResult{Hits: []SearchHit{}}
	q = strings.TrimSpace(q)
	if q == "" {
		return res, nil
	}
	p := likePattern(q)
	var spans []span
	collect := func(kind int, query string, args ...any) error {
		rows, err := db.sql.Query(query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var sp span
			rows.Scan(&sp.s, &sp.e, &sp.name, &sp.ref)
			sp.kind = kind
			if sp.s < a {
				sp.s = a
			}
			if sp.e > b {
				sp.e = b
			}
			if sp.e > sp.s {
				spans = append(spans, sp)
			}
		}
		return rows.Err()
	}
	err := collect(1, `SELECT x.s, x.e, CASE WHEN t.text = '' THEN a.name ELSE t.text END, x.app
		FROM app_seg x JOIN title t ON t.id = x.title JOIN app a ON a.id = x.app
		WHERE x.s >= ? AND x.s < ? AND x.e > ? AND (
			x.title IN (SELECT id FROM title WHERE text LIKE ? ESCAPE '\')
			OR x.app IN (SELECT id FROM app WHERE name LIKE ? ESCAPE '\' OR key LIKE ? ESCAPE '\'))`,
		db.lowS(TApp, a), b, a, p, p, p)
	if err != nil {
		return nil, err
	}
	err = collect(2, `SELECT x.s, x.e, d.key, x.app FROM doc_seg x JOIN doc d ON d.id = x.doc
		WHERE x.s >= ? AND x.s < ? AND x.e > ? AND x.doc IN (SELECT id FROM doc WHERE key LIKE ? ESCAPE '\' OR name LIKE ? ESCAPE '\')`,
		db.lowS(TDoc, a), b, a, p, p)
	if err != nil {
		return nil, err
	}
	err = collect(3, `SELECT s, e, tags, 0 FROM tag_seg WHERE s >= ? AND s < ? AND e > ? AND (tags LIKE ? ESCAPE '\' OR notes LIKE ? ESCAPE '\')`,
		db.lowS(TTag, a), b, a, p, p)
	if err != nil {
		return nil, err
	}
	if len(spans) == 0 {
		return res, nil
	}

	bounds := dayStarts(a, b)
	type dayAgg struct {
		ivals [][2]int64
		items map[string]*Total
		first int64
	}
	days := map[int]*dayAgg{}
	for _, sp := range spans {
		splitDays(bounds, sp.s, sp.e, func(i int, ps, pe int64) {
			d := days[i]
			if d == nil {
				d = &dayAgg{items: map[string]*Total{}}
				days[i] = d
			}
			d.ivals = append(d.ivals, [2]int64{ps, pe})
			if d.first == 0 || ps < d.first {
				d.first = ps
			}
			it := d.items[sp.name]
			if it == nil {
				it = &Total{Key: sp.name, Name: sp.name, Kind: sp.kind, ID: sp.ref}
				d.items[sp.name] = it
			}
			it.Secs += pe - ps
		})
	}
	for i, d := range days {
		sort.Slice(d.ivals, func(x, y int) bool { return d.ivals[x][0] < d.ivals[y][0] })
		var total, cs, ce int64
		for j, iv := range d.ivals {
			if j == 0 || iv[0] > ce {
				total += ce - cs
				cs, ce = iv[0], iv[1]
			} else if iv[1] > ce {
				ce = iv[1]
			}
		}
		total += ce - cs
		hit := SearchHit{Date: time.Unix(bounds[i], 0).In(time.Local).Format("2006-01-02"), Secs: total, First: d.first}
		for _, it := range d.items {
			hit.Items = append(hit.Items, *it)
		}
		sortTotals(hit.Items)
		if len(hit.Items) > 8 {
			hit.Items = hit.Items[:8]
		}
		res.Hits = append(res.Hits, hit)
		res.Total += total
	}
	sort.Slice(res.Hits, func(x, y int) bool { return res.Hits[x].Date > res.Hits[y].Date })
	res.Days = len(res.Hits)
	return res, nil
}

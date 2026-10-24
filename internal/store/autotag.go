package store

import (
	"encoding/json"
	"regexp"
	"strings"
	"sync"
)

// Cond is one condition of an autotag rule.
type Cond struct {
	Field string `json:"field"` // app | title | url | domain | file | any
	Op    string `json:"op"`    // contains | equals | starts | ends | regex | notcontains
	Value string `json:"value"`
}

// Rule tags time automatically when its conditions match.
type Rule struct {
	ID       string `json:"id"`
	Tags     string `json:"tags"`
	Enabled  bool   `json:"enabled"`
	Match    string `json:"match"` // all | any
	Billable bool   `json:"billable"`
	Conds    []Cond `json:"conds"`
}

// Rules loads the autotag rules.
func (db *DB) Rules() []Rule {
	var r []Rule
	json.Unmarshal([]byte(db.Meta("rules")), &r)
	if r == nil {
		r = []Rule{}
	}
	return r
}

// SetRules saves the autotag rules.
func (db *DB) SetRules(r []Rule) error {
	b, _ := json.Marshal(r)
	db.touch()
	return db.SetMeta("rules", string(b))
}

var reCache sync.Map

func (c Cond) test(app, title, url, domain, file string) bool {
	var vals []string
	switch c.Field {
	case "app":
		vals = []string{app}
	case "title":
		vals = []string{title}
	case "url":
		vals = []string{url}
	case "domain":
		vals = []string{domain}
	case "file":
		vals = []string{file}
	default:
		vals = []string{app, title, url, file}
	}
	needle := strings.ToLower(c.Value)
	for _, v := range vals {
		lv := strings.ToLower(v)
		var ok bool
		switch c.Op {
		case "equals":
			ok = lv == needle
		case "starts":
			ok = strings.HasPrefix(lv, needle)
		case "ends":
			ok = strings.HasSuffix(lv, needle)
		case "regex":
			re, _ := reCache.Load(c.Value)
			if re == nil {
				compiled, err := regexp.Compile("(?i)" + c.Value)
				if err != nil {
					return false
				}
				reCache.Store(c.Value, compiled)
				re = compiled
			}
			ok = re.(*regexp.Regexp).MatchString(v)
		case "notcontains":
			ok = !strings.Contains(lv, needle)
		default:
			ok = needle != "" && strings.Contains(lv, needle)
		}
		if ok {
			return true
		}
	}
	return false
}

func (r Rule) matches(app, title, url, domain, file string) bool {
	if !r.Enabled || len(r.Conds) == 0 || NormalizeTags(r.Tags) == "" {
		return false
	}
	any := r.Match == "any"
	for _, c := range r.Conds {
		ok := c.test(app, title, url, domain, file)
		if any && ok {
			return true
		}
		if !any && !ok {
			return false
		}
	}
	return !any
}

// AutoTags evaluates the rules over [a, b) and returns the resulting
// (not stored) tag segments.
func (db *DB) AutoTags(a, b int64, rules []Rule) ([]Tag, error) {
	out := []Tag{}
	active := rules[:0:0]
	for _, r := range rules {
		if r.Enabled && len(r.Conds) > 0 {
			active = append(active, r)
		}
	}
	if len(active) == 0 {
		return out, nil
	}
	d, err := db.Day(a, b)
	if err != nil {
		return nil, err
	}
	const mergeGap, minDur = 120, 60
	di := 0
	for _, seg := range d.Apps {
		s, e := seg[0], seg[1]
		app := d.AppMap[seg[2]]
		title := d.TitMap[seg[3]]
		var url, domain, file string
		for di < len(d.Docs) && d.Docs[di][1] <= s {
			di++
		}
		if di < len(d.Docs) && d.Docs[di][0] < e {
			doc := d.DocMap[d.Docs[di][2]]
			if doc.Kind == KindWeb {
				url, domain = doc.Key, doc.Grp
			} else {
				file = doc.Key
			}
		}
		for _, r := range active {
			if !r.matches(app.Name+" "+app.Key, title, url, domain, file) {
				continue
			}
			tags := NormalizeTags(r.Tags)
			if n := len(out); n > 0 && out[n-1].Tags == tags && s-out[n-1].E <= mergeGap {
				if e > out[n-1].E {
					out[n-1].E = e
				}
			} else {
				out = append(out, Tag{S: s, E: e, Tags: tags, Billable: r.Billable})
			}
			break
		}
	}
	kept := out[:0]
	for _, t := range out {
		if t.S < a {
			t.S = a
		}
		if t.E > b {
			t.E = b
		}
		if t.E-t.S >= minDur {
			kept = append(kept, t)
		}
	}
	return kept, nil
}

// ApplyAutoTags writes autotags for [a, b) onto the tag timeline. Without
// overwrite only untagged gaps are filled.
func (db *DB) ApplyAutoTags(a, b int64, overwrite bool) (int, error) {
	auto, err := db.AutoTags(a, b, db.Rules())
	if err != nil {
		return 0, err
	}
	n := 0
	for _, t := range auto {
		if overwrite {
			if _, err := db.AddTag(t.S, t.E, t.Tags, "", t.Billable); err == nil {
				n++
			}
			continue
		}
		cur := t.S
		for _, ex := range db.TagsIn(t.S, t.E) {
			if ex.S > cur && ex.S-cur >= 30 {
				db.AddTag(cur, ex.S, t.Tags, "", t.Billable)
				n++
			}
			if ex.E > cur {
				cur = ex.E
			}
		}
		if t.E-cur >= 30 {
			db.AddTag(cur, t.E, t.Tags, "", t.Billable)
			n++
		}
	}
	return n, nil
}

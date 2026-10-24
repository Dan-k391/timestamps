package server

import (
	"encoding/json"
	"sync"
	"time"

	"timestamps/internal/store"
)

// Stopwatch tracks time that becomes a tag when stopped.
type Stopwatch struct {
	Start int64  `json:"start"`
	Tags  string `json:"tags"`
	Notes string `json:"notes"`
	Nag   int64  `json:"-"`
}

// Countdown alerts when time is up and can tag the elapsed time.
type Countdown struct {
	Start int64  `json:"start"`
	End   int64  `json:"end"`
	Tags  string `json:"tags"`
	Label string `json:"label"`
}

// Timers owns the stopwatch and countdown.
type Timers struct {
	mu        sync.Mutex
	db        *store.DB
	Stopwatch *Stopwatch `json:"stopwatch"`
	Countdown *Countdown `json:"countdown"`
	notify    func(kind string, secs int64, label string)
	emit      func(string)
	nagMin    func() int
}

func newTimers(db *store.DB, notify func(kind string, secs int64, label string), emit func(string), nagMin func() int) *Timers {
	t := &Timers{db: db, notify: notify, emit: emit, nagMin: nagMin}
	json.Unmarshal([]byte(db.Meta("timers")), t)
	go t.loop()
	return t
}

func (t *Timers) save() {
	b, _ := json.Marshal(t)
	t.db.SetMeta("timers", string(b))
}

// State returns a copy of the timers.
func (t *Timers) State() map[string]any {
	t.mu.Lock()
	defer t.mu.Unlock()
	return map[string]any{"stopwatch": t.Stopwatch, "countdown": t.Countdown, "now": time.Now().Unix()}
}

// StartStopwatch starts (or restarts) the stopwatch.
func (t *Timers) StartStopwatch(tags, notes string, start int64) {
	t.mu.Lock()
	if start == 0 {
		start = time.Now().Unix()
	}
	t.Stopwatch = &Stopwatch{Start: start, Tags: store.NormalizeTags(tags), Notes: notes}
	t.save()
	t.mu.Unlock()
	t.emit("timers")
}

// UpdateStopwatch changes the running stopwatch's tags or start.
func (t *Timers) UpdateStopwatch(tags, notes string, start int64) {
	t.mu.Lock()
	if t.Stopwatch != nil {
		t.Stopwatch.Tags, t.Stopwatch.Notes = store.NormalizeTags(tags), notes
		if start > 0 && start < time.Now().Unix() {
			t.Stopwatch.Start = start
		}
		t.save()
	}
	t.mu.Unlock()
	t.emit("timers")
}

// StopStopwatch stops the stopwatch, saving a tag when it has tags.
func (t *Timers) StopStopwatch(save bool) (int64, error) {
	t.mu.Lock()
	sw := t.Stopwatch
	t.Stopwatch = nil
	t.save()
	t.mu.Unlock()
	t.emit("timers")
	if sw == nil || !save {
		return 0, nil
	}
	tags := sw.Tags
	if tags == "" {
		tags = "Stopwatch"
	}
	return t.db.AddTag(sw.Start, time.Now().Unix(), tags, sw.Notes, false)
}

// StartCountdown starts a countdown of secs seconds.
func (t *Timers) StartCountdown(secs int64, tags, label string) {
	now := time.Now().Unix()
	t.mu.Lock()
	t.Countdown = &Countdown{Start: now, End: now + secs, Tags: store.NormalizeTags(tags), Label: label}
	t.save()
	t.mu.Unlock()
	t.emit("timers")
}

// CancelCountdown stops the countdown without alerting.
func (t *Timers) CancelCountdown() {
	t.mu.Lock()
	t.Countdown = nil
	t.save()
	t.mu.Unlock()
	t.emit("timers")
}

func (t *Timers) loop() {
	for range time.Tick(time.Second) {
		now := time.Now().Unix()
		t.mu.Lock()
		cd := t.Countdown
		var done *Countdown
		if cd != nil && now >= cd.End {
			done, t.Countdown = cd, nil
			t.save()
		}
		sw := t.Stopwatch
		nag := int64(t.nagMin()) * 60
		fireNag := false
		if sw != nil && nag > 0 {
			if sw.Nag == 0 {
				sw.Nag = sw.Start
			}
			if now-sw.Nag >= nag {
				sw.Nag = now
				fireNag = true
			}
		}
		t.mu.Unlock()
		if done != nil {
			if done.Tags != "" {
				t.db.AddTag(done.Start, done.End, done.Tags, done.Label, false)
			}
			t.notify("countdown", done.End-done.Start, done.Label)
			t.emit("timers")
		}
		if fireNag {
			t.notify("stopwatch", now-sw.Start, sw.Tags)
		}
	}
}

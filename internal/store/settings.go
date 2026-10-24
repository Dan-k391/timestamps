package store

import (
	"encoding/json"
	"sync"
)

// Exclusion limits tracking for one application.
type Exclusion struct {
	App  string `json:"app"`  // executable name, e.g. "keepass.exe"
	Mode string `json:"mode"` // titles | all | screenshots
}

// Settings are the user-editable preferences.
type Settings struct {
	AwayMinutes        int         `json:"awayMinutes"`
	TreatSoundAsActive bool        `json:"treatSoundAsActive"`
	TrackTitles        bool        `json:"trackTitles"`
	TrackDocuments     bool        `json:"trackDocuments"`
	TrackPrivate       bool        `json:"trackPrivate"`
	ReadAddressBar     bool        `json:"readAddressBar"`
	AwayPrompt         bool        `json:"awayPrompt"`
	AwayPromptMinutes  int         `json:"awayPromptMinutes"`
	Exclusions         []Exclusion `json:"exclusions"`

	ScheduleOn    bool    `json:"scheduleOn"`
	ScheduleDays  [7]bool `json:"scheduleDays"` // Monday first
	ScheduleStart string  `json:"scheduleStart"`
	ScheduleEnd   string  `json:"scheduleEnd"`

	ShotsOn        bool `json:"shotsOn"`
	ShotsInterval  int  `json:"shotsInterval"` // minutes
	ShotsMaxWidth  int  `json:"shotsMaxWidth"`
	ShotsQuality   int  `json:"shotsQuality"`
	ShotsKeepDays  int  `json:"shotsKeepDays"`
	ShotsSkipSame  bool `json:"shotsSkipSame"`
	ShotsAllScreen bool `json:"shotsAllScreens"`

	Language       string `json:"language"` // auto | en | zh
	Theme          string `json:"theme"`    // system | light | dark
	DurationFormat string `json:"durationFormat"`
	WeekStart      int    `json:"weekStart"` // 1 = Monday, 0 = Sunday
	Clock24        bool   `json:"clock24"`

	RunAtStartup   bool `json:"runAtStartup"`
	StartMinimized bool `json:"startMinimized"`
	CloseToTray    bool `json:"closeToTray"`
	RecentTags     int  `json:"recentTags"`
	StopwatchNag   int  `json:"stopwatchNag"` // minutes, 0 = off

	BackupDays int `json:"backupDays"` // automatic backup interval, 0 = off
	BackupKeep int `json:"backupKeep"`

	DayLayout       []LayoutBlock `json:"dayLayout"` // order and width of the day view blocks
	MergeSubdomains bool          `json:"mergeSubdomains"`
}

// LayoutBlock is one block of the day view board.
type LayoutBlock struct {
	ID string `json:"id"`
	W  string `json:"w"` // 1/3 | 1/2 | 2/3 | full
}

// DefaultSettings mirrors ManicTime's defaults, with screenshots enabled at a
// modest size and interval.
func DefaultSettings() Settings {
	return Settings{
		AwayMinutes: 10, TreatSoundAsActive: true, TrackTitles: true, TrackDocuments: true, TrackPrivate: true,
		ReadAddressBar: true, AwayPrompt: true, AwayPromptMinutes: 10, Exclusions: []Exclusion{},
		ScheduleDays: [7]bool{true, true, true, true, true, false, false}, ScheduleStart: "08:00", ScheduleEnd: "18:00",
		ShotsOn: true, ShotsInterval: 5, ShotsMaxWidth: 1280, ShotsQuality: 60, ShotsKeepDays: 30, ShotsSkipSame: true,
		Language: "auto", Theme: "system", DurationFormat: "hm", WeekStart: 1, Clock24: true,
		CloseToTray: true, RecentTags: 10, BackupKeep: 5, MergeSubdomains: true,
	}
}

var settingsMu sync.RWMutex

// Settings loads settings, filling defaults for missing keys.
func (db *DB) Settings() Settings {
	settingsMu.RLock()
	defer settingsMu.RUnlock()
	s := DefaultSettings()
	if raw := db.Meta("settings"); raw != "" {
		json.Unmarshal([]byte(raw), &s)
	}
	if s.AwayMinutes < 1 {
		s.AwayMinutes = 1
	}
	if s.ShotsInterval < 1 {
		s.ShotsInterval = 1
	}
	if s.Exclusions == nil {
		s.Exclusions = []Exclusion{}
	}
	return s
}

// SaveSettings persists settings.
func (db *DB) SaveSettings(s Settings) error {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	b, _ := json.Marshal(s)
	return db.SetMeta("settings", string(b))
}

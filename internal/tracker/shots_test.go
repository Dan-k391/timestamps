//go:build windows

package tracker

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMoveShots(t *testing.T) {
	root := t.TempDir()
	old, dst := filepath.Join(root, "old"), filepath.Join(root, "new")
	write := func(p string) {
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte("jpg"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ts := time.Date(2026, 10, 1, 12, 0, 0, 0, time.Local).Unix()
	write(shotFile(old, ts, false))
	write(shotFile(old, ts, true))
	write(filepath.Join(old, "2026-10-02", "090000.jpg"))
	write(filepath.Join(old, "notes", "keep.txt"))        // not ours: stays
	write(filepath.Join(dst, "2026-10-01", "235959.jpg")) // already there: merged

	s := NewShooter(nil, old)
	if err := s.Move(dst); err != nil {
		t.Fatal(err)
	}
	if s.Dir() != dst {
		t.Fatalf("Dir = %q", s.Dir())
	}
	for _, p := range []string{shotFile(dst, ts, false), shotFile(dst, ts, true), filepath.Join(dst, "2026-10-02", "090000.jpg"),
		filepath.Join(dst, "2026-10-01", "235959.jpg"), filepath.Join(old, "notes", "keep.txt")} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing %s", p)
		}
	}
	for _, p := range []string{filepath.Join(old, "2026-10-01"), filepath.Join(old, "2026-10-02")} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s should be gone", p)
		}
	}
	if got := s.ShotPath(ts, true); got != shotFile(dst, ts, true) {
		t.Errorf("ShotPath = %q", got)
	}
}

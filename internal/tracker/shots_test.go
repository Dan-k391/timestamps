package tracker

import (
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"
	_ "time/tzdata"
)

func writeFile(t *testing.T, p string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte("jpg"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func TestMoveShots(t *testing.T) {
	root := t.TempDir()
	old, dst := filepath.Join(root, "old"), filepath.Join(root, "new")
	ts := time.Date(2026, 10, 1, 12, 0, 0, 0, time.Local).Unix()
	writeFile(t, shotFile(old, ts))
	writeFile(t, filepath.Join(old, "2026-10-02", "090000.jpg"))
	writeFile(t, filepath.Join(old, "notes", "keep.txt"))        // not ours: stays
	writeFile(t, filepath.Join(dst, "2026-10-01", "235959.jpg")) // already there: merged

	s := NewShooter(nil, old, filepath.Join(root, "thumbs"))
	if err := s.Move(dst); err != nil {
		t.Fatal(err)
	}
	if s.Dir() != dst {
		t.Fatalf("Dir = %q", s.Dir())
	}
	for _, p := range []string{shotFile(dst, ts), filepath.Join(dst, "2026-10-02", "090000.jpg"),
		filepath.Join(dst, "2026-10-01", "235959.jpg"), filepath.Join(old, "notes", "keep.txt")} {
		if !exists(p) {
			t.Errorf("missing %s", p)
		}
	}
	for _, p := range []string{filepath.Join(old, "2026-10-01"), filepath.Join(old, "2026-10-02")} {
		if exists(p) {
			t.Errorf("%s should be gone", p)
		}
	}
	if got := s.ShotPath(ts, false); got != shotFile(dst, ts) {
		t.Errorf("ShotPath = %q", got)
	}
}

// When the clocks go back, 01:00-02:00 comes round twice: the second time
// gets its own file names.
func TestRepeatedHourNames(t *testing.T) {
	loc, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Skip(err)
	}
	defer func(l *time.Location) { time.Local = l }(time.Local)
	time.Local = loc
	first := time.Date(2026, 10, 25, 0, 2, 53, 0, time.UTC).Unix()  // 01:02:53 BST
	second := time.Date(2026, 10, 25, 1, 2, 53, 0, time.UTC).Unix() // 01:02:53 GMT
	normal := time.Date(2026, 10, 25, 3, 0, 0, 0, time.UTC).Unix()
	for ts, want := range map[int64]string{first: "010253", second: "010253b", normal: "030000"} {
		if day, name := shotName(ts); day != "2026-10-25" || name != want {
			t.Errorf("shotName(%d) = %s %s, want %s", ts, day, name, want)
		}
	}
	// Taken before the "b" names: found under the plain name.
	root := t.TempDir()
	s := NewShooter(nil, root, filepath.Join(root, "thumbs"))
	writeFile(t, filepath.Join(root, "2026-10-25", "010253.jpg"))
	if got := s.ShotPath(second, false); got != filepath.Join(root, "2026-10-25", "010253.jpg") {
		t.Errorf("ShotPath(second) = %q", got)
	}
}

func TestThumbnails(t *testing.T) {
	root := t.TempDir()
	dir, thumbs := filepath.Join(root, "shots"), filepath.Join(root, "thumbs")
	s := NewShooter(nil, dir, thumbs)
	a := time.Date(2026, 10, 1, 12, 0, 0, 0, time.Local).Unix()
	b := time.Date(2026, 10, 1, 12, 5, 0, 0, time.Local).Unix()

	// An old thumbnail next to its screenshot moves to the thumbnail folder.
	writeFile(t, shotFile(dir, a))
	writeFile(t, filepath.Join(dir, "2026-10-01", "120000_t.jpg"))
	s.MoveOldThumbs()
	if exists(filepath.Join(dir, "2026-10-01", "120000_t.jpg")) || !exists(shotFile(thumbs, a)) {
		t.Fatal("old thumbnail not moved")
	}
	if got := s.ShotPath(a, true); got != shotFile(thumbs, a) {
		t.Errorf("thumb path = %q", got)
	}

	// A missing thumbnail is made from the screenshot.
	img := image.NewRGBA(image.Rect(0, 0, 640, 400))
	os.MkdirAll(filepath.Dir(shotFile(dir, b)), 0o755)
	if _, err := writeJPEG(shotFile(dir, b), img, 60); err != nil {
		t.Fatal(err)
	}
	if got := s.ShotPath(b, true); got != shotFile(thumbs, b) || !exists(got) {
		t.Fatalf("thumb not made: %q", got)
	}

	s.RemoveFiles([]int64{a, b})
	for _, p := range []string{shotFile(dir, a), shotFile(thumbs, a), shotFile(dir, b), shotFile(thumbs, b)} {
		if exists(p) {
			t.Errorf("%s not removed", p)
		}
	}
}

//go:build windows

package tracker

import (
	"image"
	"image/jpeg"
	"log"
	"math"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"timestamps/internal/store"
	"timestamps/internal/win"
)

// Shooter takes periodic, downscaled screenshots of the active monitor.
type Shooter struct {
	Dir      string
	db       *store.DB
	last     time.Time
	lastSig  []float64
	busy     atomic.Bool
	cleaned  time.Time
	OnShot   func(ts int64)
	ThumbW   int
	interval time.Duration
}

// NewShooter creates a shooter storing images under dir.
func NewShooter(db *store.DB, dir string) *Shooter {
	return &Shooter{Dir: dir, db: db, ThumbW: 320}
}

// ShotPath returns the image path of a screenshot (thumb selects the small one).
func (s *Shooter) ShotPath(ts int64, thumb bool) string {
	t := time.Unix(ts, 0).In(time.Local)
	name := t.Format("150405")
	if thumb {
		name += "_t"
	}
	return filepath.Join(s.Dir, t.Format("2006-01-02"), name+".jpg")
}

func (s *Shooter) maybe(now time.Time, st *store.Settings, fg uintptr, allowed bool) {
	if now.Sub(s.cleaned) > time.Hour {
		s.cleaned = now
		go s.cleanup(st.ShotsKeepDays)
	}
	if !st.ShotsOn || !allowed {
		return
	}
	interval := time.Duration(st.ShotsInterval) * time.Minute
	if !s.last.IsZero() && now.Sub(s.last) < interval {
		return
	}
	if !s.busy.CompareAndSwap(false, true) {
		return
	}
	s.last = now
	cfg := *st
	go func() {
		defer s.busy.Store(false)
		s.capture(now, cfg, fg)
	}()
}

// Capture takes a screenshot immediately (used by "take screenshot now").
func (s *Shooter) Capture(st store.Settings) {
	if !s.busy.CompareAndSwap(false, true) {
		return
	}
	defer s.busy.Store(false)
	st.ShotsSkipSame = false
	s.capture(time.Now(), st, win.ForegroundWindow())
}

func (s *Shooter) capture(now time.Time, st store.Settings, fg uintptr) {
	var r win.RECT
	if st.ShotsAllScreen {
		x, y := win.SystemMetric(76), win.SystemMetric(77)
		r = win.RECT{Left: int32(x), Top: int32(y), Right: int32(x + win.SystemMetric(78)), Bottom: int32(y + win.SystemMetric(79))}
	} else {
		r = win.MonitorRect(fg)
	}
	img, err := win.CaptureRect(r)
	if err != nil {
		log.Println("screenshot:", err)
		return
	}
	maxW := st.ShotsMaxWidth
	if maxW <= 0 || maxW > img.Rect.Dx() {
		maxW = img.Rect.Dx()
	}
	h := int(math.Round(float64(img.Rect.Dy()) * float64(maxW) / float64(img.Rect.Dx())))
	scaled := img
	if maxW != img.Rect.Dx() {
		scaled = boxResize(img, maxW, h)
	}
	sig := signature(scaled)
	if st.ShotsSkipSame && s.lastSig != nil && sigDiff(sig, s.lastSig) < 1.5 {
		return
	}
	s.lastSig = sig
	ts := now.Unix()
	path := s.ShotPath(ts, false)
	os.MkdirAll(filepath.Dir(path), 0o755)
	q := st.ShotsQuality
	if q < 20 || q > 95 {
		q = 60
	}
	n, err := writeJPEG(path, scaled, q)
	if err != nil {
		return
	}
	th := boxResize(scaled, s.ThumbW, int(math.Round(float64(scaled.Rect.Dy())*float64(s.ThumbW)/float64(scaled.Rect.Dx()))))
	writeJPEG(s.ShotPath(ts, true), th, 70)
	s.db.AddShot(ts, scaled.Rect.Dx(), scaled.Rect.Dy(), n)
	if s.OnShot != nil {
		s.OnShot(ts)
	}
}

func writeJPEG(path string, img image.Image, q int) (int, error) {
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	err = jpeg.Encode(f, img, &jpeg.Options{Quality: q})
	st, _ := f.Stat()
	f.Close()
	if err != nil {
		os.Remove(path)
		return 0, err
	}
	return int(st.Size()), nil
}

func (s *Shooter) cleanup(keepDays int) {
	if keepDays <= 0 {
		return
	}
	cut := time.Now().AddDate(0, 0, -keepDays).Unix()
	old := s.db.ShotsBefore(cut)
	for _, ts := range old {
		os.Remove(s.ShotPath(ts, false))
		os.Remove(s.ShotPath(ts, true))
	}
	s.db.DeleteShots(old)
	ents, _ := os.ReadDir(s.Dir)
	for _, e := range ents {
		if e.IsDir() {
			os.Remove(filepath.Join(s.Dir, e.Name())) // only succeeds when empty
		}
	}
}

// boxResize downsamples with area averaging (sharp and alias-free).
func boxResize(src *image.RGBA, w, h int) *image.RGBA {
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	if w <= 0 || h <= 0 {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xs := make([]int, w+1)
	for x := 0; x <= w; x++ {
		xs[x] = x * sw / w
	}
	for y := 0; y < h; y++ {
		y0, y1 := y*sh/h, (y+1)*sh/h
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < w; x++ {
			x0, x1 := xs[x], xs[x+1]
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, b, n uint32
			for yy := y0; yy < y1; yy++ {
				row := src.Pix[yy*src.Stride:]
				for xx := x0; xx < x1; xx++ {
					p := row[xx*4:]
					r += uint32(p[0])
					g += uint32(p[1])
					b += uint32(p[2])
					n++
				}
			}
			o := dst.Pix[y*dst.Stride+x*4:]
			o[0], o[1], o[2], o[3] = uint8(r/n), uint8(g/n), uint8(b/n), 255
		}
	}
	return dst
}

// signature is a 32x18 luminance thumbnail used to skip unchanged screens.
func signature(img *image.RGBA) []float64 {
	t := boxResize(img, 32, 18)
	out := make([]float64, 32*18)
	for i := range out {
		p := t.Pix[i*4:]
		out[i] = 0.299*float64(p[0]) + 0.587*float64(p[1]) + 0.114*float64(p[2])
	}
	return out
}

func sigDiff(a, b []float64) float64 {
	var d float64
	for i := range a {
		d += math.Abs(a[i] - b[i])
	}
	return d / float64(len(a))
}

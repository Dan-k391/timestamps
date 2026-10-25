package server

import (
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// updateAsset reports whether a release file is the Windows setup.
func updateAsset(name string) bool {
	return strings.HasPrefix(name, "Timestamps-Setup-") && strings.HasSuffix(name, ".exe")
}

// installUpdate downloads the newest setup, starts it in update mode and
// quits, so the setup can replace this program and start the new one.
func (s *Server) installUpdate() error {
	info := s.CheckUpdate(false)
	if !info.Available || info.Setup == "" {
		return errors.New("there is no update to install")
	}
	resp, err := updateClient.Get(info.Setup)
	if err != nil {
		return errors.New("couldn't download the update")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("couldn't download the update")
	}
	path := filepath.Join(os.TempDir(), "Timestamps-Setup-"+info.Latest+".exe")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, 300<<20))
	f.Close()
	if err != nil || n < 1<<20 {
		os.Remove(path)
		return errors.New("the download was incomplete")
	}
	if err := exec.Command(path, "/update").Start(); err != nil {
		return err
	}
	go func() {
		time.Sleep(500 * time.Millisecond)
		s.Host.Quit()
	}()
	return nil
}

package server

import (
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// bundleID is the identifier of the macOS app (see assets/Info.plist).
const bundleID = "io.github.zjj2785.timestamps"

// updateAsset reports whether a release file is the zipped macOS app.
func updateAsset(name string) bool {
	return strings.HasPrefix(name, "Timestamps-") && strings.HasSuffix(name, "-macos.zip")
}

// appBundle returns the .app folder an executable belongs to, or "".
func appBundle(exe string) string {
	dir := filepath.Dir(exe) // .../Name.app/Contents/MacOS
	if filepath.Base(dir) == "MacOS" && filepath.Base(filepath.Dir(dir)) == "Contents" {
		if app := filepath.Dir(filepath.Dir(dir)); strings.HasSuffix(app, ".app") {
			return app
		}
	}
	return ""
}

var plistString = regexp.MustCompile(`<key>(CFBundleIdentifier|CFBundleShortVersionString)</key>\s*<string>([^<]*)</string>`)

// bundleInfo reads the identifier and version from an app's Info.plist.
func bundleInfo(plist []byte) (id, version string) {
	for _, m := range plistString.FindAllSubmatch(plist, -1) {
		if string(m[1]) == "CFBundleIdentifier" {
			id = string(m[2])
		} else {
			version = string(m[2])
		}
	}
	return id, version
}

// swapScript replaces the app once this process ($1) has ended: the old
// app ($2) is moved aside, the new one ($3) put in its place (the old one
// comes back if that fails), and the app is opened again. $4 is the
// download folder, removed at the end.
const swapScript = `pid=$1; app=$2; new=$3; tmp=$4
while kill -0 "$pid" 2>/dev/null; do sleep 0.2; done
if mv "$app" "$tmp/previous.app"; then
	if mv "$new" "$app"; then rm -rf "$tmp/previous.app"; else mv "$tmp/previous.app" "$app"; fi
fi
open "$app"
rm -rf "$tmp"`

// installUpdate downloads the newest app, checks it and quits; a script
// then puts it in place of this app and opens it. When this app can't be
// replaced (not in an app bundle, run from a temporary copy, or in a folder
// it can't write to), the release page opens instead.
func (s *Server) installUpdate() error {
	info := s.CheckUpdate(false)
	if !info.Available || info.Setup == "" {
		return errors.New("there is no update to install")
	}
	exe, _ := os.Executable()
	app := appBundle(exe)
	manual := func(why string) error {
		if info.Page != "" {
			s.Host.Open(info.Page)
		}
		return errors.New(why + " Download the new version from the release page.")
	}
	if app == "" || strings.Contains(app, "/AppTranslocation/") {
		return manual("Timestamps isn't running from the Applications folder.")
	}
	probe, err := os.CreateTemp(filepath.Dir(app), ".timestamps-update-*")
	if err != nil {
		return manual("Timestamps can't replace itself in " + filepath.Dir(app) + ".")
	}
	probe.Close()
	os.Remove(probe.Name())

	tmp, err := os.MkdirTemp("", "timestamps-update-")
	if err != nil {
		return err
	}
	fail := func(msg string) error {
		os.RemoveAll(tmp)
		return errors.New(msg)
	}
	resp, err := updateClient.Get(info.Setup)
	if err != nil {
		return fail("couldn't download the update")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fail("couldn't download the update")
	}
	zip := filepath.Join(tmp, "update.zip")
	f, err := os.Create(zip)
	if err != nil {
		return fail(err.Error())
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, 300<<20))
	f.Close()
	if err != nil || n < 1<<20 {
		return fail("the download was incomplete")
	}
	if exec.Command("/usr/bin/ditto", "-x", "-k", zip, tmp).Run() != nil {
		return fail("the update couldn't be unpacked")
	}
	os.Remove(zip)
	newApp := filepath.Join(tmp, "Timestamps.app")
	plist, _ := os.ReadFile(filepath.Join(newApp, "Contents", "Info.plist"))
	if id, v := bundleInfo(plist); id != bundleID || v != info.Latest {
		return fail("the download isn't Timestamps " + info.Latest)
	}
	if exec.Command("/usr/bin/codesign", "--verify", "--strict", newApp).Run() != nil {
		return fail("the downloaded app is damaged")
	}

	cmd := exec.Command("/bin/sh", "-c", swapScript, "sh", strconv.Itoa(os.Getpid()), app, newApp, tmp)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // outlives this process
	if err := cmd.Start(); err != nil {
		return fail(err.Error())
	}
	go func() {
		time.Sleep(500 * time.Millisecond)
		s.Host.Quit()
	}()
	return nil
}

package server

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestUpdateAsset(t *testing.T) {
	for name, want := range map[string]bool{
		"Timestamps-1.1.0-macos.zip": true, "Timestamps-Setup-1.1.0.exe": false, "Timestamps-1.1.0-extension-chromium.zip": false,
	} {
		if updateAsset(name) != want {
			t.Errorf("updateAsset(%q) = %v", name, !want)
		}
	}
}

func TestBundleInfo(t *testing.T) {
	plist, err := os.ReadFile(filepath.Join("..", "..", "assets", "Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	if id, v := bundleInfo(plist); id != bundleID || v != "__VERSION__" {
		t.Fatalf("bundleInfo = %q, %q", id, v)
	}
	if appBundle("/Applications/Timestamps.app/Contents/MacOS/Timestamps") != "/Applications/Timestamps.app" || appBundle("/usr/local/bin/timestamps") != "" {
		t.Fatal("appBundle")
	}
}

// The swap script waits for the process, replaces the app and keeps the
// old one when the new one can't be moved in.
func TestSwapScript(t *testing.T) {
	root := t.TempDir()
	app, tmp := filepath.Join(root, "Apps", "T.app"), filepath.Join(root, "dl")
	newApp := filepath.Join(tmp, "T.app")
	for _, d := range []string{app, newApp} {
		os.MkdirAll(d, 0o755)
	}
	os.WriteFile(filepath.Join(app, "v"), []byte("old"), 0o644)
	os.WriteFile(filepath.Join(newApp, "v"), []byte("new"), 0o644)
	sleeper := exec.Command("sleep", "1")
	if err := sleeper.Start(); err != nil {
		t.Fatal(err)
	}
	go sleeper.Wait()
	// "open" would start the app: replace it with true for the test.
	script := "open() { true; }\n" + swapScript
	start := time.Now()
	if err := exec.Command("/bin/sh", "-c", script, "sh", strconv.Itoa(sleeper.Process.Pid), app, newApp, tmp).Run(); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 800*time.Millisecond {
		t.Error("the script didn't wait for the process")
	}
	if b, _ := os.ReadFile(filepath.Join(app, "v")); string(b) != "new" {
		t.Errorf("app not replaced: %q", b)
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Error("download folder left behind")
	}
}

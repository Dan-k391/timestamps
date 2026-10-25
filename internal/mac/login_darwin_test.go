package mac

import (
	"strings"
	"testing"
)

func TestAppBundle(t *testing.T) {
	for exe, want := range map[string]string{
		"/Applications/Timestamps.app/Contents/MacOS/Timestamps": "/Applications/Timestamps.app",
		"/Users/me/build/timestamps":                             "",
		"/Applications/Odd/Contents/MacOS/x":                     "",
	} {
		if got := AppBundle(exe); got != want {
			t.Errorf("AppBundle(%q) = %q, want %q", exe, got, want)
		}
	}
}

func TestAgentPlist(t *testing.T) {
	cmd := loginCommand("/Applications/A & B.app/Contents/MacOS/Timestamps", []string{"--minimized"})
	if strings.Join(cmd, " ") != "/usr/bin/open -g -a /Applications/A & B.app --args --minimized" {
		t.Fatalf("loginCommand = %q", cmd)
	}
	p := string(agentPlist("io.example.ts", cmd))
	for _, want := range []string{"<string>io.example.ts</string>", "<string>/Applications/A &amp; B.app</string>", "<string>--minimized</string>", "<key>RunAtLoad</key>"} {
		if !strings.Contains(p, want) {
			t.Errorf("plist lacks %q:\n%s", want, p)
		}
	}
}

func TestLoginItem(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	exe := "/Applications/Timestamps.app/Contents/MacOS/Timestamps"
	if LoginItem("io.example.ts", exe, nil) {
		t.Fatal("no login item yet")
	}
	if err := SetLoginItem("io.example.ts", exe, nil, true); err != nil {
		t.Fatal(err)
	}
	if !LoginItem("io.example.ts", exe, nil) || LoginItem("io.example.ts", "/elsewhere/Timestamps.app/Contents/MacOS/Timestamps", nil) {
		t.Fatal("LoginItem should match the current app only")
	}
	if err := SetLoginItem("io.example.ts", exe, nil, false); err != nil || LoginItem("io.example.ts", exe, nil) {
		t.Fatal("login item not removed")
	}
}

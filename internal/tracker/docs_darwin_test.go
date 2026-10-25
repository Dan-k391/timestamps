package tracker

import (
	"testing"

	"timestamps/internal/mac"
)

func TestAppKey(t *testing.T) {
	for bundle, want := range map[string]string{
		"/Applications/Google Chrome.app":      "google chrome",
		"/System/Applications/Notes.app":       "notes",
		"/usr/local/bin/emacs":                 "emacs",
		"/Applications/Visual Studio Code.app": "visual studio code",
	} {
		if got := appKey(mac.Front{Bundle: bundle}); got != want {
			t.Errorf("appKey(%q) = %q, want %q", bundle, got, want)
		}
	}
	if !browserExes["google chrome"] || !browserExes["safari"] || browserExes["notes"] {
		t.Error("browser list")
	}
}

func TestBrowserDialect(t *testing.T) {
	for id, want := range map[string]int{
		"com.apple.Safari": mac.DialectSafari, "com.google.Chrome": mac.DialectChromium, "com.google.Chrome.canary": mac.DialectChromium,
		"com.microsoft.edgemac": mac.DialectChromium, "company.thebrowser.Browser": mac.DialectChromium, "org.mozilla.firefox": 0,
	} {
		if got := browserDialect(id); got != want {
			t.Errorf("browserDialect(%q) = %d, want %d", id, got, want)
		}
	}
}

func TestFileURLToPath(t *testing.T) {
	if got := fileURLToPath("file:///Users/me/My%20Notes.txt"); got != "/Users/me/My Notes.txt" {
		t.Errorf("fileURLToPath = %q", got)
	}
	if d := webDoc("file:///Users/me/a.pdf", "a.pdf", false); d.Key != "/Users/me/a.pdf" {
		t.Errorf("webDoc(file URL) = %+v", d)
	}
}

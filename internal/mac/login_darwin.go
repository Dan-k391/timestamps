package mac

import (
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
)

// Starting at login is a LaunchAgent: a property list in
// ~/Library/LaunchAgents that launchd runs when the user logs in.

func agentPath(label string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist")
}

// AppBundle returns the .app folder an executable belongs to, or "".
func AppBundle(exe string) string {
	dir := filepath.Dir(exe) // .../Name.app/Contents/MacOS
	if filepath.Base(dir) == "MacOS" && filepath.Base(filepath.Dir(dir)) == "Contents" {
		if app := filepath.Dir(filepath.Dir(dir)); strings.HasSuffix(app, ".app") {
			return app
		}
	}
	return ""
}

// loginCommand starts the app the way the Finder does when it is a bundle,
// in the background and with args.
func loginCommand(exe string, args []string) []string {
	if app := AppBundle(exe); app != "" {
		return append([]string{"/usr/bin/open", "-g", "-a", app, "--args"}, args...)
	}
	return append([]string{exe}, args...)
}

func agentPlist(label string, cmd []string) []byte {
	var b bytes.Buffer
	esc := func(s string) string {
		var e bytes.Buffer
		xml.EscapeText(&e, []byte(s))
		return e.String()
	}
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + esc(label) + `</string>
	<key>ProgramArguments</key>
	<array>
`)
	for _, a := range cmd {
		b.WriteString("\t\t<string>" + esc(a) + "</string>\n")
	}
	b.WriteString(`	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>ProcessType</key>
	<string>Interactive</string>
</dict>
</plist>
`)
	return b.Bytes()
}

// SetLoginItem starts exe with args when the user logs in, or stops doing so.
func SetLoginItem(label, exe string, args []string, on bool) error {
	p := agentPath(label)
	if !on {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, agentPlist(label, loginCommand(exe, args)), 0o644)
}

// LoginItem reports whether exe (with args) starts at login.
func LoginItem(label, exe string, args []string) bool {
	b, err := os.ReadFile(agentPath(label))
	return err == nil && bytes.Equal(b, agentPlist(label, loginCommand(exe, args)))
}

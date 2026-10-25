//go:build windows

package favicon

import (
	"os"
	"path/filepath"
)

// browserCaches lists the icon databases of the installed browsers.
func browserCaches() (chromium, firefox []string) {
	local, roaming := os.Getenv("LOCALAPPDATA"), os.Getenv("APPDATA")
	for _, root := range []string{`Microsoft\Edge`, `Microsoft\Edge Beta`, `Microsoft\Edge Dev`, `Google\Chrome`, `Google\Chrome Beta`,
		`BraveSoftware\Brave-Browser`, `Vivaldi`, `Chromium`, `Thorium`, `CentBrowser`} {
		m, _ := filepath.Glob(filepath.Join(local, root, "User Data", "*", "Favicons"))
		chromium = append(chromium, m...)
	}
	for _, p := range []string{`Opera Software\Opera Stable`, `Opera Software\Opera GX Stable`} {
		m, _ := filepath.Glob(filepath.Join(roaming, p, "Favicons"))
		chromium = append(chromium, m...)
		m, _ = filepath.Glob(filepath.Join(roaming, p, "*", "Favicons"))
		chromium = append(chromium, m...)
	}
	for _, p := range []string{`Mozilla\Firefox`, `Waterfox`, `librewolf`, `Floorp`, `zen`} {
		m, _ := filepath.Glob(filepath.Join(roaming, p, "Profiles", "*", "favicons.sqlite"))
		firefox = append(firefox, m...)
	}
	return
}

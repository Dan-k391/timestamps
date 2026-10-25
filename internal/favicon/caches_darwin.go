package favicon

import (
	"os"
	"path/filepath"
)

// browserCaches lists the icon databases of the installed browsers.
func browserCaches() (chromium, firefox []string) {
	home, _ := os.UserHomeDir()
	support := filepath.Join(home, "Library", "Application Support")
	for _, root := range []string{"Google/Chrome", "Google/Chrome Beta", "Google/Chrome Dev", "Google/Chrome Canary",
		"Microsoft Edge", "Microsoft Edge Beta", "Microsoft Edge Dev", "BraveSoftware/Brave-Browser", "Vivaldi",
		"Chromium", "Arc/User Data", "Thorium"} {
		m, _ := filepath.Glob(filepath.Join(support, filepath.FromSlash(root), "*", "Favicons"))
		chromium = append(chromium, m...)
	}
	for _, p := range []string{"com.operasoftware.Opera", "com.operasoftware.OperaGX"} {
		m, _ := filepath.Glob(filepath.Join(support, p, "Favicons"))
		chromium = append(chromium, m...)
		m, _ = filepath.Glob(filepath.Join(support, p, "*", "Favicons"))
		chromium = append(chromium, m...)
	}
	for _, p := range []string{"Firefox", "Waterfox", "librewolf", "Floorp", "zen"} {
		m, _ := filepath.Glob(filepath.Join(support, p, "Profiles", "*", "favicons.sqlite"))
		firefox = append(firefox, m...)
	}
	return
}

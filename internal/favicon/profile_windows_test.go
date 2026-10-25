package favicon

import (
	"path/filepath"
	"testing"
)

// fakeProfile points the browser cache lookup at root and returns the
// folder of a fake Edge profile there.
func fakeProfile(t *testing.T, root string) string {
	t.Setenv("LOCALAPPDATA", root)
	t.Setenv("APPDATA", filepath.Join(root, "roaming"))
	return filepath.Join(root, `Microsoft\Edge\User Data\Default`)
}

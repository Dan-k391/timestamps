package favicon

import (
	"path/filepath"
	"testing"
)

// fakeProfile points the browser cache lookup at root and returns the
// folder of a fake Edge profile there.
func fakeProfile(t *testing.T, root string) string {
	t.Setenv("HOME", root)
	return filepath.Join(root, "Library", "Application Support", "Microsoft Edge", "Default")
}

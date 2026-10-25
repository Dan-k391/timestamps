package server

import "testing"

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"1.1.0", "1.0.0", true}, {"1.0.0", "1.0.0", false}, {"1.0.9", "1.1.0", false},
		{"1.10.0", "1.9.3", true}, {"2", "1.99.99", true}, {"1.1", "1.1.0", false}, {"1.1.1", "1.1", true},
	} {
		if got := newer(c.a, c.b); got != c.want {
			t.Errorf("newer(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

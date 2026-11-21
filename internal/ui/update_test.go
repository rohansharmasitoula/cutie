package ui

import "testing"

func TestNewerRelease(t *testing.T) {
	cases := []struct {
		current, tag string
		want         bool
	}{
		{"0.1.3", "v0.2.0", true},                                       // release binary: GoReleaser drops the v
		{"v0.2.0", "v0.2.0", false},                                     // up to date
		{"v0.3.0", "v0.2.0", false},                                     // ahead of the latest release
		{"v0.2.1-0.20261121090000-abcdef123456+dirty", "v0.2.0", false}, // built from main after v0.2.0
		{"v0.1.4-0.20261120111900-477cb511ca0e", "v0.2.0", true},        // built from main before v0.2.0
		{"(devel)", "v0.2.0", false},                                    // no version to compare
		{"v0.2.0", "", false},                                           // no release found
	}
	for _, c := range cases {
		if got := newerRelease(c.current, c.tag); got != c.want {
			t.Errorf("newerRelease(%q, %q) = %v, want %v", c.current, c.tag, got, c.want)
		}
	}
}

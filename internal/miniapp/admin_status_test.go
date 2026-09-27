package miniapp

import "testing"

func TestIsNewerRelease(t *testing.T) {
	tests := []struct {
		current, latest   string
		newer, comparable bool
	}{
		{"v2.2.4", "2.2.5", true, true},
		{"2.2.4-3-gabc", "v2.2.4", false, true},
		{"2.3.0", "v2.2.9", false, true},
		{"dev", "v2.2.5", false, false},
		{"2.2.4", "unknown", false, false},
	}
	for _, test := range tests {
		newer, comparable := isNewerRelease(test.current, test.latest)
		if newer != test.newer || comparable != test.comparable {
			t.Errorf("isNewerRelease(%q, %q) = (%v, %v)", test.current, test.latest, newer, comparable)
		}
	}
}

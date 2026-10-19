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

func TestResolvePanelUpdateStatus(t *testing.T) {
	tests := []struct {
		name, current, latest, want string
		latestOK                    bool
	}{
		{"update available", "3.4.3", "v3.4.4", "available", true},
		{"up to date", "3.4.4", "v3.4.4", "current", true},
		{"version unavailable", "", "v3.4.4", "unknown", true},
		{"release unavailable", "3.4.3", "", "unknown", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status := resolvePanelUpdateStatus(test.current, test.latest, test.latestOK)
			if status.State != test.want || status.URL != remnawaveLatestReleasePage {
				t.Fatalf("status = %+v, want state %q", status, test.want)
			}
		})
	}
}

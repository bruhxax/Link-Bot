package config

import (
	"strings"
	"testing"
)

func TestAdminBaseURL(t *testing.T) {
	for _, tt := range []struct {
		origin, label, want string
		invalid             bool
	}{
		{"", "", "", false},
		{"https://example.com", "admin", "https://admin.example.com", false},
		{"https://EXAMPLE.COM", "ADMIN", "https://admin.example.com", false},
		{"https://example.com", " Admin-Panel ", "https://admin-panel.example.com", false},
		{"https://example.com", "admin.domain.ru", "", true},
		{"https://example.com", "-admin", "", true},
		{"https://example.com", "admin-", "", true},
		{"https://example.com", strings.Repeat("a", 64), "", true},
		{"http://example.com", "admin", "", true},
		{"https://127.0.0.1", "admin", "", true},
		{"https://example.com/path", "admin", "", true},
		{"https://example.com:443", "admin", "", true},
		{"", "admin", "", true},
	} {
		got, err := adminBaseURL(tt.origin, tt.label)
		if got != tt.want || (err != nil) != tt.invalid {
			t.Errorf("adminBaseURL(%q,%q) = %q, %v", tt.origin, tt.label, got, err)
		}
		if err != nil && !strings.Contains(err.Error(), "ADMIN_SUBDOMAIN") {
			t.Errorf("wrong setting in error: %v", err)
		}
	}
}

func TestAdminNotificationURLWithAndWithoutHost(t *testing.T) {
	previous := conf.adminBaseURL
	t.Cleanup(func() { conf.adminBaseURL = previous })
	conf.adminBaseURL = ""
	if got := AdminURL("finance"); got != "/mini-app/?page=admin&section=finance" {
		t.Fatal(got)
	}
	conf.adminBaseURL = "https://admin.example.com"
	if got := AdminURL("finance"); got != "https://admin.example.com/?page=admin&section=finance" {
		t.Fatal(got)
	}
}

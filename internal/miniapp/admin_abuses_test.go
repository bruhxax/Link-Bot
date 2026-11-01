package miniapp

import (
	"testing"
	"time"

	"link-bot/internal/remnawave"
)

func TestAbuseNeedsIPScan(t *testing.T) {
	now := time.Date(2026, time.November, 1, 12, 0, 0, 0, time.UTC)
	limit := 2
	recent := now.Add(-30 * time.Minute)
	old := now.Add(-3 * time.Hour)
	tests := []struct {
		name string
		user remnawave.PanelUser
		want bool
	}{
		{"recent modern connection", remnawave.PanelUser{ID: 1, Status: "ACTIVE", HwidDeviceLimit: &limit, UserTraffic: remnawave.PanelUserTraffic{OnlineAt: &recent}}, true},
		{"old modern connection", remnawave.PanelUser{ID: 1, Status: "ACTIVE", HwidDeviceLimit: &limit, UserTraffic: remnawave.PanelUserTraffic{OnlineAt: &old}}, false},
		{"never connected", remnawave.PanelUser{ID: 1, Status: "ACTIVE", HwidDeviceLimit: &limit}, false},
		{"legacy without activity timestamp", remnawave.PanelUser{Status: "ACTIVE", HwidDeviceLimit: &limit}, true},
		{"disabled user", remnawave.PanelUser{ID: 1, Status: "DISABLED", HwidDeviceLimit: &limit, UserTraffic: remnawave.PanelUserTraffic{OnlineAt: &recent}}, false},
		{"no device limit", remnawave.PanelUser{ID: 1, Status: "ACTIVE", UserTraffic: remnawave.PanelUserTraffic{OnlineAt: &recent}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := abuseNeedsIPScan(tt.user, now); got != tt.want {
				t.Fatalf("abuseNeedsIPScan() = %v, want %v", got, tt.want)
			}
		})
	}
}

package miniapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

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

func TestFetchGitHubAheadBy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/compare/da6befa...main" || r.URL.Query().Get("per_page") != "1" {
			t.Errorf("unexpected request URL: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ahead","ahead_by":1,"behind_by":0}`))
	}))
	defer server.Close()
	ahead, ok := fetchGitHubAheadBy(context.Background(), server.Client(), server.URL+"/compare/da6befa...main?per_page=1")
	if !ok || ahead != 1 {
		t.Fatalf("ahead=%d, ok=%v", ahead, ok)
	}
}

func TestAdminStatusDetectsNewCommitWithoutRelease(t *testing.T) {
	update := resolveAdminUpdateStatus(adminUpdateStatus{State: "unknown", URL: latestReleasePage}, "2.2.6-8-gda6befa", "da6befa", "2.2.6", true, 1, true)
	if update.State != "available" || update.Kind != "commit" || update.AheadBy != 1 || update.LatestVersion != "2.2.6" {
		t.Fatalf("unexpected update: %+v", update)
	}
	if update.URL != githubComparePage+"da6befa...main" {
		t.Fatalf("unexpected compare URL: %s", update.URL)
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

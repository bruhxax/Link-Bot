package remnawave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetSystemStats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/system/stats" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":{"uptime":3601,"memory":{"used":1048576,"total":4194304},"users":{"totalUsers":123,"statusCounts":{"ACTIVE":100,"EXPIRED":15,"LIMITED":3,"DISABLED":5}}}}`))
	}))
	defer server.Close()
	stats, err := NewClient(server.URL, "token", "remote").GetSystemStats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.UptimeSeconds != 3601 || stats.MemoryUsedBytes != 1048576 || stats.MemoryTotalBytes != 4194304 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if stats.Users == nil || stats.Users.TotalUsers != 123 || stats.Users.StatusCounts["ACTIVE"] != 100 {
		t.Fatalf("unexpected user counters: %+v", stats.Users)
	}
}

func TestGetVersionFromSystemMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/system/metadata" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":{"version":" 3.4.4 "}}`))
	}))
	defer server.Close()
	version, err := NewClient(server.URL, "token", "remote").GetVersion(context.Background())
	if err != nil || version != "3.4.4" {
		t.Fatalf("version = %q, error = %v", version, err)
	}
}

func TestGetVersionRejectsEmptyMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"response":{}}`))
	}))
	defer server.Close()
	if _, err := NewClient(server.URL, "token", "remote").GetVersion(context.Background()); err == nil {
		t.Fatal("missing version must not be treated as current")
	}
}

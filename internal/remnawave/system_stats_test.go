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
		_, _ = w.Write([]byte(`{"response":{"uptime":3601,"memory":{"used":1048576,"total":4194304}}}`))
	}))
	defer server.Close()
	stats, err := NewClient(server.URL, "token", "remote").GetSystemStats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.UptimeSeconds != 3601 || stats.MemoryUsedBytes != 1048576 || stats.MemoryTotalBytes != 4194304 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

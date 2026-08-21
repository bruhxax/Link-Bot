package remnawave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestPingUsesSingleDedicatedHealthRequest(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/api/system/health" {
			t.Fatalf("path = %q, want /api/system/health", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":{"isHealthy":true}}`))
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		baseURL:    server.URL,
		token:      "token",
	}
	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("requests = %d, want 1", got)
	}
}

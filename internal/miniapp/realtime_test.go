package miniapp

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRealtimeStreamDeliversChanges(t *testing.T) {
	h := &Handler{realtime: newRealtimeHub()}
	server := httptest.NewServer(http.HandlerFunc(h.handleRealtimeForTest))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("unexpected stream response: %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	lines := bufio.NewScanner(resp.Body)
	if !lines.Scan() || lines.Text() != "event: ready" {
		t.Fatalf("missing ready event: %q", lines.Text())
	}
	h.realtime.publish()
	for lines.Scan() {
		if lines.Text() == "event: change" {
			return
		}
	}
	t.Fatal("stream closed before change event")
}

func (h *Handler) handleRealtimeForTest(w http.ResponseWriter, r *http.Request) {
	h.handleRealtime(w, r, nil, nil)
}

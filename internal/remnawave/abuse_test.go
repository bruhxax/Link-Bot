package remnawave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestFetchUserIPsModernSnapshotDeduplicatesAcrossNodes(t *testing.T) {
	reads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/connections/by-user/42":
			if r.Method != http.MethodPost {
				t.Errorf("unexpected start method %s", r.Method)
			}
			w.Write([]byte(`{"response":{"jobId":"job-1"}}`))
		case "/api/connections/by-user/job-1":
			reads++
			if reads == 1 {
				w.Write([]byte(`{"response":{"isCompleted":false,"isFailed":false}}`))
				return
			}
			w.Write([]byte(`{"response":{"isCompleted":true,"result":{"success":true,"nodes":[{"nodeName":"A","ips":[{"ip":"203.0.113.3","lastSeen":"2026-10-30T07:00:00Z"},{"ip":"2001:db8::1","lastSeen":"2026-10-30T07:00:00Z"}]},{"nodeName":"B","ips":[{"ip":"203.0.113.3","lastSeen":"2026-10-30T07:01:00Z"}]}]}}}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	ips, err := NewClient(server.URL, "token", "remote").FetchUserIPs(context.Background(), 42, uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	if reads != 2 || len(ips) != 2 || ips[1].IP != "203.0.113.3" || ips[1].Node != "B" {
		t.Fatalf("reads=%d ips=%+v", reads, ips)
	}
}

func TestFetchUserIPsLegacyFallbackAndIncompleteFailure(t *testing.T) {
	userUUID := uuid.New()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/connections/by-user/42":
			http.NotFound(w, r)
		case "/api/ip-control/fetch-ips/" + userUUID.String():
			w.Write([]byte(`{"response":{"jobId":"legacy"}}`))
		case "/api/ip-control/fetch-ips/result/legacy":
			w.Write([]byte(`{"response":{"isCompleted":true}}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	if ips, err := NewClient(server.URL, "token", "remote").FetchUserIPs(context.Background(), 42, userUUID); err == nil || ips != nil {
		t.Fatalf("incomplete result accepted: %+v, %v", ips, err)
	}
}

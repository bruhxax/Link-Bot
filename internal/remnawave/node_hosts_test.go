package remnawave

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNodeHostVisibilityAPI(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/api/hosts":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"response":[{"uuid":"host-1","remark":"Germany","nodes":["node-1"],"isDisabled":false}]}`))
		case "/api/hosts/bulk/disable", "/api/hosts/bulk/enable":
			var body struct {
				UUIDs []string `json:"uuids"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.UUIDs) != 1 || body.UUIDs[0] != "host-1" {
				t.Errorf("invalid request body: %+v, %v", body, err)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewClient(server.URL, "test-token", "")
	hosts, err := client.GetHostsStatus(context.Background())
	if err != nil || len(hosts) != 1 || hosts[0].Remark != "Germany" || len(hosts[0].Nodes) != 1 || hosts[0].Nodes[0] != "node-1" {
		t.Fatalf("hosts=%+v err=%v", hosts, err)
	}
	if err := client.SetHostsDisabled(context.Background(), []string{"host-1"}, true); err != nil {
		t.Fatal(err)
	}
	if err := client.SetHostsDisabled(context.Background(), []string{"host-1"}, false); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 3 {
		t.Fatalf("calls=%v", calls)
	}
}

package remnawave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListSquadsReadsRemnawaveThreeContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/internal-squads":
			_, _ = w.Write([]byte(`{"response":{"total":1,"internalSquads":[{"uuid":"7d3258cf-2b39-4ad0-8b11-fbcd30d76348","name":"Main","newV3Field":true}]}}`))
		case "/api/external-squads":
			_, _ = w.Write([]byte(`{"response":{"total":1,"externalSquads":[{"uuid":"8d3258cf-2b39-4ad0-8b11-fbcd30d76349","name":"External","responseHeadersAdd":{},"responseHeadersRemove":[]}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	catalog, err := NewClient(server.URL, "token", "remote").ListSquads(context.Background())
	if err != nil {
		t.Fatalf("ListSquads() error = %v", err)
	}
	if len(catalog.Internal) != 1 || catalog.Internal[0].Name != "Main" {
		t.Fatalf("internal squads = %#v", catalog.Internal)
	}
	if len(catalog.External) != 1 || catalog.External[0].Name != "External" {
		t.Fatalf("external squads = %#v", catalog.External)
	}
}

func TestListSquadsPreservesPartialCatalog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/internal-squads" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"response":{"total":1,"internalSquads":[{"uuid":"7d3258cf-2b39-4ad0-8b11-fbcd30d76348","name":"Main"}]}}`))
			return
		}
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer server.Close()

	catalog, err := NewClient(server.URL, "token", "remote").ListSquads(context.Background())
	if err == nil {
		t.Fatal("ListSquads() error = nil, want external squad error")
	}
	if len(catalog.Internal) != 1 || len(catalog.External) != 0 {
		t.Fatalf("catalog = %#v", catalog)
	}
}

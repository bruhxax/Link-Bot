package appupdate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckCommitAncestry(t *testing.T) {
	const current = "1234567"
	head := strings.Repeat("a", 40)
	for _, tc := range []struct {
		name, version, release, comparison, state, kind string
		ahead, behind                                   int
	}{
		{"new release", "2.7.3", "2.7.4", "ahead", "available", "release", 3, 0},
		{"new commit same release", "2.7.4", "2.7.4", "ahead", "available", "commit", 1, 0},
		{"old describe but current commit", "2.7.3-2-g1234567", "2.7.4", "identical", "current", "", 0, 0},
		{"installed ahead", "2.7.4", "2.7.4", "behind", "current", "", 0, 1},
		{"diverged", "2.7.3", "2.7.4", "diverged", "unknown", "", 2, 1},
		{"malformed comparison", "2.7.4", "2.7.4", "ahead", "unknown", "", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/releases/latest":
					json.NewEncoder(w).Encode(map[string]string{"tag_name": tc.release})
				case "/commits":
					if r.URL.Query().Get("sha") != "main" || r.URL.Query().Get("per_page") != "1" {
						t.Error(r.URL.String())
					}
					json.NewEncoder(w).Encode([]any{map[string]any{"sha": head, "commit": map[string]string{"message": "Fix update\n\nDetails"}}})
				case "/compare/" + current + "..." + head:
					if r.URL.Query().Get("page") != "2" {
						t.Error("comparison should omit file patches")
					}
					json.NewEncoder(w).Encode(map[string]any{"status": tc.comparison, "ahead_by": tc.ahead, "behind_by": tc.behind})
				default:
					t.Error("unexpected request", r.URL.String())
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			status := CheckAt(context.Background(), server.Client(), server.URL, tc.version, current)
			if status.State != tc.state || status.Kind != tc.kind || status.LatestCommit != head || status.Summary != "Fix update" {
				t.Fatalf("unexpected status: %+v", status)
			}
		})
	}
}

func TestCheckUnavailableAndUnknownBuild(t *testing.T) {
	for _, tc := range []struct {
		version, commit, release string
		compareDown              bool
		want                     string
	}{
		{"2.7.4", "1234567", "2.7.4", true, "unknown"},
		{"2.7.3", "1234567", "2.7.4", true, "unknown"},
		{"2.7.3", "none", "2.7.4", false, "available"},
		{"2.7.4", "none", "2.7.4", false, "current"},
		{"dev", "none", "2.7.4", false, "unknown"},
		{"2.7.3", "1234567", "", true, "unknown"},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/releases/latest" && tc.release != "" {
				json.NewEncoder(w).Encode(map[string]string{"tag_name": tc.release})
				return
			}
			http.Error(w, "rate limited", 429)
		}))
		status := CheckAt(context.Background(), server.Client(), server.URL, tc.version, tc.commit)
		server.Close()
		if status.State != tc.want {
			t.Errorf("%+v: got %+v", tc, status)
		}
	}
}

func TestHeadAlreadyInstalled(t *testing.T) {
	head := strings.Repeat("b", 40)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/latest":
			w.Write([]byte(`{"tag_name":"2.7.4"}`))
		case "/commits":
			w.Write([]byte(`[{"sha":"` + head + `"}]`))
		default:
			t.Error("identical head should not need comparison")
			http.Error(w, "bad request", 500)
		}
	}))
	defer server.Close()
	if got := CheckAt(context.Background(), server.Client(), server.URL, "2.7.3-1-gbbbbbbb", head); got.State != "current" {
		t.Fatal(got)
	}
}

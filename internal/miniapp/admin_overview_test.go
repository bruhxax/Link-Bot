package miniapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOverviewDoesNotExposeMetricsWithoutTheirPermissions(t *testing.T) {
	h := &Handler{}
	sess := &session{AdminAccess: adminAccess{IsAdmin: true, Permissions: []string{"status"}}}
	r := httptest.NewRequest(http.MethodPost, "/api/mini-app/admin/overview", strings.NewReader(`{"days":30}`))
	w := httptest.NewRecorder()
	h.handleAdminOverview(w, r, sess, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var payload struct {
		Data struct {
			Users   any `json:"users"`
			Revenue any `json:"revenue"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.Users != nil || payload.Data.Revenue != nil {
		t.Fatal("restricted metrics disclosed")
	}
}

func TestOverviewRejectsUnknownPeriodsAndNonAdministrators(t *testing.T) {
	h := &Handler{}
	for _, tc := range []struct {
		admin bool
		days  int
		want  int
	}{{false, 30, http.StatusForbidden}, {true, 365, http.StatusBadRequest}} {
		body := `{"days":30}`
		if tc.days == 365 {
			body = `{"days":365}`
		}
		w := httptest.NewRecorder()
		h.handleAdminOverview(w, httptest.NewRequest(http.MethodPost, "/api/mini-app/admin/overview", strings.NewReader(body)), &session{AdminAccess: adminAccess{IsAdmin: tc.admin}}, nil)
		if w.Code != tc.want {
			t.Fatalf("status %d want %d", w.Code, tc.want)
		}
	}
}

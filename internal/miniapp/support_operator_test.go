package miniapp

import (
	"link-bot/internal/database"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSupportHandoffThreshold(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want int
	}{{"", 4}, {"bad", 4}, {"0", 4}, {"3", 3}, {"5", 5}, {"20", 20}, {"21", 4}} {
		if got := supportAIHandoffAfter(map[string]string{"handoffAfter": tc.raw}); got != tc.want {
			t.Errorf("%q: %d, want %d", tc.raw, got, tc.want)
		}
	}
	messages := []database.SupportMessage{{AuthorRole: "customer"}, {AuthorRole: "ai"}, {AuthorRole: "admin"}, {AuthorRole: "ai"}}
	if got := supportAIAnswerCount(messages); got != 2 {
		t.Fatalf("count=%d", got)
	}
}
func TestReadOnlyOperatorCannotClaim(t *testing.T) {
	h := &Handler{}
	sess := &session{User: telegramUser{ID: 22}, AdminAccess: adminAccess{IsAdmin: true, Permissions: []string{"support.view"}}}
	w := httptest.NewRecorder()
	h.handleSupportOperator(w, httptest.NewRequest("POST", "/api/mini-app/support/operator", strings.NewReader(`{"ticketId":1}`)), sess, nil)
	if w.Code != 403 {
		t.Fatalf("read-only claim accepted: %d", w.Code)
	}
}

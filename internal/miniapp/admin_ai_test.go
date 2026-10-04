package miniapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"link-bot/internal/database"
)

func TestAIEndpointsRejectNonAdmin(t *testing.T) {
	for _, path := range []string{"/api/mini-app/admin/ai/settings", "/api/mini-app/admin/ai/models", "/api/mini-app/admin/ai/update", "/api/mini-app/admin/ai/toggle"} {
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"apiKey":"test-key"}`))
		w := httptest.NewRecorder()
		(&Handler{}).handleAdminAI(w, r, &session{}, &database.Customer{})
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s returned %d", path, w.Code)
		}
	}
}

func TestAIHistoryOnlyContainsCurrentTicketText(t *testing.T) {
	messages := make([]database.SupportMessage, 50)
	for index := range messages {
		messages[index] = database.SupportMessage{Body: "текст обращения", AuthorTelegramID: 123456, MediaStorageName: "private-path", MediaOriginalName: "private-name", AuthorRole: database.SupportAuthorRoleCustomer}
	}
	messages[49].AuthorRole = database.SupportAuthorRoleAI
	messages[48].MediaType = "image"
	history := supportAIHistory(messages)
	if len(history) != 41 || history[40].Role != "assistant" || history[0].Role != "user" {
		t.Fatalf("history roles/window incorrect: %+v", history)
	}
	raw, err := json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"123456", "private-path", "private-name"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("history leaked %s", secret)
		}
	}
	if !strings.Contains(history[39].Content, "передай обращение") {
		t.Fatal("attachment was silently omitted")
	}
}

func TestAIMessagePayloadPreservesAIIdentity(t *testing.T) {
	messages := buildSupportMessagePayloads([]database.SupportMessage{{ID: 42, AuthorRole: database.SupportAuthorRoleAI, Body: "Здравствуйте"}})
	if len(messages) != 1 || messages[0].AuthorRole != "ai" {
		t.Fatalf("AI identity lost: %+v", messages)
	}
}

func TestSavedAIKeyOnlyUsedForSameProvider(t *testing.T) {
	stored := map[string]string{"apiUrl": "https://provider.example/v1", "apiKey": "saved-private-key"}
	if got := resolveAIKey("https://provider.example/", "", stored); got != "saved-private-key" {
		t.Fatal("same server did not retain saved key")
	}
	if got := resolveAIKey("https://different-provider.example/v1", "", stored); got != "" {
		t.Fatal("saved key forwarded to another server")
	}
	if got := resolveAIKey("https://different-provider.example/v1", "new-test-key", stored); got != "new-test-key" {
		t.Fatal("explicit replacement key ignored")
	}
}

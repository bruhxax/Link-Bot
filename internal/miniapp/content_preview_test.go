package miniapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-telegram/bot"

	"link-bot/internal/database"
	telegram "link-bot/internal/handler"
	planbook "link-bot/internal/plans"
	"link-bot/internal/runtimeconfig"
)

func TestContentPreviewSendsDraftOnlyToRequestingAdministrator(t *testing.T) {
	var recipient, message string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
		}
		recipient, message = r.FormValue("chat_id"), r.FormValue("text")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"chat":{"id":123,"type":"private"}}}`))
	}))
	defer api.Close()
	telegramBot, err := bot.New("local-test", bot.WithSkipGetMe(), bot.WithServerURL(api.URL))
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{telegramBot: telegramBot}
	content := runtimeconfig.DefaultSettings().Content
	content.StartTextRU = "<b>Несохранённый тест</b>"
	content.StartImage = ""
	body, err := json.Marshal(adminContentTestRequest{Kind: "start", Content: content})
	if err != nil {
		t.Fatal(err)
	}
	sess := activitySession(true)
	sess.User.ID = 123
	w := httptest.NewRecorder()
	h.handleAdminContentTest(w, httptest.NewRequest(http.MethodPost, "/api/mini-app/admin/content/test", strings.NewReader(string(body))), sess, &database.Customer{TelegramID: 123})
	if w.Code != http.StatusOK {
		t.Fatalf("preview failed: %d %s", w.Code, w.Body.String())
	}
	if recipient != "123" || message != content.StartTextRU {
		t.Fatalf("wrong preview: recipient=%s message=%s", recipient, message)
	}
	if h.runtimeSettings != nil || h.purchaseRepository != nil {
		t.Fatal("preview needs no settings persistence or purchases")
	}
}

func TestContentPreviewRejectsNonAdministrator(t *testing.T) {
	h := &Handler{}
	w := httptest.NewRecorder()
	h.handleAdminContentTest(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`)), &session{}, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("nonadministrator: %d", w.Code)
	}
	if adminRouteAllowed(adminAccess{IsAdmin: true, Permissions: []string{"status"}}, "/api/mini-app/admin/content/test") {
		t.Fatal("status administrator can send content previews")
	}
}

func TestContentPreviewCoversVerificationAndCommerceWithoutCharging(t *testing.T) {
	settings := runtimeconfig.DefaultSettings()
	settings.Content.StartTextRU = "Тест главного меню"
	plans := []planbook.CheckoutPlan{{ID: "sample", PriceRub: 200, PriceStars: 100}}
	methods := map[string]bool{"platega": true, "balance": true}
	for _, kind := range []string{"start", "verification", "verification-failed", "verification-missing", "verification-success", "tariffs", "payment-methods", "payment-ready", "support-new", "support-customer-reply", "support-admin-reply", "support-closed", "device-added", "device-limit", "review-created"} {
		text, _, keyboard, err := buildContentPreview(kind, settings, plans, methods)
		if err != nil || text == "" {
			t.Fatalf("%s preview: text=%q err=%v", kind, text, err)
		}
		for _, row := range keyboard {
			for _, button := range row {
				if strings.HasPrefix(button.CallbackData, telegram.CallbackPayment+"?") {
					t.Fatalf("preview %s can charge a sample purchase", kind)
				}
			}
		}
	}
	if _, _, _, err := buildContentPreview("unknown", settings, plans, methods); err == nil {
		t.Fatal("unknown preview accepted")
	}
}

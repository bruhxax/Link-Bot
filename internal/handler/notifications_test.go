package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"link-bot/internal/database"
	"link-bot/internal/runtimeconfig"
)

func TestNotificationsButtonFormsAndKeyboardUpdate(t *testing.T) {
	content := runtimeconfig.DefaultSettings().Content
	if rows := appendNotificationsMenuButton(nil, content.StartMenu, true); len(rows) != 0 {
		t.Fatal("optional button enabled by default")
	}
	if content.StartMenu.NotificationsButton.Style != "success" || content.StartMenu.NotificationsOffButton.Style != "danger" {
		t.Fatal("default forms must be green and red")
	}
	content.StartMenu.NotificationsButton = runtimeconfig.OptionalTelegramButtonSettings{Enabled: true, TelegramButtonSettings: runtimeconfig.TelegramButtonSettings{Text: "On", IconCustomEmojiID: "123456789", Style: "primary"}}
	content.StartMenu.NotificationsOffButton = runtimeconfig.TelegramButtonSettings{Text: "Off", IconCustomEmojiID: "987654321", Style: "danger"}
	original := appendNotificationsMenuButton([][]models.InlineKeyboardButton{{{Text: "Account", URL: "https://example.com"}}}, content.StartMenu, true)
	for _, subscribed := range []bool{false, true} {
		markup, found := updatedNotificationsMarkup(&models.InlineKeyboardMarkup{InlineKeyboard: original}, content.StartMenu, subscribed)
		if !found || len(markup.InlineKeyboard) != 2 || markup.InlineKeyboard[0][0] != original[0][0] {
			t.Fatal("other menu buttons changed")
		}
		button := markup.InlineKeyboard[1][0]
		want := content.StartMenu.NotificationsOffButton
		callback := CallbackBroadcastSubscribe
		if subscribed {
			want, callback = content.StartMenu.NotificationsButton.TelegramButtonSettings, CallbackBroadcastUnsubscribe
		}
		if button.Text != want.Text || button.Style != want.Style || button.IconCustomEmojiID != want.IconCustomEmojiID || button.CallbackData != callback {
			t.Fatalf("wrong form: %+v", button)
		}
	}
	if original[1][0].CallbackData != CallbackBroadcastUnsubscribe {
		t.Fatal("original keyboard mutated")
	}
	if _, found := updatedNotificationsMarkup(nil, content.StartMenu, false); found {
		t.Fatal("missing keyboard accepted")
	}
}

func TestLegacyBroadcastSkipsUnsubscribedWithoutCountingDeliveryFailure(t *testing.T) {
	sent, failed, skipped := runAllcomBroadcast(context.Background(), nil, 0, nil, []int64{1, 2}, func(ctx context.Context, id int64) error {
		if id == 1 {
			return database.ErrBroadcastUnsubscribed
		}
		return nil
	})
	if sent != 1 || failed != 0 || skipped != 1 {
		t.Fatalf("sent=%d failed=%d skipped=%d", sent, failed, skipped)
	}
}

func TestNotificationsRejectsForwardedOrInaccessibleMenuBeforePersistence(t *testing.T) {
	answers := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bot123:test/answerCallbackQuery" {
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
		answers++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer server.Close()
	b, err := bot.New("123:test", bot.WithSkipGetMe(), bot.WithServerURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	h := Handler{} // No database: neither case is allowed to reach it.
	for _, message := range []*models.Message{nil, {Chat: models.Chat{ID: 99}}} {
		h.NotificationsCallbackHandler(context.Background(), b, &models.Update{CallbackQuery: &models.CallbackQuery{ID: "test", From: models.User{ID: 1}, Data: CallbackBroadcastUnsubscribe, Message: models.MaybeInaccessibleMessage{Message: message}}})
	}
	if answers != 2 {
		t.Fatalf("callbacks answered=%d", answers)
	}
}

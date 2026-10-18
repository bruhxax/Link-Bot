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

func TestNotificationsMenuAndScreenPreserveCustomAppearanceAndActions(t *testing.T) {
	content := runtimeconfig.DefaultSettings().Content
	if rows := appendNotificationsMenuButton(nil, content.StartMenu); len(rows) != 0 {
		t.Fatal("optional button enabled by default")
	}
	content.StartMenu.NotificationsButton = runtimeconfig.OptionalTelegramButtonSettings{Enabled: true, TelegramButtonSettings: runtimeconfig.TelegramButtonSettings{Text: "My news", IconCustomEmojiID: "123456789", Style: "primary"}}
	rows := appendNotificationsMenuButton([][]models.InlineKeyboardButton{{{Text: "Existing"}}}, content.StartMenu)
	button := rows[1][0]
	if len(rows) != 2 || rows[0][0].Text != "Existing" || button.CallbackData != CallbackNotifications || button.Style != "primary" || button.Text != "My news" || button.IconCustomEmojiID != "123456789" {
		t.Fatalf("menu button: %+v", rows)
	}
	for _, subscribed := range []bool{false, true} {
		text, buttons := NotificationsScreen(content, subscribed)
		wantText, wantCallback := content.Notifications.DisabledText, CallbackBroadcastSubscribe
		if subscribed {
			wantText, wantCallback = content.Notifications.EnabledText, CallbackBroadcastUnsubscribe
		}
		if text != wantText || len(buttons) != 2 || buttons[0][0].CallbackData != wantCallback || buttons[1][0].CallbackData != CallbackStart {
			t.Fatalf("screen subscribed=%v: %s %+v", subscribed, text, buttons)
		}
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

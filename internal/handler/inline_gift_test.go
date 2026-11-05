package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-telegram/bot"

	"link-bot/internal/runtimeconfig"
)

func TestInlineGiftPendingTextEscapesNames(t *testing.T) {
	got := inlineGiftPendingText("<b>{plan}</b> от {sender}", "Месяц <VIP>", "@alice&bob")
	if want := "<b>Месяц &lt;VIP&gt;</b> от @alice&amp;bob"; got != want {
		t.Fatalf("pending gift text = %q, want %q", got, want)
	}
}

func TestEditPendingInlineGiftRefreshesTelegramHTML(t *testing.T) {
	var request struct {
		InlineMessageID string `json:"inline_message_id"`
		Text            string `json:"text"`
		ParseMode       string `json:"parse_mode"`
		ReplyMarkup     struct {
			InlineKeyboard [][]struct {
				CallbackData string `json:"callback_data"`
			} `json:"inline_keyboard"`
		} `json:"reply_markup"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/editMessageText") {
			t.Errorf("unexpected Telegram method: %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("parse Telegram request: %v", err)
		}
		request.InlineMessageID = r.Form.Get("inline_message_id")
		request.Text = r.Form.Get("text")
		request.ParseMode = r.Form.Get("parse_mode")
		if err := json.Unmarshal([]byte(r.Form.Get("reply_markup")), &request.ReplyMarkup); err != nil {
			t.Errorf("decode Telegram keyboard: %v", err)
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer server.Close()
	b, err := bot.New("123:test", bot.WithSkipGetMe(), bot.WithServerURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	settings := runtimeconfig.TelegramInlineGiftSettings{
		PendingText:   `<tg-emoji emoji-id="5409160577987679547">🎁</tg-emoji> {plan} от {sender}`,
		PayButtonText: "Оплатить",
	}
	if err := editPendingInlineGift(context.Background(), b, "inline-message", 42, "premium", "Месяц <VIP>", "@alice", "", settings); err != nil {
		t.Fatal(err)
	}
	if request.InlineMessageID != "inline-message" || request.ParseMode != "HTML" {
		t.Fatalf("wrong inline edit metadata: %+v", request)
	}
	if !strings.Contains(request.Text, `<tg-emoji emoji-id="5409160577987679547">🎁</tg-emoji>`) || !strings.Contains(request.Text, "Месяц &lt;VIP&gt;") || !strings.HasSuffix(request.Text, "\u2060") {
		t.Fatalf("wrong inline edit text: %q", request.Text)
	}
	if len(request.ReplyMarkup.InlineKeyboard) != 1 || len(request.ReplyMarkup.InlineKeyboard[0]) != 1 || !strings.HasPrefix(request.ReplyMarkup.InlineKeyboard[0][0].CallbackData, "ig:pay:42:") {
		t.Fatalf("payment button changed: %+v", request.ReplyMarkup)
	}
}

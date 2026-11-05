package runtimeconfig

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-telegram/bot"
)

func TestNormalizePremiumEmojiUsesStickerFallback(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/getCustomEmojiStickers") {
			t.Errorf("unexpected Telegram method: %s", r.URL.Path)
		}
		calls++
		_, _ = fmt.Fprint(w, `{"ok":true,"result":[{"custom_emoji_id":"5409160577987679547","emoji":"🎁","type":"custom_emoji","file_id":"gift","file_unique_id":"gift","width":100,"height":100}]}`)
	}))
	defer server.Close()
	b, err := bot.New("123:test", bot.WithSkipGetMe(), bot.WithServerURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{emojiBot: b}
	settings := Settings{Content: ContentSettings{
		Gift: TelegramGiftSettings{Inline: TelegramInlineGiftSettings{
			PendingText: `<strong><tg-emoji emoji-id="5409160577987679547">🥲</tg-emoji> Подарок</strong>`,
			Text:        `<tg-emoji emoji-id="5409160577987679547">🎁</tg-emoji> Оплачен`,
		}},
		Copy: map[string]map[string]string{"ru": {"greeting": `<tg-emoji emoji-id="5409160577987679547">🥲</tg-emoji> Привет`}},
	}}
	if err := service.normalizePremiumEmoji(context.Background(), &settings); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(settings.Content.Gift.Inline.PendingText, "🥲") || !strings.Contains(settings.Content.Gift.Inline.PendingText, "🎁") {
		t.Fatalf("inline gift was not corrected: %q", settings.Content.Gift.Inline.PendingText)
	}
	if strings.Contains(settings.Content.Copy["ru"]["greeting"], "🥲") {
		t.Fatalf("nested text was not corrected: %q", settings.Content.Copy["ru"]["greeting"])
	}
	if err := service.normalizePremiumEmoji(context.Background(), &settings); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("sticker metadata fetched %d times, want once", calls)
	}
}

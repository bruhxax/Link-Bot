package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"link-bot/internal/translation"
)

func TestStartMenuFollowsPinnedMessageEvenIfPinFails(t *testing.T) {
	for _, fails := range []bool{false, true} {
		t.Run(fmt.Sprint(fails), func(t *testing.T) {
			var order []string
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					t.Error(err)
				}
				switch {
				case strings.HasSuffix(r.URL.Path, "/pinChatMessage"):
					order = append(order, "pin")
					fmt.Fprint(w, `{"ok":true,"result":true}`)
				case strings.HasSuffix(r.URL.Path, "/sendMessage"):
					order = append(order, "menu")
					if !strings.Contains(r.FormValue("reply_markup"), "Open cabinet") {
						t.Error("menu keyboard missing")
					}
					fmt.Fprint(w, `{"ok":true,"result":{"message_id":5}}`)
				default:
					t.Errorf("unexpected Telegram request: %s", r.URL.Path)
				}
			}))
			defer api.Close()
			b, err := bot.New("123:test", bot.WithSkipGetMe(), bot.WithServerURL(api.URL))
			if err != nil {
				t.Fatal(err)
			}
			h := Handler{translation: &translation.Manager{}}
			h.SetPinnedMessageOnStart(func(ctx context.Context, id int64) error {
				if id != 101 {
					t.Errorf("wrong chat: %d", id)
				}
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > 6*time.Second {
					t.Error("pin must have a bounded timeout")
				}
				if fails {
					order = append(order, "pin")
					return errors.New("Telegram unavailable")
				}
				_, err := b.PinChatMessage(ctx, &bot.PinChatMessageParams{ChatID: id, MessageID: 3})
				return err
			})
			h.sendCommandStartMenu(context.Background(), b, 101, "ru", [][]models.InlineKeyboardButton{{{Text: "Open cabinet", CallbackData: "cabinet"}}})
			if !reflect.DeepEqual(order, []string{"pin", "menu"}) {
				t.Fatalf("wrong order: %v", order)
			}
		})
	}
}

package handler

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"link-bot/internal/database"
)

type userActivityStore interface {
	FindByTelegramId(context.Context, int64) (*database.Customer, error)
	RecordUserActivity(context.Context, database.UserActivity) error
}

// This records intent; committed payment results are recorded by the database.
// Arbitrary chat messages, callback parameters and command arguments stay out
// of the log. The normal handler always runs even if recording fails.
func UserActivityMiddleware(repo userActivityStore) bot.Middleware {
	return func(next bot.HandlerFunc) bot.HandlerFunc {
		return func(ctx context.Context, b *bot.Bot, update *models.Update) {
			next(ctx, b, update)
			id, action, title := telegramActivity(update)
			if repo == nil || id <= 0 || action == "" {
				return
			}
			recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
			defer cancel()
			customer, err := repo.FindByTelegramId(recordCtx, id)
			if err == nil && customer != nil {
				err = repo.RecordUserActivity(recordCtx, database.UserActivity{CustomerID: customer.ID, Source: "telegram", Action: action, Title: title, Status: "info"})
			}
			if err != nil {
				slog.Warn("telegram: record user activity", "error", err)
			}
		}
	}
}

func telegramActivity(update *models.Update) (int64, string, string) {
	if update == nil {
		return 0, "", ""
	}
	var id int64
	var action string
	if m := update.Message; m != nil && m.From != nil && !m.From.IsBot && m.Chat.Type == models.ChatTypePrivate && m.Chat.ID == m.From.ID {
		command := strings.Fields(m.Text)
		if len(command) > 0 && strings.Split(command[0], "@")[0] == "/start" {
			id = m.From.ID
			action = "start"
		}
	} else if q := update.CallbackQuery; q != nil && !q.From.IsBot && q.Message.Message != nil && q.Message.Message.Chat.Type == models.ChatTypePrivate && q.Message.Message.Chat.ID == q.From.ID {
		id = q.From.ID
		action = strings.SplitN(q.Data, "?", 2)[0]
	}
	titles := map[string]string{
		"start": "Открыл меню бота", CallbackBuy: "Открыл тарифы в боте",
		CallbackSell: "Выбрал тариф в боте", CallbackPayment: "Запросил оплату в боте",
		CallbackTrial: "Открыл пробную подписку", CallbackActivateTrial: "Запросил пробную подписку в боте",
		CallbackConnect: "Открыл подключение", CallbackReferral: "Открыл реферальную программу",
		CallbackBroadcastSubscribe: "Запросил включение рассылок", CallbackBroadcastUnsubscribe: "Запросил отключение рассылок",
	}
	title := titles[action]
	if title == "" {
		return 0, "", ""
	}
	return id, action, title
}

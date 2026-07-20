package handler

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"log/slog"
)

func (h Handler) SyncUsersCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := "Users synced"
	if err := h.syncService.Sync(); err != nil {
		slog.Error("Error syncing users", "error", err)
		message = "Sync failed"
	}
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   message,
	})
	if err != nil {
		slog.Error("Error sending sync message", "error", err)
	}
}

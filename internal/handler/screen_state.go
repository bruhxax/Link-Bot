package handler

import (
	"context"

	"github.com/go-telegram/bot"
)

func (h Handler) rememberScreenMessage(chatID int64, messageID int) {
	if h.screenMessageCache == nil || chatID == 0 || messageID <= 0 {
		return
	}
	h.screenMessageCache.Set(chatID, messageID)
}

func (h Handler) clearScreenMessage(chatID int64) {
	if h.screenMessageCache == nil || chatID == 0 {
		return
	}
	h.screenMessageCache.Delete(chatID)
}

func (h Handler) deleteTrackedScreenMessage(ctx context.Context, b *bot.Bot, chatID int64) {
	if h.screenMessageCache == nil || b == nil || chatID == 0 {
		return
	}

	messageID, ok := h.screenMessageCache.Get(chatID)
	if !ok || messageID <= 0 {
		return
	}

	_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    chatID,
		MessageID: messageID,
	})
	h.screenMessageCache.Delete(chatID)
}

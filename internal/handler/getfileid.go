package handler

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func (h Handler) GetFileIDCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update == nil || update.Message == nil {
		return
	}
	msg := update.Message

	// Можно использовать так:
	// 1) отправить фото и затем /getfileid ответом на него
	// 2) или отправить фото с подписью "/getfileid"
	target := msg
	if msg.ReplyToMessage != nil {
		target = msg.ReplyToMessage
	}

	fileID, kind := extractFileID(target)
	if fileID == "" {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text: "Пришли/перешли фото/видео/документ и:\n" +
				"• либо ответь на него командой /getfileid\n" +
				"• либо добавь подпись /getfileid\n\n" +
				"Я верну file_id для использования в image_url.",
		})
		return
	}

	// Подсказка: это можно прямо вставлять в translations/*.json как image_url
	out := fmt.Sprintf(
		"✅ Найден %s\n\nfile_id:\n<code>%s</code>\n\nВставь это в translations/ru.json и en.json:\n<code>\"image_url\": \"%s\"</code>",
		kind,
		fileID,
		fileID,
	)

	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    msg.Chat.ID,
		Text:      out,
		ParseMode: models.ParseModeHTML,
	})
}

func extractFileID(m *models.Message) (fileID string, kind string) {
	if m == nil {
		return "", ""
	}

	// Фото: берём самое большое (последний элемент)
	if len(m.Photo) > 0 {
		p := m.Photo[len(m.Photo)-1]
		return p.FileID, "photo"
	}

	if m.Document != nil {
		return m.Document.FileID, "document"
	}

	if m.Video != nil {
		return m.Video.FileID, "video"
	}

	if m.Animation != nil {
		return m.Animation.FileID, "animation"
	}

	if m.Audio != nil {
		return m.Audio.FileID, "audio"
	}

	// Иногда отправляют как "файл" (document) — уже обработано выше.
	// Остальное (стикеры и т.п.) можно добавить при желании.

	// Если кто-то попытался /getfileid текстом — просто нет file_id.
	_ = strings.TrimSpace(m.Text)
	return "", ""
}

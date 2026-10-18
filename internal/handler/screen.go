package handler

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"log/slog"
)

var screenImageClient = &http.Client{Timeout: 5 * time.Second}

func readImageSourceContext(ctx context.Context, source string) ([]byte, string, error) {
	source = strings.TrimSpace(source)
	if strings.HasPrefix(strings.ToLower(source), "http://") || strings.HasPrefix(strings.ToLower(source), "https://") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
		if err != nil {
			return nil, "", err
		}
		resp, err := screenImageClient.Do(req)
		if err != nil {
			return nil, "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, "", io.ErrUnexpectedEOF
		}

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, "", err
		}

		filename := filepath.Base(resp.Request.URL.Path)
		if filename == "." || filename == "/" || filename == "" {
			filename = "screen.png"
		}
		return data, filename, nil
	}

	data, err := os.ReadFile(source)
	if err != nil {
		return nil, "", err
	}

	filename := filepath.Base(source)
	if filename == "." || filename == "/" || filename == "" {
		filename = "screen.png"
	}

	return data, filename, nil
}

// В твоей версии go-telegram/bot нельзя сделать EditMessageMedia с upload (Media ожидает string).
// Поэтому делаем надёжно: удаляем старое сообщение и отправляем новое фото+caption+кнопки.
// При этом мусора не будет — остаётся одно актуальное меню-сообщение.
func (h Handler) editScreen(
	ctx context.Context,
	b *bot.Bot,
	chatID int64,
	messageID int,
	imageURL string,
	caption string,
	keyboard [][]models.InlineKeyboardButton,
) {
	if strings.TrimSpace(imageURL) == "" {
		replyMarkup := models.InlineKeyboardMarkup{InlineKeyboard: keyboard}
		_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:      chatID,
			MessageID:   messageID,
			Text:        caption,
			ParseMode:   models.ParseModeHTML,
			ReplyMarkup: replyMarkup,
		})
		if err == nil {
			h.rememberScreenMessage(chatID, messageID)
			return
		}

		msg, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:      chatID,
			Text:        caption,
			ParseMode:   models.ParseModeHTML,
			ReplyMarkup: replyMarkup,
		})
		if sendErr != nil {
			slog.Error("send text-only screen failed", "error", sendErr)
			return
		}
		if messageID > 0 {
			_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: chatID, MessageID: messageID})
		}
		h.rememberScreenMessage(chatID, msg.ID)
		return
	}

	// Сначала отправляем новый экран, и только потом удаляем старый.
	// Так пользователь не увидит "пустоту", если Telegram временно не примет новое сообщение.
	msg, err := telegramScreenPhotos.send(ctx, b, imageURL, &bot.SendPhotoParams{
		ChatID:      chatID,
		Caption:     caption,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: keyboard},
	})
	if err != nil {
		slog.Error("send screen photo failed", "error", err)
		// если фото не отправилось — хотя бы текст
		fallbackMsg, _ := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    chatID,
			Text:      caption,
			ParseMode: models.ParseModeHTML,
			ReplyMarkup: models.InlineKeyboardMarkup{
				InlineKeyboard: keyboard,
			},
		})
		if fallbackMsg != nil {
			if messageID > 0 {
				_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
					ChatID:    chatID,
					MessageID: messageID,
				})
			}
			h.rememberScreenMessage(chatID, fallbackMsg.ID)
		}
		return
	}

	if messageID > 0 {
		_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
			ChatID:    chatID,
			MessageID: messageID,
		})
	}
	h.rememberScreenMessage(chatID, msg.ID)
}

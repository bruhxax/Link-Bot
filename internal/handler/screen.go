package handler

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"log/slog"
)

func (h Handler) downloadAsUpload(source string) (*models.InputFileUpload, error) {
	data, filename, err := readImageSource(source)
	if err != nil {
		return nil, err
	}
	return &models.InputFileUpload{
		Filename: filename,
		Data:     bytes.NewReader(data),
	}, nil
}

func readImageSource(source string) ([]byte, string, error) {
	source = strings.TrimSpace(source)
	if strings.HasPrefix(strings.ToLower(source), "http://") || strings.HasPrefix(strings.ToLower(source), "https://") {
		resp, err := http.Get(source)
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

	upload, err := h.downloadAsUpload(imageURL)
	if err != nil {
		slog.Error("download screen image failed", "error", err)
		// fallback: просто обновим caption если получится
		_, editErr := b.EditMessageCaption(ctx, &bot.EditMessageCaptionParams{
			ChatID:    chatID,
			MessageID: messageID,
			Caption:   caption,
			ParseMode: models.ParseModeHTML,
			ReplyMarkup: models.InlineKeyboardMarkup{
				InlineKeyboard: keyboard,
			},
		})
		if editErr == nil {
			h.rememberScreenMessage(chatID, messageID)
		}
		return
	}

	// Сначала отправляем новый экран, и только потом удаляем старый.
	// Так пользователь не увидит "пустоту", если Telegram временно не примет новое сообщение.
	msg, err := b.SendPhoto(ctx, &bot.SendPhotoParams{
		ChatID:      chatID,
		Photo:       upload,
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

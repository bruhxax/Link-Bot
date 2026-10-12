package payment

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// SendContentPreview sends an isolated message without purchases or screen tracking.
func SendContentPreview(ctx context.Context, telegramBot *bot.Bot, chatID int64, text, banner string, keyboard [][]models.InlineKeyboardButton) error {
	if telegramBot == nil || chatID <= 0 {
		return errors.New("Telegram недоступен для тестовой отправки")
	}
	if strings.TrimSpace(text) == "" {
		return errors.New("Заполните текст сообщения")
	}
	markup := models.InlineKeyboardMarkup{InlineKeyboard: keyboard}
	if strings.TrimSpace(banner) != "" {
		data, filename, err := readTelegramImage(banner)
		if err != nil {
			return err
		}
		if ext := strings.ToLower(filepath.Ext(filename)); ext == ".gif" || ext == ".mp4" {
			_, err = telegramBot.SendAnimation(ctx, &bot.SendAnimationParams{ChatID: chatID, Animation: &models.InputFileUpload{Filename: filename, Data: bytes.NewReader(data)}, Caption: text, ParseMode: models.ParseModeHTML, ReplyMarkup: markup})
			return err
		}
		_, err = telegramBot.SendPhoto(ctx, &bot.SendPhotoParams{
			ChatID: chatID, Photo: &models.InputFileUpload{Filename: filename, Data: bytes.NewReader(data)},
			Caption: text, ParseMode: models.ParseModeHTML, ReplyMarkup: markup,
		})
		return err
	}
	_, err := telegramBot.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text, ParseMode: models.ParseModeHTML, ReplyMarkup: markup})
	return err
}

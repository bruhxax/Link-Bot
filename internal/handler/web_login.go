package handler

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"link-bot/internal/config"
	"link-bot/internal/webauth"
	"link-bot/utils"
)

func (h Handler) handleWebLoginStart(ctx context.Context, b *bot.Bot, update *models.Update, startParameter string) bool {
	approvalToken, ok := webauth.ApprovalToken(startParameter)
	if !ok {
		return false
	}

	message := update.Message
	if message == nil || message.From == nil || h.webLogin == nil {
		h.sendWebLoginMessage(ctx, b, message, "expired")
		return true
	}

	user := message.From
	err := h.webLogin.Approve(approvalToken, webauth.TelegramUser{
		ID:           user.ID,
		FirstName:    user.FirstName,
		LastName:     user.LastName,
		Username:     user.Username,
		LanguageCode: user.LanguageCode,
	}, time.Now().UTC())
	if err != nil {
		if !errors.Is(err, webauth.ErrNotFound) && !errors.Is(err, webauth.ErrExpired) && !errors.Is(err, webauth.ErrAlreadyClaimed) {
			slog.Error("web login approval failed", "error", err, "telegramId", utils.MaskHalfInt64(user.ID))
		}
		h.sendWebLoginMessage(ctx, b, message, "expired")
		return true
	}

	slog.Info("web login approved", "telegramId", utils.MaskHalfInt64(user.ID))
	h.sendWebLoginMessage(ctx, b, message, "approved")
	return true
}

func (h Handler) sendWebLoginMessage(ctx context.Context, b *bot.Bot, message *models.Message, status string) {
	if b == nil || message == nil {
		return
	}

	language := ""
	if message.From != nil {
		language = strings.ToLower(strings.TrimSpace(message.From.LanguageCode))
	}
	if language == "" {
		language = strings.ToLower(strings.TrimSpace(config.DefaultLanguage()))
	}

	text := "<b>Вход подтверждён</b>\n\nВернитесь в браузер — кабинет откроется автоматически."
	if status != "approved" {
		text = "<b>QR-код уже недействителен</b>\n\nВернитесь в браузер и отсканируйте новый код."
	}
	if strings.HasPrefix(language, "en") {
		text = "<b>Sign-in confirmed</b>\n\nReturn to the browser — your dashboard will open automatically."
		if status != "approved" {
			text = "<b>This QR code has expired</b>\n\nReturn to the browser and scan the new code."
		}
	} else if strings.HasPrefix(language, "fa") {
		text = "<b>ورود تأیید شد</b>\n\nبه مرورگر برگردید؛ پنل به‌طور خودکار باز می‌شود."
		if status != "approved" {
			text = "<b>این کد QR منقضی شده است</b>\n\nبه مرورگر برگردید و کد جدید را اسکن کنید."
		}
	}

	if _, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    message.Chat.ID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	}); err != nil {
		slog.Error("send web login confirmation failed", "error", err)
	}
}

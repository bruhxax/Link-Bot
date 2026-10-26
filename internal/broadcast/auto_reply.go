package broadcast

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"link-bot/internal/config"
	"link-bot/internal/database"
)

type autoReplyStore interface {
	AutoReplySettings(context.Context) (*database.BroadcastAutoReplySettings, error)
	SaveAutoReplySettings(context.Context, database.BroadcastAutoReplySettings) (*database.BroadcastAutoReplySettings, error)
	RecordBroadcastRecipient(context.Context, int64, int, time.Time) error
	ClaimBroadcastReply(context.Context, int64, int, int, time.Time, int) (int, error)
	FinishBroadcastReply(context.Context, int64, int, int, bool) error
}

var ErrAutoReplySettings = errors.New("Некорректные настройки автоответа")

func normalizeAutoReplySettings(value *database.BroadcastAutoReplySettings) error {
	value.Text = strings.TrimSpace(value.Text)
	value.SupportButtonText = strings.TrimSpace(value.SupportButtonText)
	if value.WindowHours < 1 || value.WindowHours > 168 {
		return fmt.Errorf("%w: срок от 1 до 168 часов", ErrAutoReplySettings)
	}
	if len([]rune(value.Text)) < 1 || len([]rune(value.Text)) > 3500 {
		return fmt.Errorf("%w: текст от 1 до 3500 символов", ErrAutoReplySettings)
	}
	if looksLikeBroadcastHTML(value.Text) {
		if err := ValidateHTML(value.Text); err != nil {
			return fmt.Errorf("%w: %v", ErrAutoReplySettings, err)
		}
	}
	if value.SupportButtonText == "" {
		value.SupportButtonText = "Поддержка"
	}
	if len([]rune(value.SupportButtonText)) > 64 {
		return fmt.Errorf("%w: подпись кнопки до 64 символов", ErrAutoReplySettings)
	}
	return nil
}

func (s *Service) SaveAutoReply(ctx context.Context, value database.BroadcastAutoReplySettings) (*database.BroadcastAutoReplySettings, error) {
	if err := normalizeAutoReplySettings(&value); err != nil {
		return nil, err
	}
	if value.UpdatedAt.IsZero() {
		return nil, ErrDraftChanged
	}
	result, err := s.replies.SaveAutoReplySettings(ctx, value)
	if err == nil && result == nil {
		return nil, ErrDraftChanged
	}
	return result, err
}

// Commands, payments, service updates, groups and bot messages keep their normal handlers.
func IsAutoReplyCandidate(update *models.Update) bool {
	if update == nil || update.Message == nil {
		return false
	}
	m := update.Message
	if m.From == nil || m.From.IsBot || m.From.ID <= 0 || m.Chat.Type != models.ChatTypePrivate || m.Chat.ID != m.From.ID || m.ID <= 0 || m.SuccessfulPayment != nil {
		return false
	}
	return !strings.HasPrefix(strings.TrimSpace(m.Text), "/") && !strings.HasPrefix(strings.TrimSpace(m.Caption), "/") && messageKind(m) != ""
}

func (s *Service) HandleAutoReply(ctx context.Context, _ *bot.Bot, update *models.Update) {
	if s == nil || s.replies == nil || !IsAutoReplyCandidate(update) {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := s.replyToBroadcast(ctx, update.Message); err != nil {
		slog.Warn("broadcast: automatic reply failed", "error", err)
	}
}

func (s *Service) replyToBroadcast(ctx context.Context, message *models.Message) error {
	settings, err := s.replies.AutoReplySettings(ctx)
	if err != nil || settings == nil || !settings.Enabled {
		return err
	}
	if err = normalizeAutoReplySettings(settings); err != nil {
		return err
	}
	if s.customerRepository != nil {
		allowed, err := s.customerRepository.BroadcastTelegramAllowed(ctx, message.Chat.ID, true)
		if err != nil || !allowed {
			return err
		}
	}
	incomingAt := time.Now().UTC()
	if message.Date > 0 {
		incomingAt = time.Unix(int64(message.Date), 0).UTC()
	}
	replyToID := 0
	if message.ReplyToMessage != nil {
		replyToID = message.ReplyToMessage.ID
	}
	broadcastID, err := s.replies.ClaimBroadcastReply(ctx, message.Chat.ID, message.ID, replyToID, incomingAt, settings.WindowHours)
	if err != nil || broadcastID == 0 {
		return err
	}
	text := settings.Text
	if !looksLikeBroadcastHTML(text) {
		text = html.EscapeString(text)
	}
	params := &bot.SendMessageParams{ChatID: message.Chat.ID, Text: text, ParseMode: models.ParseModeHTML}
	if settings.SupportButton {
		if config.GetMiniAppURL() != "" {
			params.ReplyMarkup = buildKeyboard([]database.BroadcastButton{{Type: "support", Text: settings.SupportButtonText, Style: "primary", IconCustomEmojiID: "5206222720416643915"}})
		} else if url := config.SupportURL(); strings.HasPrefix(url, "https://") {
			params.ReplyMarkup = &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: settings.SupportButtonText, URL: url}}}}
		}
	}
	_, sendErr := s.telegramBot.SendMessage(ctx, params)
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	finishErr := s.replies.FinishBroadcastReply(finishCtx, message.Chat.ID, broadcastID, message.ID, sendErr == nil)
	return errors.Join(sendErr, finishErr)
}

func (s *Service) rememberBroadcastRecipient(ctx context.Context, chatID int64, messageID int, sentAt time.Time) {
	if s.replies == nil {
		return
	}
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.replies.RecordBroadcastRecipient(recordCtx, chatID, messageID, sentAt.Truncate(time.Second)); err != nil {
		slog.Warn("broadcast: record automatic reply recipient failed", "error", err)
	}
}

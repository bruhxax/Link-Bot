package broadcast

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"link-bot/internal/config"
	"link-bot/internal/database"
)

var (
	ErrDirectRecipient = errors.New("direct message recipient is unavailable")
	ErrDirectPreview   = errors.New("preview the direct message before sending")
	ErrDirectState     = errors.New("direct message is busy or changed")
)

func (s *Service) GetDirect(ctx context.Context, adminID int64) (*database.DirectMessageDraft, error) {
	return s.repository.GetDirect(ctx, adminID)
}

func (s *Service) StartDirectCapture(ctx context.Context, adminID, customerID int64) (*database.DirectMessageDraft, error) {
	customer, err := s.customerRepository.FindById(ctx, customerID)
	if err != nil {
		return nil, err
	}
	if customer == nil || customer.TelegramID <= 0 || customer.TelegramIDIsSynthetic {
		return nil, ErrDirectRecipient
	}
	broadcastDraft, err := s.repository.Get(ctx)
	if err != nil {
		return nil, err
	}
	if broadcastDraft != nil && broadcastDraft.Status == database.BroadcastStatusAwaitingMessage {
		return nil, ErrDirectState
	}
	draft, err := s.repository.StartDirectCapture(ctx, adminID, customerID)
	if err != nil {
		return nil, err
	}
	if draft == nil {
		return nil, ErrDirectState
	}
	_, err = s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      adminID,
		Text:        "<b>Личное сообщение пользователю</b>\n\nОтправьте сюда текст, фото, видео, аудио или файл. Ссылки, форматирование и Telegram HTML сохранятся. Затем вернитесь в карточку пользователя, нажмите «Проверить» и «Отправить».",
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: &models.ForceReply{ForceReply: true, InputFieldPlaceholder: "Сообщение пользователю", Selective: true},
	})
	if err != nil {
		_ = s.repository.CancelDirectCapture(ctx, adminID)
		return nil, fmt.Errorf("send direct capture prompt: %w", err)
	}
	return draft, nil
}

// CaptureDirectMessage runs before broadcast capture, only in the admin's private chat.
func (s *Service) CaptureDirectMessage(ctx context.Context, message *models.Message) (bool, error) {
	if message == nil || message.From == nil || message.Chat.ID != message.From.ID {
		return false, nil
	}
	draft, err := s.repository.GetDirect(ctx, message.From.ID)
	if err != nil || draft == nil || draft.Status != "awaiting_message" {
		return false, err
	}
	rawText := strings.TrimSpace(message.Text)
	if rawText == "" {
		rawText = strings.TrimSpace(message.Caption)
	}
	if strings.HasPrefix(rawText, "/") {
		return false, nil
	}
	kind := messageKind(message)
	sourceHTML := ""
	if kind == "text" && looksLikeBroadcastHTML(message.Text) {
		if err := ValidateHTML(message.Text); err != nil {
			_, _ = s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{ChatID: message.Chat.ID, Text: "HTML не сохранён: " + err.Error() + ". Исправьте разметку и отправьте снова."})
			return true, nil
		}
		kind = "html"
		sourceHTML = strings.TrimSpace(message.Text)
	}
	if kind == "" {
		_, _ = s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{ChatID: message.Chat.ID, Text: "Этот тип сообщения не поддерживается. Отправьте текст, фото, видео, аудио или файл."})
		return true, nil
	}
	draft, err = s.repository.SaveDirectSource(ctx, message.From.ID, message.ID, kind, messagePreview(message, kind), sourceHTML)
	if err != nil {
		return true, err
	}
	if draft == nil {
		return true, ErrDirectState
	}
	var replyMarkup models.ReplyMarkup
	if link := adminDirectURL(draft.CustomerID); link != "" {
		replyMarkup = &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "Открыть карточку пользователя", WebApp: &models.WebAppInfo{URL: link}}}}}
	}
	_, _ = s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{ChatID: message.Chat.ID, Text: "Сообщение сохранено. Вернитесь в карточку пользователя и нажмите «Проверить».", ReplyMarkup: replyMarkup})
	return true, nil
}

func (s *Service) PreviewDirect(ctx context.Context, adminID, customerID int64) (*database.DirectMessageDraft, error) {
	draft, err := s.repository.GetDirect(ctx, adminID)
	if err != nil {
		return nil, err
	}
	if draft == nil || draft.CustomerID != customerID {
		return nil, ErrDirectState
	}
	if !draft.HasSource() {
		return nil, ErrNoMessage
	}
	if draft.Status != "draft" {
		return nil, ErrDirectState
	}
	if err := s.copyTo(ctx, directAsBroadcast(draft), adminID, nil); err != nil {
		return nil, fmt.Errorf("send direct preview: %w", err)
	}
	marked, err := s.repository.MarkDirectPreviewed(ctx, adminID, *draft.SourceMessageID)
	if marked == nil && err == nil {
		return nil, ErrDirectState
	}
	return marked, err
}

func (s *Service) SendDirect(ctx context.Context, adminID, customerID int64) (*database.DirectMessageDraft, error) {
	draft, err := s.repository.GetDirect(ctx, adminID)
	if err != nil {
		return nil, err
	}
	if draft == nil || draft.CustomerID != customerID {
		return nil, ErrDirectState
	}
	if !draft.HasSource() {
		return nil, ErrNoMessage
	}
	if draft.PreviewedAt == nil {
		return nil, ErrDirectPreview
	}
	customer, err := s.customerRepository.FindById(ctx, customerID)
	if err != nil {
		return nil, err
	}
	if customer == nil || customer.TelegramID <= 0 || customer.TelegramIDIsSynthetic {
		return nil, ErrDirectRecipient
	}
	draft, err = s.repository.BeginDirectSend(ctx, adminID, customerID)
	if err != nil {
		return nil, err
	}
	if draft == nil {
		return nil, ErrDirectState
	}
	if err := s.copyTo(ctx, directAsBroadcast(draft), customer.TelegramID, nil); err != nil {
		_ = s.repository.FinishDirectSend(ctx, adminID, false)
		return nil, fmt.Errorf("send direct message: %w", err)
	}
	if err := s.repository.FinishDirectSend(ctx, adminID, true); err != nil {
		return nil, err
	}
	return s.repository.GetDirect(ctx, adminID)
}

func directAsBroadcast(draft *database.DirectMessageDraft) database.BroadcastDraft {
	return database.BroadcastDraft{SourceChatID: draft.SourceChatID, SourceMessageID: draft.SourceMessageID, SourceKind: draft.SourceKind, SourceHTML: draft.SourceHTML}
}

func adminDirectURL(customerID int64) string {
	parsed, err := url.Parse(strings.TrimSpace(config.GetMiniAppURL()))
	if err != nil || parsed.Host == "" {
		return ""
	}
	query := parsed.Query()
	query.Set("page", "admin")
	query.Set("admin", "user-message")
	query.Set("customerId", strconv.FormatInt(customerID, 10))
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

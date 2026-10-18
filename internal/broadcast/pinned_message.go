package broadcast

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"link-bot/internal/database"
)

func (s *Service) ConfigurePinnedMessages(repository *database.PinnedMessageRepository) {
	s.pinnedRepository = repository
	s.pinnedWake = make(chan struct{}, 1)
	s.pinnedChats = make(chan int64, 128)
}

func (s *Service) WakePinnedMessagesForChat(chatID int64) {
	if s == nil || s.pinnedChats == nil || chatID <= 0 {
		return
	}
	select {
	case s.pinnedChats <- chatID:
	default:
	}
	s.WakePinnedMessages()
}

func (s *Service) WakePinnedMessages() {
	if s == nil || s.pinnedWake == nil {
		return
	}
	select {
	case s.pinnedWake <- struct{}{}:
	default:
	}
}

func (s *Service) PinnedState(ctx context.Context) (*database.PinnedMessage, error) {
	if s == nil || s.pinnedRepository == nil {
		return nil, errors.New("pinned messages unavailable")
	}
	return s.pinnedRepository.State(ctx)
}

func (s *Service) checkPinnedCapture(ctx context.Context, adminID int64) error {
	if s.pinnedRepository == nil {
		return nil
	}
	p, err := s.pinnedRepository.Get(ctx)
	if err != nil {
		return err
	}
	if p.AwaitingMessage && *p.CaptureAdminID == adminID {
		return ErrRunning
	}
	return nil
}

func (s *Service) StartPinnedCapture(ctx context.Context, adminID int64) (*database.PinnedMessage, error) {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if s.pinnedRepository == nil {
		return nil, errors.New("pinned messages unavailable")
	}
	draft, err := s.repository.Get(ctx)
	if err != nil {
		return nil, err
	}
	if draft != nil && draft.Status == database.BroadcastStatusAwaitingMessage && draft.UpdatedBy != nil && *draft.UpdatedBy == adminID {
		return nil, ErrRunning
	}
	direct, err := s.repository.GetDirect(ctx, adminID)
	if err != nil {
		return nil, err
	}
	if direct != nil && direct.Status == database.BroadcastStatusAwaitingMessage {
		return nil, ErrRunning
	}
	if err = s.pinnedRepository.StartCapture(ctx, adminID); err != nil {
		return nil, err
	}
	_, err = s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: adminID, ParseMode: models.ParseModeHTML,
		Text:        "<b>Закреплённое сообщение</b>\n\nОтправьте одно сообщение: текст, фото, видео, аудио или файл. Форматирование, ссылки и premium emoji сохранятся. Можно использовать Telegram HTML.\n\nЕсли закреп включён, новое сообщение заменит его у всех пользователей. Если выключен — сохранится для проверки. Отмена: /cancel.",
		ReplyMarkup: &models.ForceReply{ForceReply: true, Selective: true, InputFieldPlaceholder: "Сообщение для закрепления"},
	})
	if err != nil {
		_ = s.pinnedRepository.CancelCapture(ctx, adminID)
		return nil, err
	}
	return s.PinnedState(ctx)
}

func (s *Service) CancelPinnedCapture(ctx context.Context, adminID int64) (*database.PinnedMessage, error) {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if err := s.pinnedRepository.CancelCapture(ctx, adminID); err != nil {
		return nil, err
	}
	return s.PinnedState(ctx)
}

func (s *Service) CapturePinnedMessage(ctx context.Context, message *models.Message) (bool, error) {
	if s == nil || s.pinnedRepository == nil || message == nil || message.From == nil || message.Chat.ID != message.From.ID {
		return false, nil
	}
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	p, err := s.pinnedRepository.Get(ctx)
	if err != nil {
		return false, err
	}
	if !p.AwaitingMessage || *p.CaptureAdminID != message.From.ID {
		return false, nil
	}
	if strings.TrimSpace(message.Text) == "/cancel" {
		err = s.pinnedRepository.CancelCapture(ctx, message.From.ID)
		if err == nil {
			_, _ = s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{ChatID: message.Chat.ID, Text: "Ввод отменён. Прежнее закреплённое сообщение сохранено."})
		}
		return true, err
	}
	text := strings.TrimSpace(message.Text)
	if text == "" {
		text = strings.TrimSpace(message.Caption)
	}
	if strings.HasPrefix(text, "/") {
		return false, nil
	}
	kind := messageKind(message)
	if kind == "" || message.MediaGroupID != "" {
		_, err = s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{ChatID: message.Chat.ID, Text: "Отправьте одно сообщение: текст, фото, видео, аудио или файл. Альбом разделите на отдельные сообщения."})
		return true, err
	}
	var sourceID int
	if kind == "text" && looksLikeBroadcastHTML(message.Text) && len(message.Entities) == 0 {
		if err = ValidateHTML(message.Text); err != nil {
			_, _ = s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{ChatID: message.Chat.ID, Text: "HTML не сохранён: " + err.Error() + ". Исправьте разметку и отправьте снова."})
			return true, nil
		}
		// Save a bot-owned, already formatted copy. Deleting the admin's input
		// will not invalidate copies for thousands of recipients later on.
		var saved *models.Message
		saved, err = s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{ChatID: message.Chat.ID, Text: message.Text, ParseMode: models.ParseModeHTML})
		if err == nil {
			sourceID = saved.ID
			kind = "html"
		}
	} else {
		var saved *models.MessageID
		saved, err = s.telegramBot.CopyMessage(ctx, &bot.CopyMessageParams{ChatID: message.Chat.ID, FromChatID: message.Chat.ID, MessageID: message.ID, DisableNotification: true})
		if err == nil {
			sourceID = saved.ID
		}
	}
	if err != nil {
		return true, fmt.Errorf("save pinned source: %w", err)
	}
	if err = s.pinnedRepository.SaveSource(ctx, message.From.ID, sourceID, kind, messagePreview(message, kind)); err != nil {
		return true, err
	}
	s.WakePinnedMessages()
	var markup models.ReplyMarkup
	if link := adminBroadcastURL(); link != "" {
		if u, e := url.Parse(link); e == nil {
			q := u.Query()
			q.Set("admin", "content")
			q.Set("content", "start")
			u.RawQuery = q.Encode()
			link = u.String()
		}
		markup = &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "Открыть редактор контента", WebApp: &models.WebAppInfo{URL: link}}}}}
	}
	status := "Сообщение сохранено. Вернитесь в редактор контента: проверьте его и включите «Закрепить»."
	if p.Enabled {
		status = "Сообщение сохранено. Бот заменяет закреп у пользователей в фоне."
	}
	_, _ = s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{ChatID: message.Chat.ID, Text: status, ReplyMarkup: markup})
	return true, nil
}

func (s *Service) SetPinnedEnabled(ctx context.Context, enabled bool, revision int64) (*database.PinnedMessage, error) {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if s.pinnedRepository == nil {
		return nil, errors.New("pinned messages unavailable")
	}
	if err := s.pinnedRepository.SetEnabled(ctx, enabled, revision); err != nil {
		return nil, err
	}
	s.WakePinnedMessages()
	return s.PinnedState(ctx)
}

func (s *Service) PreviewPinnedMessage(ctx context.Context, adminID int64) (*database.PinnedMessage, error) {
	p, err := s.pinnedRepository.Get(ctx)
	if err != nil {
		return nil, err
	}
	if !p.HasMessage {
		return nil, ErrNoMessage
	}
	_, err = s.telegramBot.CopyMessage(ctx, &bot.CopyMessageParams{ChatID: adminID, FromChatID: *p.SourceChatID, MessageID: *p.SourceMessageID, DisableNotification: true})
	if err != nil {
		return nil, err
	}
	return s.PinnedState(ctx)
}

func (s *Service) RetryPinnedMessages(ctx context.Context) (*database.PinnedMessage, error) {
	if err := s.pinnedRepository.Retry(ctx); err != nil {
		return nil, err
	}
	s.WakePinnedMessages()
	return s.PinnedState(ctx)
}

// Persistent reconciliation runs independently of Telegram update handlers.
// Version checkpoints resume interrupted sends and include newly registered users.
func (s *Service) RunPinnedMessages(ctx context.Context) {
	if s.pinnedRepository == nil {
		return
	}
	for ctx.Err() == nil {
		var chats []int64
	drainChats:
		for len(chats) < 128 {
			select {
			case chatID := <-s.pinnedChats:
				chats = append(chats, chatID)
			default:
				break drainChats
			}
		}
		if len(chats) > 0 {
			if err := s.pinnedRepository.RetryChats(ctx, chats); err != nil {
				slog.Warn("pinned message retry after start", "error", err)
			}
			s.pinnedCursor = 0
		}
		worked, err := s.processPinnedBatch(ctx)
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, database.ErrPinnedStateChanged) {
			slog.Warn("pinned messages worker", "error", err)
		}
		if worked && err == nil {
			continue
		}
		timer := time.NewTimer(30 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-s.pinnedWake:
			timer.Stop()
		case <-timer.C:
		}
	}
}

func (s *Service) processPinnedBatch(ctx context.Context) (bool, error) {
	// The connection holding this lock is reserved only while a batch is active.
	lockCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	unlock, err := s.pinnedRepository.WorkerLock(lockCtx)
	cancel()
	if err != nil {
		return false, err
	}
	defer unlock()
	p, err := s.pinnedRepository.Get(ctx)
	if err != nil {
		return false, err
	}
	if p.Enabled && !p.HasMessage {
		return false, ErrNoMessage
	}
	if s.pinnedRevision != p.Revision {
		s.pinnedCursor = 0
		s.pinnedRevision = p.Revision
	}
	items, err := s.pinnedRepository.Pending(ctx, p, s.pinnedCursor)
	if err != nil {
		return false, err
	}
	for _, d := range items {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		current, err := s.pinnedRepository.Get(ctx)
		if err != nil {
			return false, err
		}
		if current.Revision != p.Revision {
			return true, nil
		}
		// At most five recipients/second leaves capacity for ordinary bot replies.
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return false, ctx.Err()
		case <-timer.C:
		}
		deliveryCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		err = s.applyPinnedMessage(deliveryCtx, p, d)
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			retryAt := time.Now().Add(time.Minute)
			var limited *bot.TooManyRequestsError
			if errors.As(err, &limited) {
				retryAt = time.Now().Add(time.Duration(limited.RetryAfter+1) * time.Second)
			}
			if saveErr := s.pinnedRepository.MarkFailed(ctx, d.ChatID, p.Revision, err.Error(), retryAt); saveErr != nil {
				return false, saveErr
			}
			if limited != nil {
				// Respect Telegram's global cooldown rather than hitting every chat.
				timer := time.NewTimer(time.Until(retryAt))
				select {
				case <-ctx.Done():
					timer.Stop()
					return false, ctx.Err()
				case <-timer.C:
				}
			}
		}
		s.pinnedCursor = d.ChatID
	}
	if len(items) == 0 {
		s.pinnedCursor = 0
	}
	return len(items) > 0, nil
}

func (s *Service) applyPinnedMessage(ctx context.Context, p *database.PinnedMessage, d database.PinnedDelivery) error {
	if p.Enabled {
		allowed, err := s.customerRepository.BroadcastTelegramAllowed(ctx, d.ChatID, true)
		if err != nil {
			return err
		}
		if !allowed {
			return errors.New("пользователь недоступен")
		}
	}
	if d.MessageID != nil && (!p.Enabled || d.SourceVersion != p.SourceVersion) {
		_, err := s.telegramBot.UnpinChatMessage(ctx, &bot.UnpinChatMessageParams{ChatID: d.ChatID, MessageID: *d.MessageID})
		if err != nil && !pinnedMessageMissing(err) {
			return err
		}
	}
	if p.Enabled {
		if d.MessageID == nil || d.SourceVersion != p.SourceVersion {
			copied, err := s.telegramBot.CopyMessage(ctx, &bot.CopyMessageParams{ChatID: d.ChatID, FromChatID: *p.SourceChatID, MessageID: *p.SourceMessageID, DisableNotification: true})
			if err != nil {
				return err
			}
			d.MessageID = &copied.ID
			d.SourceVersion = p.SourceVersion
			if err = s.pinnedRepository.SaveCopy(ctx, d); err != nil {
				return err
			}
		}
		_, err := s.telegramBot.PinChatMessage(ctx, &bot.PinChatMessageParams{ChatID: d.ChatID, MessageID: *d.MessageID, DisableNotification: true})
		if err != nil {
			if pinnedMessageMissing(err) {
				d.MessageID = nil
				d.SourceVersion = 0
				if e := s.pinnedRepository.SaveCopy(ctx, d); e != nil {
					return e
				}
			}
			return err
		}
	}
	return s.pinnedRepository.MarkApplied(ctx, d.ChatID, p.Revision)
}

func pinnedMessageMissing(err error) bool {
	if !errors.Is(err, bot.ErrorBadRequest) {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "message to unpin not found") || strings.Contains(text, "message to pin not found") || strings.Contains(text, "message_id_invalid") || strings.Contains(text, "message is not pinned")
}

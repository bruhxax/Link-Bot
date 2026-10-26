package broadcast

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"link-bot/internal/database"
)

type deletionStore interface {
	ScheduleDeletion(context.Context, int64, int, time.Time, int) error
	ClaimDeletions(context.Context, int) ([]database.BroadcastDeletion, error)
	FinishDeletion(context.Context, database.BroadcastDeletion, string, string, time.Time) error
}

var errDeletionRecording = errors.New("Не удалось сохранить срок удаления рассылки")

func (s *Service) SaveExpiration(ctx context.Context, adminID int64, minutes int, expected time.Time) (*database.BroadcastDraft, error) {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if err := database.ValidateBroadcastExpiration(minutes); err != nil {
		return nil, err
	}
	if expected.IsZero() {
		return nil, ErrDraftChanged
	}
	draft, err := s.repository.SaveExpiration(ctx, minutes, adminID, expected)
	if err != nil {
		return nil, err
	}
	if draft == nil {
		return nil, ErrDraftChanged
	}
	return draft, nil
}

func (s *Service) deliverBroadcast(ctx context.Context, draft database.BroadcastDraft, chatID int64, keyboard models.ReplyMarkup) error {
	if err := database.ValidateBroadcastExpiration(draft.DeleteAfterMinutes); err != nil {
		return err
	}
	if draft.DeleteAfterMinutes > 0 && s.deletions == nil {
		return errDeletionRecording
	}
	// CopyMessage returns the recipient's message ID, never the source ID.
	var messageID int
	sentAt := time.Now().UTC()
	if draft.SourceKind == "html" {
		message, err := s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: draft.SourceHTML, ParseMode: models.ParseModeHTML, ReplyMarkup: keyboard})
		if err != nil {
			return err
		}
		messageID = message.ID
		if message.Date > 0 {
			sentAt = time.Unix(int64(message.Date), 0).UTC()
		}
	} else {
		message, err := s.telegramBot.CopyMessage(ctx, &bot.CopyMessageParams{ChatID: chatID, FromChatID: *draft.SourceChatID, MessageID: *draft.SourceMessageID, ReplyMarkup: keyboard})
		if err != nil {
			return err
		}
		messageID = message.ID
		sentAt = time.Now().UTC()
	}
	if draft.DeleteAfterMinutes == 0 {
		s.rememberBroadcastRecipient(ctx, chatID, messageID, sentAt)
		return nil
	}

	// Once sent, persistence must survive cancellation of the sending operation.
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		err = s.deletions.ScheduleDeletion(recordCtx, chatID, messageID, sentAt, draft.DeleteAfterMinutes)
		if err == nil {
			s.rememberBroadcastRecipient(ctx, chatID, messageID, sentAt)
			return nil
		}
		if recordCtx.Err() != nil {
			break
		}
	}
	// Do not leave an untracked temporary message or retry sending a second copy.
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cleanupCancel()
	_, cleanupErr := s.telegramBot.DeleteMessage(cleanupCtx, &bot.DeleteMessageParams{ChatID: chatID, MessageID: messageID})
	if cleanupErr != nil {
		slog.Error("broadcast: rollback untracked temporary message failed", "error", cleanupErr, "messageId", messageID)
	}
	return fmt.Errorf("%w: %v", errDeletionRecording, err)
}

// RunMessageDeletions resumes the durable queue on startup. A lease makes a
// crashed attempt available again; deleting the same message is safe to repeat.
func (s *Service) RunMessageDeletions(ctx context.Context) {
	if s == nil || s.deletions == nil || s.telegramBot == nil {
		return
	}
	for ctx.Err() == nil {
		count, err := s.deleteExpiredMessages(ctx)
		if err != nil {
			slog.Error("broadcast: message deletion queue failed", "error", err)
		}
		delay := 10 * time.Second
		if err == nil && count == 25 {
			delay = 100 * time.Millisecond
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (s *Service) deleteExpiredMessages(ctx context.Context) (int, error) {
	items, err := s.deletions.ClaimDeletions(ctx, 25)
	if err != nil {
		return 0, err
	}
	for _, item := range items {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		now := time.Now().UTC()
		status, lastError, next := "deleted", "", now
		if !now.Before(item.SentAt.Add(48 * time.Hour)) {
			status, lastError = "failed", "Истёк срок удаления Telegram (48 часов)"
		} else {
			requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			_, deleteErr := s.telegramBot.DeleteMessage(requestCtx, &bot.DeleteMessageParams{ChatID: item.ChatID, MessageID: item.MessageID})
			cancel()
			status, lastError, next = deletionResult(deleteErr, item, time.Now().UTC())
		}
		if err := s.deletions.FinishDeletion(ctx, item, status, lastError, next); err != nil {
			return 0, err
		}
		if status == "failed" {
			slog.Warn("broadcast: temporary message could not be deleted", "messageId", item.MessageID, "error", lastError)
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return 0, ctx.Err()
		case <-timer.C:
		}
	}
	return len(items), nil
}

func deletionResult(err error, item database.BroadcastDeletion, now time.Time) (string, string, time.Time) {
	if err == nil || strings.Contains(strings.ToLower(err.Error()), "message to delete not found") {
		return "deleted", "", now
	}
	last := shortError(err)
	if errors.Is(err, bot.ErrorForbidden) || errors.Is(err, bot.ErrorBadRequest) || errors.Is(err, bot.ErrorNotFound) {
		return "failed", last, now
	}
	delay := time.Minute * time.Duration(1<<min(max(item.Attempts-1, 0), 5))
	var flood *bot.TooManyRequestsError
	if errors.As(err, &flood) {
		delay = max(time.Second*time.Duration(flood.RetryAfter), time.Second)
	}
	next := now.Add(delay)
	// Keep the last retry inside Telegram's deletion window.
	deadline := item.SentAt.Add(48*time.Hour - time.Minute)
	if !next.Before(deadline) {
		if errors.As(err, &flood) || !now.Before(deadline) {
			return "failed", last, now
		}
		next = deadline
	}
	return "pending", last, next
}

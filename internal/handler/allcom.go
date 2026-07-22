package handler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"link-bot/internal/config"
	"link-bot/utils"
)

const (
	allcomParseMode              = models.ParseModeHTML
	allcomProgressUpdateInterval = time.Second
	allcomMaxWorkers             = 16
	allcomSendInterval           = 34 * time.Millisecond
	allcomRetryDelay             = 1500 * time.Millisecond
)

type allcomDeliveryFunc func(context.Context, int64) error

func (h Handler) AllcomCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update == nil || update.Message == nil {
		return
	}

	msg := update.Message

	telegramIDs, err := h.customerRepository.ListAllTelegramIDs(ctx)
	if err != nil {
		slog.Error("/allcom: failed to fetch customers", "error", err)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "Ошибка: не удалось получить список пользователей.",
		})
		return
	}

	targets := uniqueAllcomTargets(telegramIDs, config.GetAdminTelegramId())
	if len(targets) == 0 {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "Некому отправлять рассылку: список получателей пуст.",
		})
		return
	}

	var deliver allcomDeliveryFunc

	if msg.ReplyToMessage != nil {
		src := msg.ReplyToMessage
		fromChatID := msg.Chat.ID
		deliver = func(ctx context.Context, chatID int64) error {
			_, err := b.CopyMessage(ctx, &bot.CopyMessageParams{
				ChatID:     chatID,
				FromChatID: fromChatID,
				MessageID:  src.ID,
			})
			return err
		}
	} else {
		raw := strings.TrimSpace(msg.Text)
		if raw == "" {
			raw = strings.TrimSpace(msg.Caption)
		}

		parts := strings.SplitN(raw, " ", 2)
		payload := ""
		if len(parts) == 2 {
			payload = strings.TrimSpace(parts[1])
		}

		if payload == "" && (strings.HasPrefix(raw, "/allcom") || strings.HasPrefix(raw, "/allcom@")) {
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: msg.Chat.ID,
				Text: "Использование:\n" +
					"1) /allcom <текст>\n" +
					"2) Ответьте /allcom на сообщение, чтобы разослать его как есть.\n\n" +
					"Если хотите сохранить форматирование, фото или видео без изменений, используйте второй вариант.",
			})
			return
		}

		deliver = allcomDirectDelivery(b, msg, payload)
	}

	statusMessage, _ := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    msg.Chat.ID,
		Text:      formatAllcomProgress(len(targets), 0, 0, 0),
		ParseMode: models.ParseModeHTML,
	})

	sent, failed := runAllcomBroadcast(ctx, b, msg.Chat.ID, statusMessage, targets, deliver)

	finalText := formatAllcomDone(len(targets), sent, failed)
	if statusMessage != nil {
		_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    msg.Chat.ID,
			MessageID: statusMessage.ID,
			Text:      finalText,
			ParseMode: models.ParseModeHTML,
		})
		if err == nil {
			return
		}
		slog.Warn("/allcom: failed to edit final progress", "error", err)
	}

	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    msg.Chat.ID,
		Text:      finalText,
		ParseMode: models.ParseModeHTML,
	})
}

func allcomDirectDelivery(b *bot.Bot, msg *models.Message, payload string) allcomDeliveryFunc {
	switch {
	case len(msg.Photo) > 0:
		fileID := msg.Photo[len(msg.Photo)-1].FileID
		return func(ctx context.Context, chatID int64) error {
			_, err := b.SendPhoto(ctx, &bot.SendPhotoParams{
				ChatID:    chatID,
				Photo:     &models.InputFileString{Data: fileID},
				Caption:   payload,
				ParseMode: allcomParseMode,
			})
			return err
		}

	case msg.Video != nil:
		fileID := msg.Video.FileID
		return func(ctx context.Context, chatID int64) error {
			_, err := b.SendVideo(ctx, &bot.SendVideoParams{
				ChatID:    chatID,
				Video:     &models.InputFileString{Data: fileID},
				Caption:   payload,
				ParseMode: allcomParseMode,
			})
			return err
		}

	case msg.Document != nil:
		fileID := msg.Document.FileID
		return func(ctx context.Context, chatID int64) error {
			_, err := b.SendDocument(ctx, &bot.SendDocumentParams{
				ChatID:    chatID,
				Document:  &models.InputFileString{Data: fileID},
				Caption:   payload,
				ParseMode: allcomParseMode,
			})
			return err
		}

	case msg.Audio != nil:
		fileID := msg.Audio.FileID
		return func(ctx context.Context, chatID int64) error {
			_, err := b.SendAudio(ctx, &bot.SendAudioParams{
				ChatID:    chatID,
				Audio:     &models.InputFileString{Data: fileID},
				Caption:   payload,
				ParseMode: allcomParseMode,
			})
			return err
		}

	case msg.Animation != nil:
		fileID := msg.Animation.FileID
		return func(ctx context.Context, chatID int64) error {
			_, err := b.SendAnimation(ctx, &bot.SendAnimationParams{
				ChatID:    chatID,
				Animation: &models.InputFileString{Data: fileID},
				Caption:   payload,
				ParseMode: allcomParseMode,
			})
			return err
		}

	default:
		return func(ctx context.Context, chatID int64) error {
			_, err := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:    chatID,
				Text:      payload,
				ParseMode: allcomParseMode,
			})
			return err
		}
	}
}

func runAllcomBroadcast(
	ctx context.Context,
	b *bot.Bot,
	adminChatID int64,
	statusMessage *models.Message,
	targets []int64,
	deliver allcomDeliveryFunc,
) (int, int) {
	var processed atomic.Int64
	var succeeded atomic.Int64
	var failed atomic.Int64

	workerCount := resolveAllcomWorkerCount(len(targets))
	jobs := make(chan int64)
	done := make(chan struct{})
	throttle := time.NewTicker(allcomSendInterval)
	defer throttle.Stop()

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for chatID := range jobs {
				select {
				case <-ctx.Done():
					return
				case <-throttle.C:
				}
				err := deliver(ctx, chatID)
				if err != nil && shouldRetryAllcom(err) {
					time.Sleep(allcomRetryDelay)
					err = deliver(ctx, chatID)
				}

				if err != nil {
					failed.Add(1)
					slog.Warn("/allcom: delivery failed", "telegramId", utils.MaskHalfInt64(chatID), "error", err)
				} else {
					succeeded.Add(1)
				}
				processed.Add(1)
			}
		}()
	}

	if statusMessage != nil {
		go allcomProgressLoop(ctx, b, adminChatID, statusMessage.ID, int64(len(targets)), &processed, &succeeded, &failed, done)
	}

sendLoop:
	for _, chatID := range targets {
		select {
		case <-ctx.Done():
			break sendLoop
		case jobs <- chatID:
		}
	}

	close(jobs)
	wg.Wait()
	close(done)

	return int(succeeded.Load()), int(failed.Load())
}

func allcomProgressLoop(
	ctx context.Context,
	b *bot.Bot,
	chatID int64,
	messageID int,
	total int64,
	processed *atomic.Int64,
	succeeded *atomic.Int64,
	failed *atomic.Int64,
	done <-chan struct{},
) {
	ticker := time.NewTicker(allcomProgressUpdateInterval)
	defer ticker.Stop()

	var lastProcessed int64 = -1

	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-ticker.C:
			currentProcessed := processed.Load()
			if currentProcessed == lastProcessed {
				continue
			}
			lastProcessed = currentProcessed

			_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
				ChatID:    chatID,
				MessageID: messageID,
				Text: formatAllcomProgress(
					int(total),
					int(currentProcessed),
					int(succeeded.Load()),
					int(failed.Load()),
				),
				ParseMode: models.ParseModeHTML,
			})
			if err != nil {
				slog.Warn("/allcom: failed to update progress", "error", err)
			}
		}
	}
}

func resolveAllcomWorkerCount(total int) int {
	if total <= 1 {
		return 1
	}
	if total < allcomMaxWorkers {
		return total
	}
	return allcomMaxWorkers
}

func uniqueAllcomTargets(ids []int64, adminID int64) []int64 {
	seen := make(map[int64]struct{}, len(ids)+1)
	targets := make([]int64, 0, len(ids)+1)

	appendTarget := func(id int64) {
		if id == 0 {
			return
		}
		if _, exists := seen[id]; exists {
			return
		}
		seen[id] = struct{}{}
		targets = append(targets, id)
	}

	for _, id := range ids {
		appendTarget(id)
	}
	appendTarget(adminID)

	return targets
}

func shouldRetryAllcom(err error) bool {
	if err == nil {
		return false
	}

	text := strings.ToLower(err.Error())
	return strings.Contains(text, "too many requests") ||
		strings.Contains(text, "retry after") ||
		strings.Contains(text, "timeout") ||
		strings.Contains(text, "connection reset")
}

func formatAllcomProgress(total, processed, success, failed int) string {
	percent := 0
	if total > 0 {
		percent = int(float64(processed) / float64(total) * 100)
	}

	return fmt.Sprintf(
		"<b>Рассылка запущена</b>\n\nПолучателей: <b>%d</b>\nПрогресс: <b>%d/%d</b> (%d%%)\nУспешно: <b>%d</b>\nОшибок: <b>%d</b>",
		total,
		processed,
		total,
		percent,
		success,
		failed,
	)
}

func formatAllcomDone(total, success, failed int) string {
	return fmt.Sprintf(
		"<b>Рассылка завершена</b>\n\nПолучателей: <b>%d</b>\nУспешно: <b>%d</b>\nОшибок: <b>%d</b>",
		total,
		success,
		failed,
	)
}

package broadcast

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"link-bot/internal/config"
	"link-bot/internal/database"
	"link-bot/utils"
)

const (
	maxButtons     = 8
	maxWorkers     = 10
	retryDelay     = 1500 * time.Millisecond
	progressStride = 10
)

var (
	ErrRunning       = errors.New("broadcast is already running")
	ErrNoMessage     = errors.New("broadcast message is not selected")
	ErrInvalidButton = errors.New("invalid broadcast button")

	customEmojiTagPattern = regexp.MustCompile(`(?is)<tg-emoji\s+emoji-id\s*=\s*["']([0-9]{5,32})["'][^>]*>.*?</tg-emoji>`)
	customEmojiIDPattern  = regexp.MustCompile(`^[0-9]{5,32}$`)
)

type Service struct {
	repository         *database.BroadcastRepository
	customerRepository *database.CustomerRepository
	promoRepository    *database.PromoCodeRepository
	telegramBot        *bot.Bot
	sendMu             sync.Mutex
}

func NewService(repository *database.BroadcastRepository, customerRepository *database.CustomerRepository, promoRepository *database.PromoCodeRepository, telegramBot *bot.Bot) *Service {
	return &Service{repository: repository, customerRepository: customerRepository, promoRepository: promoRepository, telegramBot: telegramBot}
}

func (s *Service) RecoverInterrupted(ctx context.Context) error {
	if s == nil || s.repository == nil {
		return nil
	}
	return s.repository.RecoverInterrupted(ctx)
}

func (s *Service) Get(ctx context.Context) (*database.BroadcastDraft, error) {
	if s == nil || s.repository == nil {
		return nil, errors.New("broadcast service is unavailable")
	}
	return s.repository.Get(ctx)
}

func (s *Service) StartCapture(ctx context.Context, adminID int64) (*database.BroadcastDraft, error) {
	draft, err := s.repository.StartCapture(ctx, adminID)
	if err != nil {
		return nil, normalizeRepositoryStateError(err)
	}
	if draft == nil {
		return nil, ErrRunning
	}
	if s.telegramBot != nil {
		_, sendErr := s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    adminID,
			Text:      "<b>Новое сообщение для рассылки</b>\n\nОтправьте следующим сообщением текст, фото, видео или файл. Форматирование, ссылки и premium emoji сохранятся.",
			ParseMode: models.ParseModeHTML,
			ReplyMarkup: &models.ForceReply{
				ForceReply:            true,
				InputFieldPlaceholder: "Сообщение для рассылки",
				Selective:             true,
			},
		})
		if sendErr != nil {
			_, _ = s.repository.Reset(ctx, adminID)
			return nil, fmt.Errorf("send capture prompt: %w", sendErr)
		}
	}
	return draft, nil
}

func (s *Service) CaptureMessage(ctx context.Context, message *models.Message) (bool, error) {
	if s == nil || s.repository == nil || message == nil || message.From == nil {
		return false, nil
	}
	draft, err := s.repository.Get(ctx)
	if err != nil || draft == nil || draft.Status != database.BroadcastStatusAwaitingMessage {
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
	if kind == "" {
		_, _ = s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: message.Chat.ID,
			Text:   "Этот тип сообщения нельзя использовать в рассылке. Отправьте текст, фото, видео, аудио или файл.",
		})
		return true, nil
	}
	preview := messagePreview(message, kind)
	draft, err = s.repository.SaveSource(ctx, message.Chat.ID, message.ID, kind, preview, message.From.ID)
	if err != nil {
		return true, err
	}
	if draft == nil {
		return true, errors.New("broadcast capture is no longer active")
	}

	replyMarkup := models.ReplyMarkup(nil)
	if miniAppURL := adminBroadcastURL(); miniAppURL != "" {
		replyMarkup = &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{
			Text:   "Открыть редактор рассылки",
			WebApp: &models.WebAppInfo{URL: miniAppURL},
		}}}}
	}
	_, _ = s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      message.Chat.ID,
		Text:        "<b>Сообщение сохранено</b>\n\nТеперь добавьте кнопки, отправьте предпросмотр и запустите рассылку из админки.",
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: replyMarkup,
	})
	return true, nil
}

func (s *Service) SaveButtons(ctx context.Context, adminID int64, buttons []database.BroadcastButton) (*database.BroadcastDraft, error) {
	clean, err := ValidateButtons(buttons)
	if err != nil {
		return nil, err
	}
	if err := s.validatePromoButtons(ctx, clean); err != nil {
		return nil, err
	}
	draft, err := s.repository.SaveButtons(ctx, clean, adminID)
	if err != nil {
		return nil, normalizeRepositoryStateError(err)
	}
	if draft == nil {
		return nil, ErrRunning
	}
	return draft, nil
}

func (s *Service) validatePromoButtons(ctx context.Context, buttons []database.BroadcastButton) error {
	for _, item := range buttons {
		if item.Type != "promo" {
			continue
		}
		if s.promoRepository == nil {
			return fmt.Errorf("%w: promo code validation is unavailable", ErrInvalidButton)
		}
		promo, err := s.promoRepository.FindByCode(ctx, item.PromoCode)
		if err != nil {
			return err
		}
		if promo == nil || !promo.IsActive || (promo.ExpiresAt != nil && !promo.ExpiresAt.After(time.Now())) ||
			(promo.MaxRedemptions != nil && promo.RedemptionCount >= *promo.MaxRedemptions) {
			return fmt.Errorf("%w: promo code %s is unavailable", ErrInvalidButton, item.PromoCode)
		}
	}
	return nil
}

func (s *Service) Preview(ctx context.Context, adminID int64) (*database.BroadcastDraft, error) {
	draft, err := s.repository.Get(ctx)
	if err != nil {
		return nil, err
	}
	if draft == nil || !draft.HasSource() {
		return nil, ErrNoMessage
	}
	if _, err := s.telegramBot.CopyMessage(ctx, &bot.CopyMessageParams{
		ChatID:      adminID,
		FromChatID:  *draft.SourceChatID,
		MessageID:   *draft.SourceMessageID,
		ReplyMarkup: buildKeyboard(draft.Buttons),
	}); err != nil {
		return nil, fmt.Errorf("copy preview: %w", err)
	}
	return draft, nil
}

func (s *Service) Start(ctx context.Context, adminID int64) (*database.BroadcastDraft, error) {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()

	draft, err := s.repository.Get(ctx)
	if err != nil {
		return nil, err
	}
	if draft == nil || !draft.HasSource() {
		return nil, ErrNoMessage
	}
	if draft.Status == database.BroadcastStatusRunning {
		return nil, ErrRunning
	}
	targetIDs, err := s.customerRepository.ListAllTelegramIDs(ctx)
	if err != nil {
		return nil, err
	}
	targets := uniqueTargets(targetIDs, adminID)
	if len(targets) == 0 {
		return nil, errors.New("broadcast recipient list is empty")
	}
	draft, err = s.repository.BeginSend(ctx, len(targets), adminID)
	if err != nil {
		return nil, normalizeRepositoryStateError(err)
	}
	if draft == nil {
		return nil, ErrRunning
	}

	startedDraft := *draft
	startedDraft.Buttons = append([]database.BroadcastButton(nil), draft.Buttons...)
	go s.run(startedDraft, targets, adminID)
	return draft, nil
}

func (s *Service) Reset(ctx context.Context, adminID int64) (*database.BroadcastDraft, error) {
	draft, err := s.repository.Reset(ctx, adminID)
	if err != nil {
		return nil, normalizeRepositoryStateError(err)
	}
	if draft == nil {
		return nil, ErrRunning
	}
	return draft, nil
}

func (s *Service) run(draft database.BroadcastDraft, targets []int64, adminID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	_, _ = s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    adminID,
		Text:      fmt.Sprintf("<b>Рассылка запущена</b>\n\nПолучателей: <b>%d</b>\nПрогресс доступен в админке.", len(targets)),
		ParseMode: models.ParseModeHTML,
	})

	var sent atomic.Int64
	var failed atomic.Int64
	var processed atomic.Int64
	var lastError atomic.Value
	jobs := make(chan int64)
	workerCount := maxWorkersFor(len(targets))
	var wg sync.WaitGroup
	keyboard := buildKeyboard(draft.Buttons)
	throttle := time.NewTicker(45 * time.Millisecond)
	defer throttle.Stop()

	for index := 0; index < workerCount; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for chatID := range jobs {
				select {
				case <-ctx.Done():
					return
				case <-throttle.C:
				}
				err := s.copyTo(ctx, draft, chatID, keyboard)
				if err != nil && shouldRetry(err) {
					time.Sleep(retryDelay)
					err = s.copyTo(ctx, draft, chatID, keyboard)
				}
				if err != nil {
					failed.Add(1)
					lastError.Store(shortError(err))
					slog.Warn("broadcast delivery failed", "telegramId", utils.MaskHalfInt64(chatID), "error", err)
				} else {
					sent.Add(1)
				}
				current := processed.Add(1)
				if current%progressStride == 0 || int(current) == len(targets) {
					_ = s.repository.UpdateProgress(ctx, int(sent.Load()), int(failed.Load()), loadAtomicString(&lastError))
				}
			}
		}()
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

	status := database.BroadcastStatusFinished
	last := loadAtomicString(&lastError)
	if ctx.Err() != nil {
		status = database.BroadcastStatusFailed
		last = "Рассылка остановлена по таймауту"
	}
	if err := s.repository.Finish(context.Background(), status, int(sent.Load()), int(failed.Load()), last); err != nil {
		slog.Error("broadcast final status save failed", "error", err)
	}
	_, _ = s.telegramBot.SendMessage(context.Background(), &bot.SendMessageParams{
		ChatID: adminID,
		Text: fmt.Sprintf(
			"<b>Рассылка завершена</b>\n\nПолучателей: <b>%d</b>\nУспешно: <b>%d</b>\nОшибок: <b>%d</b>",
			len(targets), sent.Load(), failed.Load(),
		),
		ParseMode: models.ParseModeHTML,
	})
}

func (s *Service) copyTo(ctx context.Context, draft database.BroadcastDraft, chatID int64, keyboard models.ReplyMarkup) error {
	_, err := s.telegramBot.CopyMessage(ctx, &bot.CopyMessageParams{
		ChatID:      chatID,
		FromChatID:  *draft.SourceChatID,
		MessageID:   *draft.SourceMessageID,
		ReplyMarkup: keyboard,
	})
	return err
}

func ValidateButtons(buttons []database.BroadcastButton) ([]database.BroadcastButton, error) {
	if len(buttons) > maxButtons {
		return nil, fmt.Errorf("%w: maximum %d buttons", ErrInvalidButton, maxButtons)
	}
	clean := make([]database.BroadcastButton, 0, len(buttons))
	seen := make(map[string]bool, len(buttons))
	for index, item := range buttons {
		item.ID = strings.TrimSpace(item.ID)
		if item.ID == "" {
			item.ID = fmt.Sprintf("button_%d", index+1)
		}
		if seen[item.ID] {
			return nil, fmt.Errorf("%w: duplicate button id", ErrInvalidButton)
		}
		seen[item.ID] = true
		normalizedText, normalizedEmojiID, err := normalizeButtonEmoji(item.Text, item.IconCustomEmojiID)
		if err != nil {
			return nil, err
		}
		item.Text = normalizedText
		item.IconCustomEmojiID = normalizedEmojiID
		if len([]rune(item.Text)) < 1 || len([]rune(item.Text)) > 64 {
			return nil, fmt.Errorf("%w: button text must contain 1-64 characters", ErrInvalidButton)
		}
		item.Style = strings.ToLower(strings.TrimSpace(item.Style))
		switch item.Style {
		case "", "primary", "success", "danger":
		default:
			return nil, fmt.Errorf("%w: unsupported button style", ErrInvalidButton)
		}
		item.Type = strings.ToLower(strings.TrimSpace(item.Type))
		switch item.Type {
		case "url":
			parsed, err := url.Parse(strings.TrimSpace(item.URL))
			if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
				return nil, fmt.Errorf("%w: button URL must use HTTPS", ErrInvalidButton)
			}
			item.URL = parsed.String()
			item.PromoCode = ""
		case "promo":
			item.PromoCode = database.NormalizePromoCode(item.PromoCode)
			if !database.IsValidPromoCode(item.PromoCode) {
				return nil, fmt.Errorf("%w: invalid promo code", ErrInvalidButton)
			}
			item.URL = ""
		default:
			return nil, fmt.Errorf("%w: unsupported button type", ErrInvalidButton)
		}
		clean = append(clean, item)
	}
	return clean, nil
}

func normalizeButtonEmoji(text, configuredID string) (string, string, error) {
	text = strings.TrimSpace(text)
	configuredID = strings.TrimSpace(configuredID)

	configuredMatches := customEmojiTagPattern.FindAllStringSubmatch(configuredID, -1)
	if len(configuredMatches) > 1 {
		return "", "", fmt.Errorf("%w: only one premium emoji is allowed per button", ErrInvalidButton)
	}
	if len(configuredMatches) == 1 {
		configuredID = configuredMatches[0][1]
	}

	textMatches := customEmojiTagPattern.FindAllStringSubmatch(text, -1)
	if len(textMatches) > 1 {
		return "", "", fmt.Errorf("%w: only one premium emoji is allowed per button", ErrInvalidButton)
	}
	if len(textMatches) == 1 {
		configuredID = textMatches[0][1]
		text = strings.TrimSpace(customEmojiTagPattern.ReplaceAllString(text, ""))
	}

	if configuredID != "" && !customEmojiIDPattern.MatchString(configuredID) {
		return "", "", fmt.Errorf("%w: invalid premium emoji ID", ErrInvalidButton)
	}
	return text, configuredID, nil
}

func buildKeyboard(buttons []database.BroadcastButton) models.ReplyMarkup {
	if len(buttons) == 0 {
		return nil
	}
	rows := make([][]models.InlineKeyboardButton, 0, len(buttons))
	for _, item := range buttons {
		button := models.InlineKeyboardButton{
			Text:              item.Text,
			IconCustomEmojiID: item.IconCustomEmojiID,
			Style:             item.Style,
		}
		if item.Type == "promo" {
			button.WebApp = &models.WebAppInfo{URL: promoMiniAppURL(item.PromoCode)}
		} else {
			button.URL = item.URL
		}
		rows = append(rows, []models.InlineKeyboardButton{button})
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func promoMiniAppURL(code string) string {
	base := strings.TrimSpace(config.GetMiniAppURL())
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" {
		return base
	}
	query := parsed.Query()
	query.Set("page", "buy")
	query.Set("promo", database.NormalizePromoCode(code))
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func adminBroadcastURL() string {
	base := strings.TrimSpace(config.GetMiniAppURL())
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" {
		return base
	}
	query := parsed.Query()
	query.Set("page", "admin")
	query.Set("admin", "broadcast")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func messageKind(message *models.Message) string {
	switch {
	case strings.TrimSpace(message.Text) != "":
		return "text"
	case len(message.Photo) > 0:
		return "photo"
	case message.Video != nil:
		return "video"
	case message.Animation != nil:
		return "animation"
	case message.Document != nil:
		return "document"
	case message.Audio != nil:
		return "audio"
	case message.Voice != nil:
		return "voice"
	case message.VideoNote != nil:
		return "video_note"
	case message.Sticker != nil:
		return "sticker"
	default:
		return ""
	}
}

func messagePreview(message *models.Message, kind string) string {
	text := strings.TrimSpace(message.Text)
	if text == "" {
		text = strings.TrimSpace(message.Caption)
	}
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		labels := map[string]string{
			"photo": "Фото", "video": "Видео", "animation": "Анимация", "document": "Файл",
			"audio": "Аудио", "voice": "Голосовое сообщение", "video_note": "Видеосообщение", "sticker": "Стикер",
		}
		text = labels[kind]
	}
	runes := []rune(text)
	if len(runes) > 180 {
		return string(runes[:180]) + "..."
	}
	return text
}

func uniqueTargets(ids []int64, adminID int64) []int64 {
	seen := make(map[int64]bool, len(ids)+1)
	result := make([]int64, 0, len(ids)+1)
	for _, id := range append(ids, adminID) {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		result = append(result, id)
	}
	return result
}

func maxWorkersFor(total int) int {
	if total < 1 {
		return 1
	}
	if total < maxWorkers {
		return total
	}
	return maxWorkers
}

func shouldRetry(err error) bool {
	if err == nil {
		return false
	}
	value := strings.ToLower(err.Error())
	return strings.Contains(value, "too many requests") || strings.Contains(value, "retry after") ||
		strings.Contains(value, "timeout") || strings.Contains(value, "connection reset")
}

func shortError(err error) string {
	if err == nil {
		return ""
	}
	value := strings.TrimSpace(err.Error())
	runes := []rune(value)
	if len(runes) > 240 {
		return string(runes[:240])
	}
	return value
}

func loadAtomicString(value *atomic.Value) string {
	if value == nil {
		return ""
	}
	loaded := value.Load()
	if loaded == nil {
		return ""
	}
	result, _ := loaded.(string)
	return result
}

func normalizeRepositoryStateError(err error) error {
	return err
}

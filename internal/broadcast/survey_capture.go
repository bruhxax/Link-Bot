package broadcast

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"
	"link-bot/internal/database"
)

// In-progress survey drafts may contain empty questions. Full Telegram HTML,
// required-field and button validation still runs before preview and delivery.
func NormalizeSurveyDraft(cfg *database.SurveyConfig) error {
	cfg.Capture = nil
	if cfg.Title == "" {
		cfg.Title = "Опрос"
	}
	if len([]rune(cfg.Title)) > 80 || len([]rune(cfg.StartText)) > 64 || len(cfg.Questions) > 64 || len(cfg.FinalButtons) > 8 {
		return ErrSurvey
	}
	texts := []string{cfg.IntroText, cfg.FinishText}
	for _, q := range cfg.Questions {
		if len(q.ID) > 80 {
			return ErrSurvey
		}
		if len(q.Options) > 100 {
			return ErrSurvey
		}
		texts = append(texts, q.Text)
		for _, option := range q.Options {
			if len([]rune(option)) > 64 {
				return ErrSurvey
			}
		}
	}
	for _, text := range texts {
		if len([]rune(text)) > 16000 {
			return ErrSurvey
		}
	}
	for _, button := range cfg.FinalButtons {
		if len([]rune(button.Text)) > 256 || len(button.URL) > 2048 || len(button.PromoCode) > 128 || len(button.IconCustomEmojiID) > 128 {
			return ErrSurvey
		}
	}
	return nil
}

func (s *Service) checkSurveyCapture(ctx context.Context, admin int64) error {
	if s == nil || s.repository == nil {
		return nil
	}
	cfg, err := s.repository.SurveyDraft(ctx)
	if err != nil {
		return err
	}
	if cfg.Capture != nil && (admin == 0 || cfg.Capture.AdminID == admin) {
		return ErrSurveyCapturePending
	}
	return nil
}
func (s *Service) SaveSurveyDraft(ctx context.Context, admin int64, cfg database.SurveyConfig, expected time.Time) (*database.BroadcastDraft, error) {
	if err := NormalizeSurveyDraft(&cfg); err != nil {
		return nil, err
	}
	if expected.IsZero() {
		return nil, ErrDraftChanged
	}
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if err := s.checkSurveyCapture(ctx, 0); err != nil {
		return nil, err
	}
	draft, err := s.repository.SaveSurvey(ctx, cfg, admin, expected)
	if err == nil && draft == nil {
		return nil, ErrDraftChanged
	}
	return draft, err
}
func (s *Service) StartSurveyCapture(ctx context.Context, admin int64, target string, question int, cfg database.SurveyConfig, expected time.Time) (*database.BroadcastDraft, error) {
	if s == nil || s.repository == nil || s.telegramBot == nil {
		return nil, ErrNoMessage
	}
	if target != "intro" && target != "finish" && target != "question" {
		return nil, ErrSurvey
	}
	if target == "question" && (question < 0 || question >= len(cfg.Questions)) {
		return nil, ErrSurvey
	}
	if err := NormalizeSurveyDraft(&cfg); err != nil {
		return nil, err
	}
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if err := s.checkPinnedCapture(ctx, admin); err != nil {
		return nil, err
	}
	direct, err := s.repository.GetDirect(ctx, admin)
	if err != nil {
		return nil, err
	}
	if direct != nil && direct.Status == database.BroadcastStatusAwaitingMessage {
		return nil, ErrRunning
	}
	draft, err := s.repository.Get(ctx)
	if err != nil {
		return nil, err
	}
	if draft == nil || draft.Status == database.BroadcastStatusRunning || draft.Status == database.BroadcastStatusAwaitingMessage {
		return nil, ErrRunning
	}
	previous, err := s.repository.SurveyDraft(ctx)
	if err != nil {
		return nil, err
	}
	if previous.Capture != nil && previous.Capture.AdminID != admin {
		return nil, ErrSurveyCapturePending
	}
	cfg.Capture = &database.SurveyCapture{ID: uuid.NewString(), AdminID: admin, Target: target, Question: question, StartedAt: time.Now().UTC()}
	draft, err = s.repository.SaveSurvey(ctx, cfg, admin, expected)
	if err != nil {
		return nil, err
	}
	if draft == nil {
		return nil, ErrDraftChanged
	}
	title := map[string]string{"intro": "Приглашение к опросу", "finish": "Завершение опроса", "question": fmt.Sprintf("Вопрос %d", question+1)}[target]
	_, err = s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{ChatID: admin, Text: "<b>" + title + "</b>\n\nНапишите текст сообщения и оформите его средствами Telegram: жирный текст, ссылки, цитаты, спойлеры и premium emoji сохранятся. До 3500 символов. Отмена: /cancel.", ParseMode: models.ParseModeHTML, ReplyMarkup: &models.ForceReply{ForceReply: true, Selective: true, InputFieldPlaceholder: title}})
	if err != nil {
		cfg.Capture = nil
		_, _ = s.repository.SaveSurvey(ctx, cfg, admin, draft.UpdatedAt)
		return nil, err
	}
	return draft, nil
}
func (s *Service) CancelSurveyCapture(ctx context.Context, admin int64, id string) (*database.BroadcastDraft, error) {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	draft, err := s.repository.Get(ctx)
	if err != nil {
		return nil, err
	}
	cfg, err := s.repository.SurveyDraft(ctx)
	if err != nil {
		return nil, err
	}
	if cfg.Capture == nil {
		return s.Get(ctx)
	}
	if cfg.Capture.AdminID != admin {
		return nil, ErrSurveyCapturePending
	}
	if id != "" && id != cfg.Capture.ID {
		return nil, ErrDraftChanged
	}
	cfg.Capture = nil
	draft, err = s.repository.SaveSurvey(ctx, *cfg, admin, draft.UpdatedAt)
	if err == nil && draft == nil {
		return nil, ErrDraftChanged
	}
	return draft, err
}
func (s *Service) CaptureSurveyMessage(ctx context.Context, message *models.Message) (bool, error) {
	if s == nil || s.repository == nil || message == nil || message.From == nil || message.From.IsBot || message.Chat.ID != message.From.ID || message.Chat.Type != models.ChatTypePrivate {
		return false, nil
	}
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	cfg, err := s.repository.SurveyDraft(ctx)
	if err != nil {
		return false, err
	}
	capture := cfg.Capture
	if capture == nil || capture.AdminID != message.From.ID {
		return false, nil
	}
	reply := func(text string) {
		if s.telegramBot != nil {
			_, _ = s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{ChatID: message.From.ID, Text: text})
		}
	}
	text := strings.TrimSpace(message.Text)
	if strings.HasPrefix(text, "/") && text != "/cancel" {
		return false, nil
	}
	if text != "/cancel" && text == "" {
		reply("Для опроса отправьте текстовое сообщение с форматированием Telegram. Отмена: /cancel.")
		return true, nil
	}
	if text != "/cancel" {
		value, formatErr := surveyMessageHTML(message.Text, message.Entities)
		if formatErr != nil {
			reply("Сообщение не сохранено: " + formatErr.Error() + ". Исправьте его и отправьте снова.")
			return true, nil
		}
		switch capture.Target {
		case "intro":
			cfg.IntroText = value
			cfg.IntroHTML = true
		case "finish":
			cfg.FinishText = value
			cfg.FinishHTML = true
		case "question":
			if capture.Question < 0 || capture.Question >= len(cfg.Questions) {
				return true, ErrDraftChanged
			}
			cfg.Questions[capture.Question].Text = value
			cfg.Questions[capture.Question].TextHTML = true
		default:
			return true, ErrSurvey
		}
	}
	cfg.Capture = nil
	for attempt := 0; attempt < 3; attempt++ {
		draft, getErr := s.repository.Get(ctx)
		if getErr != nil {
			return true, getErr
		}
		if draft.Status == database.BroadcastStatusRunning {
			return true, ErrRunning
		}
		current, currentErr := s.repository.SurveyDraft(ctx)
		if currentErr != nil {
			return true, currentErr
		}
		if current.Capture == nil || current.Capture.ID != capture.ID {
			return true, ErrDraftChanged
		}
		saved, saveErr := s.repository.SaveSurvey(ctx, *cfg, message.From.ID, draft.UpdatedAt)
		if saveErr != nil {
			return true, saveErr
		}
		if saved != nil {
			if text == "/cancel" {
				reply("Ввод сообщения опроса отменён.")
			} else {
				reply("Сообщение опроса сохранено с форматированием. Вернитесь в редактор.")
				s.recordCapturedActivity(ctx, message, "survey", 0)
			}
			return true, nil
		}
	}
	return true, ErrDraftChanged
}

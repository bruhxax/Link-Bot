package broadcast

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"
	"link-bot/internal/database"
)

const DefaultSurveyInvitation = "Пройдите короткий опрос — помогите нам стать лучше."

var ErrSurvey = errors.New("Некорректный опрос")

func NormalizeSurvey(c *database.SurveyConfig) error {
	c.IntroText = strings.TrimSpace(c.IntroText)
	c.Title = strings.TrimSpace(c.Title)
	c.StartText = strings.TrimSpace(c.StartText)
	c.FinishText = strings.TrimSpace(c.FinishText)
	if len(c.Questions) > 64 {
		return fmt.Errorf("%w: до 64 вопросов", ErrSurvey)
	}
	if !c.Enabled {
		return nil
	}
	if len([]rune(c.Title)) < 1 || len([]rune(c.Title)) > 80 || len([]rune(c.StartText)) < 1 || len([]rune(c.StartText)) > 64 || len(c.Questions) == 0 {
		return fmt.Errorf("%w: нужны название, кнопка начала и вопросы", ErrSurvey)
	}
	if c.IntroText == "" {
		c.IntroText = DefaultSurveyInvitation
	}
	if c.FinalButtons != nil {
		buttons, err := ValidateButtons(c.FinalButtons)
		if err != nil {
			return err
		}
		c.FinalButtons = buttons
	}
	texts := []string{c.IntroText, c.FinishText}
	for i := range c.Questions {
		q := &c.Questions[i]
		q.Text = strings.TrimSpace(q.Text)
		texts = append(texts, q.Text)
		if len(q.Options) < 1 || len(q.Options) > 100 {
			return fmt.Errorf("%w: от 1 до 100 кнопок в вопросе", ErrSurvey)
		}
		for j := range q.Options {
			q.Options[j] = strings.TrimSpace(q.Options[j])
			if len([]rune(q.Options[j])) < 1 || len([]rune(q.Options[j])) > 64 {
				return fmt.Errorf("%w: подпись ответа от 1 до 64 символов", ErrSurvey)
			}
		}
	}
	for _, text := range texts {
		if len([]rune(text)) < 1 || len([]rune(text)) > 3500 {
			return fmt.Errorf("%w: текст вопроса и завершения от 1 до 3500 символов", ErrSurvey)
		}
		if looksLikeBroadcastHTML(text) {
			if err := ValidateHTML(text); err != nil {
				return fmt.Errorf("%w: %v", ErrSurvey, err)
			}
		}
	}
	return nil
}

func (s *Service) SaveSurvey(ctx context.Context, admin int64, cfg database.SurveyConfig, expected time.Time) (*database.BroadcastDraft, error) {
	if err := NormalizeSurvey(&cfg); err != nil {
		return nil, err
	}
	if expected.IsZero() {
		return nil, ErrDraftChanged
	}
	if cfg.Enabled {
		if err := s.validatePromoButtons(ctx, cfg.FinalButtons); err != nil {
			return nil, err
		}
	}
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	draft, err := s.repository.SaveSurvey(ctx, cfg, admin, expected)
	if err == nil && draft == nil {
		return nil, ErrDraftChanged
	}
	return draft, err
}

func (s *Service) loadSurvey(ctx context.Context, draft *database.BroadcastDraft) error {
	cfg, err := s.repository.SurveyDraft(ctx)
	if err != nil {
		return err
	}
	draft.Survey = cfg
	hydrateSurveyInvitation(cfg, draft)
	if err = NormalizeSurvey(cfg); err != nil {
		return err
	}
	if cfg.Enabled {
		applySurveyInvitation(draft, cfg)
		if _, err = ValidateButtons(draft.Buttons); err != nil {
			return err
		}
		if err = s.validatePromoButtons(ctx, draft.Buttons); err != nil {
			return err
		}
	}
	return nil
}

func surveyData(id string, step, option int) string {
	return "sv:" + strings.ReplaceAll(id, "-", "") + ":" + strconv.Itoa(step) + ":" + strconv.Itoa(option)
}
func surveyIntroKeyboard(id string, cfg *database.SurveyConfig) models.ReplyMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: cfg.StartText, CallbackData: surveyData(id, -1, -1)}}}}
}

func surveyScreen(run *database.SurveyRun, step int) (string, models.ReplyMarkup) {
	if step < 0 {
		intro := run.Config.IntroText
		if intro == "" {
			intro = run.Config.Title
		}
		return surveyHTML(intro), surveyIntroKeyboard(run.ID, &run.Config)
	}
	if step >= len(run.Config.Questions) {
		keys := buildKeyboard(run.Buttons)
		if keys == nil {
			keys = &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{}}
		}
		return surveyHTML(run.Config.FinishText), keys
	}
	q := run.Config.Questions[step]
	rows := make([][]models.InlineKeyboardButton, 0, len(q.Options))
	for i, label := range q.Options {
		rows = append(rows, []models.InlineKeyboardButton{{Text: label, CallbackData: surveyData(run.ID, step, i)}})
	}
	return fmt.Sprintf("<b>%d / %d</b>\n\n%s", step+1, len(run.Config.Questions), surveyHTML(q.Text)), &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func (s *Service) HandleSurvey(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update == nil || update.CallbackQuery == nil {
		return
	}
	q := update.CallbackQuery
	reply := "Не удалось открыть опрос. Попробуйте ещё раз."
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	defer func() {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID, Text: reply})
	}()
	m := q.Message.Message
	if m == nil || q.From.IsBot || m.Chat.Type != models.ChatTypePrivate || m.Chat.ID != q.From.ID {
		return
	}
	parts := strings.Split(q.Data, ":")
	if len(parts) != 4 || parts[0] != "sv" {
		return
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return
	}
	step, err := strconv.Atoi(parts[2])
	if err != nil {
		return
	}
	option, err := strconv.Atoi(parts[3])
	if err != nil {
		return
	}
	run, err := s.repository.SurveyRun(ctx, id.String())
	if err != nil || run == nil {
		return
	}
	if step < -1 || step >= len(run.Config.Questions) {
		return
	}
	options := 0
	if step >= 0 {
		options = len(run.Config.Questions[step].Options)
		if option < 0 || option >= options {
			return
		}
	} else if option != -1 {
		return
	}
	_, err = s.repository.AdvanceSurvey(ctx, run.ID, q.From.ID, m.ID, step, option, len(run.Config.Questions), options, func(current int) error {
		text, keys := surveyScreen(run, current)
		_, editErr := b.EditMessageText(ctx, &bot.EditMessageTextParams{ChatID: m.Chat.ID, MessageID: m.ID, Text: text, ParseMode: models.ParseModeHTML, ReplyMarkup: keys})
		if editErr != nil && strings.Contains(strings.ToLower(editErr.Error()), "message is not modified") {
			return nil
		}
		return editErr
	})
	if err != nil {
		slog.Warn("broadcast: survey answer failed", "error", err)
		return
	}
	reply = ""
}

func (s *Service) SurveyResults(ctx context.Context, id string, before int64) ([]database.SurveyRun, *database.SurveyReport, error) {
	if s == nil || s.repository == nil {
		return nil, nil, errors.New("survey unavailable")
	}
	runs, err := s.repository.SurveyRuns(ctx)
	if err != nil {
		return nil, nil, err
	}
	if id == "" && len(runs) > 0 {
		id = runs[0].ID
	}
	if id == "" {
		return runs, nil, nil
	}
	report, err := s.repository.SurveyReport(ctx, id, before)
	return runs, report, err
}

func surveyHTML(text string) string {
	if looksLikeBroadcastHTML(text) {
		return text
	}
	return html.EscapeString(text)
}

func applySurveyInvitation(draft *database.BroadcastDraft, cfg *database.SurveyConfig) {
	if cfg.FinalButtons == nil {
		cfg.FinalButtons = append([]database.BroadcastButton{}, draft.Buttons...)
	}
	draft.Survey = cfg
	draft.SourceKind = "html"
	draft.SourceHTML = surveyHTML(cfg.IntroText)
	draft.SourceText = cfg.IntroText
	draft.SourcePreview = cfg.IntroText
	draft.Buttons = cfg.FinalButtons
}

// Existing surveys used the ordinary message as their invitation.
func hydrateSurveyInvitation(cfg *database.SurveyConfig, draft *database.BroadcastDraft) {
	if !cfg.Enabled || cfg.IntroText != "" {
		return
	}
	if draft.SourceKind == "html" {
		cfg.IntroText = draft.SourceHTML
	} else if draft.SourceKind == "text" {
		cfg.IntroText = draft.SourceText
		if cfg.IntroText == "" {
			cfg.IntroText = draft.SourcePreview
		}
	}
}

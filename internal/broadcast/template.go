package broadcast

import (
	"context"
	"github.com/jackc/pgx/v4"
	"link-bot/internal/database"
	"time"
)

func (s *Service) RestoreTemplate(ctx context.Context, p database.BroadcastTemplatePayload, adminID int64, expected time.Time) (*database.BroadcastDraft, error) {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if err := s.checkSurveyCapture(ctx, 0); err != nil {
		return nil, err
	}
	if (p.Survey == nil || !p.Survey.Enabled) && (p.SourceChatID == nil || p.SourceMessageID == nil || *p.SourceMessageID <= 0) {
		return nil, ErrNoMessage
	}
	if err := p.Audience.Normalize(); err != nil {
		return nil, err
	}
	if err := database.ValidateBroadcastExpiration(p.DeleteAfterMinutes); err != nil {
		return nil, err
	}
	if p.Survey == nil {
		p.Survey = &database.SurveyConfig{}
	}
	p.Survey.Capture = nil
	if err := NormalizeSurvey(p.Survey); err != nil {
		return nil, err
	}
	if p.Survey.Enabled {
		if err := s.validatePromoButtons(ctx, p.Survey.FinalButtons); err != nil {
			return nil, err
		}
	}
	clean, err := ValidateButtons(p.Buttons)
	if err != nil {
		return nil, err
	}
	p.Buttons = clean
	if err = s.validatePromoButtons(ctx, clean); err != nil {
		return nil, err
	}
	if p.SourceKind == "html" {
		if err = ValidateHTML(p.SourceHTML); err != nil {
			return nil, err
		}
	}
	draft, err := s.repository.RestoreTemplate(ctx, p, adminID, expected)
	if err == pgx.ErrNoRows || err == nil && draft == nil {
		return nil, ErrDraftChanged
	}
	if err != nil {
		return nil, err
	}
	result, err := s.repository.SaveSurvey(ctx, *p.Survey, adminID, draft.UpdatedAt)
	if err == nil && result == nil {
		return nil, ErrDraftChanged
	}
	return result, err
}

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
	if p.SourceChatID == nil || p.SourceMessageID == nil || *p.SourceMessageID <= 0 {
		return nil, ErrNoMessage
	}
	if err := p.Audience.Normalize(); err != nil {
		return nil, err
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
	return draft, err
}

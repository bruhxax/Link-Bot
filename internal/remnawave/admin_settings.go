package remnawave

import (
	"context"
	"errors"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// UserSettings is an explicit allowlist: client requests cannot select a panel
// identity or forward arbitrary Remnawave fields.
type UserSettings struct {
	TelegramID           *int64      `json:"telegramId"`
	Email                string      `json:"email"`
	Description          string      `json:"description"`
	Tag                  string      `json:"tag"`
	TrafficLimitBytes    int64       `json:"trafficLimitBytes"`
	TrafficLimitStrategy string      `json:"trafficLimitStrategy"`
	HwidDeviceLimit      *int        `json:"hwidDeviceLimit"`
	ExpireAt             time.Time   `json:"expireAt"`
	ActiveInternalSquads []uuid.UUID `json:"activeInternalSquads"`
	ExternalSquadUUID    *uuid.UUID  `json:"externalSquadUuid"`
}

func (s UserSettings) Validate() error {
	if len(s.Email) > 254 || len([]rune(s.Description)) > 2000 || len(s.Tag) > 16 {
		return errors.New("Слишком длинное значение поля")
	}
	if s.Tag != "" && !regexp.MustCompile(`^[A-Z0-9_]+$`).MatchString(s.Tag) {
		return errors.New("Тег может содержать только A–Z, 0–9 и _")
	}
	if s.TelegramID != nil && (*s.TelegramID <= 0 || *s.TelegramID > 9007199254740991) {
		return errors.New("Некорректный Telegram ID")
	}
	if s.Email != "" {
		a, err := mail.ParseAddress(s.Email)
		if err != nil || a.Address != s.Email {
			return errors.New("Укажите корректный email")
		}
	}
	if s.TrafficLimitBytes < 0 || s.TrafficLimitBytes > 1000000*int64(1024*1024*1024) {
		return errors.New("Некорректный лимит трафика")
	}
	if s.HwidDeviceLimit != nil && (*s.HwidDeviceLimit < 0 || *s.HwidDeviceLimit > 1000) {
		return errors.New("Некорректный лимит устройств")
	}
	switch s.TrafficLimitStrategy {
	case "NO_RESET", "DAY", "WEEK", "MONTH":
	default:
		return errors.New("Некорректная стратегия сброса трафика")
	}
	if s.ExpireAt.IsZero() || s.ExpireAt.Year() < 2000 || s.ExpireAt.Year() > 2100 {
		return errors.New("Некорректная дата окончания")
	}
	if len(s.ActiveInternalSquads) > 500 {
		return errors.New("Слишком много сквадов")
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range s.ActiveInternalSquads {
		if id == uuid.Nil || seen[id] {
			return errors.New("Некорректный список сквадов")
		}
		seen[id] = true
	}
	if s.ExternalSquadUUID != nil && *s.ExternalSquadUUID == uuid.Nil {
		return errors.New("Некорректный внешний сквад")
	}
	return nil
}

func (r *Client) GetAdminUserSettings(ctx context.Context, id int64, userUUID uuid.UUID) (*UserSettings, error) {
	user, err := r.getPanelUserByIdentity(ctx, id, userUUID)
	if err != nil {
		return nil, err
	}
	return UserSettingsFromPanelUser(user), nil
}

// UserSettingsFromPanelUser uses the same observation as traffic/expiry, so an
// admin card cannot combine two different versions of a subscription.
func UserSettingsFromPanelUser(user *PanelUser) *UserSettings {
	if user == nil {
		return nil
	}
	s := &UserSettings{TelegramID: user.TelegramID, TrafficLimitBytes: user.TrafficLimitBytes, TrafficLimitStrategy: normalizeTrafficStrategy(user.TrafficLimitStrategy), HwidDeviceLimit: user.HwidDeviceLimit, ExpireAt: user.ExpireAt, ExternalSquadUUID: user.ExternalSquadUUID, ActiveInternalSquads: []uuid.UUID{}}
	if user.Email != nil {
		s.Email = *user.Email
	}
	if user.Description != nil {
		s.Description = *user.Description
	}
	if user.Tag != nil {
		s.Tag = *user.Tag
	}
	for _, squad := range user.ActiveInternalSquads {
		s.ActiveInternalSquads = append(s.ActiveInternalSquads, squad.UUID)
	}
	return s
}

func (r *Client) UpdateAdminUserSettings(ctx context.Context, id int64, userUUID uuid.UUID, s UserSettings) (*PanelUser, error) {
	s.Email = strings.TrimSpace(s.Email)
	s.Tag = strings.TrimSpace(s.Tag)
	if err := s.Validate(); err != nil {
		return nil, err
	}
	user, err := r.getPanelUserByIdentity(ctx, id, userUUID)
	if err != nil {
		return nil, err
	}
	nullable := func(value string) any {
		if value == "" {
			return nil
		}
		return value
	}
	squads := s.ActiveInternalSquads
	if squads == nil {
		squads = []uuid.UUID{}
	}
	// Telegram ID here is panel contact metadata. The customer's login identity
	// and subscription ownership remain selected from the application database.
	return r.patchPanelUser(ctx, user, map[string]any{"telegramId": s.TelegramID, "email": nullable(s.Email), "description": nullable(s.Description), "tag": nullable(s.Tag), "trafficLimitBytes": s.TrafficLimitBytes, "trafficLimitStrategy": s.TrafficLimitStrategy, "hwidDeviceLimit": s.HwidDeviceLimit, "expireAt": s.ExpireAt.UTC(), "activeInternalSquads": squads, "externalSquadUuid": s.ExternalSquadUUID})
}

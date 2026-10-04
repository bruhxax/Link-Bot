package runtimeconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"link-bot/internal/database"
)

type ReviewRewardSettings struct {
	Days       int                 `json:"days"`
	TrafficGB  int                 `json:"trafficGb"`
	BalanceRub int                 `json:"balanceRub"`
	Promo      ReviewPromoSettings `json:"promo"`
}

type ReviewPromoSettings struct {
	Enabled         bool   `json:"enabled"`
	RewardType      string `json:"rewardType"`
	RewardValue     int    `json:"rewardValue"`
	RewardTrafficGB int    `json:"rewardTrafficGb"`
	DiscountPercent int    `json:"discountPercent"`
	ExpiryDays      int    `json:"expiryDays"`
}

func DefaultReviewRewards() ReviewRewardSettings {
	return ReviewRewardSettings{Days: 2, TrafficGB: 20, Promo: ReviewPromoSettings{RewardType: "discount", DiscountPercent: 10}}
}

func (s ReviewRewardSettings) Validate() error {
	if s.Days < 0 || s.Days > 3650 || s.TrafficGB < 0 || s.TrafficGB > 1000000 || s.BalanceRub < 0 || s.BalanceRub > 1000000 {
		return errors.New("Дни: от 0 до 3650; гигабайты и рубли: от 0 до 1000000")
	}
	if s.Promo.ExpiryDays < 0 || s.Promo.ExpiryDays > 3650 {
		return errors.New("Срок промокода: от 0 до 3650 дней (0 — без срока)")
	}
	if s.Promo.Enabled {
		promo := database.PromoCode{RewardType: s.Promo.RewardType, RewardValue: s.Promo.RewardValue, RewardTrafficGB: s.Promo.RewardTrafficGB, DiscountPercent: s.Promo.DiscountPercent}
		if err := database.NormalizePromoReward(&promo); err != nil {
			return errors.New("Укажите корректную награду личного промокода")
		}
	}
	return nil
}

func (s *Service) UpdateReviewRewards(ctx context.Context, reward ReviewRewardSettings, updatedBy int64) (Settings, error) {
	if err := reward.Validate(); err != nil {
		return Settings{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.Snapshot()
	next.ReviewRewards = reward
	raw, err := json.Marshal(next)
	if err != nil {
		return Settings{}, err
	}
	if err := s.repository.Save(ctx, raw, updatedBy); err != nil {
		return Settings{}, fmt.Errorf("save review rewards: %w", err)
	}
	s.value.Store(next)
	return cloneSettings(next), nil
}

package database

import (
	"errors"
	"testing"
)

func TestNormalizePromoReward(t *testing.T) {
	tests := []struct {
		name  string
		promo PromoCode
		valid bool
	}{
		{"legacy discount", PromoCode{DiscountPercent: 20}, true},
		{"balance", PromoCode{RewardType: "balance", RewardValue: 150}, true},
		{"days", PromoCode{RewardType: "days", RewardValue: 30}, true},
		{"traffic", PromoCode{RewardType: "traffic", RewardValue: 10}, true},
		{"days and traffic", PromoCode{RewardType: "days_traffic", RewardValue: 30, RewardTrafficGB: 10}, true},
		{"combined missing traffic", PromoCode{RewardType: "days_traffic", RewardValue: 30}, false},
		{"combined missing days", PromoCode{RewardType: "days_traffic", RewardTrafficGB: 10}, false},
		{"discount with traffic", PromoCode{DiscountPercent: 20, RewardTrafficGB: 10}, false},
		{"balance with discount", PromoCode{RewardType: "balance", RewardValue: 100, DiscountPercent: 10}, false},
		{"too many days", PromoCode{RewardType: "days", RewardValue: 3651}, false},
		{"unknown type", PromoCode{RewardType: "mystery", RewardValue: 1}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NormalizePromoReward(&tt.promo)
			if tt.valid && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.valid && !errors.Is(err, ErrPromoCodeInvalidReward) {
				t.Fatalf("expected invalid reward, got %v", err)
			}
		})
	}
}

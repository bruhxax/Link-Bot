package miniapp

import (
	"errors"
	"testing"
	"time"

	"link-bot/internal/database"
	"link-bot/internal/remnawave"
)

func TestPromoAccessTarget(t *testing.T) {
	now := time.Date(2026, time.September, 23, 12, 0, 0, 0, time.UTC)
	expires := now.Add(48 * time.Hour)
	const gb = int64(1024 * 1024 * 1024)
	active := &remnawave.UserState{Exists: true, Active: true, ExpireAt: &expires, TrafficLimitBytes: 20 * gb}

	t.Run("days extend current expiry", func(t *testing.T) {
		expiry, traffic, err := promoAccessTarget(active, &database.PromoCode{RewardType: "days", RewardValue: 5}, now)
		if err != nil || expiry == nil || !expiry.Equal(expires.AddDate(0, 0, 5)) || traffic != nil {
			t.Fatalf("expiry=%v traffic=%v err=%v", expiry, traffic, err)
		}
	})
	t.Run("traffic increases finite limit", func(t *testing.T) {
		expiry, traffic, err := promoAccessTarget(active, &database.PromoCode{RewardType: "traffic", RewardValue: 3}, now)
		if err != nil || expiry != nil || traffic == nil || *traffic != 23*gb {
			t.Fatalf("expiry=%v traffic=%v err=%v", expiry, traffic, err)
		}
	})
	t.Run("combined applies both", func(t *testing.T) {
		expiry, traffic, err := promoAccessTarget(active, &database.PromoCode{RewardType: "days_traffic", RewardValue: 5, RewardTrafficGB: 3}, now)
		if err != nil || expiry == nil || !expiry.Equal(expires.AddDate(0, 0, 5)) || traffic == nil || *traffic != 23*gb {
			t.Fatalf("expiry=%v traffic=%v err=%v", expiry, traffic, err)
		}
	})
	t.Run("combined reactivates expired subscription", func(t *testing.T) {
		expired := &remnawave.UserState{Exists: true, TrafficLimitBytes: 20 * gb}
		expiry, traffic, err := promoAccessTarget(expired, &database.PromoCode{RewardType: "days_traffic", RewardValue: 5, RewardTrafficGB: 3}, now)
		if err != nil || expiry == nil || !expiry.Equal(now.AddDate(0, 0, 5)) || traffic == nil || *traffic != 23*gb {
			t.Fatalf("expiry=%v traffic=%v err=%v", expiry, traffic, err)
		}
	})
	t.Run("traffic rejects unlimited", func(t *testing.T) {
		unlimited := &remnawave.UserState{Exists: true, Active: true}
		_, _, err := promoAccessTarget(unlimited, &database.PromoCode{RewardType: "traffic", RewardValue: 3}, now)
		if !errors.Is(err, errPromoTrafficUnlimited) {
			t.Fatalf("expected unlimited error, got %v", err)
		}
	})
}

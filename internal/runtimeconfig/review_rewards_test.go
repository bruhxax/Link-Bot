package runtimeconfig

import "testing"

func TestReviewRewardValidation(t *testing.T) {
	for _, reward := range []ReviewRewardSettings{{}, DefaultReviewRewards(), {Days: 3650, TrafficGB: 1000000, BalanceRub: 1000000}} {
		if err := reward.Validate(); err != nil {
			t.Fatalf("valid reward %#v: %v", reward, err)
		}
	}
	for _, reward := range []ReviewRewardSettings{{Days: -1}, {Days: 3651}, {TrafficGB: -1}, {TrafficGB: 1000001}, {BalanceRub: 1000001}, {Promo: ReviewPromoSettings{Enabled: true, RewardType: "discount", DiscountPercent: 100}}, {Promo: ReviewPromoSettings{Enabled: true, RewardType: "days_traffic", RewardValue: 2}}, {Promo: ReviewPromoSettings{ExpiryDays: -1}}} {
		if err := reward.Validate(); err == nil {
			t.Fatalf("accepted invalid reward %#v", reward)
		}
	}
	for _, promo := range []ReviewPromoSettings{{Enabled: true, RewardType: "discount", DiscountPercent: 20}, {Enabled: true, RewardType: "balance", RewardValue: 100}, {Enabled: true, RewardType: "days", RewardValue: 7}, {Enabled: true, RewardType: "traffic", RewardValue: 50}, {Enabled: true, RewardType: "days_traffic", RewardValue: 7, RewardTrafficGB: 50, ExpiryDays: 30}} {
		if err := (ReviewRewardSettings{Promo: promo}).Validate(); err != nil {
			t.Fatalf("promo %#v: %v", promo, err)
		}
	}
}

func TestReviewRewardMigrationPreservesDisabledRewards(t *testing.T) {
	settings := DefaultSettings()
	settings.Version = 26
	settings.ReviewRewards = ReviewRewardSettings{}
	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatal(err)
	}
	if settings.ReviewRewards != DefaultReviewRewards() {
		t.Fatal("legacy rewards were not preserved")
	}
	settings.ReviewRewards = ReviewRewardSettings{}
	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatal(err)
	}
	if settings.ReviewRewards != (ReviewRewardSettings{}) {
		t.Fatal("disabled rewards were re-enabled")
	}
}

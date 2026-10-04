package miniapp

import (
	"link-bot/internal/remnawave"
	"link-bot/internal/runtimeconfig"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestReviewRewardSettingsRejectNonAdmin(t *testing.T) {
	h := &Handler{}
	w := httptest.NewRecorder()
	h.handleAdminReviewRewards(w, httptest.NewRequest(http.MethodPost, "/api/mini-app/admin/reviews/rewards", nil), &session{}, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("unauthorized status=%d", w.Code)
	}
}

func TestReviewAccessTargets(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	future := now.Add(10 * 24 * time.Hour)
	state := &remnawave.UserState{Exists: true, Active: true, ExpireAt: &future, TrafficLimitBytes: 10 * adminUserTrafficGB}
	expires, traffic, err := reviewAccessTarget(state, runtimeconfig.ReviewRewardSettings{Days: 5, TrafficGB: 30}, now)
	if err != nil || expires == nil || !expires.Equal(future.Add(5*24*time.Hour)) || traffic == nil || *traffic != 40*adminUserTrafficGB {
		t.Fatalf("combined target: %v %v %v", expires, traffic, err)
	}
	state.TrafficLimitBytes = 0
	_, traffic, err = reviewAccessTarget(state, runtimeconfig.ReviewRewardSettings{TrafficGB: 30}, now)
	if err != nil || traffic != nil {
		t.Fatal("unlimited traffic must remain unlimited")
	}
	past := now.Add(-10 * 24 * time.Hour)
	state.ExpireAt = &past
	expires, _, err = reviewAccessTarget(state, runtimeconfig.ReviewRewardSettings{Days: 5}, now)
	if err != nil || !expires.Equal(now.Add(5*24*time.Hour)) {
		t.Fatal("expired account must start from now")
	}
	_, _, err = reviewAccessTarget(nil, runtimeconfig.ReviewRewardSettings{TrafficGB: 30}, now)
	if err == nil {
		t.Fatal("traffic alone must not create an account without an expiry")
	}
	expires, traffic, err = reviewAccessTarget(nil, runtimeconfig.DefaultReviewRewards(), now)
	if err != nil || !expires.Equal(now.Add(2*24*time.Hour)) || traffic == nil || *traffic != 20*adminUserTrafficGB {
		t.Fatal("new accounts must retain the legacy gift")
	}
	expires, traffic, err = reviewAccessTarget(nil, runtimeconfig.ReviewRewardSettings{BalanceRub: 100}, now)
	if err != nil || expires != nil || traffic != nil {
		t.Fatal("balance-only rewards must not require panel access")
	}
}

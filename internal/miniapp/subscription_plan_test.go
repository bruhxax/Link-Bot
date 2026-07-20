package miniapp

import (
	"os"
	"sync"
	"testing"
	"time"

	"link-bot/internal/config"
	"link-bot/internal/database"
	"link-bot/internal/remnawave"
)

var testConfigOnce sync.Once

func initMiniAppTestConfig() {
	testConfigOnce.Do(func() {
		env := map[string]string{
			"DISABLE_ENV_FILE":        "true",
			"ADMIN_TELEGRAM_ID":       "1",
			"TELEGRAM_TOKEN":          "test-token",
			"REFERRAL_DAYS":           "7",
			"MINI_APP_URL":            "https://example.com/mini-app/",
			"REMNAWAVE_URL":           "https://example.com",
			"REMNAWAVE_MODE":          "remote",
			"REMNAWAVE_TOKEN":         "test-remnawave-token",
			"DATABASE_URL":            "postgres://postgres:postgres@db:5432/postgres?sslmode=disable",
			"POSTGRES_USER":           "postgres",
			"POSTGRES_PASSWORD":       "postgres",
			"POSTGRES_DB":             "postgres",
			"CRYPTO_PAY_ENABLED":      "false",
			"YOOKASA_ENABLED":         "false",
			"ENABLE_AUTO_PAYMENT":     "false",
			"TELEGRAM_STARS_ENABLED":  "false",
			"TRAFFIC_LIMIT":           "150",
			"TRAFFIC_LIMIT_1":         "150",
			"TRAFFIC_LIMIT_3":         "500",
			"TRAFFIC_LIMIT_6":         "1000",
			"TRAFFIC_LIMIT_12":        "0",
			"HWID_DEVICE_LIMIT_1":     "5",
			"HWID_DEVICE_LIMIT_3":     "7",
			"HWID_DEVICE_LIMIT_6":     "10",
			"HWID_DEVICE_LIMIT_12":    "0",
			"TRIAL_TRAFFIC_LIMIT":     "20",
			"TRIAL_HWID_DEVICE_LIMIT": "5",
			"TRIAL_DAYS":              "2",
			"PRICE_1":                 "89",
			"PRICE_3":                 "239",
			"PRICE_6":                 "350",
			"PRICE_12":                "700",
		}

		for key, value := range env {
			_ = os.Setenv(key, value)
		}

		config.InitConfig()
	})
}

func TestResolveSubscriptionPlanMonthsPrefersHighestPurchase(t *testing.T) {
	initMiniAppTestConfig()

	highestPurchase := &database.Purchase{Month: 3}
	panelState := &remnawave.UserState{
		Exists:            true,
		TrafficLimitBytes: 1000 * 1024 * 1024 * 1024,
		DeviceLimit:       10,
	}

	if got := resolveSubscriptionPlanMonths(highestPurchase, panelState); got != 3 {
		t.Fatalf("expected highest purchase months to win, got %d", got)
	}
}

func TestResolveSubscriptionPlanMonthsFallsBackToPanelState(t *testing.T) {
	initMiniAppTestConfig()

	panelState := &remnawave.UserState{
		Exists:            true,
		TrafficLimitBytes: 1000 * 1024 * 1024 * 1024,
		DeviceLimit:       10,
	}

	if got := resolveSubscriptionPlanMonths(nil, panelState); got != 6 {
		t.Fatalf("expected 6 months from panel state, got %d", got)
	}
}

func TestBuildSubscriptionPayloadDoesNotTreatPaidPanelPlanAsTrial(t *testing.T) {
	initMiniAppTestConfig()

	expireAt := time.Now().Add(24 * time.Hour)
	customer := &database.Customer{
		Language:  "ru",
		ExpireAt:  &expireAt,
		TrialUsed: true,
	}
	panelState := &remnawave.UserState{
		Exists:            true,
		Active:            true,
		TrafficLimitBytes: 1000 * 1024 * 1024 * 1024,
		DeviceLimit:       10,
	}

	payload := buildSubscriptionPayload(customer, nil, panelState)
	if payload.IsTrial {
		t.Fatalf("expected paid panel plan not to be marked as trial")
	}
	if payload.PlanMonths != 6 {
		t.Fatalf("expected 6 months from panel state, got %d", payload.PlanMonths)
	}
	if payload.PlanLabel != "6 Месяцев" {
		t.Fatalf("expected russian 6 months label, got %q", payload.PlanLabel)
	}
}

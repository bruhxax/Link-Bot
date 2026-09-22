package payment

import (
	"testing"
	"time"

	"link-bot/internal/config"
	"link-bot/internal/database"
)

func TestPurchaseDurationDays(t *testing.T) {
	if got := purchaseDurationDays(&database.Purchase{Month: 2}); got != 2*config.DaysInMonth() {
		t.Fatalf("monthly duration = %d", got)
	}
	if got := purchaseDurationDays(&database.Purchase{Days: 7}); got != 7 {
		t.Fatalf("daily duration = %d", got)
	}
}

func TestDailyFreePlanRenewalWindow(t *testing.T) {
	now := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	expires := now.Add(18 * time.Hour)
	if err := validateFreePlanEligibilityWindow(false, false, &expires, now, 6*time.Hour); err != ErrFreePlanTooEarly {
		t.Fatalf("early renewal error = %v", err)
	}
	expires = now.Add(5 * time.Hour)
	if err := validateFreePlanEligibilityWindow(false, false, &expires, now, 6*time.Hour); err != nil {
		t.Fatalf("late renewal error = %v", err)
	}
}

func TestDailyPurchaseDisablesMonthlyAutopay(t *testing.T) {
	updates := (PaymentService{}).buildAutoPaymentCustomerUpdates(&database.Customer{}, &database.Purchase{Days: 7})
	if updates["autopay_enabled"] != false {
		t.Fatalf("daily plan must disable monthly autopay: %+v", updates)
	}
	if value, ok := updates["autopay_plan_months"]; !ok || value != nil {
		t.Fatalf("daily plan must clear monthly autopay plan: %+v", updates)
	}
}

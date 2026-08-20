package miniapp

import "testing"

func TestApplyDiscountToSubscriptionAndDevicePackTotal(t *testing.T) {
	const subscriptionPrice = 100
	const devicePackPrice = 100

	if got := applyDiscount(subscriptionPrice+devicePackPrice, 20); got != 160 {
		t.Fatalf("applyDiscount(combined price, 20) = %d, want 160", got)
	}
}

package payment

import (
	"math"
	"time"
)

const DeviceBillingMonth = 30 * 24 * time.Hour

// DevicePackAmount uses the quoted expiry, not a plan's nominal duration.
// The same expiry is stored with the invoice and cannot move on renewal.
func DevicePackAmount(monthlyPrice int, remaining time.Duration) int {
	if monthlyPrice <= 0 || remaining <= 0 {
		return 0
	}
	amount := int(math.Round(float64(monthlyPrice) * float64(remaining) / float64(DeviceBillingMonth)))
	if amount < 1 {
		return 1
	}
	return amount
}

package runtimeconfig

import (
	"testing"
	"time"
)

func TestPlanDiscountUsesCurrentPriceUntilDeadline(t *testing.T) {
	settings := DefaultSettings()
	settings.Plans = []PlanSettings{{ID: "discounted", Enabled: true, Months: 1, PriceRub: 67, OriginalPriceRub: 100,
		DiscountEndsAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339)}}
	if err := NormalizeAndValidate(&settings); err != nil {
		t.Fatal(err)
	}
	service := &Service{}
	service.value.Store(settings)
	plan := service.CheckoutPlans()[0]
	if plan.PriceRub != 67 || plan.OriginalPriceRub != 100 || plan.PriceStars != 46 || plan.DiscountEndsAt == "" {
		t.Fatalf("active discount changed checkout: %+v", plan)
	}
	settings.Plans[0].DiscountEndsAt = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	service.value.Store(settings)
	plan = service.CheckoutPlans()[0]
	if plan.PriceRub != 100 || plan.OriginalPriceRub != 0 || plan.PriceStars != 68 || plan.DiscountEndsAt != "" {
		t.Fatalf("expired discount did not restore checkout price: %+v", plan)
	}
}

func TestPlanDiscountRejectsMisleadingPrice(t *testing.T) {
	for _, original := range []int{-1, 50, 67} {
		settings := DefaultSettings()
		settings.Plans = []PlanSettings{{ID: "discounted", Enabled: true, Months: 1, PriceRub: 67, OriginalPriceRub: original}}
		if err := NormalizeAndValidate(&settings); err == nil {
			t.Fatalf("accepted original price %d", original)
		}
	}
}

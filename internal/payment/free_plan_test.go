package payment

import (
	"errors"
	"testing"
	"time"
)

func TestValidateFreePlanEligibility(t *testing.T) {
	now := time.Date(2026, time.August, 21, 12, 0, 0, 0, time.UTC)
	moreThanWeek := now.Add(freePlanRenewalWindow + time.Second)
	lastWeek := now.Add(freePlanRenewalWindow - time.Second)
	exactBoundary := now.Add(freePlanRenewalWindow)

	tests := []struct {
		name           string
		oneTime        bool
		previouslyUsed bool
		expireAt       *time.Time
		wantErr        error
	}{
		{name: "one time plan was already used", oneTime: true, previouslyUsed: true, wantErr: ErrFreePlanAlreadyUsed},
		{name: "active subscription has more than a week", expireAt: &moreThanWeek, wantErr: ErrFreePlanTooEarly},
		{name: "exactly seven days remain", expireAt: &exactBoundary},
		{name: "last week is eligible", expireAt: &lastWeek},
		{name: "no active expiry is eligible"},
		{name: "unused one time plan is eligible", oneTime: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateFreePlanEligibility(test.oneTime, test.previouslyUsed, test.expireAt, now)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("validateFreePlanEligibility() error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

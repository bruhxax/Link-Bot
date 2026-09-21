package bedolagaimport

import "testing"

func TestImportableStatus(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"active", " ACTIVE ", "trial"} {
		if !importableStatus(status) {
			t.Fatalf("%q must be importable", status)
		}
	}
	for _, status := range []string{"pending", "limited", "disabled", "expired"} {
		if importableStatus(status) {
			t.Fatalf("%q must not be importable", status)
		}
	}
}

func TestNextPositionUsesTenSubscriptionLimit(t *testing.T) {
	t.Parallel()
	used := map[int]bool{1: true, 2: true, 3: true}
	if got := nextPosition(used); got != 4 {
		t.Fatalf("nextPosition() = %d, want 4", got)
	}
	for position := 1; position <= maxSubscriptionsPerCustomer; position++ {
		used[position] = true
	}
	if got := nextPosition(used); got != 0 {
		t.Fatalf("nextPosition() = %d, want 0 for full account", got)
	}
}

func TestValidateRejectsOverflow(t *testing.T) {
	t.Parallel()
	user := sourceUser{ID: 1, TelegramID: 123}
	user.Subscriptions = make([]sourceSubscription, maxSubscriptionsPerCustomer+1)
	if err := validate([]sourceUser{user}); err == nil {
		t.Fatal("validate() accepted too many subscriptions")
	}
}

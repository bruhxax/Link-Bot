package notification

import (
	"context"
	"link-bot/internal/database"
	"testing"
	"time"
)

func TestExpirationReminderWindowAndTrial(t *testing.T) {
	now := time.Now()
	for _, duration := range []time.Duration{24 * time.Hour, 5 * time.Hour, 4*time.Hour + time.Second, 4 * time.Hour, 3 * time.Hour, time.Second, 0, -time.Hour} {
		expiry := now.Add(duration)
		want := duration > 0 && duration <= 4*time.Hour
		if got := expirationReminderDue(now, &expiry); got != want {
			t.Errorf("%v: %v, want %v", duration, got, want)
		}
	}
	if expirationReminderDue(now, nil) {
		t.Fatal("missing expiry sent reminder")
	}
	expiry := now.Add(24 * time.Hour)
	customers := []database.Customer{{ID: 1, TrialUsed: true, ExpireAt: &expiry}}
	tributes := []database.Purchase{}
	repo := &customerRepoMock{customers: &customers}
	s := NewSubscriptionService(repo, &purchaseRepoMock{tributes: &tributes}, &paymentServiceMock{}, nil, nil, nil)
	sent := 0
	s.notify = func(context.Context, database.Customer) error { sent++; return nil }
	if err := s.ProcessSubscriptionExpiration(); err != nil {
		t.Fatal(err)
	}
	if sent != 0 || len(repo.claims) != 0 {
		t.Fatal("new trial sent or consumed reminder")
	}
	expiry = now.Add(3 * time.Hour)
	for i := 0; i < 2; i++ {
		if err := s.ProcessSubscriptionExpiration(); err != nil {
			t.Fatal(err)
		}
	}
	if sent != 1 {
		t.Fatalf("reminder should arrive once near expiry: %d", sent)
	}
}

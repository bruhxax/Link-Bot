package miniapp

import (
	"testing"
	"time"

	"link-bot/internal/database"

	"github.com/google/uuid"
)

func TestBuildAdminSubscriptionTargetPayload(t *testing.T) {
	t.Parallel()

	panelUserID := int64(1419)
	panelUserUUID := uuid.New()
	future := time.Now().UTC().Add(time.Hour)
	subscriptionLink := "https://example.test/subscription/source"
	payload := buildAdminSubscriptionTargetPayload(8544649953, []database.CustomerSubscription{
		{ID: 10, DisplayName: "Основная", Position: 1, IsPrimary: true, SubscriptionLink: &subscriptionLink, ExpireAt: &future},
		{ID: 11, DisplayName: "Работа", Position: 2},
	}, panelUserID, panelUserUUID, subscriptionLink)

	if !payload.CanAdd || payload.Maximum != database.MaxCustomerSubscriptions {
		t.Fatalf("unexpected target capacity: %+v", payload)
	}
	if len(payload.Items) != 2 {
		t.Fatalf("target items = %d, want 2", len(payload.Items))
	}
	if !payload.Items[0].MatchesSource || payload.Items[0].Status != "active" {
		t.Fatalf("source subscription was not identified: %+v", payload.Items[0])
	}
	if payload.Items[1].MatchesSource || payload.Items[1].Status != "empty" {
		t.Fatalf("empty destination was classified incorrectly: %+v", payload.Items[1])
	}
}

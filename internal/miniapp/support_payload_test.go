package miniapp

import (
	"testing"
	"time"

	"link-bot/internal/database"
)

func TestBuildSupportTicketPayloadIncludesTelegramUsernameForAdmin(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC)
	ticket := database.SupportTicket{
		ID:                  42,
		Status:              database.SupportTicketStatusOpen,
		CustomerName:        "subscription_user",
		CustomerUsername:    "telegram_user",
		SubscriptionLabel:   "Monthly",
		CreatedAt:           now,
		UpdatedAt:           now,
		AdminUnreadCount:    3,
		CustomerUnreadCount: 1,
	}

	payload := (&Handler{}).buildSupportTicketPayload(ticket, true, "")

	if payload.CustomerUsername != "telegram_user" {
		t.Fatalf("expected Telegram username in admin payload, got %q", payload.CustomerUsername)
	}
	if payload.UnreadCount != 3 {
		t.Fatalf("expected admin unread count, got %d", payload.UnreadCount)
	}
}

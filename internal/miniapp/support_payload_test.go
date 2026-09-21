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

func TestBuildSupportMessagePayloadIncludesAttachmentMetadata(t *testing.T) {
	t.Parallel()

	messages := []database.SupportMessage{{
		ID:                17,
		AuthorRole:        database.SupportAuthorRoleCustomer,
		Body:              "Скриншот ошибки",
		MediaType:         "image",
		MediaMIME:         "image/png",
		MediaStorageName:  "support-00112233445566778899aabbccddeeff.png",
		MediaOriginalName: "error.png",
		MediaSizeBytes:    4096,
		CreatedAt:         time.Date(2026, time.September, 21, 1, 0, 0, 0, time.UTC),
	}}

	payload := buildSupportMessagePayloads(messages)
	if len(payload) != 1 || payload[0].Attachment == nil {
		t.Fatalf("attachment payload missing: %+v", payload)
	}
	if payload[0].Attachment.Type != "image" || payload[0].Attachment.Name != "error.png" || payload[0].Attachment.SizeBytes != 4096 {
		t.Fatalf("unexpected attachment payload: %+v", payload[0].Attachment)
	}
}

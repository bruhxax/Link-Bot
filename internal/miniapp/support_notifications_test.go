package miniapp

import (
	"strings"
	"testing"

	"link-bot/internal/database"
)

func TestRenderSupportNotificationAlwaysQuotesMessage(t *testing.T) {
	ticket := &database.SupportTicket{
		ID:                17,
		Subject:           "Payment <issue>",
		CustomerName:      "Max & Co",
		CustomerUsername:  "max_user",
		SubscriptionLabel: "Yearly",
	}

	got := renderSupportNotification("<b>Ticket #{ticket_id}</b>\n{name}\n{message}", ticket, "Card <declined>")
	if !strings.Contains(got, "<blockquote>Card &lt;declined&gt;</blockquote>") {
		t.Fatalf("message is not an escaped quote: %q", got)
	}
	if !strings.Contains(got, "Max &amp; Co") || !strings.Contains(got, "Ticket #17") {
		t.Fatalf("ticket tokens were not rendered safely: %q", got)
	}
}

func TestRenderSupportNotificationAppendsQuoteWhenTokenMissing(t *testing.T) {
	ticket := &database.SupportTicket{ID: 18}
	got := renderSupportNotification("Ticket {ticket_id}", ticket, "Reply")
	if strings.Count(got, "<blockquote>") != 1 || !strings.Contains(got, "<blockquote>Reply</blockquote>") {
		t.Fatalf("message quote was not appended exactly once: %q", got)
	}
}

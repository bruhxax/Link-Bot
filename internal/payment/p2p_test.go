package payment

import (
	"strings"
	"testing"

	"link-bot/internal/database"
)

func TestParseP2PCallback(t *testing.T) {
	action, purchaseID, ok := parseP2PCallback("p2p:approve:42")
	if !ok || action != "approve" || purchaseID != 42 {
		t.Fatalf("parseP2PCallback() = %q, %d, %v", action, purchaseID, ok)
	}
	for _, raw := range []string{"p2p:approve:0", "p2p:unknown:42", "p2p:approve:not-a-number", "approve:42"} {
		if _, _, valid := parseP2PCallback(raw); valid {
			t.Fatalf("parseP2PCallback(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestBuildP2PReviewNotificationEscapesUserInput(t *testing.T) {
	message := buildP2PReviewNotificationMessage("<b>Оплата</b>", &database.P2PPaymentRequest{
		SenderReference: "Иван <script>",
		DestinationSnapshot: database.P2PDestinationSnapshot{
			Title: "Банк & карта", Details: "<1234>", Description: "СБП > карта",
		},
	})
	for _, escaped := range []string{"Иван &lt;script&gt;", "Банк &amp; карта", "&lt;1234&gt;", "СБП &gt; карта"} {
		if !strings.Contains(message, escaped) {
			t.Fatalf("message does not contain %q: %s", escaped, message)
		}
	}
	if strings.Contains(message, "<script>") {
		t.Fatalf("message contains unescaped user input: %s", message)
	}
}

func TestBuildP2PRejectedUserMessage(t *testing.T) {
	message := buildP2PRejectedUserMessage(&database.Purchase{ID: 717, Amount: 89, Currency: "RUB"})
	for _, expected := range []string{"P2P-платёж отклонён администратором", "89 RUB", "717", "обратитесь в поддержку"} {
		if !strings.Contains(message, expected) {
			t.Fatalf("rejection message does not contain %q: %s", expected, message)
		}
	}
}

package payment

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"link-bot/internal/database"
	"link-bot/internal/integrations"
)

func TestP2PReviewUsesGroupBeforePrivateChat(t *testing.T) {
	groups := []integrations.NotificationGroup{
		{ChatID: -1001, IsForum: true}, // Pending topic is not a destination.
		{ChatID: -1002, IsForum: true, ThreadID: 42},
		{ChatID: -1003},
	}
	tests := []struct {
		name       string
		groups     []integrations.NotificationGroup
		failedChat int64
		want       []string
		wantChat   int64
	}{
		{name: "no group", want: []string{"123:0"}, wantChat: 123},
		{name: "pending topic", groups: groups[:1], want: []string{"123:0"}, wantChat: 123},
		{name: "configured topic", groups: groups[:2], want: []string{"-1002:42"}, wantChat: -1002},
		{name: "first group fails", groups: groups, failedChat: -1002, want: []string{"-1002:42", "-1003:0"}, wantChat: -1003},
		{name: "all groups fail", groups: groups[:2], failedChat: -1002, want: []string{"-1002:42", "123:0"}, wantChat: 123},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sent []string
			chatID, messageID, err := deliverP2PReviewNotification(123, tt.groups, func(id int64, threadID int) (int64, error) {
				sent = append(sent, fmt.Sprintf("%d:%d", id, threadID))
				if id == tt.failedChat {
					return 0, errors.New("group unavailable")
				}
				return 77, nil
			}, func(int64, error) {})
			if err != nil || chatID != tt.wantChat || messageID != 77 || !reflect.DeepEqual(sent, tt.want) {
				t.Fatalf("delivery = chat %d, message %d, error %v, calls %v; want chat %d, calls %v", chatID, messageID, err, sent, tt.wantChat, tt.want)
			}
		})
	}
}

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

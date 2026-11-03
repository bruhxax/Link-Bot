package miniapp

import (
	"strings"
	"testing"

	"link-bot/internal/database"
	"link-bot/internal/runtimeconfig"
)

func TestWithdrawalNotificationEscapesUserDataAndUpdatesStatus(t *testing.T) {
	cfg := runtimeconfig.DefaultSettings().Content.WithdrawalNotification
	item := database.BalanceWithdrawal{ID: 42, TelegramID: 123, Username: "alice<&", AmountCents: 100050, PayoutDetails: "Банк <script>& карта", Status: "pending"}
	text := renderWithdrawalNotification(cfg, item, 3)
	for _, want := range []string{"1 000,50 ₽", "@alice&lt;&amp;", "Банк &lt;script&gt;&amp; карта", "· 3", "№42"} {
		if !strings.Contains(text, want) {
			t.Fatalf("notification missing %q: %s", want, text)
		}
	}
	item.Status = "rejected"
	if text := renderWithdrawalNotification(cfg, item, 2); !strings.Contains(text, "Отклонено") || strings.Contains(text, "Заявки на вывод") {
		t.Fatalf("wrong resolved template: %s", text)
	}
}

func TestParseWithdrawalCallback(t *testing.T) {
	for _, data := range []string{"wd:approve:0", "wd:approve:-1", "wd:reject:abc", "wd:pay:1", "wd:approve:1:extra", "wd:preview"} {
		if _, _, ok := parseWithdrawalCallback(data); ok {
			t.Fatalf("accepted invalid callback %q", data)
		}
	}
	if action, id, ok := parseWithdrawalCallback("wd:reject:91"); !ok || action != "reject" || id != 91 {
		t.Fatalf("valid callback rejected: %q, %d, %v", action, id, ok)
	}
}

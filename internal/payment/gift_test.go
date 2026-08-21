package payment

import (
	"strings"
	"testing"

	"link-bot/internal/database"
)

func TestRenderGiftTemplateEscapesUserControlledValues(t *testing.T) {
	message := renderGiftTemplate(
		"<b>{sender}</b> подарил {recipient}: {plan}",
		map[string]string{
			"{sender}":    "@sender<&>",
			"{recipient}": "@recipient",
			"{plan}":      "3 <месяца>",
		},
	)
	if message != "<b>@sender&lt;&amp;&gt;</b> подарил @recipient: 3 &lt;месяца&gt;" {
		t.Fatalf("unexpected rendered gift message: %q", message)
	}
}

func TestPrepareGiftButtonsResolvesOnlyGiftLinks(t *testing.T) {
	buttons := []database.BroadcastButton{
		{ID: "gift", Type: "gift", Text: "Открыть"},
		{ID: "main", Type: "main", Text: "Кабинет"},
	}
	prepared := prepareGiftButtons(buttons, "https://t.me/example_bot?start=gift_token")
	if len(prepared) != 2 || prepared[0].Type != "url" || !strings.Contains(prepared[0].URL, "start=gift_token") {
		t.Fatalf("unexpected prepared gift button: %+v", prepared)
	}
	if prepared[1].Type != "main" || prepared[1].URL != "" {
		t.Fatalf("unrelated gift button changed: %+v", prepared[1])
	}
	if buttons[0].Type != "gift" || buttons[0].URL != "" {
		t.Fatalf("source buttons were mutated: %+v", buttons)
	}
}

func TestRussianMonthWordForGiftPlans(t *testing.T) {
	cases := map[int]string{1: "месяц", 2: "месяца", 5: "месяцев", 11: "месяцев", 21: "месяц"}
	for months, want := range cases {
		if got := russianMonthWord(months); got != want {
			t.Fatalf("russianMonthWord(%d) = %q, want %q", months, got, want)
		}
	}
}

func TestGiftReplacementsKeepAddressedRecipientForSenderMessage(t *testing.T) {
	recipientUsername := "recipient_user"
	senderUsername := "sender_user"
	purchase := &database.Purchase{Month: 3, GiftRecipientUsername: &recipientUsername}
	sender := &database.Customer{TelegramUsername: &senderUsername}

	replacements := (PaymentService{}).giftReplacements(purchase, sender, nil)
	if replacements["{sender}"] != "@sender_user" || replacements["{recipient}"] != "@recipient_user" {
		t.Fatalf("unexpected gift replacements: %+v", replacements)
	}
	if replacements["{gift_url}"] != replacements["{gift_link}"] {
		t.Fatalf("gift URL aliases differ: %+v", replacements)
	}
}

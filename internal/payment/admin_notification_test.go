package payment

import (
	"strings"
	"testing"
	"time"

	"link-bot/internal/database"
)

func TestBuildPaymentNotificationMessageUsesAssignedOrderNumber(t *testing.T) {
	purchase := &database.Purchase{
		ID:       444,
		Amount:   320,
		Currency: "RUB",
		Month:    3,
	}

	message := buildPaymentNotificationMessage(purchase, &database.Customer{}, "exampleuser", "YooKassa", time.Unix(0, 0), 445)
	if !strings.Contains(message, "<code>445</code>") {
		t.Fatalf("notification must use assigned order number: %s", message)
	}
	if strings.Contains(message, "<code>444</code>") {
		t.Fatalf("notification leaked the purchase attempt id: %s", message)
	}
}

func TestBuildPaymentNotificationMessageIncludesPromoCode(t *testing.T) {
	code := "LINK<FREE>"
	discount := 20
	purchase := &database.Purchase{
		ID:                       445,
		Amount:                   256,
		Currency:                 "RUB",
		Month:                    3,
		PromoCodeSnapshot:        &code,
		PromoCodeDiscountPercent: &discount,
	}

	message := buildPaymentNotificationMessage(purchase, &database.Customer{}, "", "YooKassa", time.Unix(0, 0), 445)
	if !strings.Contains(message, "Промокод:") || !strings.Contains(message, "LINK&lt;FREE&gt; (-20%)") {
		t.Fatalf("notification must contain escaped promo details: %s", message)
	}
}

func TestBuildPaymentNotificationMessageOmitsEmptyPromoCode(t *testing.T) {
	emptyCode := "  "
	purchase := &database.Purchase{
		ID:                446,
		Amount:            89,
		Currency:          "RUB",
		Month:             1,
		PromoCodeSnapshot: &emptyCode,
	}

	message := buildPaymentNotificationMessage(purchase, &database.Customer{}, "", "YooKassa", time.Unix(0, 0), 446)
	if strings.Contains(message, "Промокод:") {
		t.Fatalf("notification must omit an empty promo code: %s", message)
	}
}

func TestNormalizeSubscriptionActivatedPreviewSupportsPremiumEmojiAndColor(t *testing.T) {
	commerce, err := normalizeSubscriptionActivatedPreview(SubscriptionActivatedPreviewOptions{
		Text:              "  <b>Подписка активирована</b>  ",
		Banner:            "  /assets/telegram/success/banner.png  ",
		ButtonText:        "  Личный кабинет  ",
		IconCustomEmojiID: `<tg-emoji emoji-id="5278413853577734640">emoji</tg-emoji>`,
		ButtonStyle:       " SUCCESS ",
	})
	if err != nil {
		t.Fatalf("normalizeSubscriptionActivatedPreview() error = %v", err)
	}
	if commerce.SuccessText != "<b>Подписка активирована</b>" {
		t.Fatalf("unexpected success text: %q", commerce.SuccessText)
	}
	if commerce.SuccessBanner != "/assets/telegram/success/banner.png" {
		t.Fatalf("unexpected success banner: %q", commerce.SuccessBanner)
	}
	if commerce.SuccessButton.Text != "Личный кабинет" || commerce.SuccessButton.IconCustomEmojiID != "5278413853577734640" || commerce.SuccessButton.Style != "success" {
		t.Fatalf("unexpected success button: %#v", commerce.SuccessButton)
	}
}

func TestNormalizeSubscriptionActivatedPreviewRejectsInvalidButtonColor(t *testing.T) {
	_, err := normalizeSubscriptionActivatedPreview(SubscriptionActivatedPreviewOptions{
		Text:        "Подписка активирована",
		ButtonText:  "Личный кабинет",
		ButtonStyle: "purple",
	})
	if err == nil {
		t.Fatal("expected invalid Telegram button color error")
	}
}

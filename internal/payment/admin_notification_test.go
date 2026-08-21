package payment

import (
	"strings"
	"testing"
	"time"

	"link-bot/internal/database"
	"link-bot/internal/runtimeconfig"
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

func TestBuildPaymentNotificationMessageShowsDeviceOnlyPurchase(t *testing.T) {
	purchase := &database.Purchase{
		ID:           447,
		Amount:       149,
		Currency:     "RUB",
		PurchaseKind: database.PurchaseKindExtraDevices,
		ExtraDevices: 3,
	}

	message := buildPaymentNotificationMessage(purchase, &database.Customer{}, "exampleuser", "YooKassa", time.Unix(0, 0), 447)
	if !strings.Contains(message, "Доп. устройства:</b> <b>+3") {
		t.Fatalf("notification must contain the purchased device count: %s", message)
	}
	if strings.Contains(message, "Тариф:") || strings.Contains(message, "0 месяцев") {
		t.Fatalf("device-only notification must not contain a zero-month tariff: %s", message)
	}
}

func TestBuildPaymentNotificationMessageUsesCustomTemplateAndEscapesValues(t *testing.T) {
	purchase := &database.Purchase{Amount: 89, Currency: "RUB", Month: 1}
	message := buildPaymentNotificationMessageWithTemplate(
		"<b>{{username}}</b> | {{integration}} | {{sub}} | {{price}} | {{number}}",
		purchase,
		"unsafe<user>",
		"Method & Test",
		time.Unix(0, 0),
		77,
		"UTC",
	)
	if message != "<b>@unsafe&lt;user&gt;</b> | Method &amp; Test | 1 месяц | 89 RUB | 77" {
		t.Fatalf("unexpected custom payment notification: %s", message)
	}
}

func TestBuildPaymentNotificationMessageRemovesEmptyOptionalLines(t *testing.T) {
	purchase := &database.Purchase{Amount: 89, Currency: "RUB", Month: 1}
	message := buildPaymentNotificationMessageWithTemplate(
		"Тариф: {{sub}}\nУстройства: {{device}}\nПромокод: {{promo}}\nЦена: {{price}}",
		purchase,
		"",
		"YooKassa",
		time.Unix(0, 0),
		78,
		"UTC",
	)
	if strings.Contains(message, "Устройства:") || strings.Contains(message, "Промокод:") {
		t.Fatalf("optional empty lines were not removed: %s", message)
	}
	if !strings.Contains(message, "Тариф: 1 месяц") || !strings.Contains(message, "Цена: 89 RUB") {
		t.Fatalf("required template values are missing: %s", message)
	}
}

func TestBuildPaymentNotificationKeyboardHonorsVisibilityAndProfileTarget(t *testing.T) {
	settings := runtimeconfig.DefaultSettings().Content.PaymentNotification
	settings.OpenUserButton.Enabled = false
	settings.ProfileButton.Style = "success"
	settings.ProfileButton.IconCustomEmojiID = "5278413853577734640"
	keyboard := buildPaymentNotificationKeyboard(settings, 0, 123456)
	if keyboard == nil || len(keyboard.InlineKeyboard) != 1 || len(keyboard.InlineKeyboard[0]) != 1 {
		t.Fatalf("unexpected payment notification keyboard: %#v", keyboard)
	}
	button := keyboard.InlineKeyboard[0][0]
	if button.URL != "tg://user?id=123456" || button.Style != "success" || button.IconCustomEmojiID != "5278413853577734640" {
		t.Fatalf("unexpected profile button: %#v", button)
	}
}

func TestBuildPaymentNotificationMessageShowsDevicesBoughtWithSubscription(t *testing.T) {
	purchase := &database.Purchase{
		ID:           448,
		Amount:       799,
		Currency:     "RUB",
		Month:        6,
		PurchaseKind: database.PurchaseKindSubscription,
		ExtraDevices: 2,
	}

	message := buildPaymentNotificationMessage(purchase, &database.Customer{}, "exampleuser", "Crypto Pay", time.Unix(0, 0), 448)
	if !strings.Contains(message, "Тариф:</b> <b>6 месяцев") {
		t.Fatalf("combined notification must contain the subscription tariff: %s", message)
	}
	if !strings.Contains(message, "Доп. устройства:</b> <b>+2") {
		t.Fatalf("combined notification must contain the purchased device count: %s", message)
	}
}

func TestBuildPaymentNotificationMessageOmitsDevicesForRegularSubscription(t *testing.T) {
	purchase := &database.Purchase{
		ID:           449,
		Amount:       299,
		Currency:     "RUB",
		Month:        1,
		PurchaseKind: database.PurchaseKindSubscription,
	}

	message := buildPaymentNotificationMessage(purchase, &database.Customer{}, "exampleuser", "Telegram Stars", time.Unix(0, 0), 449)
	if strings.Contains(message, "Устройства:") {
		t.Fatalf("regular subscription notification must not contain an extra-device line: %s", message)
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

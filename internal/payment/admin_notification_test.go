package payment

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"link-bot/internal/adminnotify"
	"link-bot/internal/database"
	"link-bot/internal/integrations"
	"link-bot/internal/runtimeconfig"
)

func TestPaymentNotificationPlanNameUsesExactPlanAndPreservesDurationVariable(t *testing.T) {
	plans := []runtimeconfig.PlanSettings{
		{ID: "limited", Name: "Старт", Months: 1, Enabled: true},
		{ID: "unlimited", Name: "  Премиум <VIP> & {{price}}  ", Months: 1, Enabled: true},
		{ID: "archived", Name: "Архивный тариф", Months: 1, Enabled: false},
		{ID: "unnamed", Months: 1, Enabled: true},
	}
	for _, tc := range []struct {
		id   string
		want string
	}{
		{"limited", "Старт"},
		{"unlimited", "Премиум &lt;VIP&gt; &amp; {{price}}"},
		{"archived", "Архивный тариф"},
		{"unnamed", "1 месяц"},
		{"removed", "1 месяц"},
		{"", "1 месяц"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			purchase := &database.Purchase{PlanID: &tc.id, Month: 1, Amount: 300, Currency: "RUB"}
			message := buildPaymentNotificationMessageWithTemplate(
				"Тариф: <b>{{plan_name}}</b>\nСрок: {{sub}}\nЦена: {{price}}", purchase, "user", "Smerch", time.Unix(0, 0), 42, "UTC", notificationPlanName(purchase, plans),
			)
			want := "Тариф: <b>" + tc.want + "</b>\nСрок: 1 месяц\nЦена: 300 RUB"
			if message != want {
				t.Fatalf("got %q, want %q", message, want)
			}
		})
	}
}

func TestPaymentNotificationPlanNameHandlesGiftsDailyAndAddonPurchases(t *testing.T) {
	id := "gift_week"
	plans := []runtimeconfig.PlanSettings{{ID: id, Name: "Неделя свободы", Days: 7}}
	for _, kind := range []database.PurchaseKind{database.PurchaseKindSubscription, database.PurchaseKindGift, database.PurchaseKindExtraDevices, database.PurchaseKindExtraTraffic} {
		purchase := &database.Purchase{PlanID: &id, Days: 7, PurchaseKind: kind, Amount: 99, Currency: "RUB"}
		message := buildPaymentNotificationMessageWithTemplate("Тариф: {{plan_name}}\nЦена: {{price}}", purchase, "", "YooKassa", time.Unix(0, 0), 42, "UTC", notificationPlanName(purchase, plans))
		want := "Тариф: Неделя свободы\nЦена: 99 RUB"
		if kind == database.PurchaseKindExtraDevices || kind == database.PurchaseKindExtraTraffic {
			want = "Цена: 99 RUB"
		}
		if message != want {
			t.Errorf("%s: %q, want %q", kind, message, want)
		}
	}
	purchase := &database.Purchase{Days: 7}
	if got := notificationPlanName(purchase, plans); got != "7 дней" {
		t.Fatalf("unnamed daily fallback: %q", got)
	}
}

func TestPaymentNotificationPreviewUsesConfiguredPlanName(t *testing.T) {
	plans := []runtimeconfig.PlanSettings{
		{ID: "old", Name: "Скрытый тариф", Months: 1},
		{ID: "unnamed", Enabled: true, Months: 3},
		{ID: "vip", Name: "Премиум", Enabled: true, Days: 7},
	}
	purchase := paymentNotificationPreviewPurchase(plans)
	if purchase.PlanID == nil || *purchase.PlanID != "vip" || purchase.Days != 7 || purchase.Month != 0 {
		t.Fatalf("incorrect preview purchase: %+v", purchase)
	}
	message := buildPaymentNotificationMessageWithTemplate("{{plan_name}} · {{sub}}", purchase, "", "Smerch", time.Unix(0, 0), 42, "UTC", notificationPlanName(purchase, plans))
	if message != "Премиум · 7 дней" {
		t.Fatalf("incorrect named preview: %q", message)
	}
	unnamed := paymentNotificationPreviewPurchase(nil)
	if got := notificationPlanName(unnamed, nil); got != "3 месяца" {
		t.Fatalf("preview fallback: %q", got)
	}
}

func TestPaymentNotificationsUseGroupsInsteadOfPrivateChat(t *testing.T) {
	ordinary := integrations.NotificationGroup{ChatID: -1001, Title: "Payments"}
	forum := integrations.NotificationGroup{ChatID: -1002, Title: "Forum", IsForum: true, ThreadID: 42}
	pending := integrations.NotificationGroup{ChatID: -1003, Title: "Pending", IsForum: true}
	tests := []struct {
		name       string
		groups     []integrations.NotificationGroup
		failedChat int64
		want       []string
	}{
		{name: "no group", want: []string{"123:0"}},
		{name: "forum awaiting topic", groups: []integrations.NotificationGroup{pending}, want: []string{"123:0"}},
		{name: "ordinary group", groups: []integrations.NotificationGroup{ordinary}, want: []string{"-1001:0"}},
		{name: "selected topic", groups: []integrations.NotificationGroup{forum}, want: []string{"-1002:42"}},
		{name: "two groups", groups: []integrations.NotificationGroup{ordinary, forum}, want: []string{"-1001:0", "-1002:42"}},
		{name: "group removed", want: []string{"123:0"}},
		{name: "group failed", groups: []integrations.NotificationGroup{forum}, failedChat: forum.ChatID, want: []string{"-1002:42", "123:0"}},
		{name: "another group succeeded", groups: []integrations.NotificationGroup{ordinary, forum}, failedChat: ordinary.ChatID, want: []string{"-1001:0", "-1002:42"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sent []string
			deliverPaymentNotification(123, tt.groups, func(chatID int64, threadID int) error {
				sent = append(sent, fmt.Sprintf("%d:%d", chatID, threadID))
				if chatID == tt.failedChat {
					return errors.New("Telegram rejected message")
				}
				return nil
			}, func(int64, error) {})
			if !reflect.DeepEqual(sent, tt.want) {
				t.Fatalf("destinations = %v, want %v", sent, tt.want)
			}
		})
	}
}

type recordingPaymentPushNotifier struct {
	events chan adminnotify.Event
}

func (n *recordingPaymentPushNotifier) Notify(_ context.Context, event adminnotify.Event) error {
	n.events <- event
	return nil
}

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

func TestFormatPushPurchaseAmountUsesLockScreenFriendlyCurrency(t *testing.T) {
	if got := formatPushPurchaseAmount(&database.Purchase{Amount: 350, Currency: "RUB"}); got != "350 ₽" {
		t.Fatalf("RUB push amount = %q, want %q", got, "350 ₽")
	}
	if got := formatPushPurchaseAmount(&database.Purchase{Amount: 150, Currency: "XTR"}); got != "150 Stars" {
		t.Fatalf("Stars push amount = %q, want %q", got, "150 Stars")
	}
}

func TestCompletedPaymentDispatchesAutomaticAdminPush(t *testing.T) {
	notifier := &recordingPaymentPushNotifier{events: make(chan adminnotify.Event, 1)}
	service := &PaymentService{adminPushNotifier: notifier}
	service.notifyAdminAboutPaymentByPush(&database.Purchase{
		ID:       73,
		Amount:   350,
		Currency: "RUB",
		Month:    1,
	}, "bruh_user", "СБП")

	select {
	case event := <-notifier.events:
		if event.Title != "Новая оплата" || event.Tag != "payment-73" || event.URL != "/mini-app/?page=admin&section=finance" {
			t.Fatalf("automatic payment push = %+v", event)
		}
		if event.Body != "350 ₽ · Подписка на 1 месяц · @bruh_user · СБП" {
			t.Fatalf("automatic payment push body = %q", event.Body)
		}
	case <-time.After(time.Second):
		t.Fatal("automatic payment push was not dispatched")
	}
}

func TestPaymentPushDeliveryWindowCoversNetworkRequest(t *testing.T) {
	if adminPaymentPushTimeout < 15*time.Second {
		t.Fatalf("payment push timeout = %s, want at least 15s", adminPaymentPushTimeout)
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
	keyboard := buildPaymentNotificationKeyboard(settings, 0, "Example_User")
	if keyboard == nil || len(keyboard.InlineKeyboard) != 1 || len(keyboard.InlineKeyboard[0]) != 1 {
		t.Fatalf("unexpected payment notification keyboard: %#v", keyboard)
	}
	button := keyboard.InlineKeyboard[0][0]
	if button.URL != "https://t.me/Example_User" || button.Style != "success" || button.IconCustomEmojiID != "5278413853577734640" {
		t.Fatalf("unexpected profile button: %#v", button)
	}
}

func TestBuildPaymentNotificationKeyboardOmitsProfileWithoutValidUsername(t *testing.T) {
	settings := runtimeconfig.DefaultSettings().Content.PaymentNotification
	settings.OpenUserButton.Enabled = false
	for _, username := range []string{"", "@", "invalid username", "https://example.com/user"} {
		if keyboard := buildPaymentNotificationKeyboard(settings, 0, username); keyboard != nil {
			t.Fatalf("profile button must be omitted for %q: %#v", username, keyboard)
		}
	}
}

func TestPaymentNotificationProfileURLNormalizesTelegramUsername(t *testing.T) {
	for input, expected := range map[string]string{
		"@Example_User":             "https://t.me/Example_User",
		"https://t.me/Example_User": "https://t.me/Example_User",
		"t.me/example123":           "https://t.me/example123",
	} {
		if actual := paymentNotificationProfileURL(input); actual != expected {
			t.Fatalf("paymentNotificationProfileURL(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestSendTelegramNotificationIncludesAPIErrorDescription(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusBadRequest)
		_, _ = response.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: BUTTON_USER_INVALID"}`))
	}))
	defer server.Close()

	err := sendTelegramNotificationRequest(context.Background(), server.Client(), server.URL, telegramSendMessageRequest{ChatID: 1, Text: "test"})
	var apiErr *telegramAPIError
	if !errors.As(err, &apiErr) || apiErr.ErrorCode != 400 || !strings.Contains(err.Error(), "BUTTON_USER_INVALID") {
		t.Fatalf("unexpected Telegram API error: %v", err)
	}
	if !isTelegramButtonUserInvalid(err) {
		t.Fatalf("BUTTON_USER_INVALID must be recognized: %v", err)
	}
}

func TestSendTelegramNotificationRetriesServerErrors(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		attempts++
		response.Header().Set("Content-Type", "application/json")
		if attempts < 3 {
			response.WriteHeader(http.StatusBadGateway)
			_, _ = response.Write([]byte(`{"ok":false,"error_code":502,"description":"Bad Gateway"}`))
			return
		}
		_, _ = response.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := sendTelegramNotificationRequest(ctx, server.Client(), server.URL, telegramSendMessageRequest{ChatID: 1, Text: "test"}); err != nil {
		t.Fatalf("retry delivery failed: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
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

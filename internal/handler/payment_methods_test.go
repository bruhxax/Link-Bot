package handler

import (
	"net/url"
	"strings"
	"testing"

	"link-bot/internal/database"
	"link-bot/internal/payment"
	planbook "link-bot/internal/plans"
	"link-bot/internal/runtimeconfig"
)

func TestPaymentKeyboardIncludesEveryConfiguredProvider(t *testing.T) {
	settings := runtimeconfig.DefaultSettings()
	methods := map[string]bool{}
	for _, id := range runtimeconfig.DefaultPaymentMethodOrder() {
		methods[id] = true
	}
	plan := planbook.CheckoutPlan{ID: "custom", PriceRub: 200, PriceStars: 100}
	keyboard := PaymentMethodsKeyboard(methods, plan, settings.Content.Commerce, []string{"platega", "stars", "platega"}, true)
	if keyboard[0][0].Text != "Platega" || !strings.Contains(keyboard[0][0].CallbackData, "invoiceType=platega") {
		t.Fatalf("Platega not first: %+v", keyboard)
	}
	seen := map[database.InvoiceType]bool{}
	for _, row := range keyboard {
		button := row[0]
		if button.Text == "" {
			t.Fatal("payment button has no label")
		}
		if button.WebApp != nil {
			continue
		}
		query, err := url.ParseQuery(strings.SplitN(button.CallbackData, "?", 2)[1])
		if err != nil {
			t.Fatal(err)
		}
		invoiceType := database.InvoiceType(query.Get("invoiceType"))
		if seen[invoiceType] {
			t.Fatalf("duplicate provider %s", invoiceType)
		}
		seen[invoiceType] = true
		if !(Handler{}).isPaymentMethodAllowedForPlan(methods, plan, invoiceType) {
			t.Fatalf("displayed provider %s cannot be selected", invoiceType)
		}
		if query.Get("planId") != plan.ID {
			t.Fatalf("wrong plan in %s", button.CallbackData)
		}
	}
	for _, method := range runtimeconfig.DefaultPaymentMethodOrder() {
		invoiceType, err := payment.InvoiceTypeForMethod(method)
		if err != nil {
			t.Fatal(err)
		}
		if method != "p2p" && !seen[invoiceType] {
			t.Errorf("missing provider %s", method)
		}
	}
}

func TestPaymentKeyboardSkipsDisabledProvidersAndUnpricedStars(t *testing.T) {
	commerce := runtimeconfig.DefaultSettings().Content.Commerce
	methods := map[string]bool{"stars": true, "platega": true, "payhot": false}
	plan := planbook.CheckoutPlan{ID: "rub", PriceRub: 100}
	keyboard := PaymentMethodsKeyboard(methods, plan, commerce, nil, false)
	if len(keyboard) != 1 || keyboard[0][0].Text != "Platega" {
		t.Fatalf("unexpected payment options: %+v", keyboard)
	}
	if (Handler{}).isPaymentMethodAllowedForPlan(methods, plan, database.InvoiceTypePayHot) {
		t.Fatal("disabled provider accepted")
	}
	if (Handler{}).isPaymentMethodAllowedForPlan(methods, plan, database.InvoiceType("unknown")) {
		t.Fatal("unknown provider accepted")
	}
	plan.PriceRub = 0
	if got := PaymentMethodsKeyboard(methods, plan, commerce, nil, false); len(got) != 0 {
		t.Fatalf("unpriced plan has payment options: %+v", got)
	}
}

func TestP2PPaymentUsesMiniAppForm(t *testing.T) {
	t.Setenv("MINI_APP_URL", "https://example.com/mini-app/?cabinet=1")
	plan := planbook.CheckoutPlan{ID: "3m", PriceRub: 200}
	commerce := runtimeconfig.DefaultSettings().Content.Commerce
	methods := map[string]bool{"p2p": true}
	keyboard := PaymentMethodsKeyboard(methods, plan, commerce, nil, true)
	if len(keyboard) != 1 || keyboard[0][0].WebApp == nil {
		t.Fatalf("P2P form missing: %+v", keyboard)
	}
	parsed, err := url.Parse(keyboard[0][0].WebApp.URL)
	if err != nil {
		t.Fatal(err)
	}
	if q := parsed.Query(); q.Get("planId") != "3m" || q.Get("paymentMethod") != "p2p" || q.Get("page") != "plans" {
		t.Fatalf("wrong P2P checkout URL: %s", parsed)
	}
	if got := PaymentMethodsKeyboard(methods, plan, commerce, nil, false); len(got) != 0 {
		t.Fatal("P2P offered without a Mini App form")
	}
}

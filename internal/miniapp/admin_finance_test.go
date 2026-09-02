package miniapp

import (
	"strings"
	"testing"
	"time"

	"link-bot/internal/database"
)

func TestResolveAdminFinanceRange(t *testing.T) {
	now := time.Date(2026, time.September, 2, 18, 30, 0, 0, adminFinanceLocation)
	period, from, to, err := resolveAdminFinanceRange(adminFinanceRequest{Period: "7d"}, now)
	if err != nil {
		t.Fatalf("resolve 7d range: %v", err)
	}
	if period != "7d" {
		t.Fatalf("unexpected period: %q", period)
	}
	if got := from.In(adminFinanceLocation).Format(adminFinanceDateLayout); got != "2026-08-27" {
		t.Fatalf("unexpected range start: %s", got)
	}
	if got := to.In(adminFinanceLocation).Format(adminFinanceDateLayout); got != "2026-09-03" {
		t.Fatalf("unexpected exclusive range end: %s", got)
	}

	period, from, to, err = resolveAdminFinanceRange(adminFinanceRequest{Period: "custom", From: "2026-08-10", To: "2026-08-12"}, now)
	if err != nil {
		t.Fatalf("resolve custom range: %v", err)
	}
	if period != "custom" || from.In(adminFinanceLocation).Format(adminFinanceDateLayout) != "2026-08-10" || to.In(adminFinanceLocation).Format(adminFinanceDateLayout) != "2026-08-13" {
		t.Fatalf("unexpected custom range: %q %s %s", period, from, to)
	}
}

func TestResolveAdminFinanceRangeRejectsInvalidCustomDates(t *testing.T) {
	now := time.Date(2026, time.September, 2, 18, 30, 0, 0, adminFinanceLocation)
	for _, req := range []adminFinanceRequest{
		{Period: "custom", From: "2026-09-02", To: "2026-08-10"},
		{Period: "custom", From: "2025-08-01", To: "2026-09-02"},
		{Period: "custom", From: "not-a-date", To: "2026-09-02"},
		{Period: "unknown"},
	} {
		if _, _, _, err := resolveAdminFinanceRange(req, now); err == nil {
			t.Fatalf("expected invalid range error for %#v", req)
		}
	}
}

func TestAdminFinancePresentationLabels(t *testing.T) {
	tests := []struct {
		name     string
		payment  database.AdminFinancePayment
		provider string
		plan     string
		status   string
	}{
		{
			name:     "yookassa method and subscription",
			payment:  database.AdminFinancePayment{InvoiceType: database.InvoiceTypeYookasa, YookasaPaymentMethodTitle: "СБП", Month: 3, PurchaseKind: database.PurchaseKindSubscription, Status: database.PurchaseStatusPaid, WasPaid: true},
			provider: "СБП", plan: "Подписка на 3 месяца", status: "paid",
		},
		{
			name:     "stars devices",
			payment:  database.AdminFinancePayment{InvoiceType: database.InvoiceTypeTelegram, ExtraDevices: 2, PurchaseKind: database.PurchaseKindExtraDevices, Status: database.PurchaseStatusPaid, WasPaid: true},
			provider: "Telegram Stars", plan: "Дополнительные устройства · 2", status: "paid",
		},
		{
			name:     "recorded refund",
			payment:  database.AdminFinancePayment{InvoiceType: database.InvoiceTypeCrypto, Month: 1, PurchaseKind: database.PurchaseKindGift, Status: database.PurchaseStatusCancel, WasPaid: true},
			provider: "Crypto Pay", plan: "Подарок · 1 месяц", status: "refunded",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := adminFinanceProvider(test.payment); got != test.provider {
				t.Fatalf("provider = %q, want %q", got, test.provider)
			}
			if got := adminFinancePlan(test.payment); got != test.plan {
				t.Fatalf("plan = %q, want %q", got, test.plan)
			}
			if got := adminFinanceStatus(test.payment); got != test.status {
				t.Fatalf("status = %q, want %q", got, test.status)
			}
		})
	}
}

func TestAdminFinanceStaticSurface(t *testing.T) {
	appRaw, err := embeddedStatic.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	stylesRaw, err := embeddedStatic.ReadFile("static/styles.css")
	if err != nil {
		t.Fatalf("read styles.css: %v", err)
	}
	app := string(appRaw)
	for _, fragment := range []string{
		`"Финансы"`,
		`renderAdminFinancePage`,
		`/api/mini-app/admin/finance`,
		`data-input="admin-finance-period"`,
		`admin-finance-chart__tooltip`,
		`История платежей`,
		`financePaymentIdentity`,
	} {
		if !strings.Contains(app, fragment) {
			t.Fatalf("app.js does not contain %q", fragment)
		}
	}
	styles := string(stylesRaw)
	for _, fragment := range []string{
		`.admin-finance__metrics`,
		`.admin-finance-chart__point:focus`,
		`.admin-finance-payment`,
		`@media (prefers-reduced-motion: reduce)`,
	} {
		if !strings.Contains(styles, fragment) {
			t.Fatalf("styles.css does not contain %q", fragment)
		}
	}
}

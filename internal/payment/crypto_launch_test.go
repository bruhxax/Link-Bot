package payment

import (
	"testing"

	"link-bot/internal/cryptopay"
)

func TestCryptoInvoiceLaunchURLUsesHostedPageForAppAndWeb(t *testing.T) {
	invoice := &cryptopay.InvoiceResponse{BotInvoiceUrl: "https://t.me/CryptoBot?start=invoice", MiniAppInvoiceUrl: "https://t.me/CryptoBot/app?startapp=invoice", WebAppInvoiceUrl: "https://pay.crypt.bot/invoice"}
	for _, surface := range []string{"telegram", "web", "browser", " WEB "} {
		if got, err := cryptoInvoiceLaunchURL(invoice, surface); err != nil || got != invoice.WebAppInvoiceUrl {
			t.Fatalf("%s: got %s, %v", surface, got, err)
		}
	}
	if got, err := cryptoInvoiceLaunchURL(invoice, ""); err != nil || got != invoice.BotInvoiceUrl {
		t.Fatalf("bot checkout: %s, %v", got, err)
	}
}

func TestCryptoInvoiceLaunchURLRequiresWebPageForMiniApp(t *testing.T) {
	for _, link := range []string{"", "https://", "http://pay.crypt.bot/invoice", "javascript:alert(1)", "https://user:secret@pay.crypt.bot/invoice"} {
		if _, err := cryptoInvoiceLaunchURL(&cryptopay.InvoiceResponse{WebAppInvoiceUrl: link}, "telegram"); err == nil {
			t.Errorf("accepted unsupported web checkout: %s", link)
		}
	}
	if _, err := cryptoInvoiceLaunchURL(nil, "telegram"); err == nil {
		t.Fatal("accepted missing invoice")
	}
}

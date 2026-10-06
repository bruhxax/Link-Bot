package payment

import (
	"errors"
	"net/url"
	"strings"

	"link-bot/internal/cryptopay"
)

func cryptoInvoiceLaunchURL(invoice *cryptopay.InvoiceResponse, returnTarget string) (string, error) {
	if invoice == nil {
		return "", errors.New("Crypto Pay не вернул счёт")
	}
	switch strings.ToLower(strings.TrimSpace(returnTarget)) {
	case "telegram", "web", "browser":
		// Mini App and web checkout both need the hosted payment page. A bot
		// deep link cannot stay in the current desktop Mini App webview.
		link := strings.TrimSpace(invoice.WebAppInvoiceUrl)
		parsed, err := url.Parse(link)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
			return "", errors.New("Crypto Pay не вернул веб-страницу оплаты")
		}
		return link, nil
	default:
		return invoice.BotInvoiceUrl, nil
	}
}

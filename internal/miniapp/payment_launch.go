package miniapp

import "link-bot/internal/database"

// The client chooses desktop in-app navigation or mobile browser navigation.
// All hosted checkouts use this route, including future payment providers.
func paymentLaunchAction(invoiceType database.InvoiceType) string {
	switch invoiceType {
	case database.InvoiceTypeFree, database.InvoiceTypeBalance:
		return "completed"
	case database.InvoiceTypeTelegram:
		return "open_invoice"
	case database.InvoiceTypeP2P:
		return "p2p_pending"
	default:
		return "open_in_app"
	}
}

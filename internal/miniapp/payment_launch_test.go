package miniapp

import (
	"testing"

	"link-bot/internal/database"
)

func TestPaymentLaunchActionRoutesEveryHostedProviderThroughClient(t *testing.T) {
	for _, invoiceType := range []database.InvoiceType{
		database.InvoiceTypeYookasa, database.InvoiceTypeCrypto, database.InvoiceTypeTribute,
		database.InvoiceTypeLava, database.InvoiceTypeWata, database.InvoiceTypePlatega,
		database.InvoiceTypeFreeKassa, database.InvoiceTypeHeleket, database.InvoiceTypePally,
		database.InvoiceTypeRollyPay, database.InvoiceTypeCisPay, database.InvoiceTypeAnore,
		database.InvoiceTypeMulenPay, database.InvoiceTypeAuraPay, database.InvoiceTypeParityPay,
		database.InvoiceTypeAntiloPay, database.InvoiceTypeTributeShop, database.InvoiceTypeCloudPayments,
		database.InvoiceTypeDatagio, database.InvoiceTypeKassaAI, database.InvoiceTypePayHot,
		database.InvoiceType("future_provider"),
	} {
		if got := paymentLaunchAction(invoiceType); got != "open_in_app" {
			t.Errorf("%s opens as %s instead of device-aware checkout", invoiceType, got)
		}
	}
}

func TestPaymentLaunchActionKeepsNativeAndCompletedPayments(t *testing.T) {
	for invoiceType, want := range map[database.InvoiceType]string{
		database.InvoiceTypeTelegram: "open_invoice",
		database.InvoiceTypeP2P:      "p2p_pending",
		database.InvoiceTypeBalance:  "completed",
		database.InvoiceTypeFree:     "completed",
	} {
		if got := paymentLaunchAction(invoiceType); got != want {
			t.Errorf("%s: got %s, want %s", invoiceType, got, want)
		}
	}
}

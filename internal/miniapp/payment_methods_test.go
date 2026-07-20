package miniapp

import (
	"reflect"
	"testing"

	"link-bot/internal/database"
)

func TestMapPaymentMethodsKeepsCheckoutOrder(t *testing.T) {
	methods := map[string]bool{
		"sbp":    true,
		"card":   true,
		"stars":  true,
		"crypto": true,
	}

	got := mapPaymentMethods(methods)
	ids := make([]string, 0, len(got))
	for _, method := range got {
		ids = append(ids, method.ID)
	}

	want := []string{"sbp", "card", "stars", "crypto"}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("payment method order = %v, want %v", ids, want)
	}
}

func TestMapPaymentMethodRoutesSbpAndCardToYookassa(t *testing.T) {
	for _, method := range []string{"sbp", "card"} {
		invoiceType, err := mapPaymentMethod(method)
		if err != nil {
			t.Fatalf("mapPaymentMethod(%q): %v", method, err)
		}
		if invoiceType != database.InvoiceTypeYookasa {
			t.Fatalf("mapPaymentMethod(%q) = %q, want %q", method, invoiceType, database.InvoiceTypeYookasa)
		}
	}
}

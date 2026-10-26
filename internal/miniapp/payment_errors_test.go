package miniapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"link-bot/internal/integrations"
)

func TestPayHotPurchaseErrorExplainsFailureWithoutExposingAPIToCustomer(t *testing.T) {
	for _, owner := range []bool{false, true} {
		sess := &session{}
		if owner {
			sess.AdminAccess = ownerAccess()
		}
		w := httptest.NewRecorder()
		err := fmt.Errorf("create order: %w", &integrations.PayHotPaymentError{StatusCode: 401, Code: "authentication_failed", RequestID: "request-1"})
		if !(&Handler{}).writeIntegrationPurchaseError(w, sess, err) {
			t.Fatal("did not handle payment error")
		}
		var response struct {
			Error struct{ Code, Message string }
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if w.Code != 502 || response.Error.Code != "payhot_payment_failed" {
			t.Fatalf("wrong status: %d %s", w.Code, w.Body.String())
		}
		if owner && (!strings.Contains(response.Error.Message, "API-ключ") || !strings.Contains(response.Error.Message, "request-1")) {
			t.Fatal("admin cannot diagnose failure")
		}
		if !owner && (strings.Contains(response.Error.Message, "API-ключ") || strings.Contains(response.Error.Message, "request-1") || strings.Contains(response.Error.Message, "authentication_failed")) {
			t.Fatal("customer received diagnostic information")
		}
	}
	if (&Handler{}).writeIntegrationPurchaseError(httptest.NewRecorder(), &session{}, errors.New("database failed")) {
		t.Fatal("handled unrelated error")
	}
}

func TestSmerchPurchaseErrorIsSafeAndActionable(t *testing.T) {
	for _, owner := range []bool{false, true} {
		sess := &session{}
		if owner {
			sess.AdminAccess = ownerAccess()
		}
		w := httptest.NewRecorder()
		err := fmt.Errorf("create: %w", &integrations.SmerchPaymentError{StatusCode: 401, Code: "payment_api_error"})
		if !(&Handler{}).writeIntegrationPurchaseError(w, sess, err) {
			t.Fatal("Smerch error not handled")
		}
		var response struct {
			Error struct{ Code, Message string }
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if w.Code != 502 || response.Error.Code != "smerch_payment_failed" {
			t.Fatalf("wrong response: %s", w.Body.String())
		}
		if owner && !strings.Contains(response.Error.Message, "API-ключ") {
			t.Fatal("admin cannot diagnose Smerch")
		}
		if !owner && strings.Contains(response.Error.Message, "API-ключ") {
			t.Fatal("diagnostics leaked to customer")
		}
	}
}

func TestLaveshkaPurchaseErrorUsesSafeRoleAppropriateMessage(t *testing.T) {
	for _, owner := range []bool{false, true} {
		sess := &session{}
		if owner {
			sess.AdminAccess = ownerAccess()
		}
		w := httptest.NewRecorder()
		if !(&Handler{}).writeIntegrationPurchaseError(w, sess, fmt.Errorf("create: %w", &integrations.LaveshkaPaymentError{StatusCode: 401, Code: "payment_api_error"})) {
			t.Fatal("unhandled provider error")
		}
		var response struct {
			Error struct{ Code, Message string }
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if w.Code != 502 || response.Error.Code != "laveshka_payment_failed" {
			t.Fatal(w.Body.String())
		}
		if strings.Contains(response.Error.Message, "API-ключ") != owner {
			t.Fatal("incorrect role-specific diagnostics")
		}
	}
}

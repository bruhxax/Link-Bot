package miniapp

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebPushRequestAcceptsBrowserExpirationTime(t *testing.T) {
	for _, expiration := range []string{"", `,"expirationTime":null`, `,"expirationTime":1791200000000`} {
		t.Run(expiration, func(t *testing.T) {
			body := `{"endpoint":"https://push.example/device","keys":{"p256dh":"public-key","auth":"auth-key"}` + expiration + `}`
			request := httptest.NewRequest("POST", "/api/mini-app/admin/push/subscribe", strings.NewReader(body))
			var subscription adminWebPushSubscriptionRequest
			if err := (&Handler{}).decodeJSONRequest(httptest.NewRecorder(), request, 8192, &subscription); err != nil {
				t.Fatalf("valid browser subscription rejected: %v", err)
			}
			if subscription.Endpoint != "https://push.example/device" || subscription.Keys.P256DH != "public-key" || subscription.Keys.Auth != "auth-key" {
				t.Fatalf("subscription data changed: %+v", subscription)
			}
			if strings.Contains(expiration, "1791200000000") && (subscription.ExpirationTime == nil || *subscription.ExpirationTime != 1791200000000) {
				t.Fatal("browser expiration timestamp lost")
			}
		})
	}
}

func TestWebPushRequestStillRejectsUnknownFields(t *testing.T) {
	request := httptest.NewRequest("POST", "/api/mini-app/admin/push/subscribe", strings.NewReader(`{"endpoint":"https://push.example/device","unexpected":true}`))
	var subscription adminWebPushSubscriptionRequest
	if err := (&Handler{}).decodeJSONRequest(httptest.NewRecorder(), request, 8192, &subscription); err == nil {
		t.Fatal("unknown fields must still be rejected")
	}
}

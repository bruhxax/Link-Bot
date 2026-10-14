package miniapp

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoginHeadersAllowProviderOriginChecksAndPopupReplies(t *testing.T) {
	response := httptest.NewRecorder()
	setHTMLSecurityHeaders(response)
	if got := response.Header().Get("Referrer-Policy"); got != "strict-origin-when-cross-origin" {
		t.Fatalf("login provider must receive origin without cabinet paths or query tokens: %q", got)
	}
	if got := response.Header().Get("Cross-Origin-Opener-Policy"); got != "same-origin-allow-popups" {
		t.Fatalf("popup cannot reply with COOP %q", got)
	}
	csp := response.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "style-src 'self' 'unsafe-inline' https://accounts.google.com/gsi/style") {
		t.Fatal("Google button stylesheet is blocked")
	}
	apiResponse := httptest.NewRecorder()
	setAPIHeaders(apiResponse)
	if got := apiResponse.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("API referrer policy changed: %q", got)
	}
}

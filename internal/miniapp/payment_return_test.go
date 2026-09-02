package miniapp

import (
	"net/url"
	"testing"
)

func TestBuildPaymentReturnTargetOpensTelegramMiniApp(t *testing.T) {
	target, err := url.Parse(buildPaymentReturnTargetForBot("BruhvpnBot", 123, "paid"))
	if err != nil {
		t.Fatalf("parse payment return target: %v", err)
	}
	if target.Scheme != "https" || target.Host != "t.me" || target.Path != "/BruhvpnBot" {
		t.Fatalf("unexpected Telegram Mini App target: %s", target.String())
	}
	if got := target.Query().Get("startapp"); got != "payment_return_123_paid" {
		t.Fatalf("startapp = %q, want payment_return_123_paid", got)
	}
}

func TestBuildPaymentReturnTargetFallsBackToWebWithoutBotURL(t *testing.T) {
	target := buildPaymentReturnTargetForBot("", 456, "pending")
	if target != "/mini-app/?paymentReturn=1&paymentStatus=cancel&provider=yookasa&purchaseId=456" {
		t.Fatalf("unexpected fallback target: %s", target)
	}
}

func TestBuildPaymentReturnTargetKeepsWebCheckoutInWeb(t *testing.T) {
	target := buildPaymentReturnTargetForSurface("BruhvpnBot", 789, "paid", "browser")
	if target != "/mini-app/?paymentReturn=1&paymentStatus=paid&provider=yookasa&purchaseId=789" {
		t.Fatalf("unexpected web return target: %s", target)
	}
}

func TestBuildPaymentReturnTargetDefaultsUnknownSurfaceToTelegram(t *testing.T) {
	target, err := url.Parse(buildPaymentReturnTargetForSurface("BruhvpnBot", 321, "paid", "unexpected"))
	if err != nil {
		t.Fatalf("parse payment return target: %v", err)
	}
	if target.Host != "t.me" || target.Query().Get("startapp") != "payment_return_321_paid" {
		t.Fatalf("unexpected Telegram fallback target: %s", target.String())
	}
}

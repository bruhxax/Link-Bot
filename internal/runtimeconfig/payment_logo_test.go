package runtimeconfig

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPaymentLogosRoundTripAndIndependentCopies(t *testing.T) {
	s := DefaultSettings()
	s.PaymentMethodVariants = map[string]PaymentMethodVariant{"sbp__aabbccdd": {Method: "sbp"}}
	s.PaymentMethodText = map[string]PaymentMethodText{
		"card":          {LogoURL: "  https://cdn.example.com/logo.svg?size=64  "},
		"sbp":           {Name: "Моя СБП", HideLogo: true},
		"sbp__aabbccdd": {LogoURL: "/mini-app/uploads/logo-0123456789abcdef.webp"},
	}
	if err := NormalizeAndValidate(&s); err != nil {
		t.Fatal(err)
	}
	if s.PaymentMethodText["card"].LogoURL != "https://cdn.example.com/logo.svg?size=64" {
		t.Fatal("URL not normalized")
	}
	if !s.PaymentMethodText["sbp"].HideLogo || s.PaymentMethodText["sbp__aabbccdd"].HideLogo {
		t.Fatal("copy shares visibility")
	}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var loaded Settings
	if err = json.Unmarshal(raw, &loaded); err != nil {
		t.Fatal(err)
	}
	if err = NormalizeAndValidate(&loaded); err != nil {
		t.Fatal(err)
	}
	for id, text := range s.PaymentMethodText {
		if loaded.PaymentMethodText[id] != text {
			t.Fatal("logo lost", id)
		}
	}
	clone := cloneSettings(loaded)
	clone.PaymentMethodText["card"] = PaymentMethodText{}
	if err = NormalizeAndValidate(&clone); err != nil {
		t.Fatal(err)
	}
	if _, ok := clone.PaymentMethodText["card"]; ok {
		t.Fatal("reset should remove empty override")
	}
	if loaded.PaymentMethodText["card"].LogoURL == "" {
		t.Fatal("shared mutable map")
	}
}

func TestPaymentLogoURLsRejectUnsafeOrUnsupportedSources(t *testing.T) {
	for _, raw := range []string{
		"javascript:alert(1)", "data:image/svg+xml,test", "//example.com/logo.png", "http://example.com/a.png",
		"https://user:pass@example.com/logo.png", "https://", "https://example.com\n/a.png", "https://example.com\\a.png",
		"/mini-app/uploads/../secret", "/mini-app/uploads/logo-0123456789abcdef.svg", "https://example.com/" + strings.Repeat("x", 2049),
	} {
		t.Run(raw[:min(len(raw), 60)], func(t *testing.T) {
			s := DefaultSettings()
			s.PaymentMethodText = map[string]PaymentMethodText{"card": {LogoURL: raw}}
			if err := NormalizeAndValidate(&s); err == nil {
				t.Fatal("invalid URL accepted", raw)
			}
		})
	}
	s := DefaultSettings()
	s.PaymentMethodText = map[string]PaymentMethodText{"card": {LogoURL: "https://example.com/logo.png", HideLogo: true}}
	if err := NormalizeAndValidate(&s); err != nil {
		t.Fatal(err)
	}
	if s.PaymentMethodText["card"].LogoURL != "" || !s.PaymentMethodText["card"].HideLogo {
		t.Fatal("removal must clear custom image")
	}
}

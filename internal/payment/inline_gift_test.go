package payment

import (
	"testing"

	"link-bot/internal/runtimeconfig"
)

func TestInlineGiftBannerIsOptionalAndUsesPublicMiniAppOrigin(t *testing.T) {
	if got := runtimeconfig.DefaultSettings().Content.Gift.Inline.BannerURL; got != "" {
		t.Fatalf("default inline gift unexpectedly has a banner: %q", got)
	}
	for _, tc := range []struct{ configured, appURL, want string }{
		{"", "https://vpn.example/mini-app/?v=2", ""},
		{"/mini-app/uploads/logo-1234567890abcdef.png", "https://vpn.example/mini-app/?v=2", "https://vpn.example/mini-app/uploads/logo-1234567890abcdef.png"},
		{"https://cdn.example/gift.png", "https://vpn.example/mini-app/", "https://cdn.example/gift.png"},
		{"/mini-app/uploads/logo-1234567890abcdef.png", "http://localhost:8080/mini-app/", ""},
		{"javascript:alert(1)", "https://vpn.example/mini-app/", ""},
	} {
		if got := InlineGiftBannerURL(tc.configured, tc.appURL); got != tc.want {
			t.Errorf("InlineGiftBannerURL(%q, %q) = %q, want %q", tc.configured, tc.appURL, got, tc.want)
		}
	}
}

func TestInlineGiftMessagePreviewKeepsBannerAboveText(t *testing.T) {
	preview := InlineGiftMessagePreview("/mini-app/uploads/logo-1234567890abcdef.jpg", "https://vpn.example/mini-app/")
	if preview == nil || preview.URL == nil || *preview.URL != "https://vpn.example/mini-app/uploads/logo-1234567890abcdef.jpg" || preview.ShowAboveText == nil || !*preview.ShowAboveText {
		t.Fatalf("unexpected inline gift preview: %+v", preview)
	}
	if got := InlineGiftMessagePreview("", "https://vpn.example/mini-app/"); got != nil {
		t.Fatalf("bannerless gift has preview: %+v", got)
	}
}

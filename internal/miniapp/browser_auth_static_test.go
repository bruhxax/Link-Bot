package miniapp

import (
	"strings"
	"testing"
)

func TestBrowserAuthUsesRuntimeBrandWithoutRemovedHints(t *testing.T) {
	raw, err := embeddedStatic.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read embedded app.js: %v", err)
	}
	appJS := string(raw)

	removed := []string{
		"Войдите через Telegram или Gmail, чтобы открыть браузерную версию кабинета с подпиской, оплатами и поддержкой.",
		"Откроется Telegram — подтвердите вход одним нажатием. Номер и код вводить не нужно.",
		"Use Telegram or Gmail to open your browser dashboard with your subscription, payments and support history.",
		"Telegram will open for one-tap confirmation — no phone number or code needed.",
	}
	for _, value := range removed {
		if strings.Contains(appJS, value) {
			t.Fatalf("removed browser auth hint is still present: %q", value)
		}
	}

	required := []string{
		"function browserAuthBrand()",
		"content.brandName",
		"content.logoUrl",
		"data-browser-brand-logo",
		`class="browser-auth__logo-fallback" hidden`,
		"browserFallback.hidden = false",
		"browserFallback.hidden = true",
		"`Войти в ${name}`",
		"`Sign in to ${name}`",
		"function renderBrowserAuthQR()",
		"data-telegram-qr-login",
		`renderQRCodeSVG(url, {`,
		`/api/mini-app/auth/telegram/qr/start`,
		`/api/mini-app/auth/telegram/qr/status`,
		`writeSessionSetting(STORAGE_KEYS.telegramLogin, sessionData)`,
		`window.localStorage.getItem(key)`,
		`window.localStorage.setItem(key, value)`,
		`window.sessionStorage.getItem(key)`,
		`persistBrowserSessionFromResponse(response)`,
	}
	for _, value := range required {
		if !strings.Contains(appJS, value) {
			t.Fatalf("runtime browser branding fragment is missing: %q", value)
		}
	}
}

package miniapp

import (
	"strings"
	"testing"
)

func TestContentEditorKeepsAllSectionsInCompactNavigation(t *testing.T) {
	raw, err := embeddedStatic.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	app := string(raw)

	required := []string{
		`renderAdminEditorPage("Контент"`,
		`class="admin-content-groups"`,
		`class="admin-content-tabs"`,
		`data-action="admin-content-group"`,
		`role="tablist"`,
		`tabindex="${id === active ? "0" : "-1"}"`,
		`["ArrowLeft", "ArrowRight", "Home", "End"]`,
		`class="admin-content-panel"`,
		`class="admin-telegram-button"`,
		`class="admin-content-disclosure"`,
		`["start", "Главное меню", "Telegram"]`,
		`["verification", "Проверка подписки", "Telegram"]`,
		`["commerce", "Покупка", "Telegram"]`,
		`["success", "После оплаты", "Telegram"]`,
		`["gift", "Подарок", "Telegram"]`,
		`["payment-notifications", "Уведомления оплат", "Telegram"]`,
		`["support", "Поддержка", "Mini App"]`,
		`["web", "Веб-страница", "Mini App"]`,
		`["notifications", "Уведомления", "Mini App"]`,
		`["panel", "Панель", "Mini App"]`,
		`["faq", "FAQ", "Mini App"]`,
		`["advanced", "Служебные тексты", "Mini App"]`,
	}
	for _, fragment := range required {
		if !strings.Contains(app, fragment) {
			t.Fatalf("content editor fragment is missing: %q", fragment)
		}
	}

	paths := []string{
		"content.brandName",
		"content.webPage.title",
		"content.webPage.description",
		"content.webPage.faviconUrl",
		"content.startMenu.trialButton",
		"content.verification.channelButton",
		"content.commerce.yookassaButton",
		"content.commerce.successButton",
		"content.gift.${kind}.text",
		"content.paymentNotification.text",
		"content.support.newTicketText",
		"content.copy.ru.subscriptionExpiringTemplate",
		"panel.usernameTemplate",
		"content.faq.ru.${index}.question",
		"content.copy.ru",
	}
	for _, path := range paths {
		if !strings.Contains(app, path) {
			t.Fatalf("content editor setting was removed: %q", path)
		}
	}

	if strings.Contains(app, `data-input="admin-content-section"`) {
		t.Fatal("native content section select is still rendered")
	}
}

func TestContentEditorSupportsWebMetadataAndFaviconUpload(t *testing.T) {
	raw, err := embeddedStatic.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	app := string(raw)
	for _, fragment := range []string{
		`function renderAdminWebPageContent()`,
		`data-input="admin-favicon-file"`,
		`/api/mini-app/admin/favicon/upload`,
		`function applyWebPageMetadata()`,
		`document.title = title`,
	} {
		if !strings.Contains(app, fragment) {
			t.Fatalf("web page editor fragment is missing: %q", fragment)
		}
	}
}

func TestContentEditorHasCompactAccessibleStyles(t *testing.T) {
	raw, err := embeddedStatic.ReadFile("static/styles.css")
	if err != nil {
		t.Fatalf("read styles.css: %v", err)
	}
	styles := string(raw)
	for _, fragment := range []string{
		`.admin-content-groups button:focus-visible`,
		`.admin-content-tabs button:focus-visible`,
		`.admin-content-panel .admin-editor__section`,
		`.admin-content-disclosure summary:focus-visible`,
		`.admin-reminder-template summary:focus-visible`,
		`.admin-telegram-button summary:focus-visible`,
		`.admin-content-panel .admin-editor__grid--two { grid-template-columns: 1fr; }`,
	} {
		if !strings.Contains(styles, fragment) {
			t.Fatalf("content editor style is missing: %q", fragment)
		}
	}
}

package miniapp

import (
	"strings"
	"testing"
)

func TestAdminLogoEditorUploadsPreviewsAndFallsBack(t *testing.T) {
	appRaw, err := embeddedStatic.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	stylesRaw, err := embeddedStatic.ReadFile("static/styles.css")
	if err != nil {
		t.Fatalf("read styles.css: %v", err)
	}

	app := string(appRaw)
	for _, expected := range []string{
		`function renderAdminLogoField()`,
		`data-input="admin-logo-file"`,
		`/api/mini-app/admin/logo/upload`,
		`function uploadAdminLogo(file)`,
		`data-brand-logo`,
		`image.src = BRAND_MARK_URL`,
		`Link-Bot сам сохранит его и подставит правильный адрес`,
	} {
		if !strings.Contains(app, expected) {
			t.Fatalf("logo editor fragment is missing: %q", expected)
		}
	}

	styles := string(stylesRaw)
	for _, expected := range []string{
		`.admin-logo-field {`,
		`.admin-logo-field__preview {`,
		`.admin-logo-field__upload:focus-within`,
		`.admin-logo-field__status.is-error`,
	} {
		if !strings.Contains(styles, expected) {
			t.Fatalf("logo editor style is missing: %q", expected)
		}
	}
}
